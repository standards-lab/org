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
// first (a refusal is a blobfs.NameError), the id is minted or taken from
// WithID and checked (a refusal is a blobfs.IDError), and the key is
// built from the id and the name and validated against keys (a refusal is a
// blobfs.KeyError), all before any SQL. A name already held in the
// directory, by a row of any status, is blobfs.ErrNameTaken, an id
// another file carries is blobfs.ErrIDTaken, and a directory that does
// not exist is blobfs.ErrNotFound. The content type is what the caller
// declares; the row's size and entity tag stay nil until the write
// completes.
//
// The session may be the pool or a transaction. Inside a caller's
// transaction the pending row commits with the caller's own rows, so a
// consumer that records something about the file writes both in one unit;
// on the pool the row is durable as soon as the call returns. The caller
// then stores the object under the row's Key and calls CompleteFileWrite
// with the row's ID and Version. A stop between the two steps leaves the
// row pending, where a listing or FileByName finds it; a retry of the same
// write finds the row through FileByName, or through
// BeginOrResumeFileWrite, and completes it, and an abandoned write is
// removed through the delete steps.
func (s *Store) BeginFileWrite(ctx context.Context, sess sqlate.Session, keys blobfs.KeyValidator, directoryID, name, contentType string, opts ...WriteOption) (blobfs.File, error) {
	name, err := validName(name)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("data: begin write: %w", err)
	}
	id, err := rowID(opts)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("data: begin write of %q: %w", name, err)
	}
	key, err := blobfs.NewKey(keys, id, name)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("data: begin write of %q: %w", name, err)
	}
	f, err := s.insertFile(ctx, sess, id, directoryID, name, key, contentType)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("data: begin write of %q in %s: %w", name, directoryID, err)
	}
	return f, nil
}

// WriteOutcome is what BeginOrResumeFileWrite did with the name: inserted
// a pending row, took up a pending row an earlier write left, or found a
// row in another status and inserted nothing.
type WriteOutcome string

const (
	// WriteCreated reports a new pending row: the write's first step ran.
	WriteCreated WriteOutcome = "created"

	// WriteResumed reports a pending row an earlier write left, returned
	// for the caller to store the object and complete at the row's version.
	WriteResumed WriteOutcome = "resumed"

	// WriteExists reports a row that is available or deleting, returned
	// unchanged; its Status says which. The caller decides what that means:
	// a put refuses the name, and a seeder skips it.
	WriteExists WriteOutcome = "exists"
)

// BeginOrResumeFileWrite is the first step of the two-phase write as a
// retry-safe operation: it returns the file row that holds name in the
// directory with directoryID and the WriteOutcome that says how. No row
// is BeginFileWrite under the same arguments and WriteCreated. A pending
// row is returned as it is and WriteResumed, so the caller stores the
// object under its Key and completes it at its Version, as a retry of a
// stopped write does. An available or deleting row is returned as it is
// and WriteExists, and nothing is inserted. The name, the id, and the key
// are validated as in BeginFileWrite, before any SQL; a found row keeps
// its own id and key whatever WithID supplied.
//
// The lookup runs first and the insert only when it found no row, so the
// common paths run no failing statement and compose into a caller's
// transaction. A writer that commits the name between the lookup and the
// insert makes the insert fail as blobfs.ErrNameTaken. On the pool the
// row is then looked up again and reported by its status. Inside a
// transaction the error is returned instead, because on Postgres the
// failed insert has aborted the transaction, and the caller retries the
// transaction. The other refusals are BeginFileWrite's.
func (s *Store) BeginOrResumeFileWrite(ctx context.Context, sess sqlate.Session, keys blobfs.KeyValidator, directoryID, name, contentType string, opts ...WriteOption) (blobfs.File, WriteOutcome, error) {
	name, err := validName(name)
	if err != nil {
		return blobfs.File{}, "", fmt.Errorf("data: begin or resume write: %w", err)
	}
	id, err := rowID(opts)
	if err != nil {
		return blobfs.File{}, "", fmt.Errorf("data: begin or resume write of %q: %w", name, err)
	}
	key, err := blobfs.NewKey(keys, id, name)
	if err != nil {
		return blobfs.File{}, "", fmt.Errorf("data: begin or resume write of %q: %w", name, err)
	}
	args := query.Args{"directory_id": directoryID, "name": name}
	f, err := s.fileByName.One(ctx, sess, args)
	switch {
	case err == nil:
		return f, found(f), nil
	case !errors.Is(err, sql.ErrNoRows):
		return blobfs.File{}, "", fmt.Errorf("data: begin or resume write of %q in %s: %w", name, directoryID, err)
	}
	f, err = s.insertFile(ctx, sess, id, directoryID, name, key, contentType)
	switch {
	case err == nil:
		return f, WriteCreated, nil
	case !errors.Is(err, blobfs.ErrNameTaken) || inTransaction(sess):
		return blobfs.File{}, "", fmt.Errorf("data: begin or resume write of %q in %s: %w", name, directoryID, err)
	}
	// A concurrent writer committed the name between the lookup and the
	// insert; the row exists now.
	f, err = s.fileByName.One(ctx, sess, args)
	if err != nil {
		return blobfs.File{}, "", fmt.Errorf("data: begin or resume write of %q in %s after a concurrent write: %w", name, directoryID, notFound(err))
	}
	return f, found(f), nil
}

