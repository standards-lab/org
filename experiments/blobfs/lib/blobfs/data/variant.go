package data

import (
	"context"
	"fmt"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// Variant is the set of operations an engine may implement with its own
// native statements: the two variation points of the persistence package.
// The Store runs every other operation from its standard-tier statements
// and forwards these two to the variant it was built with. The default is
// Standard, which is complete on any engine; pgnative is the Postgres
// variant; and a consumer supplies its own by implementing the interface,
// typically by embedding one of the two and overriding one method.
//
// LockTree serializes tree-shape changes: the caller takes it inside the
// transaction that will move a directory, before the cycle check, so that
// two opposing moves cannot each pass the check and together form a cycle.
// The lock is held until the transaction commits or rolls back, and every
// process that moves directories must take it for the serialization to
// hold. A session that is not a *sqlate.Tx is refused with
// query.ErrTransactionRequired, because a lock the transaction does not
// hold serializes nothing. Serializes reports whether the variant's
// LockTree serializes at all: Standard has no lock, because standard SQL
// has none, and returns false; a caller that needs the guarantee checks
// it before the first move.
//
// BeginFileDelete moves the file with id to blobfs.StatusDeleting and
// returns the row as the database holds it afterward, so the caller can
// delete the object by its Key and then complete the delete by removing
// the row. The transition is allowed from pending and available; on a row
// that is already deleting it changes nothing and returns the row again,
// so a retry converges. The version advances once, on the first begin,
// and updated_at is stamped then. A file that does not exist is
// blobfs.ErrNotFound. A portable caller passes a *sqlate.Tx: Standard runs
// two statements and requires a transaction so they read one row; a
// variant whose begin is one statement may also accept the pool.
type Variant interface {
	LockTree(ctx context.Context, sess sqlate.Session) error
	Serializes() bool
	BeginFileDelete(ctx context.Context, sess sqlate.Session, id string) (blobfs.File, error)
}

// Option configures New beyond its required arguments.
type Option func(*options)

// options collects what the options set.
type options struct {
	variant Variant
}

// WithVariant makes the store forward its variation points to v instead
// of the standard baseline. The variant is built by its own constructor
// against the same catalog and dialect, so a consumer composes
// pgnative.New(catalog, dialect) and then New(catalog, dialect,
// WithVariant(v)), or passes an implementation of its own.
func WithVariant(v Variant) Option {
	return func(o *options) { o.variant = v }
}

// Variant returns the variant the store forwards to, so a consumer can
// tell which one it was built with or reach a capability beyond the
// interface.
func (s *Store) Variant() Variant {
	return s.variant
}

// LockTree takes the variant's tree lock inside sess, which must be a
// transaction. See Variant.
func (s *Store) LockTree(ctx context.Context, sess sqlate.Session) error {
	if err := s.variant.LockTree(ctx, sess); err != nil {
		return fmt.Errorf("data: lock tree: %w", err)
	}
	return nil
}

// Serializes reports whether the store's LockTree serializes tree-shape
// changes across transactions. See Variant.
func (s *Store) Serializes() bool {
	return s.variant.Serializes()
}

// BeginFileDelete moves the file with id to deleting through the variant
// and returns the row. See Variant.
func (s *Store) BeginFileDelete(ctx context.Context, sess sqlate.Session, id string) (blobfs.File, error) {
	f, err := s.variant.BeginFileDelete(ctx, sess, id)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("data: begin delete of file %s: %w", id, err)
	}
	return f, nil
}

// inventory is the optional capability of a variant that compiled
// statements of its own: pgnative has one, Standard does not, because its
// statements are the persistence package's own and the Store already
// lists and verifies them.
type inventory interface {
	Statements() []query.Statement
}

// Standard is the standard-tier variant, the baseline every engine runs:
// the file-delete begin as two statements in the caller's transaction and
// no tree lock. It is the variant New uses when no WithVariant option is
// given, and a consumer that wants to override one of its methods builds
// it with NewStandard and embeds it.
type Standard struct {
	beginFileDelete query.Statement
	fileByID        query.Rows[blobfs.File]
}

// NewStandard compiles the baseline variant's statements against catalog
// for dialect. They are the persistence package's own statement files, so
// a consumer that builds a Standard to embed compiles the same directory
// New does.
func NewStandard(catalog *query.Catalog, dialect sqlate.Dialect) (*Standard, error) {
	stmts, err := catalog.Compile(statementFiles, "statements", dialect)
	if err != nil {
		return nil, fmt.Errorf("data: %w", err)
	}
	return newStandard(stmts), nil
}

// newStandard binds the baseline over an already compiled set.
func newStandard(stmts *query.Statements) *Standard {
	return &Standard{
		beginFileDelete: stmts.Statement("begin_file_delete"),
		fileByID:        stmts.Statement("file_by_id").Scan(query.Scanner[blobfs.File]()),
	}
}

// LockTree takes no lock: standard SQL has no statement that holds a lock
// to commit, so the baseline cannot serialize tree-shape changes, and two
// opposing concurrent moves on it can form a cycle. It still refuses a
// session that is not a transaction, so a caller sees the same refusal on
// every variant. A consumer that needs the guarantee on an engine without
// a native variant serializes moves outside the database.
func (*Standard) LockTree(_ context.Context, sess sqlate.Session) error {
	if _, ok := sess.(*sqlate.Tx); !ok {
		return query.ErrTransactionRequired
	}
	return nil
}

// Serializes reports false: the baseline's LockTree is a no-op.
func (*Standard) Serializes() bool {
	return false
}

// BeginFileDelete runs the baseline's two statements: the update that
// moves the row to deleting unless it already is, then the read of the
// row by id. Both run in sess, which must be a transaction: the update's
// statement requires one, and the row lock it takes holds to commit, so
// the read sees the row the update left. No row on the read is
// blobfs.ErrNotFound, whether the id never existed or a concurrent delete
// completed before this transaction's update.
func (v *Standard) BeginFileDelete(ctx context.Context, sess sqlate.Session, id string) (blobfs.File, error) {
	args := query.Args{"id": id}
	if _, err := v.beginFileDelete.Exec(ctx, sess, args); err != nil {
		return blobfs.File{}, err
	}
	f, err := v.fileByID.One(ctx, sess, args)
	if err != nil {
		return blobfs.File{}, notFound(err)
	}
	return f, nil
}
