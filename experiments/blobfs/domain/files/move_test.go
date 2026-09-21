package files_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/domain/files"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// noDirectory scripts the read of a directory name no row holds.
func noDirectory() sqltest.Response { return sqltest.Response{Columns: directoryColumns} }

// within scripts the cycle check's count.
func within(n int64) sqltest.Response {
	return sqltest.Response{Columns: []string{"matches"}, Rows: [][]driver.Value{{n}}}
}

// TestMoveRefusesBeforeIO proves the checks that run before any SQL: the
// root as the source, a relative destination, and a source ending with a
// slash, each with nothing reaching the driver.
func TestMoveRefusesBeforeIO(t *testing.T) {
	s, rec := newStore(t)
	ctx := context.Background()
	if _, err := s.Move(ctx, "/", "/x"); !errors.Is(err, blobfs.ErrRootDirectory) {
		t.Errorf("Move(/) = %v, want ErrRootDirectory", err)
	}
	if _, err := s.Move(ctx, "/a", "b"); !errors.Is(err, blobfs.ErrInvalidPath) {
		t.Errorf("Move to a relative path = %v, want ErrInvalidPath", err)
	}
	if _, err := s.Move(ctx, "/a/", "/b"); !errors.Is(err, blobfs.ErrInvalidPath) {
		t.Errorf("Move of a path ending with a slash = %v, want ErrInvalidPath", err)
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("the refusals reached the driver with %d calls", len(calls))
	}
}

// TestMoveFileIsOneTransaction proves a file rename runs in one
// transaction: the destination is resolved as a directory and, not being
// one, as a parent and a name; the source is resolved as a directory and,
// not being one, as a parent and a file; the file's guarded update runs
// and the row is read back; and the transaction commits. The result
// names the file and both paths.
func TestMoveFileIsOneTransaction(t *testing.T) {
	s, rec := newStore(t,
		root(), noDirectory(), root(), // the destination: not a directory, so the root is its parent
		root(), noDirectory(), // the source is not a directory
		root(), file("F", "a.txt", blobfs.StatusAvailable, 1), // the source's parent and the file
		affected(), file("F", "b.txt", blobfs.StatusAvailable, 2),
	)
	res, err := s.Move(context.Background(), "/a.txt", "/b.txt")
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if res.Kind != files.EntryFile || res.ID != "F" || res.From != "/a.txt" || res.To != "/b.txt" {
		t.Errorf("Move = %+v", res)
	}
	if got := ops(rec); got != "begin query query query query query query query exec query commit" {
		t.Errorf("ops = %q", got)
	}
	execs := rec.SQL(sqltest.OpExec)
	if len(execs) != 1 || !strings.HasPrefix(execs[0], "UPDATE blobfs_file") {
		t.Errorf("execs = %q, want the file's guarded update", execs)
	}
	if args := rec.Calls()[8].Args; len(args) != 4 || args[0] != blobfs.RootID || args[1] != "b.txt" || args[2] != "F" || args[3] != int64(1) {
		t.Errorf("the update bound %v, want the root, the new name, the id, and the version read", args)
	}
}

// TestMoveDirectoryIsOneTransactionUnderTheLock proves a directory move
// into an existing directory runs in one transaction with the library's
// three steps inside it: the destination resolves as a directory, the
// source resolves as a directory, then the baseline's no-op lock, the
// cycle check, the guarded update, and the read-back, then the commit.
// The result appends the source's name to the destination.
func TestMoveDirectoryIsOneTransactionUnderTheLock(t *testing.T) {
	s, rec := newStore(t,
		root(), directory("A", blobfs.RootID, "a"), directory("Y", "A", "y"), // the destination /a/y
		root(), directory("A", blobfs.RootID, "a"), directory("X", "A", "x"), // the source /a/x
		within(0), affected(), directory("X", "Y", "x"),
	)
	res, err := s.Move(context.Background(), "/a/x", "/a/y")
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if res.Kind != files.EntryDirectory || res.ID != "X" || res.To != "/a/y/x" {
		t.Errorf("Move = %+v", res)
	}
	if got := ops(rec); got != "begin query query query query query query query exec query commit" {
		t.Errorf("ops = %q", got)
	}
	calls := rec.Calls()
	if !strings.HasPrefix(calls[7].SQL, "WITH RECURSIVE up") || calls[7].Args[0] != "Y" || calls[7].Args[1] != "X" {
		t.Errorf("the check ran %q with %v, want the walk up from the destination looking for the source", calls[7].SQL, calls[7].Args)
	}
	if !strings.HasPrefix(calls[8].SQL, "UPDATE blobfs_directory") || calls[8].Args[0] != "Y" || calls[8].Args[1] != "x" || calls[8].Args[2] != "X" {
		t.Errorf("the update ran %q with %v", calls[8].SQL, calls[8].Args)
	}
}

