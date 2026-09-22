package data

import (
	"github.com/standards-lab/sqlate"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// WriteOption configures one call of Mkdir, EnsureDirectory,
// BeginFileWrite, or BeginOrResumeFileWrite beyond its required arguments.
type WriteOption func(*writeOptions)

// writeOptions collects what the write options set.
type writeOptions struct {
	id    string
	hasID bool
}

// WithID supplies the id of the row the call inserts, in place of one the
// store mints, so a seeded directory or file keeps the same id across
// resets and a re-seeded file reuses its key. The id is checked with
// blobfs.ParseID before any SQL: text that is not a UUID or the nil UUID
// is blobfs.ErrInvalidID, and the canonical form is what the row carries.
// An id a row of the same table already carries fails the primary key as
// blobfs.ErrIDTaken. When the call finds a row instead of inserting one,
// the found row keeps its own id and the option has no effect.
func WithID(id string) WriteOption {
	return func(o *writeOptions) {
		o.id = id
		o.hasID = true
	}
}

// rowID resolves the id a write inserts under: the caller's, in canonical
// form, or a minted one when no option supplied any.
func rowID(opts []WriteOption) (string, error) {
	var o writeOptions
	for _, opt := range opts {
		opt(&o)
	}
	if !o.hasID {
		return blobfs.NewID(), nil
	}
	return blobfs.ParseID(o.id)
}

// inTransaction reports whether sess is a transaction. An insert-or-find
// operation that hit a unique violation does not look the row up again
// inside one, because on Postgres a failed statement aborts the
// transaction and every later statement in it fails.
func inTransaction(sess sqlate.Session) bool {
	_, ok := sess.(*sqlate.Tx)
	return ok
}
