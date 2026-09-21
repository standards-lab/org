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

// TestSchemaUpDown proves the client over the migrator: Status on an
// empty database reports everything pending; both sets apply fresh, every
// table, index, and history table exists at the expected head, and Status
// reports both sets clean; Down reverts both sets and leaves no object
// table, and the history tables survive Down, empty; Reset then drops the
// history tables too.
func TestSchemaUpDown(t *testing.T) {
	ctx := context.Background()
	db := livetest.Open(t)
	client, err := schema.NewClient(db, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	status, err := client.Status(ctx)
	if err != nil {
		t.Fatalf("Status on an empty database: %v", err)
	}
	if len(status) != 2 || status[0].Version != 0 || len(status[0].Pending) != 3 || status[1].Version != 0 || len(status[1].Pending) != 2 {
		t.Errorf("Status on an empty database = %+v, want everything pending", status)
	}
	if err := client.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	for _, table := range objectTables {
		if !livetest.Exists(ctx, t, db, table) {
			t.Errorf("after Up, table %s is missing", table)
		}
	}
	for _, index := range []string{"blobfs_uq_directory_root", "blobfs_ix_file_directory_created", "ix_directory_owner_unit", "uq_bookmark_active"} {
		if !livetest.Exists(ctx, t, db, index) {
			t.Errorf("after Up, index %s is missing", index)
		}
	}
	if head := livetest.Head(ctx, t, db, blobfsmigrations.Table); head != 3 {
		t.Errorf("%s head = %d, want 3", blobfsmigrations.Table, head)
	}
	if head := livetest.Head(ctx, t, db, "schema_version"); head != 2 {
		t.Errorf("schema_version head = %d, want 2", head)
	}
	status, err = client.Status(ctx)
	if err != nil {
		t.Fatalf("Status after Up: %v", err)
	}
	for i, want := range []int{3, 2} {
		if s := status[i]; s.Version != want || s.Latest != want || len(s.Pending) != 0 || s.Dirty {
			t.Errorf("set %s status after Up = %+v, want version %d clean", s.Name, s, want)
		}
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

	if err := client.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	for _, table := range []string{blobfsmigrations.Table, "schema_version"} {
		if livetest.Exists(ctx, t, db, table) {
			t.Errorf("after Reset, history table %s still exists", table)
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
// caller unmapped and never matches sqlate.ErrForeignKeyViolation. The
// revert stops at the failing migration: migration 3, the index, is
// already reverted in its own transaction, so blobfs's head is 2.
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
	// Each revert ran in its own transaction: migration 3 is reverted and
	// migration 2 is not, the history agrees with the schema, and the
	// client's Down, in the right order, still succeeds.
	if head := livetest.Head(ctx, t, db, blobfsmigrations.Table); head != 2 {
		t.Errorf("%s head after the refused Down = %d, want 2", blobfsmigrations.Table, head)
	}
	if livetest.Exists(ctx, t, db, "blobfs_ix_file_directory_created") {
		t.Error("migration 3's index survived the refused Down; the revert did not stop at migration 2")
	}
	if err := client.Down(ctx); err != nil {
		t.Fatalf("Down in the right order: %v", err)
	}
}