// found is the outcome for a row the lookup returned: resumed when it is
// pending, exists otherwise.
func found(f blobfs.File) WriteOutcome {
	if f.Status == blobfs.StatusPending {
		return WriteResumed
	}
	return WriteExists
}

// insertFile inserts the pending row under id and key through the variant
// and returns it as the database holds it. The name is normalized and
// validated and the key validated already. A constraint violation is
// classified through the write mapping and returned without context, so
// each caller adds its own.
func (s *Store) insertFile(ctx context.Context, sess sqlate.Session, id, directoryID, name, key, contentType string) (blobfs.File, error) {
	f, err := s.variant.InsertFile(ctx, sess, id, directoryID, name, key, contentType)
	if err != nil {
		return blobfs.File{}, classifyWrite(err)
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
// the row is read once more to classify, through CompleteRefusal on every
// variant. The step is the variant's: the baseline runs the guard and a
// read, and the Postgres variant one statement. One statement changes the
// row, so the session may be the pool or a transaction.
func (s *Store) CompleteFileWrite(ctx context.Context, sess sqlate.Session, id string, version int64, obj blobfs.Object) (blobfs.File, error) {
	f, err := s.variant.CompleteFileWrite(ctx, sess, id, version, obj)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("data: complete write of file %s: %w", id, err)
	}
	return f, nil
}

// HoldOption configures one call of HoldFile beyond its required
// arguments.
type HoldOption func(*holdOptions)

// holdOptions collects what the hold options set.
type holdOptions struct {
	version    int64
	hasVersion bool
}

// AtVersion makes HoldFile match the row only at version, the value the
// caller read from a listing or an earlier read, so a caller that acts on
// a row it has not read inside its transaction learns that the row moved
// on. A row at another version is query.ErrVersionMismatch.
func AtVersion(version int64) HoldOption {
	return func(o *holdOptions) {
		o.version = version
		o.hasVersion = true
	}
}

// HoldFile locks the row of the file with id for the rest of the
// caller's transaction, so that no delete of the file begins before the
// transaction ends: the first half of the reference-then-delete rule. A
// consumer calls it before it inserts a row that references the file, in
// the same transaction as the insert, and the delete protocol's begin
// step, which takes the same lock, waits for that transaction and then
// sees the reference. The hold is an update that assigns a column to
// itself: it changes no value and advances no version, so other holders
// of the row's version stay valid. Only a row that is not deleting is
// held, because a file whose delete has begun must take no new
// reference; a pending row is held like an available one.
//
// The session must be a transaction, and any other is
// query.ErrTransactionRequired, refused before any SQL: on the pool the
// lock would be released as the statement ends and hold nothing. A file
// that does not exist is blobfs.ErrNotFound. A row that is deleting is
// blobfs.ErrDeleting, whatever its version, since no version will make
// it holdable. With AtVersion, a row that is not deleting and sits at
// another version is query.ErrVersionMismatch, with the expected and
// current versions in the text. When the hold matches no row, the row is
// read once more, in the same transaction, to classify.
func (s *Store) HoldFile(ctx context.Context, sess sqlate.Session, id string, opts ...HoldOption) error {
	var o holdOptions
	for _, opt := range opts {
		opt(&o)
	}
	args := query.Args{"id": id}
	hold := s.holdFile
	if o.hasVersion {
		args["version"] = o.version
		hold = s.holdFileAtVersion
	}
	n, err := hold.Exec(ctx, sess, args)
	if err != nil {
		return fmt.Errorf("data: hold file %s: %w", id, err)
	}
	if n > 0 {
		return nil
	}
	// No row was held: the row is gone, deleting, or at another version.
	f, err := s.fileByID.One(ctx, sess, args)
	switch {
	case err != nil:
		return fmt.Errorf("data: hold file %s: %w", id, notFound(err))
	case f.Status == blobfs.StatusDeleting:
		return fmt.Errorf("data: hold file %s: the row is %s: %w", id, f.Status, blobfs.ErrDeleting)
	case o.hasVersion && f.Version != o.version:
		return fmt.Errorf("data: hold file %s: %w: expected %d, current %d", id, query.ErrVersionMismatch, o.version, f.Version)
	}
	return fmt.Errorf("data: hold file %s: the hold matched no row, yet the row is %s at version %d", id, f.Status, f.Version)
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
