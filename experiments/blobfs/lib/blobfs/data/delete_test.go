package data_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// violation scripts an exec refused by a constraint, as the dialect maps
// it.
func violation(constraint string, class error) sqltest.Response {
	return sqltest.Response{Err: &sqlate.ConstraintError{Constraint: constraint, Class: class, Err: errors.New("driver")}}
}

// TestCompleteFileDelete proves the complete step against the script: one
// exec of remove_file bound to the id removes a deleting row and nothing
// else runs; when it removes nothing the row is read once, and a row that
// is gone is success, a row that is not deleting is ErrNotDeleting naming
// its status with nothing changed, and a row that became deleting in
// between has the removal repeated once. A foreign key blobfs does not
// own classifies as ErrReferenced with the sqlate.ConstraintError
// reachable.
func TestCompleteFileDelete(t *testing.T) {
	ctx := context.Background()
	t.Run("RemovesTheDeletingRow", func(t *testing.T) {
		s, db, rec := openStore(t, sqltest.Response{Affected: 1})
		if err := s.CompleteFileDelete(ctx, db, "F"); err != nil {
			t.Fatalf("CompleteFileDelete: %v", err)
		}
		if got := rec.SQL(sqltest.OpExec); len(got) != 1 || !strings.HasPrefix(got[0], "DELETE FROM blobfs_file") || !strings.Contains(got[0], "status = 'deleting'") {
			t.Errorf("execs = %q, want the guarded removal", got)
		}
		if calls := rec.Calls(); len(calls) != 1 || calls[0].Args[0] != "F" {
			t.Errorf("calls = %+v, want one exec bound to the id", calls)
		}
	})
	t.Run("GoneIsSuccess", func(t *testing.T) {
		s, db, rec := openStore(t, sqltest.Response{Affected: 0}, sqltest.Response{Columns: fileColumns})
		if err := s.CompleteFileDelete(ctx, db, "F"); err != nil {
			t.Fatalf("CompleteFileDelete of a gone row = %v, want success", err)
		}
		if got := strings.Join(opNames(rec), " "); got != "exec query" {
			t.Errorf("ops = %q, want the removal and one read", got)
		}
	})
	t.Run("NotDeletingIsRefused", func(t *testing.T) {
		s, db, rec := openStore(t, sqltest.Response{Affected: 0}, fileResponse("F", "a.txt", blobfs.StatusAvailable, 1))
		err := s.CompleteFileDelete(ctx, db, "F")
		if !errors.Is(err, blobfs.ErrNotDeleting) || !strings.Contains(err.Error(), "the row is available") {
			t.Fatalf("CompleteFileDelete of an available row = %v, want ErrNotDeleting naming the status", err)
		}
		if got := strings.Join(opNames(rec), " "); got != "exec query" {
			t.Errorf("ops = %q, want the removal and one read and nothing more", got)
		}
	})
	t.Run("BecameDeletingIsRepeatedOnce", func(t *testing.T) {
		s, db, rec := openStore(t, sqltest.Response{Affected: 0}, fileResponse("F", "a.txt", blobfs.StatusDeleting, 2), sqltest.Response{Affected: 1})
		if err := s.CompleteFileDelete(ctx, db, "F"); err != nil {
			t.Fatalf("CompleteFileDelete = %v, want success from the repeated removal", err)
		}
		if got := strings.Join(opNames(rec), " "); got != "exec query exec" {
			t.Errorf("ops = %q, want the removal, the read, and the removal again", got)
		}
	})
	t.Run("ReferencedByAConsumer", func(t *testing.T) {
		s, db, _ := openStore(t, violation("fk_bookmark_file", sqlate.ErrForeignKeyViolation))
		err := s.CompleteFileDelete(ctx, db, "F")
		var ce *sqlate.ConstraintError
		if !errors.Is(err, blobfs.ErrReferenced) || !errors.As(err, &ce) || ce.Constraint != "fk_bookmark_file" {
			t.Errorf("CompleteFileDelete under a consumer's key = %v, want ErrReferenced with the constraint reachable", err)
		}
		if errors.Is(err, blobfs.ErrNotEmpty) || errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("a consumer's key classified as blobfs's own: %v", err)
		}
	})
}

// TestRemoveDirectory proves the directory removal against the script: the
// root is refused before any SQL; one exec of remove_directory bound to
// the id removes a directory; no row affected is ErrNotFound; blobfs's two
// foreign keys classify as ErrNotEmpty and a consumer's as ErrReferenced,
// each with the sqlate.ConstraintError reachable.
func TestRemoveDirectory(t *testing.T) {
	ctx := context.Background()
	s, db, rec := openStore(t)
	if err := s.RemoveDirectory(ctx, db, blobfs.RootID); !errors.Is(err, blobfs.ErrRootDirectory) {
		t.Errorf("RemoveDirectory(root) = %v, want ErrRootDirectory", err)
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("the root's refusal reached the driver: %+v", calls)
	}

	s, db, rec = openStore(t, sqltest.Response{Affected: 1}, sqltest.Response{Affected: 0})
	if err := s.RemoveDirectory(ctx, db, "D"); err != nil {
		t.Fatalf("RemoveDirectory: %v", err)
	}
	if got := rec.SQL(sqltest.OpExec); len(got) != 1 || !strings.HasPrefix(got[0], "DELETE FROM blobfs_directory") || !strings.Contains(got[0], "parent_id IS NOT NULL") {
		t.Errorf("execs = %q, want the removal that keeps the root", got)
	}
	if calls := rec.Calls(); calls[0].Args[0] != "D" {
		t.Errorf("the removal bound %v, want the id", calls[0].Args)
	}
	if err := s.RemoveDirectory(ctx, db, "D"); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("RemoveDirectory of a missing directory = %v, want ErrNotFound", err)
	}

	for _, c := range []struct {
		constraint string
		want       error
	}{
		{blobfs.ConstraintForeignKeyDirectoryParent, blobfs.ErrNotEmpty},
		{blobfs.ConstraintForeignKeyFileDirectory, blobfs.ErrNotEmpty},
		{"fk_directory_owner_directory", blobfs.ErrReferenced},
	} {
		s, db, _ := openStore(t, violation(c.constraint, sqlate.ErrForeignKeyViolation))
		err := s.RemoveDirectory(ctx, db, "D")
		var ce *sqlate.ConstraintError
		if !errors.Is(err, c.want) || !errors.As(err, &ce) || ce.Constraint != c.constraint {
			t.Errorf("RemoveDirectory under %s = %v, want %v with the constraint reachable", c.constraint, err, c.want)
		}
	}
}

// opNames lists the recorder's operations in order.
func opNames(rec *sqltest.Recorder) []string {
	var out []string
	for _, op := range rec.Ops() {
		out = append(out, string(op))
	}
	return out
}
