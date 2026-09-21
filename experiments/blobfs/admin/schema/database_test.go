package schema_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/admin/schema"
	blobfsmigrations "github.com/standards-lab/org/experiments/blobfs/lib/blobfs/migrations"
)

// postgresDialect is the stub dialect under the name the Postgres engine
// reports, with the lock capability, so Sets selects the Postgres directory
// without the driver and the inner migrators run their locked protocol
// against the recorder.
type postgresDialect struct{ sqltest.Dialect }

func (postgresDialect) Name() string { return "postgres" }

func (postgresDialect) Lock(ctx context.Context, conn *sql.Conn, name string) error {
	_, err := conn.ExecContext(ctx, "SELECT lock($1)", name)
	return err
}

func (postgresDialect) Unlock(ctx context.Context, conn *sql.Conn, name string) error {
	var held bool
	if err := conn.QueryRowContext(ctx, "SELECT unlock($1)", name).Scan(&held); err != nil {
		return err
	}
	if !held {
		return errors.New("lock not held")
	}
	return nil
}

// TestSets_OrdersBlobfsFirstAndTheConsumerLast fixes the canonical order:
// blobfs's set under its own history table, then the consumer's under
// sqlate's default table, each holding its source's migrations.
func TestSets_OrdersBlobfsFirstAndTheConsumerLast(t *testing.T) {
	sets, err := schema.Sets(postgresDialect{})
	if err != nil {
		t.Fatalf("Sets: %v", err)
	}
	if len(sets) != 2 {
		t.Fatalf("Sets returned %d sets, want 2", len(sets))
	}
	first, last := sets[0], sets[1]
	if first.Name != blobfsmigrations.Source || first.Table != blobfsmigrations.Table {
		t.Errorf("first set = %q under %q, want %q under %q", first.Name, first.Table, blobfsmigrations.Source, blobfsmigrations.Table)
	}
	if len(first.Migrations) != 3 || first.Migrations[0].Name != "directory" {
		t.Errorf("first set holds %d migrations starting with %q, want blobfs's 3 starting with directory", len(first.Migrations), first.Migrations[0].Name)
	}
	if last.Name != schema.ConsumerSet || last.Table != "" {
		t.Errorf("last set = %q under %q, want %q under the default table", last.Name, last.Table, schema.ConsumerSet)
	}
	if len(last.Migrations) != 2 || last.Migrations[0].Name != "directory_owner" {
		t.Errorf("last set holds %d migrations starting with %q, want the consumer's 2 starting with directory_owner", len(last.Migrations), last.Migrations[0].Name)
	}
}

// TestNewClient_RefusesADialectWithoutDDL proves the selection by dialect
// reaches the client: an engine blobfs ships no directory for fails with
// the source's sentinel.
func TestNewClient_RefusesADialectWithoutDDL(t *testing.T) {
	pool, _ := sqltest.Open(t)
	_, err := schema.NewClient(sqlate.Wrap(pool, sqltest.Dialect{}), nil)
	if !errors.Is(err, blobfsmigrations.ErrUnsupportedEngine) {
		t.Fatalf("NewClient(test dialect) = %v, want ErrUnsupportedEngine", err)
	}
}

var historyCols = []string{"version", "name", "dirty"}

// The scripted responses of the migrator's protocol: the outer lock and
// unlock, the history-table existence check of a set with no history yet,
// and one set's unlocked run over an empty history.
var (
	locked   = sqltest.Response{}
	unlocked = sqltest.Response{Columns: []string{"unlock"}, Rows: [][]driver.Value{{true}}}
	absent   = sqltest.Response{Columns: []string{"count"}, Rows: [][]driver.Value{{int64(0)}}}
)

// setRun scripts one set's unlocked run over an empty history: the history
// table's create, the history read, and one transaction per migration.
func setRun(steps int) []sqltest.Response {
	out := []sqltest.Response{
		{}, // CREATE TABLE IF NOT EXISTS
		{Columns: historyCols},
	}
	for range steps {
		out = append(out, sqltest.Response{}, sqltest.Response{}) // the text, the history row
	}
	return out
}

