//go:build integration

package data_test

import (
	"errors"
	"testing"

	"github.com/standards-lab/sqlate"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// TestRemoveDirectoryOnTheEngine proves the directory removal against
// the schema, which is the same on every variant: the root is refused
// before any SQL and the seeded row stays; a directory with a child
// directory, with a file, or with a file whose delete has begun is
// ErrNotEmpty under the foreign key that names what remains; a
// directory a consumer's table references is ErrReferenced under the
// consumer's constraint; a missing directory is ErrNotFound; and an empty
// directory is removed, after which its name is free again.
func TestRemoveDirectoryOnTheEngine(t *testing.T) {
	e := open(t)
	if err := e.store.RemoveDirectory(e.ctx, e.db, blobfs.RootID); !errors.Is(err, blobfs.ErrRootDirectory) {
		t.Errorf("RemoveDirectory(root) = %v, want ErrRootDirectory", err)
	}
	if _, err := e.store.Root(e.ctx, e.db); err != nil {
		t.Fatalf("the root after the refusal: %v", err)
	}

	parent := e.mkdir(t, blobfs.RootID, "parent")
	child := e.mkdir(t, parent.ID, "child")
	err := e.store.RemoveDirectory(e.ctx, e.db, parent.ID)
	if !errors.Is(err, blobfs.ErrNotEmpty) {
		t.Errorf("RemoveDirectory of a directory with a child = %v, want ErrNotEmpty", err)
	}
	if name := livetest.Constraint(t, err, sqlate.ErrForeignKeyViolation); name != blobfs.ConstraintForeignKeyDirectoryParent {
		t.Errorf("the refusal names %q, want blobfs_fk_directory_parent", name)
	}
	if err := e.store.RemoveDirectory(e.ctx, e.db, child.ID); err != nil {
		t.Fatalf("RemoveDirectory of the empty child: %v", err)
	}

	id := insertFile(e.ctx, t, e.db, parent.ID, "a.txt")
	err = e.store.RemoveDirectory(e.ctx, e.db, parent.ID)
	if !errors.Is(err, blobfs.ErrNotEmpty) {
		t.Errorf("RemoveDirectory of a directory with a file = %v, want ErrNotEmpty", err)
	}
	if name := livetest.Constraint(t, err, sqlate.ErrForeignKeyViolation); name != blobfs.ConstraintForeignKeyFileDirectory {
		t.Errorf("the refusal names %q, want blobfs_fk_file_directory", name)
	}
	if _, err := e.db.Transact(e.ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
		return e.store.BeginFileDelete(e.ctx, tx, id)
	}); err != nil {
		t.Fatalf("BeginFileDelete: %v", err)
	}
	if err := e.store.RemoveDirectory(e.ctx, e.db, parent.ID); !errors.Is(err, blobfs.ErrNotEmpty) {
		t.Errorf("RemoveDirectory while a file is deleting = %v, want ErrNotEmpty: a deleting row keeps its slot", err)
	}
	if err := e.store.CompleteFileDelete(e.ctx, e.db, id); err != nil {
		t.Fatalf("CompleteFileDelete: %v", err)
	}

	if _, err := e.db.ExecContext(e.ctx, "CREATE TABLE consumer_owner (directory_id uuid NOT NULL, CONSTRAINT consumer_fk_owner_directory FOREIGN KEY (directory_id) REFERENCES blobfs_directory (id))"); err != nil {
		t.Fatalf("create the consumer's table: %v", err)
	}
	if _, err := e.db.ExecContext(e.ctx, "INSERT INTO consumer_owner (directory_id) VALUES ($1)", parent.ID); err != nil {
		t.Fatalf("insert the consumer's row: %v", err)
	}
	err = e.store.RemoveDirectory(e.ctx, e.db, parent.ID)
	if !errors.Is(err, blobfs.ErrReferenced) || errors.Is(err, blobfs.ErrNotEmpty) {
		t.Errorf("RemoveDirectory of a directory a consumer references = %v, want ErrReferenced and not ErrNotEmpty", err)
	}
	if name := livetest.Constraint(t, err, sqlate.ErrForeignKeyViolation); name != "consumer_fk_owner_directory" {
		t.Errorf("the refusal names %q, want the consumer's constraint", name)
	}
	if _, err := e.db.ExecContext(e.ctx, "DELETE FROM consumer_owner"); err != nil {
		t.Fatal(err)
	}

	if err := e.store.RemoveDirectory(e.ctx, e.db, parent.ID); err != nil {
		t.Fatalf("RemoveDirectory of the emptied directory: %v", err)
	}
	if _, err := e.store.Directory(e.ctx, e.db, parent.ID); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("after the removal Directory = %v, want ErrNotFound", err)
	}
	if err := e.store.RemoveDirectory(e.ctx, e.db, parent.ID); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("RemoveDirectory of the removed directory = %v, want ErrNotFound", err)
	}
	if _, err := e.store.Mkdir(e.ctx, e.db, blobfs.RootID, "parent"); err != nil {
		t.Errorf("Mkdir under the freed name: %v", err)
	}
}
