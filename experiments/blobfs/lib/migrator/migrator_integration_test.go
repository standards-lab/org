//go:build integration

package migrator_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/migrate"
	"github.com/standards-lab/sqlate/postgres"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	blobfsmigrations "github.com/standards-lab/org/experiments/blobfs/lib/blobfs/migrations"
	"github.com/standards-lab/org/experiments/blobfs/lib/migrator"
	appmigrations "github.com/standards-lab/org/experiments/blobfs/migrations"
)

// The tables the two sets create, and their history tables.
var (
	objectTables  = []string{"blobfs_directory", "blobfs_file", "directory_owner", "bookmark"}
	historyTables = []string{blobfsmigrations.Table, "schema_version"}
)

// canonicalSets returns blobfs's set and the consumer's, in canonical
// order, with blobfs's set cut to its first n migrations when n is
// positive, which stands for the version an installed database holds.
func canonicalSets(t *testing.T, dialect sqlate.Dialect, n int) []migrator.Set {
	t.Helper()
	blobfsSet, err := blobfsmigrations.Migrations(dialect)
	if err != nil {
		t.Fatalf("blobfs Migrations: %v", err)
	}
	if n > 0 {
		blobfsSet = blobfsSet[:n]
	}
	consumerSet, err := appmigrations.Migrations()
	if err != nil {
		t.Fatalf("consumer Migrations: %v", err)
	}
	return []migrator.Set{
		{Name: blobfsmigrations.Source, Table: blobfsmigrations.Table, Migrations: blobfsSet},
		{Name: "consumer", Migrations: consumerSet},
	}
}

