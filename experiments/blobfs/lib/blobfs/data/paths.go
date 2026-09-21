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
// native variant could resolve a path in one statement.
func (s *Store) ResolveDirectory(ctx context.Context, sess sqlate.Session, path string) (blobfs.Directory, error) {
	segments, err := splitPath(path)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: resolve %q: %w", path, err)
	}
	dir, err := s.Root(ctx, sess)
	if err != nil {
		return blobfs.Directory{}, err
	}
	for i, name := range segments {
		dir, err = s.directoryChild.One(ctx, sess, query.Args{"parent_id": dir.ID, "name": name})
		if err != nil {
			return blobfs.Directory{}, fmt.Errorf("data: resolve %q at /%s: %w", path, strings.Join(segments[:i+1], "/"), notFound(err))
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
	segments := strings.Split(path[1:], "/")
	for i, segment := range segments {
		name, err := validName(segment)
		if err != nil {
			return nil, fmt.Errorf("%w: segment %d: %w", blobfs.ErrInvalidPath, i+1, err)
		}
		segments[i] = name
	}
	return segments, nil
}
