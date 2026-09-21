package migrator_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/migrate"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/lib/migrator"
)

// lockingDialect is the stub dialect with the lock capability, so the
// migrator takes its outer lock and the recorder sees every call.
type lockingDialect struct{ sqltest.Dialect }

func (lockingDialect) Lock(ctx context.Context, conn *sql.Conn, name string) error {
	_, err := conn.ExecContext(ctx, "SELECT lock($1)", name)
	return err
}

func (lockingDialect) Unlock(ctx context.Context, conn *sql.Conn, name string) error {
	var held bool
	if err := conn.QueryRowContext(ctx, "SELECT unlock($1)", name).Scan(&held); err != nil {
		return err
	}
	if !held {
		return errors.New("lock not held")
	}
	return nil
}

var (
	library = migrator.Set{
		Name:  "lib",
		Table: "lib_schema_version",
		Migrations: []migrate.Migration{
			{Version: 1, Name: "lib_one", Up: "CREATE TABLE lib_one ()", Down: "DROP TABLE lib_one", Transactional: true},
		},
	}
	app = migrator.Set{
		Name: "app",
		Migrations: []migrate.Migration{
			{Version: 1, Name: "app_one", Up: "CREATE TABLE app_one ()", Down: "DROP TABLE app_one", Transactional: true},
		},
	}
)

var historyCols = []string{"version", "name", "dirty"}

// The scripted responses of the protocol's steps.
var (
	locked   = sqltest.Response{}                                                            // SELECT lock
	unlocked = sqltest.Response{Columns: []string{"unlock"}, Rows: [][]driver.Value{{true}}} // SELECT unlock
	created  = sqltest.Response{}                                                            // CREATE TABLE IF NOT EXISTS
	step     = sqltest.Response{}                                                            // a migration's text, or its history row
)

// exists scripts the history-table existence check.
func exists(yes bool) sqltest.Response {
	n := int64(0)
	if yes {
		n = 1
	}
	return sqltest.Response{Columns: []string{"count"}, Rows: [][]driver.Value{{n}}}
}

// history scripts the history read returning applied.
func history(applied ...[]driver.Value) sqltest.Response {
	return sqltest.Response{Columns: historyCols, Rows: applied}
}

// check scripts one set's pre-run check: the existence check and, when the
// table exists, the history read.
func check(applied ...[]driver.Value) []sqltest.Response {
	if applied == nil {
		return []sqltest.Response{exists(false)}
	}
	return []sqltest.Response{exists(true), history(applied...)}
}

// setRun scripts one set's unlocked run: the history table's create, the
// history read returning applied, and one transaction per migration.
func setRun(applied [][]driver.Value, steps int) []sqltest.Response {
	out := []sqltest.Response{created, history(applied...)}
	for range steps {
		out = append(out, step, step)
	}
	return out
}

func newMigrator(t *testing.T, sets []migrator.Set, responses ...sqltest.Response) (*migrator.Migrator, *sqltest.Recorder) {
	t.Helper()
	pool, rec := sqltest.Open(t, responses...)
	m, err := migrator.New(sqlate.Wrap(pool, lockingDialect{}), sets, migrator.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return m, rec
}

func script(parts ...[]sqltest.Response) []sqltest.Response {
	var out []sqltest.Response
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func one(r ...sqltest.Response) []sqltest.Response { return r }

// TestNewValidation covers the checks New makes before any I/O: at least one
// set, a name on every set, no repeated name, no shared history table (an
// empty table and the explicit default are the same table), and a set whose
// migrations migrate.New refuses.
func TestNewValidation(t *testing.T) {
	pool, _ := sqltest.Open(t)
	db := sqlate.Wrap(pool, lockingDialect{})
	cases := []struct {
		name string
		sets []migrator.Set
		want string
	}{
		{"no sets", nil, "no sets"},
		{"unnamed", []migrator.Set{{Table: "t"}}, "no name"},
		{"repeated name", []migrator.Set{library, {Name: "lib", Table: "other"}}, `"lib" is declared twice`},
		{"shared table", []migrator.Set{app, {Name: "other", Table: "schema_version"}}, `share the history table "schema_version"`},
		{"bad migrations", []migrator.Set{{Name: "x", Migrations: []migrate.Migration{{Version: 0, Name: "z", Up: "u"}}}}, `set "x": migrate:`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := migrator.New(db, c.sets, migrator.Options{})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("New = %v, want an error containing %q", err, c.want)
			}
		})
	}
	if _, err := migrator.New(db, []migrator.Set{library, app}, migrator.Options{}); err != nil {
		t.Fatalf("New(valid sets) = %v", err)
	}
	if _, err := migrator.New(nil, []migrator.Set{library}, migrator.Options{}); err == nil {
		t.Fatal("New(nil db) succeeded")
	}
}