// freshUp scripts a whole Up over an empty database: the lock, both sets'
// checks, blobfs's three migrations, the consumer's two, and the unlock.
func freshUp() []sqltest.Response {
	out := []sqltest.Response{locked, absent, absent}
	out = append(out, setRun(3)...)
	out = append(out, setRun(2)...)
	return append(out, unlocked)
}

// TestUp_RunsBlobfsBeforeTheConsumer proves the client hands the sets to
// the migrator in canonical order: on a fresh database, blobfs's history
// table and DDL run before the consumer's, under one outer lock.
func TestUp_RunsBlobfsBeforeTheConsumer(t *testing.T) {
	pool, rec := sqltest.Open(t, freshUp()...)
	c, err := schema.NewClient(sqlate.Wrap(pool, postgresDialect{}), nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := c.Up(context.Background()); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if rec.Pending() != 0 {
		t.Errorf("%d scripted responses unconsumed", rec.Pending())
	}
	execs := rec.SQL(sqltest.OpExec)
	position := func(prefix string) int {
		return slices.IndexFunc(execs, func(s string) bool { return strings.Contains(s, prefix) })
	}
	order := []string{
		"CREATE TABLE IF NOT EXISTS " + blobfsmigrations.Table,
		"CREATE TABLE blobfs_directory",
		"CREATE TABLE blobfs_file",
		"CREATE INDEX blobfs_ix_file_directory_created",
		"CREATE TABLE IF NOT EXISTS schema_version",
		"CREATE TABLE directory_owner",
		"CREATE TABLE bookmark",
	}
	last := -1
	for _, text := range order {
		at := position(text)
		if at < 0 {
			t.Errorf("no exec contains %q:\n%s", text, strings.Join(execs, "\n"))
			continue
		}
		if at <= last {
			t.Errorf("%q ran at %d, before the text that must precede it at %d", text, at, last)
		}
		last = at
	}
	if locks := slices.DeleteFunc(slices.Clone(execs), func(s string) bool { return !strings.HasPrefix(s, "SELECT lock(") }); len(locks) != 1 {
		t.Errorf("the run took %d locks, want the one outer lock", len(locks))
	}
}

// TestStatus_ReadsBothSets proves Status returns the sets in canonical
// order from the migrator's reads: blobfs at version 2 of 3 with the index
// migration pending, and the consumer with no history table yet.
func TestStatus_ReadsBothSets(t *testing.T) {
	present := sqltest.Response{Columns: []string{"count"}, Rows: [][]driver.Value{{int64(1)}}}
	head := sqltest.Response{Columns: []string{"version", "dirty"}, Rows: [][]driver.Value{{int64(2), false}}}
	applied := sqltest.Response{Columns: historyCols, Rows: [][]driver.Value{{int64(1), "directory", false}, {int64(2), "file", false}}}
	pool, _ := sqltest.Open(t, present, head, present, applied, absent, absent)
	c, err := schema.NewClient(sqlate.Wrap(pool, postgresDialect{}), nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	sets, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(sets) != 2 {
		t.Fatalf("Status returned %d sets, want 2", len(sets))
	}
	b, consumer := sets[0], sets[1]
	if b.Name != blobfsmigrations.Source || b.Table != blobfsmigrations.Table || b.Version != 2 || b.Latest != 3 || len(b.Pending) != 1 || b.Pending[0].Name != "file_created_index" || b.Dirty {
		t.Errorf("blobfs status = %+v", b)
	}
	if consumer.Name != schema.ConsumerSet || consumer.Table != "schema_version" || consumer.Version != 0 || consumer.Latest != 2 || len(consumer.Pending) != 2 {
		t.Errorf("consumer status = %+v", consumer)
	}
}
