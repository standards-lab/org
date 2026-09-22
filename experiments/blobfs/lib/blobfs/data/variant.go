package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// Variant is the set of operations an engine may implement with its own
// native statements: the variation points of the persistence package.
// The Store runs every other operation from its standard-tier statements
// and forwards these to the variant it was built with. The default is
// Standard, which is complete on any engine; the postgres package is the
// Postgres variant; and a consumer supplies its own by implementing the
// interface, typically by embedding one of the two and overriding the
// methods it needs, so the interface can grow without burdening a
// variant.
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
//
// InsertFile inserts the pending row of a file write and returns it as
// the database holds it, and InsertDirectory does the same for a
// directory. The Store validates the name, mints or checks the id, and
// builds the key before it calls either, so a variant binds its arguments
// as given. A constraint violation is returned as the session mapped it,
// a sqlate.ConstraintError, and the Store classifies it into blobfs's
// sentinels; a variant does not classify. Standard runs the insert and a
// read by id; a variant whose insert returns the row runs one statement.
//
// CompleteFileWrite is the last step of a file write: it moves the
// pending row with id to blobfs.StatusAvailable, records obj, advances the
// version, stamps updated_at, and returns the row as the database holds
// it afterward, only when the row is pending at version. A row that does
// not exist is blobfs.ErrNotFound; a row at another version is
// query.ErrVersionMismatch naming the expected and current versions; a
// row at the expected version that is not pending is the
// blobfs.TransitionError from its status to available. Standard runs the
// query library's guard and then reads the row; a variant whose update
// returns the row runs one statement on success and reads the row once
// when the update changed none, and classifies through CompleteRefusal so
// both report alike.
//
// ResolvePath walks segments, each a normalized and validated directory
// name, downward from the directory with startID and returns the deepest
// directory reached and its depth: the number of segments matched, so a
// depth equal to len(segments) is the resolved directory and a smaller
// depth means segments[depth] named no directory under the one returned.
// No segments returns the start at depth 0. A start that does not exist
// is blobfs.ErrNotFound. Standard reads the start and then one child per
// segment; a variant may walk the whole path in one statement.
//
// Keyset renders the cursor predicate of a listing page, the text the
// composer appends after the listing's WHERE clause when a page continues
// from a cursor. Standard renders the expanded form (a > x) OR (a = x AND
// b > y) OR ... through the query library's patterns; a variant with a
// row-value comparison renders (a, b) > (x, y) instead. A single term is
// one comparison on every variant. See Keyset for the contract a
// rendering keeps.
type Variant interface {
	LockTree(ctx context.Context, sess sqlate.Session) error
	Serializes() bool
	BeginFileDelete(ctx context.Context, sess sqlate.Session, id string) (blobfs.File, error)
	InsertFile(ctx context.Context, sess sqlate.Session, id, directoryID, name, key, contentType string) (blobfs.File, error)
	InsertDirectory(ctx context.Context, sess sqlate.Session, id, parentID, name string) (blobfs.Directory, error)
	CompleteFileWrite(ctx context.Context, sess sqlate.Session, id string, version int64, obj blobfs.Object) (blobfs.File, error)
	ResolvePath(ctx context.Context, sess sqlate.Session, startID string, segments []string) (blobfs.Directory, int, error)
	Keyset(k Keyset) string
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
// postgres.New(catalog, dialect) and then New(catalog, dialect,
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

// CompleteRefusal classifies a complete step that changed no row, from
// the row as it was read afterward: a row at another version than the
// caller expected is query.ErrVersionMismatch naming both versions, and a
// row at the expected version failed the status predicate, so it is the
// blobfs.TransitionError from its status to available. Every variant's
// CompleteFileWrite classifies through it, so the two tiers report a
// refusal alike; a row the read did not find is blobfs.ErrNotFound, which
// the variant reports before it gets here.
func CompleteRefusal(current blobfs.File, version int64) error {
	if current.Version != version {
		return fmt.Errorf("%w: expected %d, current %d", query.ErrVersionMismatch, version, current.Version)
	}
	return blobfs.Transition(current.Status, blobfs.StatusAvailable)
}

// inventory is the optional capability of a variant that compiled
// statements of its own: the Postgres variant has one, Standard does not, because its
// statements are the persistence package's own and the Store already
// lists and verifies them.
type inventory interface {
	Statements() []query.Statement
}

// Standard is the standard-tier variant, the baseline every engine runs:
// the file-delete begin as two statements in the caller's transaction, no
// tree lock, each insert followed by a read of the row, the write's
// complete step through the query library's guard, path resolution one
// child per segment, and the cursor predicate in its expanded form. It is
// the variant New uses when no WithVariant option is given, and a
// consumer that wants to override one of its methods builds it with
// NewStandard and embeds it.
type Standard struct {
	beginFileDelete   query.Statement
	beginFileWrite    query.Statement
	createDirectory   query.Statement
	completeFileWrite query.Guard
	fileByID          query.Rows[blobfs.File]
	directoryByID     query.Rows[blobfs.Directory]
	directoryChild    query.Rows[blobfs.Directory]
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
		beginFileDelete:   stmts.Statement("begin_file_delete"),
		beginFileWrite:    stmts.Statement("begin_file_write"),
		createDirectory:   stmts.Statement("create_directory"),
		completeFileWrite: stmts.Statement("complete_file_write").Guarded(stmts.Statement("file_version"), "version"),
		fileByID:          stmts.Statement("file_by_id").Scan(query.Scanner[blobfs.File]()),
		directoryByID:     stmts.Statement("directory_by_id").Scan(query.Scanner[blobfs.Directory]()),
		directoryChild:    stmts.Statement("directory_child").Scan(query.Scanner[blobfs.Directory]()),
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

// File reads the file with id through the baseline's file_by_id, or
// blobfs.ErrNotFound. It is the read a variant that embeds the baseline
// classifies a refused complete step from, when its own update returns
// the row on success and nothing on a refusal.
func (v *Standard) File(ctx context.Context, sess sqlate.Session, id string) (blobfs.File, error) {
	f, err := v.fileByID.One(ctx, sess, query.Args{"id": id})
	if err != nil {
		return blobfs.File{}, notFound(err)
	}
	return f, nil
}

// InsertFile runs the baseline's two statements: the insert of the
// pending row, whose defaults the table fills, then the read of the row by
// id. A constraint violation is returned as the session mapped it.
func (v *Standard) InsertFile(ctx context.Context, sess sqlate.Session, id, directoryID, name, key, contentType string) (blobfs.File, error) {
	args := query.Args{"id": id, "directory_id": directoryID, "name": name, "key": key, "content_type": contentType}
	if _, err := v.beginFileWrite.Exec(ctx, sess, args); err != nil {
		return blobfs.File{}, err
	}
	f, err := v.fileByID.One(ctx, sess, query.Args{"id": id})
	if err != nil {
		return blobfs.File{}, fmt.Errorf("read back: %w", err)
	}
	return f, nil
}

// InsertDirectory runs the baseline's two statements: the insert of the
// directory row, whose defaults the table fills, then the read of the row
// by id. A constraint violation is returned as the session mapped it.
func (v *Standard) InsertDirectory(ctx context.Context, sess sqlate.Session, id, parentID, name string) (blobfs.Directory, error) {
	if _, err := v.createDirectory.Exec(ctx, sess, query.Args{"id": id, "parent_id": parentID, "name": name}); err != nil {
		return blobfs.Directory{}, err
	}
	d, err := v.directoryByID.One(ctx, sess, query.Args{"id": id})
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("read back: %w", err)
	}
	return d, nil
}

// CompleteFileWrite runs the guarded update through the query library's
// optimistic-concurrency protocol and reads the row back: two statements
// on success. When the update changes no row the guard reads the row's
// version, and no row is blobfs.ErrNotFound; otherwise the row is read
// once more and classified through CompleteRefusal, because the guard
// checks the version alone and cannot tell a version conflict from a row
// at the expected version that failed the status predicate.
func (v *Standard) CompleteFileWrite(ctx context.Context, sess sqlate.Session, id string, version int64, obj blobfs.Object) (blobfs.File, error) {
	args := query.Args{"id": id, "size": obj.Size, "content_type": obj.ContentType, "etag": obj.ETag}
	_, err := v.completeFileWrite.Run(ctx, sess, version, args)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return blobfs.File{}, blobfs.ErrNotFound
	case errors.Is(err, query.ErrVersionMismatch):
		f, readErr := v.fileByID.One(ctx, sess, query.Args{"id": id})
		if readErr != nil {
			return blobfs.File{}, notFound(readErr)
		}
		return blobfs.File{}, CompleteRefusal(f, version)
	case err != nil:
		return blobfs.File{}, err
	}
	f, err := v.fileByID.One(ctx, sess, query.Args{"id": id})
	if err != nil {
		return blobfs.File{}, fmt.Errorf("read back: %w", err)
	}
	return f, nil
}

// Keyset renders the baseline's cursor predicate: for one term the
// comparison q.f > v (or < under a descending sort), and for several the
// expanded form (a > x) OR (a = x AND b > y) OR ..., each comparison and
// each cast spelled by the query library's own patterns, which is
// standard SQL on every engine. Every occurrence of a value binds its
// own placeholder, so the text holds for a dialect whose placeholders
// cannot be repeated.
func (*Standard) Keyset(k Keyset) string {
	after := query.OpGt
	if k.Terms[0].Descending {
		after = query.OpLt
	}
	if len(k.Terms) == 1 {
		return k.Compare(after, 0)
	}
	disjuncts := make([]string, len(k.Terms))
	for i := range k.Terms {
		conjuncts := make([]string, 0, i+1)
		for j := range i {
			conjuncts = append(conjuncts, k.Compare(query.OpEq, j))
		}
		conjuncts = append(conjuncts, k.Compare(after, i))
		disjuncts[i] = strings.Join(conjuncts, " AND ")
		if i > 0 {
			disjuncts[i] = "(" + disjuncts[i] + ")"
		}
	}
	return "(" + strings.Join(disjuncts, " OR ") + ")"
}
