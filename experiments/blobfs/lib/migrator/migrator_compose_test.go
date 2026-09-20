//go:build compose

package migrator_test

import (
	"context"
	"testing"

	"github.com/standards-lab/sqlate/migrate"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/migrator"
)

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
	m, err := migrator.New(db, []migrator.Set{{Name: "probe", Table: "probe_schema_version", Migrations: set}}, migrator.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := m.Up(ctx); err != nil {
		t.Fatalf("Up with a two-statement migration: %v", err)
	}
	for _, relation := range []string{"two_a", "two_a_idx"} {
		if !livetest.Exists(ctx, t, db, relation) {
			t.Errorf("after Up, %s is missing", relation)
		}
	}
	if err := m.Down(ctx); err != nil {
		t.Fatalf("Down with a two-statement migration: %v", err)
	}
	if livetest.Exists(ctx, t, db, "two_a") {
		t.Error("after Down, two_a still exists")
	}
}
