package data

import (
	"context"
	"fmt"
	"strings"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// ResolveDirectory returns the directory at path. A path is / for the root
// or /a/b below it: it starts with a slash, and every segment between
// slashes is a directory name, normalized before it is compared. A path
// that does not start with a slash, or one with an empty segment or a
// segment ValidateName refuses, is blobfs.ErrInvalidPath; a segment that
// names no directory is blobfs.ErrNotFound, naming the prefix that failed.
//
// Resolution is iterative: one directory_child read per segment, starting
// from the root. Standard SQL has no ordered array parameter, so the
// segments cannot bind as one list and be walked by a single recursive
// statement at the standard tier; the round trips are one per segment. A
// native variant could resolve a path in one statement. It walks the
// way ResolveDirectoryFrom does, starting at blobfs.RootID.
func (s *Store) ResolveDirectory(ctx context.Context, sess sqlate.Session, path string) (blobfs.Directory, error) {
	segments, err := splitPath(path)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: resolve %q: %w", path, err)
	}
	dir, err := s.Root(ctx, sess)
	if err != nil {
		return blobfs.Directory{}, err
	}
	dir, err = s.walk(ctx, sess, dir, segments, "/")
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: resolve %q %w", path, err)
	}
	return dir, nil
}

// ResolveDirectoryFrom returns the directory at rel below the directory
// with startID. A relative path is a/b: names separated by slashes with no
// leading slash, each normalized before it is compared. The empty string
// and . name the start directory itself. A path that starts with a slash
// is absolute and is blobfs.ErrInvalidPath, as is one with an empty
// segment, a trailing slash, or a segment ValidateName refuses, which
// covers .. and so rules out upward navigation. A start that is not a
// directory, including a file's id, is blobfs.ErrNotFound; a segment that
// names no directory is blobfs.ErrNotFound naming the prefix that failed.
//
// Resolution is one directory_by_id read of the start and then one
// directory_child read per segment, as ResolveDirectory walks from the
// root; the session may be the pool or a transaction. A consumer that
// holds a directory's id resolves below it without repeating the walk
// from the root.
func (s *Store) ResolveDirectoryFrom(ctx context.Context, sess sqlate.Session, startID, rel string) (blobfs.Directory, error) {
	segments, err := splitRelativePath(rel)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: resolve %q from %s: %w", rel, startID, err)
	}
	dir, err := s.directoryByID.One(ctx, sess, query.Args{"id": startID})
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: resolve %q from %s: %w", rel, startID, notFound(err))
	}
	dir, err = s.walk(ctx, sess, dir, segments, "")
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: resolve %q from %s %w", rel, startID, err)
	}
	return dir, nil
}

// walk resolves segments below dir, one directory_child read each, and
// returns the last directory reached. A segment that names no directory is
// blobfs.ErrNotFound. The error reads "at <prefix>: ...", where the prefix
// is the segments walked so far joined by slashes after lead: / for an
// absolute path and nothing for a relative one.
func (s *Store) walk(ctx context.Context, sess sqlate.Session, dir blobfs.Directory, segments []string, lead string) (blobfs.Directory, error) {
	for i, name := range segments {
		var err error
		dir, err = s.directoryChild.One(ctx, sess, query.Args{"parent_id": dir.ID, "name": name})
		if err != nil {
			return blobfs.Directory{}, fmt.Errorf("at %s%s: %w", lead, strings.Join(segments[:i+1], "/"), notFound(err))
		}
	}
	return dir, nil
}

// DirectoryPath returns the path of the directory with id: / for the root
// and /a/b below it, the names of the chain from the root's child down to
// the directory joined by slashes. The root's own name, /, contributes
// nothing. It is one recursive statement that walks upward from the
// directory, so its cost is the directory's depth. A directory that does
// not exist is blobfs.ErrNotFound.
func (s *Store) DirectoryPath(ctx context.Context, sess sqlate.Session, id string) (string, error) {
	chain, err := s.directoryAncestors.All(ctx, sess, query.Args{"id": id})
	if err != nil {
		return "", fmt.Errorf("data: path of %s: %w", id, err)
	}
	if len(chain) == 0 {
		return "", fmt.Errorf("data: path of %s: %w", id, blobfs.ErrNotFound)
	}
	if chain[0].ParentID != nil {
		return "", fmt.Errorf("data: path of %s: the chain of %d ancestors does not reach the root", id, len(chain))
	}
	var b strings.Builder
	for _, a := range chain[1:] {
		b.WriteString("/")
		b.WriteString(a.Name)
	}
	if b.Len() == 0 {
		return "/", nil
	}
	return b.String(), nil
}

// splitPath checks that path is absolute and returns its normalized,
// validated segments; the root path / has none. A trailing slash is an
// empty segment and is refused.
func splitPath(path string) ([]string, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("%w: %q does not start with /", blobfs.ErrInvalidPath, path)
	}
	if path == "/" {
		return nil, nil
	}
	return splitSegments(path[1:])
}

// splitRelativePath checks that rel is relative and returns its
// normalized, validated segments; the empty string and . have none. A
// leading slash is an absolute path and is refused, and a trailing slash
// is an empty segment and is refused.
func splitRelativePath(rel string) ([]string, error) {
	if strings.HasPrefix(rel, "/") {
		return nil, fmt.Errorf("%w: %q starts with /", blobfs.ErrInvalidPath, rel)
	}
	if rel == "" || rel == "." {
		return nil, nil
	}
	return splitSegments(rel)
}

// splitSegments splits names, one or more directory names joined by
// slashes, and returns each normalized and validated in order. A segment
// validName refuses, including an empty one, is blobfs.ErrInvalidPath
// wrapping the name's error, so an absolute and a relative path accept and
// refuse the same names.
func splitSegments(names string) ([]string, error) {
	segments := strings.Split(names, "/")
	for i, segment := range segments {
		name, err := validName(segment)
		if err != nil {
			return nil, fmt.Errorf("%w: segment %d: %w", blobfs.ErrInvalidPath, i+1, err)
		}
		segments[i] = name
	}
	return segments, nil
}
