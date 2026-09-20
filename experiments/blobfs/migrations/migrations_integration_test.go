//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/standards-lab/sqlate"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	blobfsmigrations "github.com/standards-lab/org/experiments/blobfs/lib/blobfs/migrations"
	"github.com/standards-lab/org/experiments/blobfs/lib/migrator"
	appmigrations "github.com/standards-lab/org/experiments/blobfs/migrations"
)

// applied applies blobfs's set and then the consumer's, each under its own
// history table, to a throwaway database through the migrator directly, and
// returns the session.
func applied(t *testing.T) (context.Context, *sqlate.DB) {
	t.Helper()
	ctx := context.Background()
	db := livetest.Open(t)
	blobfsSet, err := blobfsmigrations.Migrations(db.Dialect())
	if err != nil {
		t.Fatalf("blobfs Migrations: %v", err)
	}
	consumerSet, err := appmigrations.Migrations()
	if err != nil {
		t.Fatalf("consumer Migrations: %v", err)
	}
	m, err := migrator.New(db, []migrator.Set{
		{Name: blobfsmigrations.Source, Table: blobfsmigrations.Table, Migrations: blobfsSet},
		{Name: "consumer", Migrations: consumerSet},
	}, migrator.Options{})
	if err != nil {
		t.Fatalf("migrator.New: %v", err)
	}
	if err := m.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	return ctx, db
}

// TestBookmarkOneActivePerVolume proves the partial unique index: a second
// active bookmark for one volume is refused under the index's name, while a
// second inactive one is accepted.
func TestBookmarkOneActivePerVolume(t *testing.T) {
	ctx, db := applied(t)
	volume, dir := seedBookmark(ctx, t, db)

	second := insertFile(ctx, t, db, dir, "second.txt")
	_, err := db.ExecContext(ctx, "INSERT INTO volume_bookmark (volume_id, file_id) VALUES ($1, $2)", volume, second)
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

// TestVolumeOwnerReferencesVolume proves the consumer's ownership row is
// bound to a real volume: a volume_id that names no volume is refused under
// the foreign key's name, and the same row is accepted once the volume
// exists.
func TestVolumeOwnerReferencesVolume(t *testing.T) {
	ctx, db := applied(t)
	volume, unit := blobfs.NewID(), blobfs.NewID()
	_, err := db.ExecContext(ctx, "INSERT INTO volume_owner (volume_id, unit_id) VALUES ($1, $2)", volume, unit)
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
