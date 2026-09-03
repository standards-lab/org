package migrate_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"slices"
	"testing"

	"github.com/standards-lab/org/experiments/sql-dsl/lib/drivertest"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/migrate"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/pgdialect"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

var (
	errDriver = errors.New("engine says no")
	set       = []migrate.Migration{
		{Version: 1, Name: "a", Up: "CREATE TABLE a (x int)", Down: "DROP TABLE a", Transactional: true},
		{Version: 2, Name: "b", Up: "CREATE INDEX CONCURRENTLY ix ON a (x)", Down: "DROP INDEX CONCURRENTLY ix"},
	}
	historyCols = []string{"version", "name", "dirty"}
	locked      = drivertest.Response{}                                                                        // pg_advisory_lock
	created     = drivertest.Response{}                                                                        // CREATE TABLE IF NOT EXISTS
	unlocked    = drivertest.Response{Columns: []string{"pg_advisory_unlock"}, Rows: [][]driver.Value{{true}}} // pg_advisory_unlock
)

func history(rows ...[]driver.Value) drivertest.Response {
	return drivertest.Response{Columns: historyCols, Rows: rows}
}

func exists(yes bool) drivertest.Response {
	n := int64(0)
	if yes {
		n = 1
	}
	return drivertest.Response{Columns: []string{"count"}, Rows: [][]driver.Value{{n}}}
}

