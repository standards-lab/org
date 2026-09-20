//go:build compose

package migrations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/migrate"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/migrations"
)

// applied applies blobfs's set alone, under its own history table, to a
// throwaway database and returns the session.
func applied(t *testing.T) (context.Context, *sqlate.DB) {
	t.Helper()
	ctx := context.Background()
	db := livetest.Open(t)
	set, err := migrations.Migrations(db.Dialect())
	if err != nil {
		t.Fatalf("Migrations: %v", err)
	}
	m, err := migrate.New(db, set, migrate.Options{Table: migrations.Table})
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	if err := m.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	return ctx, db
}

func ptr(s string) *string { return &s }

func insertVolume(ctx context.Context, t *testing.T, db *sqlate.DB, name string) (string, error) {
	t.Helper()
	id := blobfs.NewID()
	_, err := db.ExecContext(ctx, "INSERT INTO blobfs_volume (id, name) VALUES ($1, $2)", id, name)
	return id, err
}

// insertDirectory binds all three nullable columns, so a test can build any
// combination the root rule allows or refuses.
func insertDirectory(ctx context.Context, t *testing.T, db *sqlate.DB, parent, volume, name *string) (string, error) {
	t.Helper()
	id := blobfs.NewID()
	_, err := db.ExecContext(ctx, "INSERT INTO blobfs_directory (id, parent_id, volume_id, name) VALUES ($1, $2, $3, $4)", id, parent, volume, name)
	return id, err
}

// newTree inserts a volume named name and its root, and returns both ids.
func newTree(ctx context.Context, t *testing.T, db *sqlate.DB, name string) (volume, root string) {
	t.Helper()
	volume, err := insertVolume(ctx, t, db, name)
	if err != nil {
		t.Fatalf("insert volume %s: %v", name, err)
	}
	root, err = insertDirectory(ctx, t, db, nil, &volume, nil)
	if err != nil {
		t.Fatalf("insert root of %s: %v", name, err)
	}
	return volume, root
}

func insertFile(ctx context.Context, t *testing.T, db *sqlate.DB, dir, name, status string) (string, error) {
	t.Helper()
	id := blobfs.NewID()
	_, err := db.ExecContext(ctx,
		"INSERT INTO blobfs_file (id, directory_id, name, status, key, content_type) VALUES ($1, $2, $3, $4, $5, 'text/plain')",
		id, dir, name, status, id+"/"+blobfs.SanitizeFilename(name))
	return id, err
}

// TestRootRule proves the two check constraints that state the root rule.
// Each case violates exactly one of them, so the reported constraint name
// is unambiguous: a root with no volume and a non-root with a volume fail
// under blobfs_cc_directory_root_volume; a root with a name and a non-root
// without one fail under blobfs_cc_directory_root_name.
func TestRootRule(t *testing.T) {
	ctx, db := applied(t)
	volume, root := newTree(ctx, t, db, "vol")

	cases := []struct {
		label                string
		parent, volume, name *string
		want                 string
	}{
		{"root with no volume", nil, nil, nil, "blobfs_cc_directory_root_volume"},
		{"non-root with a volume", &root, &volume, ptr("docs"), "blobfs_cc_directory_root_volume"},
		{"root with a name", nil, &volume, ptr("root"), "blobfs_cc_directory_root_name"},
		{"non-root with no name", &root, nil, nil, "blobfs_cc_directory_root_name"},
	}
	for _, c := range cases {
		_, err := insertDirectory(ctx, t, db, c.parent, c.volume, c.name)
		if !errors.Is(err, sqlate.ErrCheckViolation) {
			t.Errorf("%s = %v, want ErrCheckViolation", c.label, err)
			continue
		}
		if name := livetest.Constraint(t, err, sqlate.ErrCheckViolation); name != c.want {
			t.Errorf("%s: violated constraint = %q, want %s", c.label, name, c.want)
		}
	}
}

