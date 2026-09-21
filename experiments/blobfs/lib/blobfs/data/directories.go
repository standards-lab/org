package data

import (
	"context"
	"fmt"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// Root returns the root directory, the row seeded with blobfs.RootID. A
// database whose schema is not applied, or whose seed row is gone, is
// blobfs.ErrNotFound.
func (s *Store) Root(ctx context.Context, sess sqlate.Session) (blobfs.Directory, error) {
	return s.Directory(ctx, sess, blobfs.RootID)
}

// Directory returns the directory with id, or blobfs.ErrNotFound.
func (s *Store) Directory(ctx context.Context, sess sqlate.Session, id string) (blobfs.Directory, error) {
	d, err := s.directoryByID.One(ctx, sess, query.Args{"id": id})
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: directory %s: %w", id, notFound(err))
	}
	return d, nil
}

// Mkdir creates a directory named name under the directory with parentID
// and returns the row as the database holds it. The name is normalized and
// validated first; a refusal is a blobfs.NameError, and an empty name is
// one, so no call creates a row without a name. Every row Mkdir writes has
// a parent, so no call creates a root either: the one root is seeded by
// the schema. A name already held by a directory under the same parent is
// blobfs.ErrNameTaken, and a parent that does not exist is
// blobfs.ErrNotFound. One row is written, so the session may be the pool
// or a transaction.
func (s *Store) Mkdir(ctx context.Context, sess sqlate.Session, parentID, name string) (blobfs.Directory, error) {
	name, err := validName(name)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: mkdir: %w", err)
	}
	id := blobfs.NewID()
	if _, err := s.createDirectory.Exec(ctx, sess, query.Args{"id": id, "parent_id": parentID, "name": name}); err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: mkdir %q under %s: %w", name, parentID, classifyWrite(err))
	}
	d, err := s.directoryByID.One(ctx, sess, query.Args{"id": id})
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: read back directory %q: %w", name, err)
	}
	return d, nil
}

// RemoveDirectory removes the directory with id. The root is refused
// with blobfs.ErrRootDirectory before any SQL, and the statement itself
// never removes a row without a parent. A directory that still has child
// directories or files is blobfs.ErrNotEmpty, reported by the foreign
// keys blobfs_fk_directory_parent and blobfs_fk_file_directory, since
// there is no cascade; a consumer removes the contents first, deepest
// first. A consumer's own foreign key into blobfs_directory refuses the
// removal as blobfs.ErrReferenced, with the sqlate.ConstraintError
// reachable. A directory that does not exist is blobfs.ErrNotFound. One
// statement, so the session may be the pool or a transaction; a consumer
// that keeps a row of its own about the directory removes both in one
// transaction.
func (s *Store) RemoveDirectory(ctx context.Context, sess sqlate.Session, id string) error {
	if id == blobfs.RootID {
		return fmt.Errorf("data: remove directory %s: %w", id, blobfs.ErrRootDirectory)
	}
	n, err := s.removeDirectory.Exec(ctx, sess, query.Args{"id": id})
	if err != nil {
		return fmt.Errorf("data: remove directory %s: %w", id, classifyDelete(err))
	}
	if n == 0 {
		return fmt.Errorf("data: remove directory %s: %w", id, blobfs.ErrNotFound)
	}
	return nil
}

// Children lists the directories whose parent is parentID: one page under
// l, sorted and filtered by the declared fields id, parent_id, name,
// version, created_at, and updated_at, with the total under the same
// filters when l asks for one. The default sort is by name, and name is
// the key, so every sort is total. The root has no parent and never
// appears in a listing; Children of blobfs.RootID lists the depth-one
// directories. A parent that does not exist lists no rows and, on the
// first page, a total of zero.
func (s *Store) Children(ctx context.Context, sess sqlate.Session, parentID string, l Listing) (Page[blobfs.Directory], error) {
	page, err := s.children.run(ctx, sess, query.Args{"parent_id": parentID}, l)
	if err != nil {
		return Page[blobfs.Directory]{}, fmt.Errorf("data: children of %s: %w", parentID, err)
	}
	return page, nil
}

// ListFiles lists the files in the directory with directoryID: one page
// under l, sorted and filtered by the declared fields id, directory_id,
// name, status, size, content_type, etag, version, created_at, and
// updated_at, with the total under the same filters when l asks for one.
// The default sort is by name, and name is the key, so every sort is
// total. A directory that does not exist lists no rows and, on the first
// page, a total of zero.
func (s *Store) ListFiles(ctx context.Context, sess sqlate.Session, directoryID string, l Listing) (Page[blobfs.File], error) {
	page, err := s.files.run(ctx, sess, query.Args{"directory_id": directoryID}, l)
	if err != nil {
		return Page[blobfs.File]{}, fmt.Errorf("data: files in %s: %w", directoryID, err)
	}
	return page, nil
}
