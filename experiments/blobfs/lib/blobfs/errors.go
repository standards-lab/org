package blobfs

import "errors"

// The sentinels the persistence layer maps its outcomes onto. A database
// violation of one of blobfs's own documented constraints becomes one of
// these; a violation of a consumer's constraint is left unclassified.
var (
	// ErrNotFound reports a directory or file that does not exist, including
	// a parent or directory id that an insert or move referenced.
	ErrNotFound = errors.New("blobfs: not found")

	// ErrNameTaken reports a name already held in the target directory by a
	// row of the same table. A deleting row holds its name until it is
	// removed.
	ErrNameTaken = errors.New("blobfs: name taken")

	// ErrInvalidName reports a name ValidateName refused. A NameError
	// carries the reason.
	ErrInvalidName = errors.New("blobfs: invalid name")

	// ErrInvalidPath reports a path the persistence layer could not read:
	// one that does not start with a slash, or one with a segment
	// ValidateName refuses, in which case the error also matches
	// ErrInvalidName. A path is / for the root and /a/b below it.
	ErrInvalidPath = errors.New("blobfs: invalid path")

	// ErrRootDirectory reports an operation refused because it targets the
	// root: creating a second row with no parent, or deleting, moving, or
	// renaming the root. There is exactly one root per install, seeded by
	// the schema with the id RootID.
	ErrRootDirectory = errors.New("blobfs: the root directory")

	// ErrInvalidKey reports a key the store refused. A KeyError carries the
	// key and the store's reason.
	ErrInvalidKey = errors.New("blobfs: invalid key")

	// ErrNotEmpty reports a directory delete refused because the directory
	// still has child directories or files.
	ErrNotEmpty = errors.New("blobfs: directory not empty")

	// ErrInvalidTransition reports a status change the transition table
	// does not allow. A TransitionError carries the two statuses.
	ErrInvalidTransition = errors.New("blobfs: invalid status transition")

	// ErrDeleting reports a mutation refused because the row is deleting:
	// a move or rename, or a status change out of deleting.
	ErrDeleting = errors.New("blobfs: row is deleting")

	// ErrCycle reports a directory move whose new parent sits inside the
	// directory's own subtree, the directory itself included.
	ErrCycle = errors.New("blobfs: move would create a cycle")
)
