//go:build integration

package schema_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/migrate"

	"github.com/standards-lab/org/experiments/blobfs/admin/schema"
	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	blobfsmigrations "github.com/standards-lab/org/experiments/blobfs/lib/blobfs/migrations"
)

var objectTables = []string{"blobfs_directory", "blobfs_file", "directory_owner", "bookmark"}

// TestSchemaUpDown is the stage gate: both sets apply fresh through the
// client, every table and both history tables exist at the expected head,
// and Down reverts both sets and leaves no object table. The history tables
// survive Down, empty: sqlate creates them on every run and never drops
// them.
func TestSchemaUpDown(t *testing.T) {
	ctx := context.Background()
	db := livetest.Open(t)
	client, err := schema.NewClient(db, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := client.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	for _, table := range objectTables {
		if !livetest.Exists(ctx, t, db, table) {
			t.Errorf("after Up, table %s is missing", table)
		}
	}
	for _, index := range []string{"blobfs_uq_directory_root", "ix_directory_owner_unit", "uq_bookmark_active"} {
		if !livetest.Exists(ctx, t, db, index) {
			t.Errorf("after Up, index %s is missing", index)
		}
	}
	if head := livetest.Head(ctx, t, db, blobfsmigrations.Table); head != 2 {
		t.Errorf("%s head = %d, want 2", blobfsmigrations.Table, head)
	}
	if head := livetest.Head(ctx, t, db, "schema_version"); head != 2 {
		t.Errorf("schema_version head = %d, want 2", head)
	}

	// Up again is a no-op.
	if err := client.Up(ctx); err != nil {
		t.Fatalf("second Up: %v", err)
	}

	if err := client.Down(ctx); err != nil {
		t.Fatalf("Down: %v", err)
	}
	for _, table := range objectTables {
		if livetest.Exists(ctx, t, db, table) {
			t.Errorf("after Down, table %s still exists", table)
		}
	}
	for _, table := range []string{blobfsmigrations.Table, "schema_version"} {
		if !livetest.Exists(ctx, t, db, table) {
			t.Errorf("after Down, history table %s is gone", table)
		} else if head := livetest.Head(ctx, t, db, table); head != 0 {
			t.Errorf("after Down, %s head = %d, want 0", table, head)
		}
	}
}

// TestWrongOrderDownIsRefused is the evidence for the reverse-order rule:
// with both sets applied, reverting blobfs's set before the consumer's
// fails because bookmark's foreign key depends on blobfs_file. The
// inner migrator is driven directly, in the wrong order.
//
// Finding: the refusal is not a class-23 foreign-key violation. Postgres
// refuses the DROP TABLE through its dependency tracker with SQLSTATE 2BP01
// (dependent objects still exist), whether or not any row exists, and
// sqlate's dialect maps only class 22 and class 23, so the error reaches the
// caller unmapped and never matches sqlate.ErrForeignKeyViolation.
func TestWrongOrderDownIsRefused(t *testing.T) {
	ctx := context.Background()
	db := livetest.Open(t)
	client, err := schema.NewClient(db, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := client.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}

	blobfsSet, err := blobfsmigrations.Migrations(db.Dialect())
	if err != nil {
		t.Fatalf("Migrations: %v", err)
	}
	inner, err := migrate.New(db, blobfsSet, migrate.Options{Table: blobfsmigrations.Table})
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	err = inner.Down(ctx, len(blobfsSet))
	if err == nil {
		t.Fatal("blobfs Down before the consumer's succeeded; bookmark's foreign key did not block it")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "2BP01" {
		t.Fatalf("blobfs Down before the consumer's = %v, want SQLSTATE 2BP01 (dependent objects still exist)", err)
	}
	if errors.Is(err, sqlate.ErrForeignKeyViolation) {
		t.Error("the refusal matched ErrForeignKeyViolation; the finding above no longer holds")
	}
	// The failed revert ran in a transaction, so blobfs's history is intact
	// and the client's Down, in the right order, still succeeds.
	if head := livetest.Head(ctx, t, db, blobfsmigrations.Table); head != 2 {
		t.Errorf("%s head after the refused Down = %d, want 2", blobfsmigrations.Table, head)
	}
	if err := client.Down(ctx); err != nil {
		t.Fatalf("Down in the right order: %v", err)
	}
}