// TestOneRootPerVolume proves the unique constraint on volume_id: a second
// root in the same volume is refused under its name, while roots of two
// volumes coexist.
func TestOneRootPerVolume(t *testing.T) {
	ctx, db := applied(t)
	volume, _ := newTree(ctx, t, db, "vol")
	_, err := insertDirectory(ctx, t, db, nil, &volume, nil)
	if name := livetest.Constraint(t, err, sqlate.ErrUniqueViolation); name != "blobfs_uq_directory_volume" {
		t.Errorf("second root: violated constraint = %q, want blobfs_uq_directory_volume", name)
	}
	newTree(ctx, t, db, "other")
}

// TestVolumeConstraints proves the volume table's constraints: a second
// volume of the same name is refused under the unique constraint's name and
// an empty name under the check's.
func TestVolumeConstraints(t *testing.T) {
	ctx, db := applied(t)
	if _, err := insertVolume(ctx, t, db, "vol"); err != nil {
		t.Fatalf("insert volume: %v", err)
	}
	_, err := insertVolume(ctx, t, db, "vol")
	if name := livetest.Constraint(t, err, sqlate.ErrUniqueViolation); name != "blobfs_uq_volume_name" {
		t.Errorf("duplicate volume name: violated constraint = %q, want blobfs_uq_volume_name", name)
	}
	_, err = insertVolume(ctx, t, db, "")
	if name := livetest.Constraint(t, err, sqlate.ErrCheckViolation); name != "blobfs_cc_volume_name" {
		t.Errorf("empty volume name: violated constraint = %q, want blobfs_cc_volume_name", name)
	}
}

// TestDirectoryUniqueness proves the unique constraint on (parent_id,
// name) is per parent: a duplicate under one root is refused under the
// constraint's name, while two volumes each hold a child of the same name.
func TestDirectoryUniqueness(t *testing.T) {
	ctx, db := applied(t)
	_, rootA := newTree(ctx, t, db, "a")
	_, rootB := newTree(ctx, t, db, "b")
	if _, err := insertDirectory(ctx, t, db, &rootA, nil, ptr("docs")); err != nil {
		t.Fatalf("insert docs under a: %v", err)
	}
	_, err := insertDirectory(ctx, t, db, &rootA, nil, ptr("docs"))
	if !errors.Is(err, sqlate.ErrUniqueViolation) {
		t.Fatalf("duplicate child = %v, want ErrUniqueViolation", err)
	}
	if name := livetest.Constraint(t, err, sqlate.ErrUniqueViolation); name != "blobfs_uq_directory_parent_name" {
		t.Errorf("violated constraint = %q, want blobfs_uq_directory_parent_name", name)
	}
	if _, err := insertDirectory(ctx, t, db, &rootB, nil, ptr("docs")); err != nil {
		t.Errorf("docs under b = %v, want accepted (uniqueness is per parent)", err)
	}
}

// TestDirectoryChecks proves the remaining check constraints on the
// directory table: an empty name and a directory that is its own parent are
// refused under their constraint names.
func TestDirectoryChecks(t *testing.T) {
	ctx, db := applied(t)
	_, root := newTree(ctx, t, db, "vol")
	_, err := insertDirectory(ctx, t, db, &root, nil, ptr(""))
	if name := livetest.Constraint(t, err, sqlate.ErrCheckViolation); name != "blobfs_cc_directory_name" {
		t.Errorf("empty name: violated constraint = %q, want blobfs_cc_directory_name", name)
	}
	id := blobfs.NewID()
	_, err = db.ExecContext(ctx, "INSERT INTO blobfs_directory (id, parent_id, name) VALUES ($1, $1, 'self')", id)
	if name := livetest.Constraint(t, err, sqlate.ErrCheckViolation); name != "blobfs_cc_directory_parent_not_self" {
		t.Errorf("self parent: violated constraint = %q, want blobfs_cc_directory_parent_not_self", name)
	}
}