func newLive(t *testing.T, db *sqlate.DB, sets []migrator.Set, opts migrator.Options) *migrator.Migrator {
	t.Helper()
	m, err := migrator.New(db, sets, opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return m
}

// assertClean checks Status reports every set at its latest version, with
// nothing pending and nothing dirty.
func assertClean(ctx context.Context, t *testing.T, m *migrator.Migrator, versions ...int) {
	t.Helper()
	sets, err := m.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(sets) != len(versions) {
		t.Fatalf("Status returned %d sets, want %d", len(sets), len(versions))
	}
	for i, s := range sets {
		if s.Version != versions[i] || s.Latest != versions[i] || len(s.Pending) != 0 || s.Dirty {
			t.Errorf("set %s status = %+v, want version %d, nothing pending, clean", s.Name, s, versions[i])
		}
	}
}

// assertRelations checks that each named relation exists, or does not.
func assertRelations(ctx context.Context, t *testing.T, db *sqlate.DB, want bool, relations ...string) {
	t.Helper()
	for _, r := range relations {
		if got := livetest.Exists(ctx, t, db, r); got != want {
			t.Errorf("%s exists = %v, want %v", r, got, want)
		}
	}
}

// seedRows writes the rows the reset-order proof needs: a directory with
// an owner row and a file with a bookmark row, so both consumer foreign
// keys hold a reference into blobfs's tables.
func seedRows(ctx context.Context, t *testing.T, db *sqlate.DB) {
	t.Helper()
	dir, file, unit := blobfs.NewID(), blobfs.NewID(), blobfs.NewID()
	for _, s := range []struct {
		q    string
		args []any
	}{
		{"INSERT INTO blobfs_directory (id, parent_id, name) VALUES ($1, $2, 'docs')", []any{dir, blobfs.RootID}},
		{"INSERT INTO directory_owner (directory_id, unit_id) VALUES ($1, $2)", []any{dir, unit}},
		{"INSERT INTO blobfs_file (id, directory_id, name, status, key, content_type) VALUES ($1, $2, 'a.txt', 'available', $3, 'text/plain')", []any{file, dir, file + "/a.txt"}},
		{"INSERT INTO bookmark (unit_id, file_id) VALUES ($1, $2)", []any{unit, file}},
	} {
		if _, err := db.ExecContext(ctx, s.q, s.args...); err != nil {
			t.Fatalf("%s: %v", s.q, err)
		}
	}
}

// count returns a one-row integer query's value.
func count(ctx context.Context, t *testing.T, db *sqlate.DB, q string, args ...any) int {
	t.Helper()
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	defer func() { _ = rows.Close() }()
	var n int
	if !rows.Next() || rows.Scan(&n) != nil {
		t.Fatalf("%s: no row: %v", q, rows.Err())
	}
	return n
}

// TestMultiStatementMigration answers whether a transactional migration
// whose text holds two statements runs through migrate on the pgx driver:
// migrate hands the text to one ExecContext with no arguments, and pgx runs
// a zero-argument Exec over the simple protocol, which accepts several
// statements. The consumer's bookmark migration (a table and a partial
// unique index in one file) relies on the answer.
func TestMultiStatementMigration(t *testing.T) {
	ctx := context.Background()
	db := livetest.Open(t)
	set := []migrate.Migration{{
		Version:       1,
		Name:          "two_statements",
		Up:            "CREATE TABLE two_a (id int PRIMARY KEY);\n\nCREATE UNIQUE INDEX two_a_idx ON two_a (id) WHERE id > 0;\n",
		Down:          "DROP INDEX two_a_idx;\nDROP TABLE two_a;\n",
		Transactional: true,
	}}
	m := newLive(t, db, []migrator.Set{{Name: "probe", Table: "probe_schema_version", Migrations: set}}, migrator.Options{})
	if err := m.Up(ctx); err != nil {
		t.Fatalf("Up with a two-statement migration: %v", err)
	}
	assertRelations(ctx, t, db, true, "two_a", "two_a_idx")
	if err := m.Down(ctx); err != nil {
		t.Fatalf("Down with a two-statement migration: %v", err)
	}
	assertRelations(ctx, t, db, false, "two_a")
}

// TestFreshReplay is the gate's first item: Up on an empty database
// applies both sets to their latest versions and Status reports them
// clean; Reset then leaves neither the sets' objects nor their history
// tables; and Up again replays every set from zero to the same state.
func TestFreshReplay(t *testing.T) {
	ctx := context.Background()
	db := livetest.Open(t)
	m := newLive(t, db, canonicalSets(t, db.Dialect(), 0), migrator.Options{})

	before, err := m.Status(ctx)
	if err != nil {
		t.Fatalf("Status on an empty database: %v", err)
	}
	if before[0].Version != 0 || len(before[0].Pending) != 3 || before[1].Version != 0 || len(before[1].Pending) != 2 {
		t.Errorf("Status on an empty database = %+v, want everything pending", before)
	}

	if err := m.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	assertClean(ctx, t, m, 3, 2)
	assertRelations(ctx, t, db, true, append(append([]string{"blobfs_ix_file_directory_created"}, objectTables...), historyTables...)...)

	if err := m.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	assertRelations(ctx, t, db, false, append(append([]string{"blobfs_ix_file_directory_created"}, objectTables...), historyTables...)...)
	if after, err := m.Status(ctx); err != nil || after[0].Version != 0 || after[1].Version != 0 {
		t.Errorf("Status after Reset = %+v, %v; want version 0 for both sets", after, err)
	}

	if err := m.Up(ctx); err != nil {
		t.Fatalf("Up after Reset: %v", err)
	}
	assertClean(ctx, t, m, 3, 2)
	assertRelations(ctx, t, db, true, append(objectTables, historyTables...)...)
	if n := count(ctx, t, db, "SELECT COUNT(*) FROM blobfs_directory"); n != 1 {
		t.Errorf("after the replay, blobfs_directory holds %d rows, want the one seeded root", n)
	}
}

// TestUpgradeAfterRestart is the gate's second item: a database installed
// at blobfs version 2 (the set cut to its first two migrations stands for
// the earlier binary) holds rows; a new migrator over the full set, built
// on a new pool as a restarted process would, applies only migration 3;
// the rows survive; and Status shows version 3.
func TestUpgradeAfterRestart(t *testing.T) {
	ctx := context.Background()
	db, dsn := livetest.OpenDSN(t)
	installed := newLive(t, db, canonicalSets(t, db.Dialect(), 2), migrator.Options{})
	if err := installed.Up(ctx); err != nil {
		t.Fatalf("Up at version 2: %v", err)
	}
	assertClean(ctx, t, installed, 2, 2)
	assertRelations(ctx, t, db, false, "blobfs_ix_file_directory_created")
	seedRows(ctx, t, db)
	rowsBefore := count(ctx, t, db, "SELECT (SELECT COUNT(*) FROM blobfs_directory) + (SELECT COUNT(*) FROM blobfs_file) + (SELECT COUNT(*) FROM directory_owner) + (SELECT COUNT(*) FROM bookmark)")

	// The restart: a new pool over the same database, and a migrator over
	// the full set.
	pool, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = pool.Close() }()
	restarted := sqlate.Wrap(pool, postgres.Dialect{})
	upgraded := newLive(t, restarted, canonicalSets(t, restarted.Dialect(), 0), migrator.Options{})

	status, err := upgraded.Status(ctx)
	if err != nil {
		t.Fatalf("Status before the upgrade: %v", err)
	}
	if b := status[0]; b.Version != 2 || b.Latest != 3 || len(b.Pending) != 1 || b.Pending[0].Version != 3 || b.Pending[0].Name != "file_created_index" {
		t.Errorf("blobfs status before the upgrade = %+v, want version 2 of 3 with file_created_index pending", b)
	}
	if err := upgraded.Up(ctx); err != nil {
		t.Fatalf("Up after the restart: %v", err)
	}
	assertClean(ctx, t, upgraded, 3, 2)
	assertRelations(ctx, t, restarted, true, "blobfs_ix_file_directory_created")
	if n := count(ctx, t, restarted, "SELECT COUNT(*) FROM "+blobfsmigrations.Table); n != 3 {
		t.Errorf("blobfs history holds %d rows after the upgrade, want 3", n)
	}
	// Only migration 3 was applied by the upgrade: rows 1 and 2 keep the
	// applied_at of the install, which precedes row 3's.
	if n := count(ctx, t, restarted, "SELECT COUNT(*) FROM "+blobfsmigrations.Table+" WHERE version < 3 AND applied_at >= (SELECT applied_at FROM "+blobfsmigrations.Table+" WHERE version = 3)"); n != 0 {
		t.Errorf("%d rows below version 3 were applied at or after version 3; the upgrade re-applied them", n)
	}
	if rowsAfter := count(ctx, t, restarted, "SELECT (SELECT COUNT(*) FROM blobfs_directory) + (SELECT COUNT(*) FROM blobfs_file) + (SELECT COUNT(*) FROM directory_owner) + (SELECT COUNT(*) FROM bookmark)"); rowsAfter != rowsBefore {
		t.Errorf("%d rows after the upgrade, %d before; the rows did not survive", rowsAfter, rowsBefore)
	}
}

