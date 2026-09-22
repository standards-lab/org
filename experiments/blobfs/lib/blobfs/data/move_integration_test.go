//go:build integration

package data_test

import (
	"errors"
	"testing"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// TestMoveFileOnTheEngine proves the file move against the schema, which
// is the same on every variant: a move into another directory and a
// rename each advance the version and leave the key as it was; a pending
// row moves; a decomposed name is stored composed; a name held in the
// target by a row of any status is ErrNameTaken under the unique
// constraint; a missing directory is ErrNotFound under the foreign key;
// a stale version is ErrVersionMismatch; a deleting row is ErrDeleting
// and unchanged; a missing file is ErrNotFound; and a directory of the
// same name in the target is no conflict.
func TestMoveFileOnTheEngine(t *testing.T) {
	e := open(t)
	src := e.mkdir(t, blobfs.RootID, "src")
	dst := e.mkdir(t, blobfs.RootID, "dst")
	id := insertFile(e.ctx, t, e.db, src.ID, "a.txt")
	before, err := e.store.File(e.ctx, e.db, id)
	if err != nil {
		t.Fatal(err)
	}

	moved, err := e.store.MoveFile(e.ctx, e.db, id, dst.ID, "a.txt", before.Version)
	if err != nil {
		t.Fatalf("MoveFile into dst: %v", err)
	}
	if moved.DirectoryID != dst.ID || moved.Name != "a.txt" || moved.Version != before.Version+1 || moved.Key != before.Key || !moved.UpdatedAt.After(before.UpdatedAt) {
		t.Errorf("the moved row is %+v, want it in dst at the next version with the key %q", moved, before.Key)
	}
	if _, err := e.store.FileByName(e.ctx, e.db, src.ID, "a.txt"); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("the file is still found in src: %v", err)
	}
	renamed, err := e.store.MoveFile(e.ctx, e.db, id, dst.ID, decomposed, moved.Version)
	if err != nil {
		t.Fatalf("MoveFile as a rename: %v", err)
	}
	if renamed.Name != composed || renamed.Key != before.Key || renamed.Version != moved.Version+1 {
		t.Errorf("the renamed row is %+v, want the composed name and the key unchanged", renamed)
	}
	if f, err := e.store.FileByName(e.ctx, e.db, dst.ID, decomposed); err != nil || f.ID != id {
		t.Errorf("FileByName under the new name = %+v, %v", f, err)
	}

	// A pending row moves; its key was fixed at the insert.
	pending := insertFile(e.ctx, t, e.db, src.ID, "pending.txt")
	if _, err := e.db.ExecContext(e.ctx, "UPDATE blobfs_file SET status = 'pending', size = NULL WHERE id = $1", pending); err != nil {
		t.Fatal(err)
	}
	if f, err := e.store.MoveFile(e.ctx, e.db, pending, dst.ID, "pending.txt", 1); err != nil || f.Status != blobfs.StatusPending || f.DirectoryID != dst.ID {
		t.Errorf("MoveFile of a pending row = %+v, %v; want it moved and still pending", f, err)
	}

	// Refusals.
	held := insertFile(e.ctx, t, e.db, src.ID, "held.txt")
	other := insertFile(e.ctx, t, e.db, dst.ID, "held.txt")
	_, err = e.store.MoveFile(e.ctx, e.db, held, dst.ID, "held.txt", 1)
	if !errors.Is(err, blobfs.ErrNameTaken) {
		t.Errorf("MoveFile under a taken name = %v, want ErrNameTaken", err)
	}
	if name := livetest.Constraint(t, err, sqlate.ErrUniqueViolation); name != blobfs.ConstraintUniqueFileDirectoryName {
		t.Errorf("the refusal names %q, want blobfs_uq_file_directory_name", name)
	}
	if _, err := e.db.Transact(e.ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
		return e.store.BeginFileDelete(e.ctx, tx, other)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.MoveFile(e.ctx, e.db, held, dst.ID, "held.txt", 1); !errors.Is(err, blobfs.ErrNameTaken) {
		t.Errorf("MoveFile under a name a deleting row holds = %v, want ErrNameTaken: the slot is kept until the row goes", err)
	}
	_, err = e.store.MoveFile(e.ctx, e.db, held, blobfs.NewID(), "held.txt", 1)
	if !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("MoveFile into a missing directory = %v, want ErrNotFound", err)
	}
	if name := livetest.Constraint(t, err, sqlate.ErrForeignKeyViolation); name != blobfs.ConstraintForeignKeyFileDirectory {
		t.Errorf("the refusal names %q, want blobfs_fk_file_directory", name)
	}
	if _, err := e.store.MoveFile(e.ctx, e.db, id, dst.ID, "stale.txt", before.Version); !errors.Is(err, query.ErrVersionMismatch) {
		t.Errorf("MoveFile at a stale version = %v, want ErrVersionMismatch", err)
	}
	deleting, err := e.store.File(e.ctx, e.db, other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.MoveFile(e.ctx, e.db, other, src.ID, "elsewhere.txt", deleting.Version); !errors.Is(err, blobfs.ErrDeleting) {
		t.Errorf("MoveFile of a deleting row = %v, want ErrDeleting", err)
	}
	if after, err := e.store.File(e.ctx, e.db, other); err != nil || after.DirectoryID != dst.ID || after.Name != "held.txt" || after.Version != deleting.Version {
		t.Errorf("the deleting row changed to %+v, %v", after, err)
	}
	if _, err := e.store.MoveFile(e.ctx, e.db, blobfs.NewID(), dst.ID, "ghost.txt", 1); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("MoveFile of a missing file = %v, want ErrNotFound", err)
	}

	// Directories and files have separate name spaces.
	e.mkdir(t, dst.ID, "shared")
	if _, err := e.store.MoveFile(e.ctx, e.db, held, dst.ID, "shared", 1); err != nil {
		t.Errorf("MoveFile under the name of a directory = %v, want the move to succeed", err)
	}
}
