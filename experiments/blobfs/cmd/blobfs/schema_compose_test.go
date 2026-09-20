//go:build compose

package main

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/migrate"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/migrations"
)

var objectTables = []string{"blobfs_volume", "blobfs_directory", "blobfs_file", "volume_owner", "volume_bookmark"}

// TestSchemaUpDown is the stage gate: both sets apply fresh through the
// command's own composition, every table and both history tables exist at
// the expected head, and Down reverts both sets and leaves no object table.
// The history tables survive Down, empty: sqlate creates them on every run
// and never drops them.
func TestSchemaUpDown(t *testing.T) {
	ctx := context.Background()
	db := livetest.Open(t)
	schema, err := newSchema(db, nil)
	if err != nil {
		t.Fatalf("newSchema: %v", err)
	}
	if err := schema.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	for _, table := range objectTables {
		if !livetest.Exists(ctx, t, db, table) {
			t.Errorf("after Up, table %s is missing", table)
		}
	}
	for _, index := range []string{"ix_volume_owner_unit", "uq_volume_bookmark_active"} {
		if !livetest.Exists(ctx, t, db, index) {
			t.Errorf("after Up, index %s is missing", index)
		}
	}
	if head := livetest.Head(ctx, t, db, migrations.Table); head != 3 {
		t.Errorf("%s head = %d, want 3", migrations.Table, head)
	}
	if head := livetest.Head(ctx, t, db, "schema_version"); head != 2 {
		t.Errorf("schema_version head = %d, want 2", head)
	}

	// Up again is a no-op.
	if err := schema.Up(ctx); err != nil {
		t.Fatalf("second Up: %v", err)
	}

	if err := schema.Down(ctx); err != nil {
		t.Fatalf("Down: %v", err)
	}
	for _, table := range objectTables {
		if livetest.Exists(ctx, t, db, table) {
			t.Errorf("after Down, table %s still exists", table)
		}
	}
	for _, table := range []string{migrations.Table, "schema_version"} {
		if !livetest.Exists(ctx, t, db, table) {
			t.Errorf("after Down, history table %s is gone", table)
		} else if head := livetest.Head(ctx, t, db, table); head != 0 {
			t.Errorf("after Down, %s head = %d, want 0", table, head)
		}
	}
}

// TestWrongOrderDownIsRefused is the evidence for the reverse-order rule:
// with both sets applied, reverting blobfs's set before the consumer's
// fails because volume_bookmark's foreign key depends on blobfs_file. The
// inner migrators are driven directly, in the wrong order.
//
// Finding: the refusal is not a class-23 foreign-key violation. Postgres
// refuses the DROP TABLE through its dependency tracker with SQLSTATE 2BP01
// (dependent objects still exist), whether or not any row exists, and
// sqlate's dialect maps only class 22 and class 23, so the error reaches the
// caller unmapped and never matches sqlate.ErrForeignKeyViolation.
func TestWrongOrderDownIsRefused(t *testing.T) {
	ctx := context.Background()
	db := livetest.Open(t)
	schema, err := newSchema(db, nil)
	if err != nil {
		t.Fatalf("newSchema: %v", err)
	}
	if err := schema.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}

	blobfsSet, err := migrations.Migrations(db.Dialect())
	if err != nil {
		t.Fatalf("Migrations: %v", err)
	}
	inner, err := migrate.New(db, blobfsSet, migrate.Options{Table: migrations.Table})
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	err = inner.Down(ctx, len(blobfsSet))
	if err == nil {
		t.Fatal("blobfs Down before the consumer's succeeded; volume_bookmark's foreign key did not block it")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "2BP01" {
		t.Fatalf("blobfs Down before the consumer's = %v, want SQLSTATE 2BP01 (dependent objects still exist)", err)
	}
	if errors.Is(err, sqlate.ErrForeignKeyViolation) {
		t.Error("the refusal matched ErrForeignKeyViolation; the finding above no longer holds")
	}
	// The failed revert ran in a transaction, so blobfs's history is intact
	// and the shim's Down, in the right order, still succeeds.
	if head := livetest.Head(ctx, t, db, migrations.Table); head != 3 {
		t.Errorf("%s head after the refused Down = %d, want 3", migrations.Table, head)
	}
	if err := schema.Down(ctx); err != nil {
		t.Fatalf("Down in the right order: %v", err)
	}
}