// TestResetOrderAcrossForeignKeys is the gate's third item. With rows in
// directory_owner and bookmark, whose foreign keys reference blobfs's
// tables, Reset succeeds because the consumer's set reverts first. The
// order matters: a migrator declared in the wrong order (the consumer's
// set first) reverts blobfs's set first and fails, and Down of blobfs's
// set alone refuses while the consumer's tables exist. Both refusals are
// a SetError naming blobfs's set over Postgres's SQLSTATE 2BP01 (dependent
// objects still exist), which sqlate's dialect does not map.
//
// Finding: the refusal stops the set at the migration that fails, and the
// migrations above it are already reverted. The wrong-order reset drops
// the index of migration 3 in its own transaction and then fails at
// migration 2's DROP TABLE, so blobfs's head is 2 afterwards, not 3. The
// history is consistent with the schema, and the correct order still
// succeeds after.
func TestResetOrderAcrossForeignKeys(t *testing.T) {
	ctx := context.Background()
	db := livetest.Open(t)
	sets := canonicalSets(t, db.Dialect(), 0)
	m := newLive(t, db, sets, migrator.Options{})
	if err := m.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	seedRows(ctx, t, db)
	if err := m.Reset(ctx); err != nil {
		t.Fatalf("Reset with rows under both foreign keys: %v", err)
	}
	assertRelations(ctx, t, db, false, append(objectTables, historyTables...)...)

	if err := m.Up(ctx); err != nil {
		t.Fatalf("Up again: %v", err)
	}
	seedRows(ctx, t, db)
	wrongOrder := newLive(t, db, []migrator.Set{sets[1], sets[0]}, migrator.Options{})
	assertDependentObjects(t, "Reset in the wrong order", wrongOrder.Reset(ctx))
	blobfsAlone := newLive(t, db, sets[:1], migrator.Options{})
	assertDependentObjects(t, "Down of blobfs's set alone", blobfsAlone.Down(ctx))
	assertRelations(ctx, t, db, true, append(objectTables, historyTables...)...)
	if head := livetest.Head(ctx, t, db, blobfsmigrations.Table); head != 2 {
		t.Errorf("blobfs head after the refused reverts = %d, want 2 (the index reverted, the table drop refused)", head)
	}
	assertRelations(ctx, t, db, false, "blobfs_ix_file_directory_created")
	if err := m.Reset(ctx); err != nil {
		t.Fatalf("Reset in the right order after the refusals: %v", err)
	}
	assertRelations(ctx, t, db, false, append(objectTables, historyTables...)...)
}

