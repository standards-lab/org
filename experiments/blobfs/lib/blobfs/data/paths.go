package data

import (
	"context"
	"database/sql"
	"errors"
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
// Resolution is the variant's: the baseline reads the root and then one
// directory_child per segment, because standard SQL has no ordered array
// parameter to walk by in one statement, so its round trips are one per
// segment; the Postgres variant walks the whole path in one statement.
// Both resolve the way ResolveDirectoryFrom does, starting at
// blobfs.RootID.
func (s *Store) ResolveDirectory(ctx context.Context, sess sqlate.Session, path string) (blobfs.Directory, error) {
	segments, err := splitPath(path)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: resolve %q: %w", path, err)
	}
	dir, depth, err := s.variant.ResolvePath(ctx, sess, blobfs.RootID, segments)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: resolve %q: %w", path, err)
	}
	if depth < len(segments) {
		return blobfs.Directory{}, fmt.Errorf("data: resolve %q %w", path, missingAt("/", segments, depth))
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
// Resolution is the variant's, as in ResolveDirectory: the baseline reads
// the start by id and then one directory_child per segment, and the
// Postgres variant walks the whole path in one statement. The session may
// be the pool or a transaction. A consumer that holds a directory's id
// resolves below it without repeating the walk from the root.
func (s *Store) ResolveDirectoryFrom(ctx context.Context, sess sqlate.Session, startID, rel string) (blobfs.Directory, error) {
	segments, err := splitRelativePath(rel)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: resolve %q from %s: %w", rel, startID, err)
	}
	dir, depth, err := s.variant.ResolvePath(ctx, sess, startID, segments)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: resolve %q from %s: %w", rel, startID, err)
	}
	if depth < len(segments) {
		return blobfs.Directory{}, fmt.Errorf("data: resolve %q from %s %w", rel, startID, missingAt("", segments, depth))
	}
	return dir, nil
}

// missingAt is the error for a walk that stopped at depth, because
// segments[depth] named no directory: blobfs.ErrNotFound reading
// "at <prefix>: ...", where the prefix is the segments up to and including
// the missing one joined by slashes after lead: / for an absolute path
// and nothing for a relative one.
func missingAt(lead string, segments []string, depth int) error {
	return fmt.Errorf("at %s%s: %w", lead, strings.Join(segments[:depth+1], "/"), blobfs.ErrNotFound)
}

// ResolvePath runs the baseline's walk: the read of the start by id, then
// one directory_child read per segment until one names no directory or
// the segments run out, so a resolved path of n segments is n+1
// statements. See Variant.
func (v *Standard) ResolvePath(ctx context.Context, sess sqlate.Session, startID string, segments []string) (blobfs.Directory, int, error) {
	dir, err := v.directoryByID.One(ctx, sess, query.Args{"id": startID})
	if err != nil {
		return blobfs.Directory{}, 0, notFound(err)
	}
	for depth, name := range segments {
		child, err := v.directoryChild.One(ctx, sess, query.Args{"parent_id": dir.ID, "name": name})
		if errors.Is(err, sql.ErrNoRows) {
			return dir, depth, nil
		}
		if err != nil {
			return blobfs.Directory{}, 0, err
		}
		dir = child
	}
	return dir, len(segments), nil
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
