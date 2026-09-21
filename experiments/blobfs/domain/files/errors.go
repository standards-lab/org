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

	// ErrNotOwned reports an operation refused by the ownership check: the
	// unit named does not own the depth-one ancestor of the listed path,
	// or, for an id-keyed operation, the unit does not own the scope
	// directory it named, or the target does not lie within that scope.
	// Another unit owns the directory, or no unit does.
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

	// ErrContainerMissing reports an object delete that found the store's
	// container gone. The provider reports it apart from a missing object,
	// which is the idempotent success a delete wants, because a missing
	// container says the configured target is gone rather than that this
	// object is; the delete refuses the step and leaves the row deleting.
	ErrContainerMissing = errors.New("files: the object store's container is missing")

	// ErrStopped reports a put or an rm that stopped after the step
	// --fail-after named, as asked. A StopError carries the command, the
	// step, and the row as it was left.
	ErrStopped = errors.New("files: stopped as requested")

	// ErrBookmarked reports a file delete refused because a unit bookmarks
	// the file: rm checks the bookmark table in the transaction that
	// begins the delete, after the begin has locked the row, and the
	// foreign key fk_bookmark_file refuses the row's removal should a
	// bookmark be inserted without holding the file. The caller removes
	// the bookmarks and reruns rm.
	ErrBookmarked = errors.New("files: the file is bookmarked")

	// ErrTreeBusy reports a recursive delete that stopped because a
	// directory it was emptying received rows at least as fast as they
	// were removed: a pass over the directory's listing left its total
	// no smaller than before. Nothing is inconsistent; a rerun continues.
	ErrTreeBusy = errors.New("files: the directory keeps receiving rows while it is being removed")

	// ErrAlreadyBookmarked reports a bookmark add of a file the unit has
	// bookmarked already, active or not: the violation of the bookmark
	// table's primary key. A bookmark is added once and removed once; there
	// is no activation of an existing one.
	ErrAlreadyBookmarked = errors.New("files: the unit has bookmarked the file already")

	// ErrActiveBookmark reports a bookmark add with --active while another
	// bookmark of the unit is active: the violation of the partial unique
	// index uq_bookmark_active, which allows one active bookmark per unit.
	// The other bookmark is left as it is; the caller removes it first.
	ErrActiveBookmark = errors.New("files: the unit has an active bookmark already")

	// ErrNoBookmark reports a bookmark rm of a file the unit has not
	// bookmarked. The file exists; the bookmark does not.
	ErrNoBookmark = errors.New("files: the unit has no bookmark of the file")

	// ErrMoveAcrossScopes reports a mv whose source and destination lie
	// under different top-level directories, or one of them at the top
	// level and the other below it. An owner row binds a top-level
	// directory, and a listing under --unit is scoped at that ancestor, so
	// a move that crossed it would carry an entry out of one unit's scope
	// into another's, or give a top-level directory an owner row at another
	// depth. A rename of a top-level directory stays at the top level and
	// is allowed.
	ErrMoveAcrossScopes = errors.New("files: a move stays under one top-level directory")
)

// The names of the constraints and the unique index the consumer's
// bookmark migration declares and database.go maps to the sentinels
// above. They are the consumer's own names, in the workspace's scheme
// <kind>_<table>_<detail> without the blobfs_ prefix, so a violation of a
// consumer constraint is told from one of blobfs's. The consumer's
// migration tests check that every constant names an object in the DDL.
const (
	// ConstraintPrimaryKeyBookmark is the primary key on bookmark
	// (unit_id, file_id). A violation on an add is ErrAlreadyBookmarked.
	ConstraintPrimaryKeyBookmark = "pk_bookmark"

	// ConstraintForeignKeyBookmarkFile is the foreign key from
	// bookmark.file_id to blobfs_file.id. A violation on an add is
	// blobfs.ErrNotFound: the file was removed between its resolution and
	// the insert.
	ConstraintForeignKeyBookmarkFile = "fk_bookmark_file"

	// ConstraintUniqueBookmarkActive is the partial unique index on
	// bookmark (unit_id) WHERE active. A violation on an add is
	// ErrActiveBookmark.
	ConstraintUniqueBookmarkActive = "uq_bookmark_active"
)

// StopError reports a put or an rm that --fail-after stopped between its
// steps, before the row was completed or removed: the command, the step
// it stopped after, and the row as it was left, pending after a put and
// deleting after an rm. It matches ErrStopped under errors.Is. The
// message says how to finish: a rerun of the same command at the same
// path resumes the row.
type StopError struct {
	Command string
	Step    Step
	Path    string
	File    blobfs.File
}

func (e *StopError) Error() string {
	return fmt.Sprintf("files: %s %s: stopped after step %s as requested; the row is %s (id %s); rerun %s to complete it", e.Command, e.Path, e.Step, e.File.Status, e.File.ID, e.Command)
}

// Unwrap returns ErrStopped, so errors.Is matches the sentinel.
func (e *StopError) Unwrap() error {
	return ErrStopped
}