// assertDependentObjects checks err is a SetError naming blobfs's set over
// SQLSTATE 2BP01.
func assertDependentObjects(t *testing.T, what string, err error) {
	t.Helper()
	var se *migrator.SetError
	if !errors.As(err, &se) || se.Set != blobfsmigrations.Source {
		t.Fatalf("%s = %v, want a SetError naming %s", what, err, blobfsmigrations.Source)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "2BP01" {
		t.Errorf("%s = %v, want SQLSTATE 2BP01 (dependent objects still exist)", what, err)
	}
}

// gateName is the advisory lock the test holds so a starter's first
// migration blocks inside its run, holding the outer lock, until the test
// has seen the second starter wait.
const gateName = "blobfs.test.gate"

// gateSet is a test-only set whose one migration creates a table and then
// waits on the gate inside its transaction, so a run that reaches it
// holds every lock it took and stays there until the test releases the
// gate.
var gateSet = migrator.Set{
	Name:  "gate",
	Table: "gate_schema_version",
	Migrations: []migrate.Migration{{
		Version:       1,
		Name:          "gate",
		Up:            "CREATE TABLE gate_probe (id int);\nSELECT pg_advisory_xact_lock(hashtext('" + gateName + "'));",
		Down:          "DROP TABLE gate_probe",
		Transactional: true,
	}},
}

// holdGate takes the gate on a pinned connection and returns the release.
func holdGate(ctx context.Context, t *testing.T, db *sqlate.DB) func() {
	t.Helper()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("conn: %v", err)
	}
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock(hashtext($1))", gateName); err != nil {
		t.Fatalf("hold the gate: %v", err)
	}
	return func() {
		if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_unlock(hashtext($1))", gateName); err != nil {
			t.Errorf("release the gate: %v", err)
		}
		_ = conn.Close()
	}
}

// waitForWaiters polls pg_stat_activity, scoped to the test's database,
// until n backends wait on a lock.
func waitForWaiters(ctx context.Context, t *testing.T, db *sqlate.DB, n int) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		got := count(ctx, t, db, "SELECT COUNT(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'")
		if got >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d backends wait on a lock, want %d", got, n)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// startBoth releases two starters through a barrier and runs each one's
// Up, returning the two errors once both have ended.
func startBoth(ctx context.Context, a, b *migrator.Migrator) (errA, errB error) {
	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(2)
	run := func(m *migrator.Migrator, out *error) {
		defer done.Done()
		start.Wait()
		*out = m.Up(ctx)
	}
	go run(a, &errA)
	go run(b, &errB)
	start.Done()
	done.Wait()
	return errA, errB
}