// TestMoveStaysUnderOneTopLevelDirectory proves the scope rule is
// checked once the destination resolves and before the source is touched:
// a top-level directory moved below another, an entry moved across two
// top-level directories, and an entry moved up to the top level are each
// ErrMoveAcrossScopes, with no update run; a rename of a top-level
// directory passes the rule.
func TestMoveStaysUnderOneTopLevelDirectory(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		label     string
		src, dst  string
		responses []sqltest.Response
	}{
		{"a top-level directory below another", "/a", "/b", []sqltest.Response{root(), directory("B", blobfs.RootID, "b")}},
		{"across two top-level directories", "/a/x", "/b/x", []sqltest.Response{root(), directory("B", blobfs.RootID, "b"), noDirectory(), root(), directory("B", blobfs.RootID, "b")}},
		{"up to the top level", "/a/x", "/x", []sqltest.Response{root(), noDirectory(), root()}},
		{"a file into a top-level directory", "/f.txt", "/a/f.txt", []sqltest.Response{root(), directory("A", blobfs.RootID, "a"), noDirectory(), root(), directory("A", blobfs.RootID, "a")}},
	}
	for _, tc := range cases {
		s, rec := newStore(t, tc.responses...)
		_, err := s.Move(ctx, tc.src, tc.dst)
		if !errors.Is(err, files.ErrMoveAcrossScopes) {
			t.Errorf("%s: Move(%s, %s) = %v, want ErrMoveAcrossScopes", tc.label, tc.src, tc.dst, err)
		}
		if execs := rec.SQL(sqltest.OpExec); len(execs) != 0 {
			t.Errorf("%s: the refused move ran %v", tc.label, execs)
		}
		if rec.Pending() != 0 {
			t.Errorf("%s: the source was resolved after the rule refused the move", tc.label)
		}
	}

	s, rec := newStore(t,
		root(), noDirectory(), root(), // /c is not a directory; the root is its parent
		root(), directory("A", blobfs.RootID, "a"),
		within(0), affected(), directory("A", blobfs.RootID, "c"),
	)
	res, err := s.Move(ctx, "/a", "/c")
	if err != nil || res.To != "/c" {
		t.Errorf("a rename at the top level = %+v, %v", res, err)
	}
	if got := ops(rec); got != "begin query query query query query query exec query commit" {
		t.Errorf("ops = %q", got)
	}
}

// TestCommands_RenderMove proves mv prints the result line with both
// paths and the id.
func TestCommands_RenderMove(t *testing.T) {
	s, _ := newStore(t,
		root(), noDirectory(), root(),
		root(), noDirectory(),
		root(), file("F", "a.txt", blobfs.StatusAvailable, 1),
		affected(), file("F", "b.txt", blobfs.StatusAvailable, 2),
	)
	out, err := run(t, func() (*files.Store, error) { return s, nil }, "mv", "/a.txt", "/b.txt")
	if err != nil {
		t.Fatalf("mv: %v", err)
	}
	if out != "mv: /a.txt -> /b.txt (id F)\n" {
		t.Errorf("mv stdout = %q", out)
	}
}
