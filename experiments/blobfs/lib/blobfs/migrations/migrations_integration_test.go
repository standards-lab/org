//go:build integration

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

// insertDirectory binds both nullable columns, so a test can build any
// combination the root rule allows or refuses.
func insertDirectory(ctx context.Context, t *testing.T, db *sqlate.DB, parent, name *string) (string, error) {
	t.Helper()
	id := blobfs.NewID()
	_, err := db.ExecContext(ctx, "INSERT INTO blobfs_directory (id, parent_id, name) VALUES ($1, $2, $3)", id, parent, name)
	return id, err
}

// mkdir inserts a named directory under parent and fails the test on any
// error.
func mkdir(ctx context.Context, t *testing.T, db *sqlate.DB, parent, name string) string {
	t.Helper()
	id, err := insertDirectory(ctx, t, db, &parent, &name)
	if err != nil {
		t.Fatalf("insert %s under %s: %v", name, parent, err)
	}
	return id
}

func insertFile(ctx context.Context, t *testing.T, db *sqlate.DB, dir, name, status string) (string, error) {
	t.Helper()
	id := blobfs.NewID()
	_, err := db.ExecContext(ctx,
		"INSERT INTO blobfs_file (id, directory_id, name, status, key, content_type) VALUES ($1, $2, $3, $4, $5, 'text/plain')",
		id, dir, name, status, id+"/"+blobfs.SanitizeFilename(name))
	return id, err
}