// TestBookmarkOneActivePerVolume proves the partial unique index: a second
// active bookmark for one volume is refused under the index's name, while a
// second inactive one is accepted.
func TestBookmarkOneActivePerVolume(t *testing.T) {
	ctx := context.Background()
	db := livetest.Open(t)
	schema, err := newSchema(db, nil)
	if err != nil {
		t.Fatalf("newSchema: %v", err)
	}
	if err := schema.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	volume, dir := seedBookmark(ctx, t, db)

	second := insertFile(ctx, t, db, dir, "second.txt")
	_, err = db.ExecContext(ctx, "INSERT INTO volume_bookmark (volume_id, file_id) VALUES ($1, $2)", volume, second)
	if !errors.Is(err, sqlate.ErrUniqueViolation) {
		t.Fatalf("second active bookmark = %v, want ErrUniqueViolation", err)
	}
	if name := livetest.Constraint(t, err, sqlate.ErrUniqueViolation); name != "uq_volume_bookmark_active" {
		t.Errorf("violated constraint = %q, want uq_volume_bookmark_active", name)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO volume_bookmark (volume_id, file_id, active) VALUES ($1, $2, false)", volume, second); err != nil {
		t.Fatalf("second inactive bookmark: %v", err)
	}
}

// TestSchemaCommands runs the command's own entry point for up and down
// against the throwaway database, so the argument parsing and the DSN
// wiring are exercised too.
func TestSchemaCommands(t *testing.T) {
	ctx := context.Background()
	db, dsn := livetest.OpenDSN(t)
	t.Setenv("BLOBFS_DSN", dsn)
	if err := run([]string{"schema", "up"}); err != nil {
		t.Fatalf("schema up: %v", err)
	}
	if !livetest.Exists(ctx, t, db, "volume_bookmark") {
		t.Error("after schema up, table volume_bookmark is missing")
	}
	if err := run([]string{"schema", "down"}); err != nil {
		t.Fatalf("schema down: %v", err)
	}
	if livetest.Exists(ctx, t, db, "blobfs_directory") {
		t.Error("after schema down, table blobfs_directory still exists")
	}
	if err := run([]string{"schema"}); err == nil {
		t.Error("schema without a subcommand succeeded")
	}
}

// TestVolumeOwnerReferencesVolume proves the consumer's ownership row is
// bound to a real volume: a volume_id that names no volume is refused under
// the foreign key's name, and the same row is accepted once the volume
// exists.
func TestVolumeOwnerReferencesVolume(t *testing.T) {
	ctx := context.Background()
	db := livetest.Open(t)
	schema, err := newSchema(db, nil)
	if err != nil {
		t.Fatalf("newSchema: %v", err)
	}
	if err := schema.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	volume, unit := blobfs.NewID(), blobfs.NewID()
	_, err = db.ExecContext(ctx, "INSERT INTO volume_owner (volume_id, unit_id) VALUES ($1, $2)", volume, unit)
	if !errors.Is(err, sqlate.ErrForeignKeyViolation) {
		t.Fatalf("owner of a missing volume = %v, want ErrForeignKeyViolation", err)
	}
	if name := livetest.Constraint(t, err, sqlate.ErrForeignKeyViolation); name != "fk_volume_owner_volume" {
		t.Errorf("violated constraint = %q, want fk_volume_owner_volume", name)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO blobfs_volume (id, name) VALUES ($1, 'vol')", volume); err != nil {
		t.Fatalf("insert volume: %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO volume_owner (volume_id, unit_id) VALUES ($1, $2)", volume, unit); err != nil {
		t.Fatalf("owner of an existing volume: %v", err)
	}
}

// seedBookmark inserts a volume and its root directory in one transaction,
// then one available file in the root and an active bookmark on the file;
// it returns the volume and root directory ids.
func seedBookmark(ctx context.Context, t *testing.T, db *sqlate.DB) (volume, dir string) {
	t.Helper()
	volume, dir = blobfs.NewID(), blobfs.NewID()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "INSERT INTO blobfs_volume (id, name) VALUES ($1, 'vol')", volume); err != nil {
		t.Fatalf("insert volume: %v", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO blobfs_directory (id, volume_id) VALUES ($1, $2)", dir, volume); err != nil {
		t.Fatalf("insert root: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	file := insertFile(ctx, t, db, dir, "first.txt")
	if _, err := db.ExecContext(ctx, "INSERT INTO volume_bookmark (volume_id, file_id) VALUES ($1, $2)", volume, file); err != nil {
		t.Fatalf("insert bookmark: %v", err)
	}
	return volume, dir
}

// insertFile inserts an available file named name in directory dir and
// returns the file's id.
func insertFile(ctx context.Context, t *testing.T, db *sqlate.DB, dir, name string) string {
	t.Helper()
	id := blobfs.NewID()
	key := id + "/" + blobfs.SanitizeFilename(name)
	if _, err := db.ExecContext(ctx,
		"INSERT INTO blobfs_file (id, directory_id, name, status, key, content_type) VALUES ($1, $2, $3, 'available', $4, 'text/plain')",
		id, dir, name, key); err != nil {
		t.Fatalf("insert file: %v", err)
	}
	return id
}
