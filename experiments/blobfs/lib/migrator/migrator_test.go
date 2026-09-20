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

// lockingDialect is the stub dialect with the lock capability, so the inner
// migrators run their locked protocol and the recorder sees every call.
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

// setRun scripts one set's locked run: the lock, the history table's
// create, the history read returning applied, one transaction per
// migration, and the unlock.
func setRun(applied [][]driver.Value, steps int) []sqltest.Response {
	out := []sqltest.Response{
		{}, // lock
		{}, // CREATE TABLE IF NOT EXISTS
		{Columns: historyCols, Rows: applied},
	}
	for range steps {
		out = append(out, sqltest.Response{}, sqltest.Response{}) // the text, the history row
	}
	return append(out, sqltest.Response{Columns: []string{"unlock"}, Rows: [][]driver.Value{{true}}})
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
}

// TestUpOrder proves Up runs the sets in declared order, each under its own
// history table: the library's create and up text precede the app's.
func TestUpOrder(t *testing.T) {
	responses := append(setRun(nil, 1), setRun(nil, 1)...)
	m, rec := newMigrator(t, []migrator.Set{library, app}, responses...)
	if err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if rec.Pending() != 0 {
		t.Errorf("%d scripted responses unconsumed", rec.Pending())
	}
	execs := rec.SQL(sqltest.OpExec)
	want := []string{
		"SELECT lock($1)", "CREATE TABLE IF NOT EXISTS lib_schema_version", "CREATE TABLE lib_one", "INSERT INTO lib_schema_version",
		"SELECT lock($1)", "CREATE TABLE IF NOT EXISTS schema_version", "CREATE TABLE app_one", "INSERT INTO schema_version",
	}
	assertPrefixes(t, execs, want)
}

// TestDownOrder proves Down reverts the sets in reverse declared order, the
// app's down text before the library's, and reverts every applied
// migration of a set.
func TestDownOrder(t *testing.T) {
	appApplied := [][]driver.Value{{int64(1), "app_one", false}}
	libApplied := [][]driver.Value{{int64(1), "lib_one", false}}
	responses := append(setRun(appApplied, 1), setRun(libApplied, 1)...)
	m, rec := newMigrator(t, []migrator.Set{library, app}, responses...)
	if err := m.Down(context.Background()); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if rec.Pending() != 0 {
		t.Errorf("%d scripted responses unconsumed", rec.Pending())
	}
	want := []string{
		"SELECT lock($1)", "CREATE TABLE IF NOT EXISTS schema_version", "DROP TABLE app_one", "DELETE FROM schema_version",
		"SELECT lock($1)", "CREATE TABLE IF NOT EXISTS lib_schema_version", "DROP TABLE lib_one", "DELETE FROM lib_schema_version",
	}
	assertPrefixes(t, rec.SQL(sqltest.OpExec), want)
}

// TestUpStopsAtFailure proves a failing set stops the run before the sets
// after it, and the error names the set.
func TestUpStopsAtFailure(t *testing.T) {
	failing := []sqltest.Response{
		{}, {}, {Columns: historyCols}, {Err: errors.New("boom")},
		{Columns: []string{"unlock"}, Rows: [][]driver.Value{{true}}},
	}
	m, rec := newMigrator(t, []migrator.Set{library, app}, failing...)
	err := m.Up(context.Background())
	if err == nil || !strings.Contains(err.Error(), `set "lib"`) {
		t.Fatalf("Up = %v, want an error naming set lib", err)
	}
	if ops := rec.Ops(); slices.Contains(rec.SQL(sqltest.OpExec), "CREATE TABLE app_one") || slices.Contains(ops, sqltest.OpCommit) {
		t.Errorf("the app set ran after the library set failed: %v", rec.SQL(sqltest.OpExec))
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