// TestUpOrder proves Up takes one outer lock, checks every set, then runs
// the sets in declared order, each under its own history table and none
// under a lock of its own: the only lock call is the outer one, first,
// under the default name, and the unlock is last.
func TestUpOrder(t *testing.T) {
	responses := script(one(locked), check(), check(), setRun(nil, 1), setRun(nil, 1), one(unlocked))
	m, rec := newMigrator(t, []migrator.Set{library, app}, responses...)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if rec.Pending() != 0 {
		t.Errorf("%d scripted responses unconsumed", rec.Pending())
	}
	want := []string{
		"SELECT lock($1)",
		"CREATE TABLE IF NOT EXISTS lib_schema_version", "CREATE TABLE lib_one", "INSERT INTO lib_schema_version",
		"CREATE TABLE IF NOT EXISTS schema_version", "CREATE TABLE app_one", "INSERT INTO schema_version",
	}
	assertPrefixes(t, rec.SQL(sqltest.OpExec), want)
	assertOneLock(t, rec, "migrator.sets")
}

// TestLockName proves Options.LockName names the outer lock.
func TestLockName(t *testing.T) {
	pool, rec := sqltest.Open(t, script(one(locked), check(), setRun(nil, 1), one(unlocked))...)
	m, err := migrator.New(sqlate.Wrap(pool, lockingDialect{}), []migrator.Set{library}, migrator.Options{LockName: "acme.schema"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up: %v", err)
	}
	assertOneLock(t, rec, "acme.schema")
}

// TestNoLocker proves a dialect without the lock capability refuses a run
// with migrate.ErrNoLocker, and runs without any lock under Unlocked.
func TestNoLocker(t *testing.T) {
	pool, _ := sqltest.Open(t)
	m, err := migrator.New(sqlate.Wrap(pool, sqltest.Dialect{}), []migrator.Set{library}, migrator.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := m.Up(context.Background()); !errors.Is(err, migrate.ErrNoLocker) {
		t.Fatalf("Up on a dialect without a locker = %v, want ErrNoLocker", err)
	}

	pool, rec := sqltest.Open(t, script(check(), setRun(nil, 1))...)
	m, err = migrator.New(sqlate.Wrap(pool, sqltest.Dialect{}), []migrator.Set{library}, migrator.Options{Unlocked: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("unlocked Up: %v", err)
	}
	if rec.Pending() != 0 {
		t.Errorf("%d scripted responses unconsumed", rec.Pending())
	}
	for _, text := range rec.SQL(sqltest.OpExec) {
		if strings.HasPrefix(text, "SELECT lock") {
			t.Errorf("an unlocked run took a lock: %s", text)
		}
	}
}

// TestDownOrder proves Down reverts the sets in reverse declared order, the
// app's down text before the library's, reverts every applied migration of
// a set, and drops no history table.
func TestDownOrder(t *testing.T) {
	appApplied := [][]driver.Value{{int64(1), "app_one", false}}
	libApplied := [][]driver.Value{{int64(1), "lib_one", false}}
	responses := script(one(locked), check(libApplied...), check(appApplied...), setRun(appApplied, 1), setRun(libApplied, 1), one(unlocked))
	m, rec := newMigrator(t, []migrator.Set{library, app}, responses...)
	if err := m.Down(context.Background()); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if rec.Pending() != 0 {
		t.Errorf("%d scripted responses unconsumed", rec.Pending())
	}
	want := []string{
		"SELECT lock($1)",
		"CREATE TABLE IF NOT EXISTS schema_version", "DROP TABLE app_one", "DELETE FROM schema_version",
		"CREATE TABLE IF NOT EXISTS lib_schema_version", "DROP TABLE lib_one", "DELETE FROM lib_schema_version",
	}
	assertPrefixes(t, rec.SQL(sqltest.OpExec), want)
}

// TestResetOrder proves Reset reverts the sets in reverse declared order
// and drops each set's history table once that set is reverted: the app's
// down, then DROP TABLE schema_version, then the library's down, then DROP
// TABLE lib_schema_version, all under the one outer lock.
func TestResetOrder(t *testing.T) {
	appApplied := [][]driver.Value{{int64(1), "app_one", false}}
	libApplied := [][]driver.Value{{int64(1), "lib_one", false}}
	responses := script(one(locked), check(libApplied...), check(appApplied...), setRun(appApplied, 1), one(step), setRun(libApplied, 1), one(step), one(unlocked))
	m, rec := newMigrator(t, []migrator.Set{library, app}, responses...)
	if err := m.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if rec.Pending() != 0 {
		t.Errorf("%d scripted responses unconsumed", rec.Pending())
	}
	want := []string{
		"SELECT lock($1)",
		"CREATE TABLE IF NOT EXISTS schema_version", "DROP TABLE app_one", "DELETE FROM schema_version", "DROP TABLE schema_version",
		"CREATE TABLE IF NOT EXISTS lib_schema_version", "DROP TABLE lib_one", "DELETE FROM lib_schema_version", "DROP TABLE lib_schema_version",
	}
	assertPrefixes(t, rec.SQL(sqltest.OpExec), want)
	assertOneLock(t, rec, "migrator.sets")
}

// TestUpStopsAtFailure proves a failing set stops the run before the sets
// after it, the error names the set, and the lock is released.
func TestUpStopsAtFailure(t *testing.T) {
	responses := script(one(locked), check(), check(), one(created, history(), sqltest.Response{Err: errors.New("boom")}), one(unlocked))
	m, rec := newMigrator(t, []migrator.Set{library, app}, responses...)
	err := m.Up(context.Background())
	var se *migrator.SetError
	if !errors.As(err, &se) || se.Set != "lib" {
		t.Fatalf("Up = %v, want a SetError naming set lib", err)
	}
	if ops := rec.Ops(); slices.Contains(rec.SQL(sqltest.OpExec), "CREATE TABLE app_one") || slices.Contains(ops, sqltest.OpCommit) {
		t.Errorf("the app set ran after the library set failed: %v", rec.SQL(sqltest.OpExec))
	}
	if rec.Pending() != 0 {
		t.Errorf("%d scripted responses unconsumed; the unlock did not run", rec.Pending())
	}
}

// TestDirtyRefusal proves a dirty set refuses Up, Down, and Reset before
// any set runs, even when the dirty set is not the first: the library's
// set is clean and pending, the app's history has a dirty row, and no
// migration text of either set runs. The error names the set, classifies
// as migrate.ErrDirty, and carries the version.
func TestDirtyRefusal(t *testing.T) {
	dirty := [][]driver.Value{{int64(1), "app_one", true}}
	for _, op := range []struct {
		name string
		run  func(*migrator.Migrator, context.Context) error
	}{
		{"Up", (*migrator.Migrator).Up},
		{"Down", (*migrator.Migrator).Down},
		{"Reset", (*migrator.Migrator).Reset},
	} {
		t.Run(op.name, func(t *testing.T) {
			responses := script(one(locked), check(), check(dirty...), one(unlocked))
			m, rec := newMigrator(t, []migrator.Set{library, app}, responses...)
			err := op.run(m, context.Background())
			if !errors.Is(err, migrate.ErrDirty) {
				t.Fatalf("%s = %v, want ErrDirty", op.name, err)
			}
			var se *migrator.SetError
			if !errors.As(err, &se) || se.Set != "app" {
				t.Errorf("%s = %v, want a SetError naming set app", op.name, err)
			}
			var de *migrate.DirtyError
			if !errors.As(err, &de) || de.Version != 1 {
				t.Errorf("%s = %v, want the dirty version 1", op.name, err)
			}
			for _, text := range rec.SQL(sqltest.OpExec) {
				if strings.HasPrefix(text, "CREATE TABLE") || strings.HasPrefix(text, "DROP TABLE") {
					t.Errorf("%s ran %q after the dirty check", op.name, text)
				}
			}
			if rec.Pending() != 0 {
				t.Errorf("%d scripted responses unconsumed", rec.Pending())
			}
		})
	}
}

// TestUnknownHistoryRefusal proves a history row the set does not contain
// refuses the run, classified as migrate.ErrUnknownVersion.
func TestUnknownHistoryRefusal(t *testing.T) {
	responses := script(one(locked), check([]driver.Value{int64(1), "someone_elses", false}), one(unlocked))
	m, _ := newMigrator(t, []migrator.Set{library}, responses...)
	err := m.Up(context.Background())
	var se *migrator.SetError
	if !errors.Is(err, migrate.ErrUnknownVersion) || !errors.As(err, &se) || se.Set != "lib" {
		t.Fatalf("Up = %v, want a SetError for lib classified as ErrUnknownVersion", err)
	}
}

// TestStatus proves Status reads each set without a lock: the library at
// its head, clean; the app with no history table, so version 0 with its
// one migration pending. A dirty head is reported as such, and a history
// that does not match the set is an error.
func TestStatus(t *testing.T) {
	libRead := one(exists(true), sqltest.Response{Columns: []string{"version", "dirty"}, Rows: [][]driver.Value{{int64(1), false}}}, exists(true), history([]driver.Value{int64(1), "lib_one", false}))
	appRead := one(exists(false), exists(false))
	m, rec := newMigrator(t, []migrator.Set{library, app}, script(libRead, appRead)...)
	got, err := m.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Status returned %d sets, want 2", len(got))
	}
	lib, ap := got[0], got[1]
	if lib.Name != "lib" || lib.Table != "lib_schema_version" || lib.Version != 1 || lib.Latest != 1 || len(lib.Pending) != 0 || lib.Dirty {
		t.Errorf("lib status = %+v", lib)
	}
	if ap.Name != "app" || ap.Table != "schema_version" || ap.Version != 0 || ap.Latest != 1 || len(ap.Pending) != 1 || ap.Pending[0].Name != "app_one" || ap.Dirty {
		t.Errorf("app status = %+v", ap)
	}
	for _, text := range rec.SQL(sqltest.OpExec) {
		if strings.HasPrefix(text, "SELECT lock") {
			t.Errorf("Status took a lock: %s", text)
		}
	}
	if rec.Pending() != 0 {
		t.Errorf("%d scripted responses unconsumed", rec.Pending())
	}

	dirtyRead := one(exists(true), sqltest.Response{Columns: []string{"version", "dirty"}, Rows: [][]driver.Value{{int64(1), true}}}, exists(true), history([]driver.Value{int64(1), "lib_one", true}))
	m, _ = newMigrator(t, []migrator.Set{library}, dirtyRead...)
	got, err = m.Status(context.Background())
	if err != nil || len(got) != 1 || !got[0].Dirty || got[0].Version != 1 {
		t.Errorf("dirty Status = %+v, %v; want version 1 dirty", got, err)
	}

	unknownRead := one(exists(true), sqltest.Response{Columns: []string{"version", "dirty"}, Rows: [][]driver.Value{{int64(1), false}}}, exists(true), history([]driver.Value{int64(1), "other", false}))
	m, _ = newMigrator(t, []migrator.Set{library}, unknownRead...)
	if _, err := m.Status(context.Background()); !errors.Is(err, migrate.ErrUnknownVersion) {
		t.Errorf("Status over a mismatched history = %v, want ErrUnknownVersion", err)
	}
}

// TestForce proves Force runs the named set's override under the outer
// lock and refuses a set the migrator does not hold.
func TestForce(t *testing.T) {
	m, rec := newMigrator(t, []migrator.Set{library, app}, script(one(locked), one(created, step), one(unlocked))...)
	if err := m.Force(context.Background(), "app", 0); err != nil {
		t.Fatalf("Force: %v", err)
	}
	assertPrefixes(t, rec.SQL(sqltest.OpExec), []string{"SELECT lock($1)", "CREATE TABLE IF NOT EXISTS schema_version", "DELETE FROM schema_version WHERE version >"})
	if err := m.Force(context.Background(), "nope", 0); err == nil || !strings.Contains(err.Error(), `no set "nope"`) {
		t.Errorf("Force of an unknown set = %v", err)
	}
}

// assertPrefixes checks that got has one text per want, in order, each
// starting with its want.
func assertPrefixes(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("exec texts:\n%s\nwant %d texts", strings.Join(got, "\n"), len(want))
	}
	for i := range want {
		if !strings.HasPrefix(got[i], want[i]) {
			t.Errorf("exec %d = %q, want prefix %q", i, got[i], want[i])
		}
	}
}

// assertOneLock checks that the run took exactly one lock, under name, as
// its first call, and released it as its last.
func assertOneLock(t *testing.T, rec *sqltest.Recorder, name string) {
	t.Helper()
	calls := rec.Calls()
	var locks []sqltest.Call
	for _, c := range calls {
		if strings.HasPrefix(c.SQL, "SELECT lock(") {
			locks = append(locks, c)
		}
	}
	if len(locks) != 1 || len(locks[0].Args) != 1 || locks[0].Args[0] != name {
		t.Fatalf("lock calls = %+v, want one under %q", locks, name)
	}
	if calls[0].SQL != "SELECT lock($1)" {
		t.Errorf("the first call is %q, want the lock", calls[0].SQL)
	}
	if last := calls[len(calls)-1]; last.SQL != "SELECT unlock($1)" || last.Args[0] != name {
		t.Errorf("the last call is %+v, want the unlock of %q", last, name)
	}
}
