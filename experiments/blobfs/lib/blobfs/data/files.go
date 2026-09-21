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

// File returns the file with id, or blobfs.ErrNotFound.
func (s *Store) File(ctx context.Context, sess sqlate.Session, id string) (blobfs.File, error) {
	f, err := s.fileByID.One(ctx, sess, query.Args{"id": id})
	if err != nil {
		return blobfs.File{}, fmt.Errorf("data: file %s: %w", id, notFound(err))
	}
	return f, nil
}

// FileByName returns the file named name in the directory with
// directoryID, whatever its status, or blobfs.ErrNotFound. The name is
// normalized before it is compared and validated first; a refusal is a
// blobfs.NameError. It is the last step of resolving a file's path, and it
// is how a retried write finds the pending row it resumes.
func (s *Store) FileByName(ctx context.Context, sess sqlate.Session, directoryID, name string) (blobfs.File, error) {
	name, err := validName(name)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("data: file %q: %w", name, err)
	}
	f, err := s.fileByName.One(ctx, sess, query.Args{"directory_id": directoryID, "name": name})
	if err != nil {
		return blobfs.File{}, fmt.Errorf("data: file %q in %s: %w", name, directoryID, notFound(err))
	}
	return f, nil
}

// BeginFileWrite is the first step of the two-phase write: it inserts the
// file's row as blobfs.StatusPending, before any object exists, and returns
// the row as the database holds it. The name is normalized and validated
// first (a refusal is a blobfs.NameError), the id is minted, and the key is
// built from the id and the name and validated against keys (a refusal is a
// blobfs.KeyError), all before any SQL. A name already held in the
// directory, by a row of any status, is blobfs.ErrNameTaken, and a
// directory that does not exist is blobfs.ErrNotFound. The content type is
// what the caller declares; the row's size and entity tag stay nil until
// the write completes.
//
// The session may be the pool or a transaction. Inside a caller's
// transaction the pending row commits with the caller's own rows, so a
// consumer that records something about the file writes both in one unit;
// on the pool the row is durable as soon as the call returns. The caller
// then stores the object under the row's Key and calls CompleteFileWrite
// with the row's ID and Version. A stop between the two steps leaves the
// row pending, where a listing or FileByName finds it; a retry of the same
// write finds the row through FileByName and completes it, and an
// abandoned write is removed through the delete steps.
func (s *Store) BeginFileWrite(ctx context.Context, sess sqlate.Session, keys blobfs.KeyValidator, directoryID, name, contentType string) (blobfs.File, error) {
	name, err := validName(name)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("data: begin write: %w", err)
	}
	id := blobfs.NewID()
	key, err := blobfs.NewKey(keys, id, name)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("data: begin write of %q: %w", name, err)
	}
	args := query.Args{"id": id, "directory_id": directoryID, "name": name, "key": key, "content_type": contentType}
	if _, err := s.beginFileWrite.Exec(ctx, sess, args); err != nil {
		return blobfs.File{}, fmt.Errorf("data: begin write of %q in %s: %w", name, directoryID, classifyWrite(err))
	}
	f, err := s.fileByID.One(ctx, sess, query.Args{"id": id})
	if err != nil {
		return blobfs.File{}, fmt.Errorf("data: read back file %q: %w", name, err)
	}
	return f, nil
}

// CompleteFileWrite is the last step of the two-phase write: it moves the
// pending row with id to blobfs.StatusAvailable, records what the store
// reported about the object, advances the version, and returns the row as
// the database holds it afterward. The update is guarded by version, the
// value the caller read from the pending row, through the query library's
// optimistic-concurrency protocol, and by the row's status: only a pending
// row completes.
//
// A row that does not exist is blobfs.ErrNotFound. A row whose version
// moved on is query.ErrVersionMismatch. A row at the expected version that
// is no longer pending is a blobfs.TransitionError from its status to
// available, which matches blobfs.ErrDeleting when a delete began in the
// meantime and blobfs.ErrInvalidTransition when the write was completed
// already; the guard alone cannot tell those from a version conflict, so
// the row is read once more to classify. One statement changes the row,
// so the session may be the pool or a transaction.
func (s *Store) CompleteFileWrite(ctx context.Context, sess sqlate.Session, id string, version int64, obj blobfs.Object) (blobfs.File, error) {
	args := query.Args{"id": id, "size": obj.Size, "content_type": obj.ContentType, "etag": obj.ETag}
	_, err := s.completeFileWrite.Run(ctx, sess, version, args)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return blobfs.File{}, fmt.Errorf("data: complete write of file %s: %w", id, blobfs.ErrNotFound)
	case errors.Is(err, query.ErrVersionMismatch):
		// The guard checks the version alone, so a row at the expected
		// version that failed the status predicate reports as a mismatch.
		f, readErr := s.File(ctx, sess, id)
		if readErr != nil {
			return blobfs.File{}, fmt.Errorf("data: complete write of file %s: %w", id, readErr)
		}
		if f.Version == version {
			return blobfs.File{}, fmt.Errorf("data: complete write of file %s: %w", id, blobfs.Transition(f.Status, blobfs.StatusAvailable))
		}
		return blobfs.File{}, fmt.Errorf("data: complete write of file %s: %w", id, err)
	case err != nil:
		return blobfs.File{}, fmt.Errorf("data: complete write of file %s: %w", id, err)
	}
	f, err := s.File(ctx, sess, id)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("data: read back completed file %s: %w", id, err)
	}
	return f, nil
}

// CompleteFileDelete is the last step of a file delete: it removes the
// row with id, after the caller has deleted the object under the row's
// Key. Only a deleting row is removed. A row that is already gone is
// success, because the step's postcondition is the row's absence and a
// retry after a crash cannot tell its own earlier completion from a row
// that never existed; a caller that wants to report a missing file
// resolves it before the begin step. A row that exists and is not
// deleting is blobfs.ErrNotDeleting and is left as it is: the delete has
// not begun, so the object may still be wanted.
//
// A foreign key from a consumer's table that references the row refuses
// the removal as blobfs.ErrReferenced, with the sqlate.ConstraintError
// reachable, so the consumer matches the constraint's name against its
// own; the row stays deleting, and a retry after the consumer's row is
// gone converges. One statement removes the row, so the session may be
// the pool or a transaction, and the statement is the same for every
// variant.
func (s *Store) CompleteFileDelete(ctx context.Context, sess sqlate.Session, id string) error {
	n, err := s.removeFile.Exec(ctx, sess, query.Args{"id": id})
	if err != nil {
		return fmt.Errorf("data: complete delete of file %s: %w", id, classifyDelete(err))
	}
	if n > 0 {
		return nil
	}
	// No row was removed: the row is gone, which is success, or it exists
	// in a status the removal refuses.
	f, err := s.File(ctx, sess, id)
	switch {
	case errors.Is(err, blobfs.ErrNotFound):
		return nil
	case err != nil:
		return fmt.Errorf("data: complete delete of file %s: %w", id, err)
	case f.Status == blobfs.StatusDeleting:
		// A concurrent begin moved the row to deleting between the removal
		// and this read. The removal is repeated once; a row that is
		// deleting never leaves that status except by removal, so the
		// second attempt removes it or finds it gone.
		if _, err := s.removeFile.Exec(ctx, sess, query.Args{"id": id}); err != nil {
			return fmt.Errorf("data: complete delete of file %s: %w", id, classifyDelete(err))
		}
		return nil
	}
	return fmt.Errorf("data: complete delete of file %s: the row is %s: %w", id, f.Status, blobfs.ErrNotDeleting)
}
