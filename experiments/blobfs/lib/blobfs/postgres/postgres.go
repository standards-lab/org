package postgres

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

//go:embed statements/*.sql
var statementFiles embed.FS

// TreeLockName is the name the tree lock is derived from: the blobfs_
// namespace and the table whose shape the lock serializes. An advisory
// lock is keyed by a number, so the name is hashed to TreeLockKey; a
// consumer that takes advisory locks of its own avoids that key.
const TreeLockName = "blobfs_directory.tree"

// TreeLockKey is the bigint key of the tree lock: the 64-bit FNV-1a hash
// of TreeLockName, fixed for every install, since one install per database
// means one tree per database.
var TreeLockKey = treeLockKey()

// treeLockKey hashes TreeLockName to the advisory lock's key space.
func treeLockKey() int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(TreeLockName))
	return int64(h.Sum64())
}

// Variant is the Postgres implementation of data.Variant: the standard
// baseline embedded, so a variation point this package does not override
// runs as the baseline does, and the compiled native statements bound to
// their handles. It holds no session; every method takes one.
type Variant struct {
	*data.Standard
	stmts             *query.Statements
	lockTree          query.Statement
	beginFileDelete   query.Rows[blobfs.File]
	beginFileWrite    query.Rows[blobfs.File]
	createDirectory   query.Rows[blobfs.Directory]
	completeFileWrite query.Rows[blobfs.File]
	resolvePath       query.Rows[resolved]
}

// resolved is one row of resolve_path: the deepest directory reached and
// its depth, the number of segments matched.
type resolved struct {
	blobfs.Directory
	Depth int
}

// scanResolved reads a resolve_path row: the published directory columns
// in their order, then the depth. The struct scanner maps columns to
// fields by name and does not descend into the embedded entity, so the
// scan is spelled out.
func scanResolved(rows *sql.Rows) (resolved, error) {
	var r resolved
	err := rows.Scan(&r.ID, &r.ParentID, &r.Name, &r.Version, &r.CreatedAt, &r.UpdatedAt, &r.Depth)
	return r, err
}

var (
	_ data.Variant   = (*Variant)(nil)
	_ query.Verifier = (*Variant)(nil)
)

// New compiles the variant's statements against catalog for dialect and
// binds them, over a baseline compiled the same way. The catalog must
// carry the blobfs namespace, registered from data.Patterns(), because the
// statements that return a row return the published column lists. No I/O
// happens here.
func New(catalog *query.Catalog, dialect sqlate.Dialect) (*Variant, error) {
	standard, err := data.NewStandard(catalog, dialect)
	if err != nil {
		return nil, fmt.Errorf("blobfs/postgres: %w", err)
	}
	stmts, err := catalog.Compile(statementFiles, "statements", dialect)
	if err != nil {
		return nil, fmt.Errorf("blobfs/postgres: %w", err)
	}
	file := query.Scanner[blobfs.File]()
	return &Variant{
		Standard:          standard,
		stmts:             stmts,
		lockTree:          stmts.Statement("lock_tree"),
		beginFileDelete:   stmts.Statement("begin_file_delete").Scan(file),
		beginFileWrite:    stmts.Statement("begin_file_write").Scan(file),
		createDirectory:   stmts.Statement("create_directory").Scan(query.Scanner[blobfs.Directory]()),
		completeFileWrite: stmts.Statement("complete_file_write").Scan(file),
		resolvePath:       stmts.Statement("resolve_path").Scan(scanResolved),
	}, nil
}

// Statements returns the variant's compiled inventory in name order, for a
// consumer that lists the SQL its program runs. The store appends it to
// its own.
func (v *Variant) Statements() []query.Statement {
	return v.stmts.Statements()
}

// Verify prepares the variant's statements against the schema the session
// reaches. The store's Verify includes it.
func (v *Variant) Verify(ctx context.Context, sess sqlate.Session) error {
	return v.stmts.Verify(ctx, sess)
}

// LockTree takes the transaction-scoped advisory lock under TreeLockKey in
// sess and returns once it is held. A transaction that holds it blocks
// every other LockTree until it commits or rolls back. A session that is
// not a transaction is query.ErrTransactionRequired, refused before any
// SQL.
func (v *Variant) LockTree(ctx context.Context, sess sqlate.Session) error {
	_, err := v.lockTree.Exec(ctx, sess, query.Args{"key": TreeLockKey})
	return err
}

// Serializes reports true: LockTree holds an engine lock to the end of the
// transaction.
func (*Variant) Serializes() bool {
	return true
}

