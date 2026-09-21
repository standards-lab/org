//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/standards-lab/sqlate"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/postgres"
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
	blobfsSet, err := postgres.Migrations()
	if err != nil {
		t.Fatalf("blobfs Migrations: %v", err)
	}
	consumerSet, err := appmigrations.Migrations()
	if err != nil {
		t.Fatalf("consumer Migrations: %v", err)
	}
	m, err := migrator.New(db, []migrator.Set{
		blobfsSet,
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

// TestBookmarkOneActivePerUnit proves the partial unique index: a second
// active bookmark for one unit is refused under the index's name, while a
// second inactive one is accepted, and the same file may be another unit's
// active bookmark.
func TestBookmarkOneActivePerUnit(t *testing.T) {
	ctx, db := applied(t)
	unit := blobfs.NewID()
	first := insertFile(ctx, t, db, blobfs.RootID, "first.txt")
	second := insertFile(ctx, t, db, blobfs.RootID, "second.txt")
	if _, err := db.ExecContext(ctx, "INSERT INTO bookmark (unit_id, file_id) VALUES ($1, $2)", unit, first); err != nil {
		t.Fatalf("first bookmark: %v", err)
	}
	_, err := db.ExecContext(ctx, "INSERT INTO bookmark (unit_id, file_id) VALUES ($1, $2)", unit, second)
	if !errors.Is(err, sqlate.ErrUniqueViolation) {
		t.Fatalf("second active bookmark = %v, want ErrUniqueViolation", err)
	}
	if name := livetest.Constraint(t, err, sqlate.ErrUniqueViolation); name != "uq_bookmark_active" {
		t.Errorf("violated constraint = %q, want uq_bookmark_active", name)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO bookmark (unit_id, file_id, active) VALUES ($1, $2, false)", unit, second); err != nil {
		t.Fatalf("second inactive bookmark: %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO bookmark (unit_id, file_id) VALUES ($1, $2)", blobfs.NewID(), first); err != nil {
		t.Fatalf("another unit's active bookmark on the same file: %v", err)
	}
}

// TestBookmarkReferencesFile proves a bookmark is bound to a real file: a
// file_id that names no file is refused under the foreign key's name.
func TestBookmarkReferencesFile(t *testing.T) {
	ctx, db := applied(t)
	_, err := db.ExecContext(ctx, "INSERT INTO bookmark (unit_id, file_id) VALUES ($1, $2)", blobfs.NewID(), blobfs.NewID())
	if !errors.Is(err, sqlate.ErrForeignKeyViolation) {
		t.Fatalf("bookmark of a missing file = %v, want ErrForeignKeyViolation", err)
	}
	if name := livetest.Constraint(t, err, sqlate.ErrForeignKeyViolation); name != "fk_bookmark_file" {
		t.Errorf("violated constraint = %q, want fk_bookmark_file", name)
	}
}

// TestDirectoryOwnerReferencesDirectory proves the consumer's ownership
// row is bound to a real directory: a directory_id that names no directory
// is refused under the foreign key's name, the same row is accepted once
// the directory exists, and a directory has at most one owner.
func TestDirectoryOwnerReferencesDirectory(t *testing.T) {
	ctx, db := applied(t)
	dir, unit := blobfs.NewID(), blobfs.NewID()
	_, err := db.ExecContext(ctx, "INSERT INTO directory_owner (directory_id, unit_id) VALUES ($1, $2)", dir, unit)
	if !errors.Is(err, sqlate.ErrForeignKeyViolation) {
		t.Fatalf("owner of a missing directory = %v, want ErrForeignKeyViolation", err)
	}
	if name := livetest.Constraint(t, err, sqlate.ErrForeignKeyViolation); name != "fk_directory_owner_directory" {
		t.Errorf("violated constraint = %q, want fk_directory_owner_directory", name)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO blobfs_directory (id, parent_id, name) VALUES ($1, $2, 'docs')", dir, blobfs.RootID); err != nil {
		t.Fatalf("insert directory: %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO directory_owner (directory_id, unit_id) VALUES ($1, $2)", dir, unit); err != nil {
		t.Fatalf("owner of an existing directory: %v", err)
	}
	_, err = db.ExecContext(ctx, "INSERT INTO directory_owner (directory_id, unit_id) VALUES ($1, $2)", dir, blobfs.NewID())
	if name := livetest.Constraint(t, err, sqlate.ErrUniqueViolation); name != "pk_directory_owner" {
		t.Errorf("second owner: violated constraint = %q, want pk_directory_owner", name)
	}
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
