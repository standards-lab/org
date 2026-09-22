//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/migrate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data/datatest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/postgres"
)

// env is one test's throwaway database with blobfs's migration set
// applied, its DSN, and two stores over it: one over the Postgres variant
// and one over the baseline, compiled against the same catalog.
type env struct {
	ctx      context.Context
	db       *sqlate.DB
	dsn      string
	native   *data.Store
	standard *data.Store
}

// open builds the environment and verifies both stores against the
// migrated schema, the native statements included.
func open(t *testing.T) env {
	t.Helper()
	ctx := context.Background()
	db, dsn := livetest.OpenDSN(t)
	set, err := postgres.Migrations()
	if err != nil {
		t.Fatalf("Migrations: %v", err)
	}
	m, err := migrate.New(db, set.Migrations, migrate.Options{Table: set.Table})
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	if err := m.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	c, err := query.NewCatalog(query.Patterns(), data.Patterns())
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	v, err := postgres.New(c, db.Dialect())
	if err != nil {
		t.Fatalf("postgres.New: %v", err)
	}
	native, err := data.New(c, db.Dialect(), data.WithVariant(v))
	if err != nil {
		t.Fatalf("data.New over the Postgres variant: %v", err)
	}
	if err := native.Verify(ctx, db); err != nil {
		t.Fatalf("Verify over the Postgres variant against the migrated schema: %v", err)
	}
	standard, err := data.New(c, db.Dialect())
	if err != nil {
		t.Fatalf("data.New over the baseline: %v", err)
	}
	return env{ctx: ctx, db: db, dsn: dsn, native: native, standard: standard}
}

// insertFile inserts an available file row of three bytes directly and
// returns its id.
func (e env) insertFile(t *testing.T, dir, name string) string {
	t.Helper()
	id := blobfs.NewID()
	_, err := e.db.ExecContext(e.ctx,
		"INSERT INTO blobfs_file (id, directory_id, name, status, key, content_type, size) VALUES ($1, $2, $3, 'available', $4, 'text/plain', 3)",
		id, dir, name, id+"/"+name)
	if err != nil {
		t.Fatalf("insert file %s: %v", name, err)
	}
	return id
}

// advisoryLocks counts the advisory locks the connected database holds.
func (e env) advisoryLocks(t *testing.T) int {
	t.Helper()
	rows, err := e.db.QueryContext(e.ctx, "SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND granted AND database = (SELECT oid FROM pg_database WHERE datname = current_database())")
	if err != nil {
		t.Fatalf("pg_locks: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var n int
	if !rows.Next() || rows.Scan(&n) != nil {
		t.Fatalf("pg_locks: no count: %v", rows.Err())
	}
	return n
}

// TestConformance runs the conformance suite over the store built on the
// Postgres variant: the file-delete begin's contract through one
// RETURNING statement, and the tree lock held by a transaction until it
// commits or rolls back, a second transaction blocking meanwhile.
func TestConformance(t *testing.T) {
	e := open(t)
	if !e.native.Serializes() {
		t.Fatal("the Postgres variant reports it does not serialize")
	}
	datatest.Run(t, e.db, e.native)
}

// TestTreeLockIsAnAdvisoryLock proves the lock LockTree takes is a
// transaction-scoped advisory lock the engine reports in pg_locks while
// the transaction runs and releases when it ends.
func TestTreeLockIsAnAdvisoryLock(t *testing.T) {
	e := open(t)
	if n := e.advisoryLocks(t); n != 0 {
		t.Fatalf("%d advisory locks held before the test", n)
	}
	tx, err := e.db.Begin(e.ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := e.native.LockTree(e.ctx, tx); err != nil {
		_ = tx.Rollback()
		t.Fatalf("LockTree: %v", err)
	}
	if n := e.advisoryLocks(t); n != 1 {
		t.Errorf("%d advisory locks held inside the transaction, want 1", n)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if n := e.advisoryLocks(t); n != 0 {
		t.Errorf("%d advisory locks held after commit, want 0", n)
	}
}

// TestBeginFileDeleteAcceptsThePool proves the one-statement begin runs on
// the pool, where the baseline refuses, and leaves the row deleting.
func TestBeginFileDeleteAcceptsThePool(t *testing.T) {
	e := open(t)
	dir, err := e.native.Mkdir(e.ctx, e.db, blobfs.RootID, "docs")
	if err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	id := e.insertFile(t, dir.ID, "a.txt")
	f, err := e.native.BeginFileDelete(e.ctx, e.db, id)
	if err != nil || f.Status != blobfs.StatusDeleting || f.Version != 2 {
		t.Fatalf("BeginFileDelete on the pool = %+v, %v, want the deleting row at version 2", f, err)
	}
	stored, err := e.native.File(e.ctx, e.db, id)
	if err != nil || stored.Status != blobfs.StatusDeleting || stored.Version != 2 {
		t.Errorf("the database holds %+v, %v", stored, err)
	}
}

// TestVariantsAgree proves the two variants produce the same row from the
// same fixture: two files inserted alike, one begun through each variant,
// agree on every column that does not identify the row or carry a clock,
// on the first begin and on the retry.
func TestVariantsAgree(t *testing.T) {
	e := open(t)
	dir, err := e.native.Mkdir(e.ctx, e.db, blobfs.RootID, "docs")
	if err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	begin := func(s *data.Store, id string) blobfs.File {
		t.Helper()
		f, err := e.db.Transact(e.ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
			return s.BeginFileDelete(e.ctx, tx, id)
		})
		if err != nil {
			t.Fatalf("BeginFileDelete(%s): %v", id, err)
		}
		return f
	}
	same := func(a, b blobfs.File) bool {
		return a.DirectoryID == b.DirectoryID && a.Status == b.Status && a.Version == b.Version &&
			a.ContentType == b.ContentType && *a.Size == *b.Size && a.ETag == nil && b.ETag == nil
	}
	nativeID := e.insertFile(t, dir.ID, "native.txt")
	standardID := e.insertFile(t, dir.ID, "standard.txt")
	for _, step := range []string{"first begin", "retry"} {
		n, s := begin(e.native, nativeID), begin(e.standard, standardID)
		if !same(n, s) {
			t.Errorf("%s: the variants disagree:\nnative   %+v\nstandard %+v", step, n, s)
		}
		if n.Status != blobfs.StatusDeleting || n.Version != 2 {
			t.Errorf("%s: the row is %s at version %d, want deleting at 2", step, n.Status, n.Version)
		}
	}
}
