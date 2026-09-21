package postgres

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"hash/fnv"

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

// Variant is the Postgres implementation of data.Variant: the compiled
// native statements bound to their handles. It holds no session; every
// method takes one.
type Variant struct {
	stmts           *query.Statements
	lockTree        query.Statement
	beginFileDelete query.Rows[blobfs.File]
}

var (
	_ data.Variant   = (*Variant)(nil)
	_ query.Verifier = (*Variant)(nil)
)

// New compiles the variant's statements against catalog for dialect and
// binds them. The catalog must carry the blobfs namespace, registered from
// data.Patterns(), because the begin statement returns the published file
// column list. No I/O happens here.
func New(catalog *query.Catalog, dialect sqlate.Dialect) (*Variant, error) {
	stmts, err := catalog.Compile(statementFiles, "statements", dialect)
	if err != nil {
		return nil, fmt.Errorf("blobfs/postgres: %w", err)
	}
	return &Variant{
		stmts:           stmts,
		lockTree:        stmts.Statement("lock_tree"),
		beginFileDelete: stmts.Statement("begin_file_delete").Scan(query.Scanner[blobfs.File]()),
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
