package data

import (
	"context"
	"database/sql"
	"errors"
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
// the schema. The id is minted, or taken from WithID and checked, before
// any SQL. A name already held by a directory under the same parent is
// blobfs.ErrNameTaken, an id another directory carries is
// blobfs.ErrIDTaken, and a parent that does not exist is
// blobfs.ErrNotFound. One row is written, so the session may be the pool
// or a transaction.
func (s *Store) Mkdir(ctx context.Context, sess sqlate.Session, parentID, name string, opts ...WriteOption) (blobfs.Directory, error) {
	name, err := validName(name)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: mkdir: %w", err)
	}
	id, err := rowID(opts)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: mkdir %q: %w", name, err)
	}
	d, err := s.insertDirectory(ctx, sess, id, parentID, name)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: mkdir %q under %s: %w", name, parentID, err)
	}
	return d, nil
}

// EnsureDirectory returns the directory named name under the directory
// with parentID, creating it when none exists, and reports whether this
// call created it. It is the insert-or-find a seeder needs: a seeded
// directory is read on every run after the first, without the seeder
// catching blobfs.ErrNameTaken and looking the name up itself. The name
// is normalized and validated and the id resolved as in Mkdir, before any
// SQL; a found row keeps its own id whatever WithID supplied.
//
// The lookup runs first and the insert only when it found no row, so the
// common case runs no failing statement and composes into a caller's
// transaction, where a seeder writes its own rows beside the directory.
// A creator that commits between the lookup and the insert makes the
// insert fail as blobfs.ErrNameTaken. On the pool the row is then looked
// up again and returned as found. Inside a transaction the error is
// returned instead, because on Postgres the failed insert has aborted the
// transaction, and the caller retries the transaction. The other refusals
// are Mkdir's.
func (s *Store) EnsureDirectory(ctx context.Context, sess sqlate.Session, parentID, name string, opts ...WriteOption) (blobfs.Directory, bool, error) {
	name, err := validName(name)
	if err != nil {
		return blobfs.Directory{}, false, fmt.Errorf("data: ensure directory: %w", err)
	}
	id, err := rowID(opts)
	if err != nil {
		return blobfs.Directory{}, false, fmt.Errorf("data: ensure directory %q: %w", name, err)
	}
	args := query.Args{"parent_id": parentID, "name": name}
	d, err := s.directoryChild.One(ctx, sess, args)
	switch {
	case err == nil:
		return d, false, nil
	case !errors.Is(err, sql.ErrNoRows):
		return blobfs.Directory{}, false, fmt.Errorf("data: ensure directory %q under %s: %w", name, parentID, err)
	}
	d, err = s.insertDirectory(ctx, sess, id, parentID, name)
	switch {
	case err == nil:
		return d, true, nil
	case !errors.Is(err, blobfs.ErrNameTaken) || inTransaction(sess):
		return blobfs.Directory{}, false, fmt.Errorf("data: ensure directory %q under %s: %w", name, parentID, err)
	}
	// A concurrent creator committed the name between the lookup and the
	// insert; the row exists now.
	d, err = s.directoryChild.One(ctx, sess, args)
	if err != nil {
		return blobfs.Directory{}, false, fmt.Errorf("data: ensure directory %q under %s after a concurrent create: %w", name, parentID, notFound(err))
	}
	return d, false, nil
}

// insertDirectory inserts the directory row under id and reads it back.
// The name is normalized and validated already. A constraint violation is
// classified through the write mapping and returned without context, so
// each caller adds its own.
func (s *Store) insertDirectory(ctx context.Context, sess sqlate.Session, id, parentID, name string) (blobfs.Directory, error) {
	if _, err := s.createDirectory.Exec(ctx, sess, query.Args{"id": id, "parent_id": parentID, "name": name}); err != nil {
		return blobfs.Directory{}, classifyWrite(err)
	}
	d, err := s.directoryByID.One(ctx, sess, query.Args{"id": id})
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("read back: %w", err)
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
