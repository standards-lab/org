package data_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// directoryColumns is the column list of a directory row, as the scripted
// driver must return it.
var directoryColumns = []string{"id", "parent_id", "name", "version", "created_at", "updated_at"}

// directoryResponse scripts one non-root directory row.
func directoryResponse(id, parent, name string, version int64) sqltest.Response {
	now := time.Now()
	return sqltest.Response{Columns: directoryColumns, Rows: [][]driver.Value{{id, parent, name, version, now, now}}}
}

// The two spellings of one name, built from code points so that no editor
// can normalize the fixtures. They are named apart from the engine tests'
// pair because the two files compile together under the integration tag.
var (
	nfcName = "caf" + string(rune(0x00E9))  // é as one code point
	nfdName = "cafe" + string(rune(0x0301)) // e followed by a combining acute
)

// within scripts the cycle check's count.
func within(n int64) sqltest.Response {
	return sqltest.Response{Columns: []string{"matches"}, Rows: [][]driver.Value{{n}}}
}

// TestMoveDirectoryRefusesBeforeSQL proves the refusals that happen before
// any statement runs: the root is ErrRootDirectory, an invalid name is
// ErrInvalidName, and a session that is not a transaction is
// ErrTransactionRequired from the baseline's lock, each with nothing
// reaching the driver.
func TestMoveDirectoryRefusesBeforeSQL(t *testing.T) {
	ctx := context.Background()
	s, db, rec := openStore(t)
	if _, err := s.MoveDirectory(ctx, db, blobfs.RootID, "P", "root", 1); !errors.Is(err, blobfs.ErrRootDirectory) {
		t.Errorf("MoveDirectory(root) = %v, want ErrRootDirectory", err)
	}
	if _, err := s.MoveDirectory(ctx, db, "D", "P", "a/b", 1); !errors.Is(err, blobfs.ErrInvalidName) {
		t.Errorf("MoveDirectory with a slash in the name = %v, want ErrInvalidName", err)
	}
	if _, err := s.MoveDirectory(ctx, db, "D", "P", "d", 1); !errors.Is(err, query.ErrTransactionRequired) {
		t.Errorf("MoveDirectory on the pool = %v, want ErrTransactionRequired", err)
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("the refusals reached the driver with %d calls", len(calls))
	}
}