// newMigrator builds a migrator over the driver fake with the lock
// capability, so lock and unlock calls are part of the recorded script.
func newMigrator(t *testing.T, opts migrate.Options, responses ...drivertest.Response) (*migrate.Migrator, *drivertest.Recorder) {
	t.Helper()
	pool, rec := drivertest.Open(t, responses...)
	db := sqldb.Wrap(pool, pgdialect.Wrap(drivertest.Dialect{}))
	m, err := migrate.New(db, set, opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return m, rec
}

func assertOps(t *testing.T, rec *drivertest.Recorder, want ...drivertest.Op) {
	t.Helper()
	if got := rec.Ops(); !slices.Equal(got, want) {
		t.Errorf("ops = %v\nwant  %v", got, want)
	}
	if rec.Pending() != 0 {
		t.Errorf("%d scripted responses unconsumed", rec.Pending())
	}
}

func TestNew_ValidatesTheSet(t *testing.T) {
	pool, _ := drivertest.Open(t)
	db := sqldb.Wrap(pool, drivertest.Dialect{})
	cases := map[string][]migrate.Migration{
		"out of order": {{Version: 2, Name: "b", Up: "x"}, {Version: 1, Name: "a", Up: "x"}},
		"duplicate":    {{Version: 1, Name: "a", Up: "x"}, {Version: 1, Name: "a", Up: "x"}},
		"no name":      {{Version: 1, Up: "x"}},
		"no up":        {{Version: 1, Name: "a"}},
		"zero version": {{Version: 0, Name: "a", Up: "x"}},
	}
	for name, ms := range cases {
		if _, err := migrate.New(db, ms, migrate.Options{}); err == nil {
			t.Errorf("%s: New accepted the set", name)
		}
	}
	if _, err := migrate.New(db, set, migrate.Options{Table: "bad name; drop"}); err == nil {
		t.Error("New accepted a table name that is not an identifier")
	}
	m, err := migrate.New(db, set, migrate.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got := m.Migrations()
	got[0].Name = "mutated"
	if m.Migrations()[0].Name != "a" {
		t.Error("Migrations() exposed the internal slice")
	}
}

func TestSteps_TransactionalMigrationAppliesInsideOneTransaction(t *testing.T) {
	m, rec := newMigrator(t, migrate.Options{},
		locked, created, history(),
		drivertest.Response{}, // up
		drivertest.Response{}, // history insert
		unlocked,
	)
	if err := m.Steps(context.Background(), 1); err != nil {
		t.Fatalf("Steps: %v", err)
	}
	assertOps(t, rec,
		drivertest.OpExec, drivertest.OpExec, drivertest.OpQuery,
		drivertest.OpBegin, drivertest.OpExec, drivertest.OpExec, drivertest.OpCommit,
		drivertest.OpQuery,
	)
	calls := rec.Calls()
	if calls[0].SQL != "SELECT pg_advisory_lock(hashtext($1))" || calls[0].Args[0] != "migrate.schema_version" {
		t.Errorf("first call = %+v, want the named lock", calls[0])
	}
	if calls[4].SQL != set[0].Up {
		t.Errorf("up = %q", calls[4].SQL)
	}
	if calls[5].SQL != "INSERT INTO schema_version (version, name, dirty) VALUES ($1, $2, $3)" ||
		calls[5].Args[0] != 1 || calls[5].Args[1] != "a" || calls[5].Args[2] != false {
		t.Errorf("history insert = %+v", calls[5])
	}
	if last := calls[len(calls)-1]; last.SQL != "SELECT pg_advisory_unlock(hashtext($1))" {
		t.Errorf("last call = %q, want the unlock", last.SQL)
	}
	if calls[0].Args[0] != calls[len(calls)-1].Args[0] {
		t.Error("lock and unlock names differ")
	}
}

func TestSteps_TransactionalFailureRecordsNothingAndStillUnlocks(t *testing.T) {
	m, rec := newMigrator(t, migrate.Options{},
		locked, created, history(),
		drivertest.Response{Err: errDriver}, // up fails
		unlocked,
	)
	err := m.Up(context.Background())
	if !errors.Is(err, errDriver) {
		t.Fatalf("Up = %v, want the engine error", err)
	}
	if _, ok := errors.AsType[*drivertest.MappedError](err); !ok {
		t.Error("engine error did not cross the mapping boundary")
	}
	if errors.Is(err, migrate.ErrDirty) {
		t.Error("a transactional failure reported dirty state")
	}
	assertOps(t, rec,
		drivertest.OpExec, drivertest.OpExec, drivertest.OpQuery,
		drivertest.OpBegin, drivertest.OpExec, drivertest.OpRollback,
		drivertest.OpQuery,
	)
}

func TestUp_NonTransactionalMigrationMarksDirtyThenClean(t *testing.T) {
	m, rec := newMigrator(t, migrate.Options{},
		locked, created, history([]driver.Value{int64(1), "a", false}),
		drivertest.Response{}, // insert dirty
		drivertest.Response{}, // up
		drivertest.Response{}, // set clean
		unlocked,
	)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up: %v", err)
	}
	assertOps(t, rec,
		drivertest.OpExec, drivertest.OpExec, drivertest.OpQuery,
		drivertest.OpExec, drivertest.OpExec, drivertest.OpExec,
		drivertest.OpQuery,
	)
	calls := rec.Calls()
	if calls[3].SQL != "INSERT INTO schema_version (version, name, dirty) VALUES ($1, $2, $3)" || calls[3].Args[2] != true {
		t.Errorf("dirty insert = %+v", calls[3])
	}
	if calls[4].SQL != set[1].Up {
		t.Errorf("up = %q", calls[4].SQL)
	}
	if calls[5].SQL != "UPDATE schema_version SET dirty = $1 WHERE version = $2" || calls[5].Args[0] != false || calls[5].Args[1] != 2 {
		t.Errorf("clean = %+v", calls[5])
	}
}

func TestUp_NonTransactionalFailureLeavesTheRowDirty(t *testing.T) {
	m, rec := newMigrator(t, migrate.Options{},
		locked, created, history([]driver.Value{int64(1), "a", false}),
		drivertest.Response{},               // insert dirty
		drivertest.Response{Err: errDriver}, // up fails
		unlocked,
	)
	err := m.Up(context.Background())
	var dirty *migrate.DirtyError
	if !errors.As(err, &dirty) || dirty.Version != 2 || !errors.Is(err, migrate.ErrDirty) || !errors.Is(err, errDriver) {
		t.Fatalf("Up = %v, want a DirtyError for version 2 carrying the engine error", err)
	}
	assertOps(t, rec,
		drivertest.OpExec, drivertest.OpExec, drivertest.OpQuery,
		drivertest.OpExec, drivertest.OpExec,
		drivertest.OpQuery,
	)
}

func TestSteps_RefusesDirtyHistoryBeforeTouchingTheSchema(t *testing.T) {
	m, rec := newMigrator(t, migrate.Options{},
		locked, created, history([]driver.Value{int64(1), "a", false}, []driver.Value{int64(2), "b", true}),
		unlocked,
	)
	err := m.Up(context.Background())
	var dirty *migrate.DirtyError
	if !errors.As(err, &dirty) || dirty.Version != 2 || dirty.Err != nil {
		t.Fatalf("Up = %v, want DirtyError{2} discovered, not caused", err)
	}
	assertOps(t, rec, drivertest.OpExec, drivertest.OpExec, drivertest.OpQuery, drivertest.OpQuery)
}

func TestSteps_RefusesHistoryTheSetDoesNotCarry(t *testing.T) {
	m, _ := newMigrator(t, migrate.Options{},
		locked, created, history([]driver.Value{int64(1), "renamed", false}),
		unlocked,
	)
	err := m.Up(context.Background())
	var unknown *migrate.UnknownVersionError
	if !errors.As(err, &unknown) || unknown.Version != 1 || unknown.Name != "renamed" {
		t.Fatalf("Up = %v, want UnknownVersionError", err)
	}
}

func TestDown_RevertsTheMostRecentApplied(t *testing.T) {
	m, rec := newMigrator(t, migrate.Options{},
		locked, created, history([]driver.Value{int64(1), "a", false}, []driver.Value{int64(2), "b", false}),
		drivertest.Response{}, // set dirty (b is non-transactional)
		drivertest.Response{}, // down
		drivertest.Response{}, // delete row
		drivertest.Response{}, // a: begin → down
		drivertest.Response{}, // a: delete row
		unlocked,
	)
	if err := m.Down(context.Background(), 2); err != nil {
		t.Fatalf("Down: %v", err)
	}
	assertOps(t, rec,
		drivertest.OpExec, drivertest.OpExec, drivertest.OpQuery,
		drivertest.OpExec, drivertest.OpExec, drivertest.OpExec,
		drivertest.OpBegin, drivertest.OpExec, drivertest.OpExec, drivertest.OpCommit,
		drivertest.OpQuery,
	)
	calls := rec.Calls()
	if calls[4].SQL != set[1].Down || calls[7].SQL != set[0].Down {
		t.Errorf("downs = %q, %q", calls[4].SQL, calls[7].SQL)
	}
	if calls[5].SQL != "DELETE FROM schema_version WHERE version = $1" || calls[5].Args[0] != 2 {
		t.Errorf("delete = %+v", calls[5])
	}
}

func TestDown_WithoutDownTextIsErrNoDown(t *testing.T) {
	pool, _ := drivertest.Open(t,
		locked, created, history([]driver.Value{int64(1), "a", false}), unlocked,
	)
	db := sqldb.Wrap(pool, pgdialect.Wrap(drivertest.Dialect{}))
	m, err := migrate.New(db, []migrate.Migration{{Version: 1, Name: "a", Up: "x", Transactional: true}}, migrate.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Down(context.Background(), 1); !errors.Is(err, migrate.ErrNoDown) {
		t.Errorf("Down = %v, want ErrNoDown", err)
	}
}

func TestForce_ResetsTheHistoryWithoutTouchingTheSchema(t *testing.T) {
	m, rec := newMigrator(t, migrate.Options{},
		locked, created,
		drivertest.Response{}, // delete above 1
		drivertest.Response{}, // set clean → 0 rows
		drivertest.Response{}, // insert 1
		unlocked,
	)
	if err := m.Force(context.Background(), 1); err != nil {
		t.Fatalf("Force: %v", err)
	}
	calls := rec.Calls()
	if calls[2].SQL != "DELETE FROM schema_version WHERE version > $1" || calls[2].Args[0] != 1 {
		t.Errorf("delete above = %+v", calls[2])
	}
	if calls[3].SQL != "UPDATE schema_version SET dirty = $1 WHERE version = $2" || calls[3].Args[0] != false {
		t.Errorf("clean = %+v", calls[3])
	}
	if calls[4].SQL != "INSERT INTO schema_version (version, name, dirty) VALUES ($1, $2, $3)" || calls[4].Args[1] != "a" {
		t.Errorf("insert = %+v", calls[4])
	}
	assertOps(t, rec, drivertest.OpExec, drivertest.OpExec, drivertest.OpExec, drivertest.OpExec, drivertest.OpExec, drivertest.OpQuery)
}

func TestForce_ZeroEmptiesAndUnknownVersionIsRefused(t *testing.T) {
	m, rec := newMigrator(t, migrate.Options{}, locked, created, drivertest.Response{}, unlocked)
	if err := m.Force(context.Background(), 0); err != nil {
		t.Fatalf("Force(0): %v", err)
	}
	if calls := rec.Calls(); calls[2].Args[0] != 0 || len(calls) != 4 {
		t.Errorf("Force(0) calls = %+v", calls)
	}
	if err := m.Force(context.Background(), 9); !errors.Is(err, migrate.ErrVersionNotFound) {
		t.Errorf("Force(9) = %v, want ErrVersionNotFound", err)
	}
}

func TestVerify_ReportsPendingUnknownDirtyAndClean(t *testing.T) {
	ctx := context.Background()

	m, _ := newMigrator(t, migrate.Options{}, exists(false))
	var pending *migrate.PendingError
	if err := m.Verify(ctx); !errors.As(err, &pending) || !slices.Equal(pending.Versions, []int{1, 2}) {
		t.Errorf("no table: Verify = %v, want both pending", err)
	}

	m, _ = newMigrator(t, migrate.Options{}, exists(true), history([]driver.Value{int64(1), "a", false}))
	if err := m.Verify(ctx); !errors.As(err, &pending) || !slices.Equal(pending.Versions, []int{2}) || !errors.Is(err, migrate.ErrPending) {
		t.Errorf("partial: Verify = %v, want 2 pending", err)
	}

	m, _ = newMigrator(t, migrate.Options{}, exists(true), history([]driver.Value{int64(1), "a", false}, []driver.Value{int64(2), "b", true}))
	if err := m.Verify(ctx); !errors.Is(err, migrate.ErrDirty) {
		t.Errorf("dirty: Verify = %v", err)
	}

	m, _ = newMigrator(t, migrate.Options{}, exists(true), history([]driver.Value{int64(1), "a", false}, []driver.Value{int64(2), "b", false}))
	if err := m.Verify(ctx); err != nil {
		t.Errorf("complete: Verify = %v", err)
	}
}

func TestVersion_ReadsTheHead(t *testing.T) {
	ctx := context.Background()
	m, _ := newMigrator(t, migrate.Options{}, exists(false))
	if v, err := m.Version(ctx); err != nil || v != (migrate.Version{}) {
		t.Errorf("no table: Version = %+v, %v", v, err)
	}
	m, rec := newMigrator(t, migrate.Options{}, exists(true),
		drivertest.Response{Columns: []string{"version", "dirty"}, Rows: [][]driver.Value{{int64(2), true}}})
	v, err := m.Version(ctx)
	if err != nil || v.Version != 2 || !v.Dirty {
		t.Errorf("Version = %+v, %v", v, err)
	}
	if slices.Contains(rec.Ops(), drivertest.OpBegin) || len(rec.SQL(drivertest.OpExec)) != 0 {
		t.Error("Version took a lock or wrote")
	}
	if head := rec.SQL(drivertest.OpQuery)[1]; head != "SELECT version, dirty FROM schema_version WHERE version = (SELECT MAX(version) FROM schema_version)" {
		t.Errorf("head = %q", head)
	}
}

// catalogDialect is a dialect with the lock capability and its own Catalog,
// the shape an engine without IF NOT EXISTS or information_schema ships.
type catalogDialect struct{ pgdialect.Dialect }

func (catalogDialect) CreateHistory(table string) string {
	return "IF OBJECT_ID('" + table + "') IS NULL CREATE TABLE " + table + " (version int PRIMARY KEY, name nvarchar(200) NOT NULL, applied_at datetime2 NOT NULL DEFAULT SYSUTCDATETIME(), dirty bit NOT NULL DEFAULT 0)"
}

func (catalogDialect) HistoryExists(param string) string {
	return "SELECT COUNT(*) FROM sys.tables WHERE name = " + param
}

func TestNew_TakesTheCatalogFromTheDialect(t *testing.T) {
	ctx := context.Background()
	pool, rec := drivertest.Open(t, exists(false), locked, created, history(), unlocked)
	d := catalogDialect{pgdialect.Wrap(drivertest.Dialect{})}
	m, err := migrate.New(sqldb.Wrap(pool, d), set, migrate.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Version(ctx); err != nil {
		t.Fatalf("Version: %v", err)
	}
	if err := m.Steps(ctx, 0); err != nil {
		t.Fatalf("Steps: %v", err)
	}
	_ = m.Force(ctx, 0) // one locked run: lock, create, delete above, unlock
	if got := rec.SQL(drivertest.OpQuery)[0]; got != "SELECT COUNT(*) FROM sys.tables WHERE name = $1" {
		t.Errorf("exists = %q", got)
	}
	if got := rec.SQL(drivertest.OpExec)[1]; got[:len("IF OBJECT_ID")] != "IF OBJECT_ID" {
		t.Errorf("create = %q", got)
	}
}

func TestLocked_DialectWithoutLockerFailsUnlessUnlocked(t *testing.T) {
	pool, rec := drivertest.Open(t, created, history(), drivertest.Response{}, drivertest.Response{})
	db := sqldb.Wrap(pool, drivertest.Dialect{}) // the stub dialect has no Locker
	m, err := migrate.New(db, set[:1], migrate.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Up(context.Background()); !errors.Is(err, migrate.ErrNoLocker) {
		t.Fatalf("Up = %v, want ErrNoLocker", err)
	}
	if len(rec.Calls()) != 0 {
		t.Errorf("calls made before refusing: %v", rec.Ops())
	}

	m, err = migrate.New(db, set[:1], migrate.Options{Unlocked: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("unlocked Up: %v", err)
	}
	assertOps(t, rec, drivertest.OpExec, drivertest.OpQuery, drivertest.OpBegin, drivertest.OpExec, drivertest.OpExec, drivertest.OpCommit)
}

func TestOptions_TableAndLockNameDefaultsAndOverrides(t *testing.T) {
	m, rec := newMigrator(t, migrate.Options{Table: "app_schema", LockName: "ops.schema"},
		locked, created, history(), drivertest.Response{}, drivertest.Response{}, unlocked,
	)
	if err := m.Steps(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	calls := rec.Calls()
	if calls[0].Args[0] != "ops.schema" {
		t.Errorf("lock name = %v, want ops.schema", calls[0].Args[0])
	}
	if calls[1].SQL != "CREATE TABLE IF NOT EXISTS app_schema (version integer PRIMARY KEY, name text NOT NULL, applied_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP, dirty boolean NOT NULL DEFAULT FALSE)" {
		t.Errorf("create = %q", calls[1].SQL)
	}

	m1, rec1 := newMigrator(t, migrate.Options{}, locked, created, history(), unlocked)
	m2, rec2 := newMigrator(t, migrate.Options{Table: "other"}, locked, created, history(), unlocked)
	_ = m1.Steps(context.Background(), 0) // no-op: no calls
	_ = m1.Up(context.Background())
	_ = m2.Up(context.Background())
	if rec1.Calls()[0].Args[0] != "migrate.schema_version" || rec2.Calls()[0].Args[0] != "migrate.other" {
		t.Errorf("default lock names = %v, %v", rec1.Calls()[0].Args[0], rec2.Calls()[0].Args[0])
	}
}
