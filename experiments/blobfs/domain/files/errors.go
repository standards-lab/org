package files

import "errors"

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
)