// TestMoveDirectoryIsThreeStepsUnderOneLock proves the order of the move
// on the baseline: in the caller's transaction, the lock (a no-op that
// runs no SQL), then the cycle check bound to the new parent and the
// moved directory, then the guarded update bound to the new parent, the
// normalized name, the id, and the expected version, then the read-back.
// A check that finds the moved directory above the new parent is
// ErrCycle, and the update never runs.
func TestMoveDirectoryIsThreeStepsUnderOneLock(t *testing.T) {
	ctx := context.Background()
	s, db, rec := openStore(t, within(0), sqltest.Response{Affected: 1}, directoryResponse("D", "P", nfcName, 2))
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	d, err := s.MoveDirectory(ctx, tx, "D", "P", nfdName, 1)
	if err != nil {
		t.Fatalf("MoveDirectory: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if d.ID != "D" || *d.ParentID != "P" || d.Version != 2 {
		t.Errorf("MoveDirectory returned %+v, want the row read back", d)
	}
	if ops := rec.Ops(); !slices.Equal(ops, []sqltest.Op{sqltest.OpBegin, sqltest.OpQuery, sqltest.OpExec, sqltest.OpQuery, sqltest.OpCommit}) {
		t.Errorf("ops = %v, want begin, the check, the update, the read-back, commit", ops)
	}
	calls := rec.Calls()
	if !strings.HasPrefix(calls[1].SQL, "WITH RECURSIVE up") || !slices.Equal(calls[1].Args, []any{"P", "D"}) {
		t.Errorf("the check ran %q with %v, want the upward walk from the new parent looking for the directory", calls[1].SQL, calls[1].Args)
	}
	if !strings.HasPrefix(calls[2].SQL, "UPDATE blobfs_directory") || !strings.Contains(calls[2].SQL, "AND parent_id IS NOT NULL") {
		t.Errorf("the update is %q", calls[2].SQL)
	}
	if !slices.Equal(calls[2].Args, []any{"P", nfcName, "D", int64(1)}) {
		t.Errorf("the update bound %v, want the parent, the normalized name, the id, and the expected version", calls[2].Args)
	}

	s, db, rec = openStore(t, within(1))
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := s.MoveDirectory(ctx, tx, "D", "D", "d", 1); !errors.Is(err, blobfs.ErrCycle) {
		t.Errorf("MoveDirectory into itself = %v, want ErrCycle", err)
	}
	_ = tx.Rollback()
	if execs := rec.SQL(sqltest.OpExec); len(execs) != 0 {
		t.Errorf("the refused move ran %v", execs)
	}
}

// TestMoveDirectoryClassifies proves the outcomes of the guarded update: no
// row and no version is ErrNotFound; another version is
// ErrVersionMismatch; a missing new parent is ErrNotFound through the
// foreign key, and a taken name ErrNameTaken through the unique
// constraint, each with the constraint reachable.
func TestMoveDirectoryClassifies(t *testing.T) {
	ctx := context.Background()
	inTx := func(s interface {
		MoveDirectory(context.Context, sqlate.Session, string, string, string, int64) (blobfs.Directory, error)
	}, db *sqlate.DB) error {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		defer func() { _ = tx.Rollback() }()
		_, err = s.MoveDirectory(ctx, tx, "D", "P", "d", 1)
		return err
	}
	s, db, _ := openStore(t, within(0), sqltest.Response{Affected: 0}, sqltest.Response{Columns: []string{"version"}})
	if err := inTx(s, db); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("MoveDirectory of a missing directory = %v, want ErrNotFound", err)
	}
	s, db, _ = openStore(t, within(0), sqltest.Response{Affected: 0}, version(3))
	if err := inTx(s, db); !errors.Is(err, query.ErrVersionMismatch) {
		t.Errorf("MoveDirectory at a stale version = %v, want ErrVersionMismatch", err)
	}
	for constraint, want := range map[string]error{
		blobfs.ConstraintForeignKeyDirectoryParent: blobfs.ErrNotFound,
		blobfs.ConstraintUniqueDirectoryParentName: blobfs.ErrNameTaken,
	} {
		class := sqlate.ErrForeignKeyViolation
		if want == blobfs.ErrNameTaken {
			class = sqlate.ErrUniqueViolation
		}
		s, db, _ = openStore(t, within(0), sqltest.Response{Err: &sqlate.ConstraintError{Constraint: constraint, Class: class, Err: errors.New("driver")}})
		err := inTx(s, db)
		var ce *sqlate.ConstraintError
		if !errors.Is(err, want) || !errors.As(err, &ce) || ce.Constraint != constraint {
			t.Errorf("MoveDirectory under %s = %v, want %v with the constraint reachable", constraint, err, want)
		}
	}
}

