package postgres_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"hash/fnv"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/postgres"
)

// catalog builds the catalog a consumer builds.
func catalog(t *testing.T) *query.Catalog {
	t.Helper()
	c, err := query.NewCatalog(query.Patterns(), data.Patterns())
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	return c
}

// newVariant compiles the variant under the stub dialect.
func newVariant(t *testing.T) *postgres.Variant {
	t.Helper()
	v, err := postgres.New(catalog(t), sqltest.Dialect{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return v
}

// fileColumns is the published column list's order, as the scripted
// driver must return the RETURNING row.
var fileColumns = []string{"id", "directory_id", "name", "status", "key", "size", "content_type", "etag", "version", "created_at", "updated_at"}

// TestNew proves the variant compiles against the consumer's catalog:
// two statements, both native tier, each with a port note, the lock
// requiring a transaction and the begin not, and the variant reporting
// that it serializes.
func TestNew(t *testing.T) {
	v := newVariant(t)
	var names []string
	for _, st := range v.Statements() {
		names = append(names, st.Name())
		if st.Tier() != query.TierNative {
			t.Errorf("%s is %s tier, want native", st.Name(), st.Tier())
		}
		if !strings.Contains(st.Native(), "Port:") {
			t.Errorf("%s carries no port note: %q", st.Name(), st.Native())
		}
		if st.TransactionRequired() != (st.Name() == "lock_tree") {
			t.Errorf("%s: TransactionRequired = %v; only lock_tree requires one", st.Name(), st.TransactionRequired())
		}
	}
	if want := []string{"begin_file_delete", "lock_tree"}; !slices.Equal(names, want) {
		t.Errorf("Statements = %v, want %v", names, want)
	}
	if !v.Serializes() {
		t.Error("Serializes = false, want true")
	}
	if _, err := postgres.New(must(query.NewCatalog(query.Patterns())), sqltest.Dialect{}); err == nil {
		t.Error("New without the blobfs namespace compiled; the begin statement includes blobfs.file_columns")
	}
}

func must(c *query.Catalog, err error) *query.Catalog {
	if err != nil {
		panic(err)
	}
	return c
}

// TestStoreOverTheVariant proves the store built with WithVariant lists
// the variant's statements after its own and verifies them in the same
// pass: twenty-four statements listed and thirty prepares.
func TestStoreOverTheVariant(t *testing.T) {
	v := newVariant(t)
	s, err := data.New(catalog(t), sqltest.Dialect{}, data.WithVariant(v))
	if err != nil {
		t.Fatalf("data.New: %v", err)
	}
	if s.Variant() != v {
		t.Errorf("Variant() = %T, want the Postgres variant", s.Variant())
	}
	stmts := s.Statements()
	if len(stmts) != 24 || stmts[22].Name() != "begin_file_delete" || stmts[23].Name() != "lock_tree" {
		var names []string
		for _, st := range stmts {
			names = append(names, st.Name()+":"+string(st.Tier()))
		}
		t.Errorf("Statements = %v, want the package's twenty-two then the variant's two", names)
	}
	pool, rec := sqltest.Open(t)
	if err := s.Verify(context.Background(), sqlate.Wrap(pool, sqltest.Dialect{})); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	prepared := rec.SQL(sqltest.OpPrepare)
	if len(prepared) != 30 {
		t.Errorf("Verify prepared %d, want 30 (28 for the store, 2 for the variant)", len(prepared))
	}
	native := 0
	for _, text := range prepared {
		if strings.Contains(text, "RETURNING") || strings.Contains(text, "pg_advisory_xact_lock") {
			native++
		}
	}
	if native != 2 {
		t.Errorf("Verify prepared %d native statements, want 2", native)
	}
}

// TestTreeLockKey pins the key: the 64-bit FNV-1a hash of TreeLockName,
// so a consumer that must avoid it, or a port that must derive the same
// one, can recompute it.
func TestTreeLockKey(t *testing.T) {
	h := fnv.New64a()
	_, _ = h.Write([]byte(postgres.TreeLockName))
	if want := int64(h.Sum64()); postgres.TreeLockKey != want {
		t.Errorf("TreeLockKey = %d, want %d", postgres.TreeLockKey, want)
	}
}

// TestLockTreeSQL proves LockTree refuses the pool before any SQL and, in
// a transaction, runs the advisory lock statement with the key bound.
func TestLockTreeSQL(t *testing.T) {
	v := newVariant(t)
	pool, rec := sqltest.Open(t)
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	ctx := context.Background()
	if err := v.LockTree(ctx, db); !errors.Is(err, query.ErrTransactionRequired) {
		t.Errorf("LockTree on the pool = %v, want ErrTransactionRequired", err)
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("the refusal reached the driver: %v", calls)
	}
	rec.Queue(sqltest.Response{Affected: 0})
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := v.LockTree(ctx, tx); err != nil {
		t.Fatalf("LockTree: %v", err)
	}
	_ = tx.Commit()
	execs := rec.SQL(sqltest.OpExec)
	if len(execs) != 1 || execs[0] != "SELECT pg_advisory_xact_lock(CAST($1 AS bigint))" {
		t.Errorf("LockTree ran %q", execs)
	}
	for _, c := range rec.Calls() {
		if c.Op == sqltest.OpExec && !slices.Equal(c.Args, []any{postgres.TreeLockKey}) {
			t.Errorf("LockTree bound %v, want the key %d", c.Args, postgres.TreeLockKey)
		}
	}
}

// TestBeginFileDeleteSQL proves the begin is one query, the update with
// RETURNING over the published column list, that it accepts the pool,
// and that no row is ErrNotFound.
func TestBeginFileDeleteSQL(t *testing.T) {
	v := newVariant(t)
	pool, rec := sqltest.Open(t)
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	ctx := context.Background()
	id := blobfs.NewID()
	now := time.Now()
	rec.Queue(sqltest.Response{Columns: fileColumns, Rows: [][]driver.Value{
		{id, blobfs.RootID, "a.txt", "deleting", id + "/a.txt", int64(3), "text/plain", nil, int64(2), now, now},
	}})
	f, err := v.BeginFileDelete(ctx, db, id)
	if err != nil || f.ID != id || f.Status != blobfs.StatusDeleting || f.Version != 2 {
		t.Errorf("BeginFileDelete = %+v, %v, want the scripted row", f, err)
	}
	if ops := rec.Ops(); !slices.Equal(ops, []sqltest.Op{sqltest.OpQuery}) {
		t.Errorf("ops = %v, want one query on the pool", ops)
	}
	text := rec.SQL(sqltest.OpQuery)[0]
	if !strings.HasPrefix(text, "UPDATE blobfs_file AS f") || !strings.Contains(text, "WHERE f.id = CAST($1 AS uuid)") ||
		!strings.HasSuffix(text, "RETURNING f.id, f.directory_id, f.name, f.status, f.key, f.size, f.content_type, f.etag, f.version, f.created_at, f.updated_at") {
		t.Errorf("the begin is not the one-statement form:\n%s", text)
	}
	rec.Queue(sqltest.Response{Columns: fileColumns})
	if _, err := v.BeginFileDelete(ctx, db, id); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("BeginFileDelete of a missing row = %v, want ErrNotFound", err)
	}
}