// TestConcurrentStartersSerialize is the gate's fourth item: two migrators
// start Up at the same time on one empty database, over the gate set and
// both real sets. The first to take the outer lock blocks on the gate
// inside its first migration; the test sees the second starter waiting
// on a lock (the outer one) before it releases the gate. Both succeed,
// every migration is applied exactly once, and each history table holds
// one row per migration. The control runs the same race with the outer
// lock off: the second starter then waits on the first's transaction
// instead and fails with a duplicate object, which is what the outer lock
// prevents. Postgres reports the duplicate as a unique violation on the
// catalog's own index (pg_type_typname_nsp_index, SQLSTATE 23505) when the
// second CREATE TABLE waited on the first's transaction, or as SQLSTATE
// 42P07 (duplicate table) when the first had committed before the check.
func TestConcurrentStartersSerialize(t *testing.T) {
	ctx := context.Background()
	db := livetest.Open(t)
	sets := append([]migrator.Set{gateSet}, canonicalSets(t, db.Dialect(), 0)...)

	release := holdGate(ctx, t, db)
	a := newLive(t, db, sets, migrator.Options{})
	b := newLive(t, db, sets, migrator.Options{})
	var errA, errB error
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		errA, errB = startBoth(ctx, a, b)
	}()
	waitForWaiters(ctx, t, db, 2)
	release()
	<-finished
	if errA != nil || errB != nil {
		t.Fatalf("serialized starters: a = %v, b = %v; want both to succeed", errA, errB)
	}
	assertClean(ctx, t, a, 1, 3, 2)
	for table, want := range map[string]int{"gate_schema_version": 1, blobfsmigrations.Table: 3, "schema_version": 2} {
		if n := count(ctx, t, db, "SELECT COUNT(*) FROM "+table); n != want {
			t.Errorf("%s holds %d rows, want %d: a migration was applied more or less than once", table, n, want)
		}
	}
	if err := a.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// The control: the same race without the outer lock.
	release = holdGate(ctx, t, db)
	a = newLive(t, db, sets, migrator.Options{Unlocked: true})
	b = newLive(t, db, sets, migrator.Options{Unlocked: true})
	finished = make(chan struct{})
	go func() {
		defer close(finished)
		errA, errB = startBoth(ctx, a, b)
	}()
	waitForWaiters(ctx, t, db, 2)
	release()
	<-finished
	if (errA == nil) == (errB == nil) {
		t.Fatalf("unlocked starters: a = %v, b = %v; want exactly one to fail", errA, errB)
	}
	failed := errors.Join(errA, errB)
	var pgErr *pgconn.PgError
	if !errors.As(failed, &pgErr) || (pgErr.Code != "42P07" && pgErr.Code != "23505") {
		t.Errorf("the unlocked loser failed with %v, want a duplicate object (SQLSTATE 42P07 or 23505)", failed)
	}
	t.Logf("the unlocked loser failed with SQLSTATE %s: %v", pgErr.Code, failed)
}

// TestDirtyRefusalOnEngine proves the dirty state as migrate records it: a
// non-transactional migration whose statement fails leaves its history
// row dirty. A new migrator holding blobfs's set before the dirty set
// refuses Up, Down, and Reset with a SetError naming the dirty set and
// the version, classified as migrate.ErrDirty, before blobfs's set runs;
// Status reports the set dirty. Force to version 0, the operator's
// statement that nothing of the set is applied, clears the mark, and
// Reset then leaves no history table.
func TestDirtyRefusalOnEngine(t *testing.T) {
	ctx := context.Background()
	db := livetest.Open(t)
	failing := migrator.Set{
		Name:  "probe",
		Table: "probe_schema_version",
		Migrations: []migrate.Migration{{
			Version: 1,
			Name:    "fails_midway",
			Up:      "CREATE INDEX CONCURRENTLY probe_ix ON no_such_table (x)",
			Down:    "DROP INDEX probe_ix",
		}},
	}
	first := newLive(t, db, []migrator.Set{failing}, migrator.Options{})
	err := first.Up(ctx)
	var de *migrate.DirtyError
	if !errors.As(err, &de) || de.Version != 1 {
		t.Fatalf("Up of the failing migration = %v, want a DirtyError at version 1", err)
	}
	if n := count(ctx, t, db, "SELECT COUNT(*) FROM probe_schema_version WHERE version = 1 AND dirty"); n != 1 {
		t.Fatalf("the history holds %d dirty rows at version 1, want 1", n)
	}

	sets := append(canonicalSets(t, db.Dialect(), 0), failing)
	m := newLive(t, db, sets, migrator.Options{})
	for name, op := range map[string]func(context.Context) error{"Up": m.Up, "Down": m.Down, "Reset": m.Reset} {
		err := op(ctx)
		var se *migrator.SetError
		if !errors.Is(err, migrate.ErrDirty) || !errors.As(err, &se) || se.Set != "probe" || !errors.As(err, &de) || de.Version != 1 {
			t.Errorf("%s over a dirty set = %v, want ErrDirty naming set probe at version 1", name, err)
		}
	}
	assertRelations(ctx, t, db, false, "blobfs_directory", blobfsmigrations.Table)
	status, err := m.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if probe := status[2]; !probe.Dirty || probe.Version != 1 || probe.Name != "probe" {
		t.Errorf("probe status = %+v, want version 1 dirty", probe)
	}

	if err := m.Force(ctx, "probe", 0); err != nil {
		t.Fatalf("Force: %v", err)
	}
	if err := m.Reset(ctx); err != nil {
		t.Fatalf("Reset after Force: %v", err)
	}
	assertRelations(ctx, t, db, false, "probe_schema_version", blobfsmigrations.Table, "schema_version")
	if err := m.Up(ctx); err == nil {
		t.Error("Up after the reset applied the failing migration")
	}
}