// TestMoveFile proves the file move: one guarded update on the pool bound
// to the directory, the normalized name, the id, and the expected
// version, with the status predicate in its text and the key untouched,
// then the read-back; a refused name before any SQL; and the outcomes of
// the guard: a missing row is ErrNotFound, a moved version is
// ErrVersionMismatch, a deleting row at the expected version is
// ErrDeleting after the classifying read, a missing directory is
// ErrNotFound through the foreign key, and a taken name ErrNameTaken.
func TestMoveFile(t *testing.T) {
	ctx := context.Background()
	s, db, rec := openStore(t, sqltest.Response{Affected: 1}, fileResponse("F", nfcName, blobfs.StatusAvailable, 2))
	f, err := s.MoveFile(ctx, db, "F", "P", nfdName, 1)
	if err != nil {
		t.Fatalf("MoveFile: %v", err)
	}
	if f.ID != "F" || f.Version != 2 || f.Key != "F/"+nfcName {
		t.Errorf("MoveFile returned %+v, want the row read back", f)
	}
	if ops := rec.Ops(); !slices.Equal(ops, []sqltest.Op{sqltest.OpExec, sqltest.OpQuery}) {
		t.Errorf("ops = %v, want the update and the read-back on the pool", ops)
	}
	calls := rec.Calls()
	if !strings.HasPrefix(calls[0].SQL, "UPDATE blobfs_file") || !strings.Contains(calls[0].SQL, "AND status <> 'deleting'") || strings.Contains(calls[0].SQL, "key") {
		t.Errorf("the update is %q, want the status predicate and no key column", calls[0].SQL)
	}
	if !slices.Equal(calls[0].Args, []any{"P", nfcName, "F", int64(1)}) {
		t.Errorf("the update bound %v, want the directory, the normalized name, the id, and the expected version", calls[0].Args)
	}

	s, db, rec = openStore(t)
	if _, err := s.MoveFile(ctx, db, "F", "P", "", 1); !errors.Is(err, blobfs.ErrInvalidName) {
		t.Errorf("MoveFile to an empty name = %v, want ErrInvalidName", err)
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("the refused name reached the driver with %d calls", len(calls))
	}

	s, db, _ = openStore(t, sqltest.Response{Affected: 0}, sqltest.Response{Columns: []string{"version"}})
	if _, err := s.MoveFile(ctx, db, "F", "P", "a", 1); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("MoveFile of a missing row = %v, want ErrNotFound", err)
	}
	s, db, _ = openStore(t, sqltest.Response{Affected: 0}, version(3), fileResponse("F", "a", blobfs.StatusAvailable, 3))
	if _, err := s.MoveFile(ctx, db, "F", "P", "a", 1); !errors.Is(err, query.ErrVersionMismatch) {
		t.Errorf("MoveFile at a stale version = %v, want ErrVersionMismatch", err)
	}
	s, db, _ = openStore(t, sqltest.Response{Affected: 0}, version(1), fileResponse("F", "a", blobfs.StatusDeleting, 1))
	if _, err := s.MoveFile(ctx, db, "F", "P", "a", 1); !errors.Is(err, blobfs.ErrDeleting) || errors.Is(err, query.ErrVersionMismatch) {
		t.Errorf("MoveFile of a deleting row = %v, want ErrDeleting and not a version mismatch", err)
	}
	for constraint, want := range map[string]error{
		blobfs.ConstraintForeignKeyFileDirectory: blobfs.ErrNotFound,
		blobfs.ConstraintUniqueFileDirectoryName: blobfs.ErrNameTaken,
	} {
		class := sqlate.ErrForeignKeyViolation
		if want == blobfs.ErrNameTaken {
			class = sqlate.ErrUniqueViolation
		}
		s, db, _ = openStore(t, sqltest.Response{Err: &sqlate.ConstraintError{Constraint: constraint, Class: class, Err: errors.New("driver")}})
		if _, err := s.MoveFile(ctx, db, "F", "P", "a", 1); !errors.Is(err, want) {
			t.Errorf("MoveFile under %s = %v, want %v", constraint, err, want)
		}
	}
}

// TestIsWithin proves the check's binding and reading: the walk starts at
// id and looks for ancestorID, and a count of zero is false.
func TestIsWithin(t *testing.T) {
	ctx := context.Background()
	s, db, rec := openStore(t, within(0), within(2))
	for i, want := range []bool{false, true} {
		got, err := s.IsWithin(ctx, db, "P", "D")
		if err != nil || got != want {
			t.Errorf("IsWithin %d = %v, %v, want %v", i, got, err, want)
		}
	}
	if args := rec.Calls()[0].Args; !slices.Equal(args, []any{"P", "D"}) {
		t.Errorf("IsWithin bound %v, want the start of the walk and the directory looked for", args)
	}
}