// TestSeededRoot proves the migration seeds exactly one directory: the
// root, with the well-known id, no parent, and the name /.
func TestSeededRoot(t *testing.T) {
	ctx, db := applied(t)
	rows, err := db.QueryContext(ctx, "SELECT CAST(id AS text), parent_id, name FROM blobfs_directory")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	defer func() { _ = rows.Close() }()
	n := 0
	for rows.Next() {
		var id, name string
		var parent *string
		if err := rows.Scan(&id, &parent, &name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		n++
		if id != blobfs.RootID || parent != nil || name != "/" {
			t.Errorf("seeded row = (%s, %v, %q), want (%s, NULL, /)", id, parent, name, blobfs.RootID)
		}
	}
	if n != 1 {
		t.Errorf("the migration seeded %d directories, want 1 (the root)", n)
	}
}

// TestOneRoot is the stage gate's first proof: a second root, a row with
// no parent named /, is refused as a unique violation under the partial
// index's name, blobfs_uq_directory_root, whatever its id.
func TestOneRoot(t *testing.T) {
	ctx, db := applied(t)
	_, err := insertDirectory(ctx, t, db, nil, ptr("/"))
	if !errors.Is(err, sqlate.ErrUniqueViolation) {
		t.Fatalf("second root = %v, want ErrUniqueViolation", err)
	}
	if name := livetest.Constraint(t, err, sqlate.ErrUniqueViolation); name != blobfs.ConstraintUniqueDirectoryRoot {
		t.Errorf("second root: violated constraint = %q, want %s", name, blobfs.ConstraintUniqueDirectoryRoot)
	}
	// The refusal is by the index and not by the primary key: a second root
	// under the root's own id is refused under the primary key, which shows
	// the two are distinct guards.
	_, err = db.ExecContext(ctx, "INSERT INTO blobfs_directory (id, name) VALUES ($1, '/')", blobfs.RootID)
	if name := livetest.Constraint(t, err, sqlate.ErrUniqueViolation); name != "blobfs_pk_directory" {
		t.Errorf("root reinserted under its id: violated constraint = %q, want blobfs_pk_directory", name)
	}
}

// TestRootRule proves the check constraint that states the root rule: a
// root under a name other than / and a non-root named / each fail under
// blobfs_cc_directory_root_name. A non-root with no name fails the
// column's NOT NULL constraint, since every directory has a name.
func TestRootRule(t *testing.T) {
	ctx, db := applied(t)
	cases := []struct {
		label        string
		parent, name *string
		want         string
	}{
		{"root under another name", nil, ptr("root"), "blobfs_cc_directory_root_name"},
		{"non-root named /", ptr(blobfs.RootID), ptr("/"), "blobfs_cc_directory_root_name"},
	}
	for _, c := range cases {
		_, err := insertDirectory(ctx, t, db, c.parent, c.name)
		if !errors.Is(err, sqlate.ErrCheckViolation) {
			t.Errorf("%s = %v, want ErrCheckViolation", c.label, err)
			continue
		}
		if name := livetest.Constraint(t, err, sqlate.ErrCheckViolation); name != c.want {
			t.Errorf("%s: violated constraint = %q, want %s", c.label, name, c.want)
		}
	}
	if _, err := insertDirectory(ctx, t, db, ptr(blobfs.RootID), nil); !errors.Is(err, sqlate.ErrNotNullViolation) {
		t.Errorf("non-root with no name = %v, want ErrNotNullViolation", err)
	}
}

// TestDirectoryUniqueness proves the unique constraint on (parent_id,
// name) is per parent: a duplicate under the root is refused under the
// constraint's name, while a child of the same name under another
// directory is accepted.
func TestDirectoryUniqueness(t *testing.T) {
	ctx, db := applied(t)
	docs := mkdir(ctx, t, db, blobfs.RootID, "docs")
	_, err := insertDirectory(ctx, t, db, ptr(blobfs.RootID), ptr("docs"))
	if !errors.Is(err, sqlate.ErrUniqueViolation) {
		t.Fatalf("duplicate child = %v, want ErrUniqueViolation", err)
	}
	if name := livetest.Constraint(t, err, sqlate.ErrUniqueViolation); name != blobfs.ConstraintUniqueDirectoryParentName {
		t.Errorf("violated constraint = %q, want %s", name, blobfs.ConstraintUniqueDirectoryParentName)
	}
	mkdir(ctx, t, db, docs, "docs")
}

// TestDirectoryChecks proves the remaining check constraints on the
// directory table: an empty name and a directory that is its own parent are
// refused under their constraint names.
func TestDirectoryChecks(t *testing.T) {
	ctx, db := applied(t)
	_, err := insertDirectory(ctx, t, db, ptr(blobfs.RootID), ptr(""))
	if name := livetest.Constraint(t, err, sqlate.ErrCheckViolation); name != "blobfs_cc_directory_name" {
		t.Errorf("empty name: violated constraint = %q, want blobfs_cc_directory_name", name)
	}
	id := blobfs.NewID()
	_, err = db.ExecContext(ctx, "INSERT INTO blobfs_directory (id, parent_id, name) VALUES ($1, $1, 'self')", id)
	if name := livetest.Constraint(t, err, sqlate.ErrCheckViolation); name != "blobfs_cc_directory_parent_not_self" {
		t.Errorf("self parent: violated constraint = %q, want blobfs_cc_directory_parent_not_self", name)
	}
}

// TestDeleteWhileReferenced proves the refusals that are foreign keys: the
// root with a child directory fails under blobfs_fk_directory_parent, and
// a directory with a file under blobfs_fk_file_directory. With the file
// and the child gone, both delete; the schema alone does not protect the
// root, which the persistence layer's delete refuses in a later stage.
func TestDeleteWhileReferenced(t *testing.T) {
	ctx, db := applied(t)
	child := mkdir(ctx, t, db, blobfs.RootID, "child")
	_, err := db.ExecContext(ctx, "DELETE FROM blobfs_directory WHERE id = $1", blobfs.RootID)
	if name := livetest.Constraint(t, err, sqlate.ErrForeignKeyViolation); name != blobfs.ConstraintForeignKeyDirectoryParent {
		t.Errorf("delete parent of a directory: violated constraint = %q, want %s", name, blobfs.ConstraintForeignKeyDirectoryParent)
	}
	if _, err := insertFile(ctx, t, db, child, "a.txt", "available"); err != nil {
		t.Fatalf("insert file: %v", err)
	}
	_, err = db.ExecContext(ctx, "DELETE FROM blobfs_directory WHERE id = $1", child)
	if name := livetest.Constraint(t, err, sqlate.ErrForeignKeyViolation); name != blobfs.ConstraintForeignKeyFileDirectory {
		t.Errorf("delete directory of a file: violated constraint = %q, want %s", name, blobfs.ConstraintForeignKeyFileDirectory)
	}
	for _, step := range []struct{ query, id string }{
		{"DELETE FROM blobfs_file WHERE directory_id = $1", child},
		{"DELETE FROM blobfs_directory WHERE id = $1", child},
		{"DELETE FROM blobfs_directory WHERE id = $1", blobfs.RootID},
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
	_, err := insertFile(ctx, t, db, blobfs.RootID, "a.txt", "bogus")
	if name := livetest.Constraint(t, err, sqlate.ErrCheckViolation); name != "blobfs_cc_file_status" {
		t.Errorf("bogus status: violated constraint = %q, want blobfs_cc_file_status", name)
	}
	for _, status := range []blobfs.Status{blobfs.StatusPending, blobfs.StatusAvailable, blobfs.StatusDeleting} {
		if _, err := insertFile(ctx, t, db, blobfs.RootID, string(status)+".txt", status.String()); err != nil {
			t.Errorf("status %s: %v", status, err)
		}
	}
	_, err = insertFile(ctx, t, db, blobfs.RootID, "pending.txt", "available")
	if name := livetest.Constraint(t, err, sqlate.ErrUniqueViolation); name != blobfs.ConstraintUniqueFileDirectoryName {
		t.Errorf("duplicate file: violated constraint = %q, want %s", name, blobfs.ConstraintUniqueFileDirectoryName)
	}
	mkdir(ctx, t, db, blobfs.RootID, "pending.txt")
}