// TestDeleteWhileReferenced proves the refusals that are foreign keys: a
// directory with a child directory fails under blobfs_fk_directory_parent,
// one with a file under blobfs_fk_file_directory, and a volume whose root
// still exists under blobfs_fk_directory_volume. With the root gone the
// volume deletes.
func TestDeleteWhileReferenced(t *testing.T) {
	ctx, db := applied(t)
	volume, root := newTree(ctx, t, db, "vol")
	child, err := insertDirectory(ctx, t, db, &root, nil, ptr("child"))
	if err != nil {
		t.Fatalf("insert child: %v", err)
	}
	_, err = db.ExecContext(ctx, "DELETE FROM blobfs_directory WHERE id = $1", root)
	if name := livetest.Constraint(t, err, sqlate.ErrForeignKeyViolation); name != "blobfs_fk_directory_parent" {
		t.Errorf("delete parent of a directory: violated constraint = %q, want blobfs_fk_directory_parent", name)
	}
	if _, err := insertFile(ctx, t, db, child, "a.txt", "available"); err != nil {
		t.Fatalf("insert file: %v", err)
	}
	_, err = db.ExecContext(ctx, "DELETE FROM blobfs_directory WHERE id = $1", child)
	if name := livetest.Constraint(t, err, sqlate.ErrForeignKeyViolation); name != "blobfs_fk_file_directory" {
		t.Errorf("delete directory of a file: violated constraint = %q, want blobfs_fk_file_directory", name)
	}
	_, err = db.ExecContext(ctx, "DELETE FROM blobfs_volume WHERE id = $1", volume)
	if name := livetest.Constraint(t, err, sqlate.ErrForeignKeyViolation); name != "blobfs_fk_directory_volume" {
		t.Errorf("delete volume with a root: violated constraint = %q, want blobfs_fk_directory_volume", name)
	}
	for _, step := range []struct{ query, id string }{
		{"DELETE FROM blobfs_file WHERE directory_id = $1", child},
		{"DELETE FROM blobfs_directory WHERE id = $1", child},
		{"DELETE FROM blobfs_directory WHERE id = $1", root},
		{"DELETE FROM blobfs_volume WHERE id = $1", volume},
	} {
		if _, err := db.ExecContext(ctx, step.query, step.id); err != nil {
			t.Fatalf("%s: %v", step.query, err)
		}
	}
}

// TestFileConstraints proves the file table's constraints: the status check
// rejects a value outside the vocabulary, a duplicate (directory_id, name)
// is refused under the unique constraint's name, and a file may share its
// name with a directory.
func TestFileConstraints(t *testing.T) {
	ctx, db := applied(t)
	_, root := newTree(ctx, t, db, "vol")
	_, err := insertFile(ctx, t, db, root, "a.txt", "bogus")
	if name := livetest.Constraint(t, err, sqlate.ErrCheckViolation); name != "blobfs_cc_file_status" {
		t.Errorf("bogus status: violated constraint = %q, want blobfs_cc_file_status", name)
	}
	for _, status := range []blobfs.Status{blobfs.StatusPending, blobfs.StatusAvailable, blobfs.StatusDeleting} {
		if _, err := insertFile(ctx, t, db, root, string(status)+".txt", status.String()); err != nil {
			t.Errorf("status %s: %v", status, err)
		}
	}
	_, err = insertFile(ctx, t, db, root, "pending.txt", "available")
	if name := livetest.Constraint(t, err, sqlate.ErrUniqueViolation); name != "blobfs_uq_file_directory_name" {
		t.Errorf("duplicate file: violated constraint = %q, want blobfs_uq_file_directory_name", name)
	}
	if _, err := insertDirectory(ctx, t, db, &root, nil, ptr("pending.txt")); err != nil {
		t.Errorf("directory sharing a file's name = %v, want accepted", err)
	}
}
