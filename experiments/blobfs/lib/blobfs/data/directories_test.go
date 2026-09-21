package data_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

// TestMkdirRoundTrips proves the baseline's Mkdir: the name is normalized, the id is
// minted or taken from WithID, the row is inserted in one exec, and the
// row is read back by id, two statements; a refused name fails before any
// SQL; and a violation of blobfs's own constraints classifies through the
// write mapping.
func TestMkdirRoundTrips(t *testing.T) {
	ctx := context.Background()
	id := blobfs.NewID()
	s, db, rec := openStore(t, sqltest.Response{Affected: 1}, directoryResponse(id, blobfs.RootID, "docs", 1))
	d, err := s.Mkdir(ctx, db, blobfs.RootID, "docs", data.WithID(id))
	if err != nil || d.ID != id || d.Name != "docs" || d.Version != 1 {
		t.Fatalf("Mkdir = %+v, %v, want the row read back", d, err)
	}
	if ops := rec.Ops(); !slices.Equal(ops, []sqltest.Op{sqltest.OpExec, sqltest.OpQuery}) {
		t.Fatalf("ops = %v, want the insert then the read-back", ops)
	}
	calls := rec.Calls()
	if !strings.HasPrefix(calls[0].SQL, "INSERT INTO blobfs_directory (id, parent_id, name)") || strings.Contains(calls[0].SQL, "RETURNING") {
		t.Errorf("the insert is not the baseline's:\n%s", calls[0].SQL)
	}
	if !slices.Equal(calls[0].Args, []any{id, blobfs.RootID, "docs"}) || !slices.Equal(calls[1].Args, []any{id}) {
		t.Errorf("the insert bound %v and the read-back %v", calls[0].Args, calls[1].Args)
	}

	s, db, rec = openStore(t)
	if _, err := s.Mkdir(ctx, db, blobfs.RootID, "a/b"); !errors.Is(err, blobfs.ErrInvalidName) {
		t.Errorf("Mkdir(a/b) = %v, want ErrInvalidName", err)
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("the refusal reached the driver with %v", calls)
	}

	for constraint, want := range map[string]error{
		blobfs.ConstraintUniqueDirectoryParentName: blobfs.ErrNameTaken,
		blobfs.ConstraintForeignKeyDirectoryParent: blobfs.ErrNotFound,
		blobfs.ConstraintPrimaryKeyDirectory:       blobfs.ErrIDTaken,
	} {
		class := sqlate.ErrUniqueViolation
		if want == blobfs.ErrNotFound {
			class = sqlate.ErrForeignKeyViolation
		}
		s, db, _ := openStore(t, sqltest.Response{Err: &sqlate.ConstraintError{Constraint: constraint, Class: class, Err: errors.New("refused")}})
		_, err := s.Mkdir(ctx, db, blobfs.RootID, "docs")
		var ve *blobfs.ViolationError
		if !errors.Is(err, want) || !errors.As(err, &ve) || ve.Constraint != constraint {
			t.Errorf("Mkdir under %s = %v, want %v", constraint, err, want)
		}
	}
}
