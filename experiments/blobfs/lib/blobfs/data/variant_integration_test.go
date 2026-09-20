//go:build integration

package data_test

import (
	"errors"
	"testing"

	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data/datatest"
)

// TestStandardConformance runs the conformance suite over the default
// store, whose variant is the standard baseline: the file-delete begin's
// contract holds through two standard statements, and the tree lock is
// the documented no-op that reports it does not serialize and never
// blocks.
func TestStandardConformance(t *testing.T) {
	e := open(t)
	if e.store.Serializes() {
		t.Fatal("the baseline reports it serializes")
	}
	datatest.Run(t, e.db, e.store)
}

// TestStandardBeginRequiresTransaction proves on the engine what the
// hermetic test proves against the script: the baseline's begin refuses
// the pool with ErrTransactionRequired and changes nothing, because its
// read-back needs the update's row lock to hold.
func TestStandardBeginRequiresTransaction(t *testing.T) {
	e := open(t)
	dir := e.mkdir(t, blobfs.RootID, "docs")
	id := insertFile(e.ctx, t, e.db, dir.ID, "a.txt")
	if _, err := e.store.BeginFileDelete(e.ctx, e.db, id); !errors.Is(err, query.ErrTransactionRequired) {
		t.Fatalf("BeginFileDelete on the pool = %v, want ErrTransactionRequired", err)
	}
	f, err := e.store.File(e.ctx, e.db, id)
	if err != nil || f.Status != blobfs.StatusAvailable || f.Version != 1 {
		t.Errorf("after the refusal the row is %+v, %v, want it untouched", f, err)
	}
}