// BeginFileDelete moves the file with id to deleting and returns the row
// in one statement. It accepts the pool as well as a transaction, because
// one statement is atomic on its own; a portable caller still passes a
// transaction, since the baseline requires one. No row is
// blobfs.ErrNotFound.
func (v *Variant) BeginFileDelete(ctx context.Context, sess sqlate.Session, id string) (blobfs.File, error) {
	f, err := v.beginFileDelete.One(ctx, sess, query.Args{"id": id})
	if errors.Is(err, sql.ErrNoRows) {
		return blobfs.File{}, blobfs.ErrNotFound
	}
	if err != nil {
		return blobfs.File{}, err
	}
	return f, nil
}

// InsertFile inserts the pending row and returns it in one statement, the
// insert with RETURNING over the published column list, where the
// baseline inserts and then reads by id. A constraint violation is
// returned as the session mapped it, for the store to classify.
func (v *Variant) InsertFile(ctx context.Context, sess sqlate.Session, id, directoryID, name, key, contentType string) (blobfs.File, error) {
	return v.beginFileWrite.One(ctx, sess, query.Args{"id": id, "directory_id": directoryID, "name": name, "key": key, "content_type": contentType})
}

// InsertDirectory inserts the directory row and returns it in one
// statement, the insert with RETURNING over the published column list,
// where the baseline inserts and then reads by id. A constraint violation
// is returned as the session mapped it, for the store to classify.
func (v *Variant) InsertDirectory(ctx context.Context, sess sqlate.Session, id, parentID, name string) (blobfs.Directory, error) {
	return v.createDirectory.One(ctx, sess, query.Args{"id": id, "parent_id": parentID, "name": name})
}

// CompleteFileWrite runs the update with RETURNING: a row returned is the
// completed write in one statement, where the baseline runs the guard and
// a read. No row returned means the update changed none, and the row is
// read once by id, through the embedded baseline's read, to classify: no
// row is blobfs.ErrNotFound, and a row is classified through
// data.CompleteRefusal, so the refusals are the baseline's in one round
// trip fewer.
func (v *Variant) CompleteFileWrite(ctx context.Context, sess sqlate.Session, id string, version int64, obj blobfs.Object) (blobfs.File, error) {
	args := query.Args{"id": id, "version": version, "size": obj.Size, "content_type": obj.ContentType, "etag": obj.ETag}
	f, err := v.completeFileWrite.One(ctx, sess, args)
	if err == nil {
		return f, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return blobfs.File{}, err
	}
	current, err := v.File(ctx, sess, id)
	if err != nil {
		return blobfs.File{}, err
	}
	return blobfs.File{}, data.CompleteRefusal(current, version)
}

// ResolvePath walks segments below the directory with startID in one
// statement, the recursive query resolve_path, and returns the deepest
// directory reached and its depth, where the baseline reads the start and
// then one child per segment. The segments bind as one text[] parameter,
// encoded by the driver from the Go slice, so no name is ever spliced
// into the text; a nil slice binds as an empty array. No row is
// blobfs.ErrNotFound: the start does not exist.
func (v *Variant) ResolvePath(ctx context.Context, sess sqlate.Session, startID string, segments []string) (blobfs.Directory, int, error) {
	if segments == nil {
		segments = []string{}
	}
	r, err := v.resolvePath.One(ctx, sess, query.Args{"start_id": startID, "segments": segments})
	if errors.Is(err, sql.ErrNoRows) {
		return blobfs.Directory{}, 0, blobfs.ErrNotFound
	}
	if err != nil {
		return blobfs.Directory{}, 0, err
	}
	return r.Directory, r.Depth, nil
}

// Keyset renders the cursor predicate as a row-value comparison,
// (q.a, q.b) > (x, y) with < under a descending sort, each value cast to
// its term's type and bound once, in term order. Postgres compares row
// values lexicographically, which is the order the expanded form spells,
// and it uses the comparison as an index condition on an index over the
// sort's leading columns, where the expanded form is a filter over the
// directory; without such an index the two forms cost the same. A single
// term is the baseline's one comparison, so the text is the same on
// both.
func (v *Variant) Keyset(k data.Keyset) string {
	if len(k.Terms) == 1 {
		return v.Standard.Keyset(k)
	}
	op := " > "
	if k.Terms[0].Descending {
		op = " < "
	}
	columns := make([]string, len(k.Terms))
	values := make([]string, len(k.Terms))
	for i := range k.Terms {
		columns[i] = k.Column(i)
		values[i] = k.Value(i)
	}
	return "(" + strings.Join(columns, ", ") + ")" + op + "(" + strings.Join(values, ", ") + ")"
}
