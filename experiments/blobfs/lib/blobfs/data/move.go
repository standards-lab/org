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

// IsWithin reports whether the directory with id lies within the subtree
// of the directory with ancestorID, that directory itself included. It is
// the cycle check of a directory move, exported so a caller can run it on
// its own, for example to refuse a move early in a user interface: a
// directory D may move under a new parent P only when IsWithin(P, D) is
// false. One recursive statement walks upward from id, so the cost is the
// depth of id. A directory that does not exist is within nothing. The
// answer is reliable only while no other transaction is moving
// directories, which is what MoveDirectory's lock provides; the walk
// itself terminates only while the tree has no cycle.
func (s *Store) IsWithin(ctx context.Context, sess sqlate.Session, id, ancestorID string) (bool, error) {
	n, err := s.directoryIsWithin.One(ctx, sess, query.Args{"id": id, "ancestor_id": ancestorID})
	if err != nil {
		return false, fmt.Errorf("data: is %s within %s: %w", id, ancestorID, err)
	}
	return n > 0, nil
}

// MoveDirectory moves the directory with id under the directory with
// parentID as name, which also renames it when the name differs, and
// returns the row as the database holds it afterward. The move runs in
// sess, which must be a transaction, in three steps that must see one
// tree lock: the store's LockTree, then IsWithin(parentID, id), then the
// guarded update of parent_id and name. The update is guarded by version,
// the value the caller read from the directory's row, through the query
// library's optimistic-concurrency protocol; the caller resolves the
// directory in the same transaction and passes its Version. The
// directory's children and files follow it, because they reference it by
// id and every path is computed at read time; no object moves, since no
// key encodes a path.
//
// The root is blobfs.ErrRootDirectory, refused before any SQL. A session
// that is not a transaction is query.ErrTransactionRequired. A new parent
// that is the directory itself or one of its descendants is
// blobfs.ErrCycle, and nothing changes. A directory that does not exist,
// or a new parent that does not, is blobfs.ErrNotFound (the parent's
// through the foreign key blobfs_fk_directory_parent); a name already
// held by a directory under the new parent is blobfs.ErrNameTaken. A file
// under the new parent with the same name is no conflict: directories and
// files have separate name spaces. A row whose version moved on is
// query.ErrVersionMismatch. A refused name is a blobfs.NameError.
//
// The lock is what closes the race between two opposing moves: each
// takes it before its check, so the second one's check sees the first
// one's committed update and is refused. On a variant whose Serializes
// reports false the lock is a no-op, both checks pass against the same
// committed state, and the two commits leave the two directories each
// other's ancestor, detached from the root; a caller on such a variant
// serializes directory moves outside the database.
func (s *Store) MoveDirectory(ctx context.Context, sess sqlate.Session, id, parentID, name string, version int64) (blobfs.Directory, error) {
	if id == blobfs.RootID {
		return blobfs.Directory{}, fmt.Errorf("data: move directory %s: %w", id, blobfs.ErrRootDirectory)
	}
	name, err := validName(name)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: move directory %s: %w", id, err)
	}
	if err := s.LockTree(ctx, sess); err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: move directory %s: %w", id, err)
	}
	within, err := s.IsWithin(ctx, sess, parentID, id)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: move directory %s: %w", id, err)
	}
	if within {
		return blobfs.Directory{}, fmt.Errorf("data: move directory %s under %s: the new parent is the directory or one of its descendants: %w", id, parentID, blobfs.ErrCycle)
	}
	args := query.Args{"id": id, "parent_id": parentID, "name": name}
	_, err = s.reparentDirectory.Run(ctx, sess, version, args)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return blobfs.Directory{}, fmt.Errorf("data: move directory %s: %w", id, blobfs.ErrNotFound)
	case err != nil:
		return blobfs.Directory{}, fmt.Errorf("data: move directory %s under %s as %q: %w", id, parentID, name, classifyWrite(err))
	}
	d, err := s.Directory(ctx, sess, id)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: read back moved directory %s: %w", id, err)
	}
	return d, nil
}

// MoveFile moves the file with id into the directory with directoryID as
// name, which also renames it when the name differs, and returns the row
// as the database holds it afterward. The update is guarded by version,
// the value the caller read from the file's row, through the query
// library's optimistic-concurrency protocol, and by the row's status: a
// deleting row is left as it is. The key is untouched, so the object stays
// where it is and a rename moves nothing in the store. A pending row may
// move: its key was fixed at the insert, and a retry of its write finds
// it by its new name. No lock and no cycle check precede the update,
// because a file cannot be its own ancestor; one statement changes the
// row, so the session may be the pool or a transaction.
//
// A file that does not exist is blobfs.ErrNotFound, and so is a directory
// that does not exist, through the foreign key blobfs_fk_file_directory.
// A name already held by a file in the directory, by a row of any status,
// is blobfs.ErrNameTaken. A row whose version moved on is
// query.ErrVersionMismatch. A row at the expected version that is deleting
// is blobfs.ErrDeleting; the guard alone cannot tell that from a version
// conflict, so the row is read once more to classify. A refused name is a
// blobfs.NameError.
func (s *Store) MoveFile(ctx context.Context, sess sqlate.Session, id, directoryID, name string, version int64) (blobfs.File, error) {
	name, err := validName(name)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("data: move file %s: %w", id, err)
	}
	args := query.Args{"id": id, "directory_id": directoryID, "name": name}
	_, err = s.moveFile.Run(ctx, sess, version, args)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return blobfs.File{}, fmt.Errorf("data: move file %s: %w", id, blobfs.ErrNotFound)
	case errors.Is(err, query.ErrVersionMismatch):
		// The guard checks the version alone, so a row at the expected
		// version that failed the status predicate reports as a mismatch.
		f, readErr := s.File(ctx, sess, id)
		if readErr != nil {
			return blobfs.File{}, fmt.Errorf("data: move file %s: %w", id, readErr)
		}
		if f.Version == version && !f.Status.Mutable() {
			return blobfs.File{}, fmt.Errorf("data: move file %s: the row is %s: %w", id, f.Status, blobfs.ErrDeleting)
		}
		return blobfs.File{}, fmt.Errorf("data: move file %s: %w", id, err)
	case err != nil:
		return blobfs.File{}, fmt.Errorf("data: move file %s into %s as %q: %w", id, directoryID, name, classifyWrite(err))
	}
	f, err := s.File(ctx, sess, id)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("data: read back moved file %s: %w", id, err)
	}
	return f, nil
}
