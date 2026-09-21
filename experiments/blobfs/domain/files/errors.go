package files

import (
	"errors"
	"fmt"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// The sentinels the consumer adds to the library's. A command's refusal
// matches one of these or one of blobfs's own, so a caller classifies it
// with errors.Is.
var (
	// ErrVerify reports a database the consumer's or blobfs's statements do
	// not prepare against. The usual cause is a schema that is not applied,
	// so the message says which command applies it.
	ErrVerify = errors.New("files: the database does not satisfy the statements; if the schema is not applied, run blobfs schema up")

	// ErrNotOwned reports a listing refused because the unit named does not
	// own the depth-one ancestor of the listed path: another unit owns it,
	// or no unit does.
	ErrNotOwned = errors.New("files: the unit does not own the directory")

	// ErrUnitDepth reports a mkdir with --unit at a path that is not at
	// depth one. An owner row binds a top-level directory only; every
	// directory below it is in the top-level directory's scope.
	ErrUnitDepth = errors.New("files: --unit applies to a top-level directory only")

	// ErrNoCursorAtRoot reports a listing of / under a unit that was asked
	// to continue from a cursor. That listing reads the consumer's owner
	// read model through the query library's projection, which pages by
	// number only, and lists no files.
	ErrNoCursorAtRoot = errors.New("files: ls / --unit pages by number only; the owner read model takes no cursor")

	// ErrNoStorage reports a file command on a store built without an
	// object store, which the hermetic tests do; the composition root
	// always supplies one.
	ErrNoStorage = errors.New("files: no object store is configured")

	// ErrStorageUnavailable reports an object store that could not be
	// reached or started: the endpoint is down, the credential was
	// rejected, or the store was shut down. The store's error is reachable.
	ErrStorageUnavailable = errors.New("files: the object store is unavailable")

	// ErrObjectMissing reports a file whose row says available while the
	// store holds no object under its key.
	ErrObjectMissing = errors.New("files: the file's object is missing from the store")

	// ErrObjectTooLarge reports a put whose body exceeds the object store's
	// configured bound.
	ErrObjectTooLarge = errors.New("files: the object exceeds the store's size bound")

	// ErrNotAvailable reports a read of a file whose object is not there to
	// read: a pending file, whose write has not completed, or a deleting
	// one. stat shows the status; cat refuses.
	ErrNotAvailable = errors.New("files: the file is not available")

	// ErrStopped reports a put that stopped after the step --fail-after
	// named, as asked. A StopError carries the step and the pending row.
	ErrStopped = errors.New("files: stopped as requested")
)

// StopError reports a put that --fail-after stopped between the steps of
// the write, before the row was completed: the step it stopped after and
// the row it left pending. It matches ErrStopped under errors.Is. The
// message says how to finish the write: a put of the same path resumes
// the pending row.
type StopError struct {
	Step Step
	Path string
	File blobfs.File
}

func (e *StopError) Error() string {
	return fmt.Sprintf("files: put %s: stopped after step %s as requested; the row is pending (id %s); rerun put to complete it", e.Path, e.Step, e.File.ID)
}

// Unwrap returns ErrStopped, so errors.Is matches the sentinel.
func (e *StopError) Unwrap() error {
	return ErrStopped
}
