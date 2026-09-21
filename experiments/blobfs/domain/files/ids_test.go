package files_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/sqlate/query"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/domain/files"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// fileIn scripts one file row in the directory with dirID.
func fileIn(id, dirID, name string, status blobfs.Status, version int64) sqltest.Response {
	resp := file(id, name, status, version)
	resp.Rows[0][1] = dirID
	return resp
}

// ancestors scripts the chain of directory_ancestors for a directory at
// /names..., root first: the root row with no parent, then one row per
// name.
func ancestors(names ...string) sqltest.Response {
	resp := sqltest.Response{Columns: []string{"parent_id", "name"}, Rows: [][]driver.Value{{nil, "/"}}}
	for _, name := range names {
		resp.Rows = append(resp.Rows, []driver.Value{blobfs.RootID, name})
	}
	return resp
}

// TestListDirectoryByID proves the listing by id: one read-only
// repeatable-read transaction holding the directory's read and the two
// halves, with no path resolved and no path reported; a missing id is
// ErrNotFound; with a scope the owner row of the scope directory and the
// containment check run first, in that order, and each refusal is
// ErrNotOwned with nothing listed; the listing's Unit names the scope's
// unit when the scope names none and is refused when the two differ; a
// scope naming a unit alone is refused; and at the root a unit lists its
// owned top-level directories through the owner read model, which takes
// no cursor.
func TestListDirectoryByID(t *testing.T) {
	ctx := context.Background()
	unit, other := blobfs.NewID(), blobfs.NewID()
	l := files.Listing{Page: 1, Size: 10}
	s, rec := newStore(t, directory("D", blobfs.RootID, "d"), listing(directoryColumns, true, 1, "x"), listing(fileColumns, true, 2, "a.txt", "b.txt"))
	c, err := s.ListDirectory(ctx, "D", l, files.Scope{})
	if err != nil {
		t.Fatalf("ListDirectory: %v", err)
	}
	if got := ops(rec); got != "begin query query query commit" {
		t.Errorf("ops = %q, want one transaction around the directory read and the two halves", got)
	}
	begin := rec.Calls()[0]
	if !begin.TxOptions.ReadOnly || begin.TxOptions.Isolation != driver.IsolationLevel(sql.LevelRepeatableRead) {
		t.Errorf("the transaction is not read-only repeatable read: %+v", begin.TxOptions)
	}
	read := rec.Calls()[1]
	if !strings.Contains(read.SQL, "FROM blobfs_directory d\nWHERE d.id =") || read.Args[0] != "D" {
		t.Errorf("the first read is not the directory by id: %s %v", read.SQL, read.Args)
	}
	if rec.Calls()[2].Args[0] != "D" || rec.Calls()[3].Args[0] != "D" {
		t.Errorf("the halves bound %v and %v, want the id", rec.Calls()[2].Args, rec.Calls()[3].Args)
	}
	if c.Path != "" || c.Directories.Total != 1 || c.Files.Total != 2 || c.Files.Rows[1].Name != "b.txt" {
		t.Errorf("contents = %+v, want no path and both halves", c)
	}

	s, rec = newStore(t, noDirectory())
	if _, err := s.ListDirectory(ctx, "D", l, files.Scope{}); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("ListDirectory of a missing id = %v, want ErrNotFound", err)
	}
	if got := ops(rec); got != "begin query rollback" {
		t.Errorf("ops after the missing id = %q", got)
	}

	// The scope: the owner row of the scope directory, then the walk up
	// from the listed directory looking for it, then the directory and
	// the halves.
	scope := files.Scope{Unit: unit, DirectoryID: "A"}
	s, rec = newStore(t, owner("A", unit), within(1), directory("D", "A", "d"), listing(directoryColumns, true, 0), listing(fileColumns, true, 0))
	if _, err := s.ListDirectory(ctx, "D", l, scope); err != nil {
		t.Fatalf("ListDirectory in scope: %v", err)
	}
	if got := ops(rec); got != "begin query query query query query commit" {
		t.Errorf("ops in scope = %q", got)
	}
	calls := rec.Calls()
	if !strings.Contains(calls[1].SQL, "FROM directory_owner") || calls[1].Args[0] != "A" {
		t.Errorf("the first read is not the owner of the scope directory: %s %v", calls[1].SQL, calls[1].Args)
	}
	if !strings.HasPrefix(calls[2].SQL, "WITH RECURSIVE up") || calls[2].Args[0] != "D" || calls[2].Args[1] != "A" {
		t.Errorf("the second read is not the walk up from the directory looking for the scope: %s %v", calls[2].SQL, calls[2].Args)
	}
	s, rec = newStore(t, owner("A", unit), within(1), directory("D", "A", "d"), listing(directoryColumns, true, 0), listing(fileColumns, true, 0))
	if _, err := s.ListDirectory(ctx, "D", files.Listing{Page: 1, Size: 10, Unit: unit}, files.Scope{DirectoryID: "A"}); err != nil {
		t.Errorf("the listing's unit did not name the scope's unit: %v", err)
	}
	if got := ops(rec); got != "begin query query query query query commit" {
		t.Errorf("ops with the unit from the listing = %q", got)
	}

	for _, tc := range []struct {
		label     string
		responses []sqltest.Response
		ops       string
	}{
		{"another unit owns the scope directory", []sqltest.Response{owner("A", other)}, "begin query rollback"},
		{"no unit owns the scope directory", []sqltest.Response{owner("A", "")}, "begin query rollback"},
		{"the directory is not within the scope", []sqltest.Response{owner("A", unit), within(0)}, "begin query query rollback"},
	} {
		s, rec := newStore(t, tc.responses...)
		_, err := s.ListDirectory(ctx, "D", l, scope)
		if !errors.Is(err, files.ErrNotOwned) {
			t.Errorf("%s: ListDirectory = %v, want ErrNotOwned", tc.label, err)
		}
		if got := ops(rec); got != tc.ops {
			t.Errorf("%s: ops = %q, want %q", tc.label, got, tc.ops)
		}
	}
	s, rec = newStore(t)
	if _, err := s.ListDirectory(ctx, "D", files.Listing{Page: 1, Size: 10, Unit: unit}, files.Scope{Unit: other, DirectoryID: "A"}); !errors.Is(err, files.ErrNotOwned) {
		t.Errorf("a listing naming another unit than the scope = %v, want ErrNotOwned", err)
	}
	if _, err := s.ListDirectory(ctx, "D", l, files.Scope{Unit: unit}); !errors.Is(err, files.ErrNotOwned) {
		t.Errorf("a scope naming a unit alone = %v, want ErrNotOwned", err)
	}
	if got := ops(rec); got != "begin rollback" {
		t.Errorf("ops after the malformed scope = %q, want no statement", got)
	}

	// The root as a unit: the owner read model, as List does at /.
	now := time.Now()
	s, rec = newStore(t, counted(1), sqltest.Response{Columns: ownedColumns, Rows: [][]driver.Value{{"A", blobfs.RootID, "a", int64(1), now, now, unit}}})
	c, err = s.ListDirectory(ctx, blobfs.RootID, l, files.Scope{Unit: unit})
	if err != nil {
		t.Fatalf("ListDirectory of the root as a unit: %v", err)
	}
	if got := ops(rec); got != "begin query query commit" || !strings.Contains(rec.Calls()[2].SQL, "JOIN directory_owner") {
		t.Errorf("ops = %q; the root listing did not read the owner read model", got)
	}
	if c.Path != "/" || len(c.Directories.Rows) != 1 || c.Directories.Rows[0].ID != "A" || c.Files.Total != 0 {
		t.Errorf("contents of the root as a unit = %+v", c)
	}
	s, rec = newStore(t)
	if _, err := s.ListDirectory(ctx, blobfs.RootID, files.Listing{Page: 1, Size: 10, After: files.After{Files: "x"}}, files.Scope{Unit: unit}); !errors.Is(err, files.ErrNoCursorAtRoot) {
		t.Errorf("the root as a unit with a cursor = %v, want ErrNoCursorAtRoot", err)
	}
	if n := len(rec.Calls()); n != 0 {
		t.Errorf("the refusal reached the driver with %d calls", n)
	}
}

// TestInScope proves the scope check on its own: the owner row of the
// scope directory is read first and a unit that does not own it is
// refused before the walk runs, then the walk up from the directory; the
// root, which has no owner row, cannot be a scope; and a zero scope
// checks nothing.
func TestInScope(t *testing.T) {
	ctx := context.Background()
	unit := blobfs.NewID()
	s, rec := newStore(t, owner("A", unit), within(1))
	if err := s.InScope(ctx, "D", files.Scope{Unit: unit, DirectoryID: "A"}); err != nil {
		t.Errorf("InScope = %v", err)
	}
	if got := ops(rec); got != "query query" {
		t.Errorf("ops = %q, want the owner read and the walk on the pool", got)
	}
	s, rec = newStore(t, owner("A", blobfs.NewID()), within(1))
	if err := s.InScope(ctx, "D", files.Scope{Unit: unit, DirectoryID: "A"}); !errors.Is(err, files.ErrNotOwned) {
		t.Errorf("InScope under another unit's directory = %v, want ErrNotOwned", err)
	}
	if got := ops(rec); got != "query" || rec.Pending() != 1 {
		t.Errorf("ops = %q with %d pending; the walk ran although the unit does not own the scope", got, rec.Pending())
	}
	s, _ = newStore(t, owner(blobfs.RootID, ""))
	if err := s.InScope(ctx, "D", files.Scope{Unit: unit, DirectoryID: blobfs.RootID}); !errors.Is(err, files.ErrNotOwned) {
		t.Errorf("InScope with the root as the scope = %v, want ErrNotOwned", err)
	}
	s, rec = newStore(t)
	if err := s.InScope(ctx, "D", files.Scope{}); err != nil || len(rec.Calls()) != 0 {
		t.Errorf("InScope with no scope = %v after %d calls", err, len(rec.Calls()))
	}
}

// TestStatAndOpenByID proves stat by id is one read on the pool with no
// store access, that the scope check follows the read and refuses a
// file outside the scope, that a missing id is ErrNotFound, and that
// OpenFile refuses a pending or deleting row before the store is asked,
// streams an available row's object, and reports a missing object.
func TestStatAndOpenByID(t *testing.T) {
	ctx := context.Background()
	unit := blobfs.NewID()
	opens := 0
	s, rec, _ := writeStore(t, &opens, file("P", "a.txt", blobfs.StatusPending, 1))
	f, err := s.StatFile(ctx, "P", files.Scope{})
	if err != nil || f.ID != "P" || f.Status != blobfs.StatusPending {
		t.Fatalf("StatFile = %+v, %v", f, err)
	}
	if got := ops(rec); got != "query" || opens != 0 {
		t.Errorf("ops = %q with %d store opens; want one pool read and no store", got, opens)
	}
	if read := rec.Calls()[0]; !strings.Contains(read.SQL, "FROM blobfs_file f\nWHERE f.id =") || read.Args[0] != "P" {
		t.Errorf("the read is not the file by id: %s %v", read.SQL, read.Args)
	}

	s, rec, _ = writeStore(t, &opens, fileIn("P", "D", "a.txt", blobfs.StatusAvailable, 2), owner("A", unit), within(1))
	if _, err := s.StatFile(ctx, "P", files.Scope{Unit: unit, DirectoryID: "A"}); err != nil {
		t.Errorf("StatFile in scope = %v", err)
	}
	if got := ops(rec); got != "query query query" || rec.Calls()[2].Args[0] != "D" || rec.Calls()[2].Args[1] != "A" {
		t.Errorf("ops in scope = %q; the walk bound %v, want the file's directory and the scope", got, rec.Calls()[2].Args)
	}
	s, _, _ = writeStore(t, &opens, fileIn("P", "D", "a.txt", blobfs.StatusAvailable, 2), owner("A", blobfs.NewID()))
	if _, err := s.StatFile(ctx, "P", files.Scope{Unit: unit, DirectoryID: "A"}); !errors.Is(err, files.ErrNotOwned) {
		t.Errorf("StatFile outside the scope = %v, want ErrNotOwned", err)
	}
	s, _, _ = writeStore(t, &opens, noFile())
	if _, err := s.StatFile(ctx, "missing", files.Scope{}); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("StatFile of a missing id = %v, want ErrNotFound", err)
	}

	for _, status := range []blobfs.Status{blobfs.StatusPending, blobfs.StatusDeleting} {
		s, _, _ = writeStore(t, &opens, file("P", "a.txt", status, 1))
		_, _, err := s.OpenFile(ctx, "P", files.Scope{})
		if !errors.Is(err, files.ErrNotAvailable) || !strings.Contains(err.Error(), "cat file P: the file is "+string(status)) || opens != 0 {
			t.Errorf("OpenFile of a %s file = %v with %d store opens; want ErrNotAvailable naming the file and the status, and no store", status, err, opens)
		}
	}
	s, _, fake := writeStore(t, &opens, file("A", "a.txt", blobfs.StatusAvailable, 2))
	if _, err := fake.Put(ctx, "A/a.txt", strings.NewReader("stored"), storage.PutOptions{ContentType: "text/plain"}); err != nil {
		t.Fatalf("seed the fake: %v", err)
	}
	body, f, err := s.OpenFile(ctx, "A", files.Scope{})
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer func() { _ = body.Close() }()
	if got, _ := io.ReadAll(body); string(got) != "stored" || f.ID != "A" {
		t.Errorf("OpenFile streamed %q for %+v", got, f)
	}
	s, _, _ = writeStore(t, &opens, file("G", "gone.txt", blobfs.StatusAvailable, 2))
	if _, _, err := s.OpenFile(ctx, "G", files.Scope{}); !errors.Is(err, files.ErrObjectMissing) {
		t.Errorf("OpenFile of an available row with no object = %v, want ErrObjectMissing", err)
	}
	s, _ = newStore(t, file("A", "a.txt", blobfs.StatusAvailable, 2))
	if _, _, err := s.OpenFile(ctx, "A", files.Scope{}); !errors.Is(err, files.ErrNoStorage) {
		t.Errorf("OpenFile without an object store = %v, want ErrNoStorage", err)
	}
}

// TestRemoveFileByID proves the delete by id: without a version the
// three steps run with no read before the begin; with a version the
// begin transaction holds the row at that version first, so a row that
// moved on is query.ErrVersionMismatch with the transaction rolled back
// and the object untouched, while a row already deleting is let through
// to the begin and the rerun resumes; with a scope the row is read on
// the pool and checked before the store is opened; and a missing id is
// ErrNotFound from the begin.
func TestRemoveFileByID(t *testing.T) {
	ctx := context.Background()
	unit := blobfs.NewID()
	opens := 0
	s, rec, fake := writeStore(t, &opens, affected(), file("F", "a.txt", blobfs.StatusDeleting, 2), bookmarkTotal(0), affected())
	stored(t, fake, "F/a.txt")
	f, err := s.RemoveFile(ctx, "F", 0, "", files.Scope{})
	if err != nil || f.Status != blobfs.StatusDeleting {
		t.Fatalf("RemoveFile = %+v, %v", f, err)
	}
	if got := ops(rec); got != "begin exec query query commit exec" {
		t.Errorf("ops = %q, want the three steps with no read before the begin", got)
	}
	if held(t, fake, "F/a.txt") || opens != 1 {
		t.Errorf("after the rm the object held is %v and the store was opened %d times", held(t, fake, "F/a.txt"), opens)
	}

	s, rec, fake = writeStore(t, &opens, affected(), affected(), file("F", "a.txt", blobfs.StatusDeleting, 2), bookmarkTotal(0), affected())
	stored(t, fake, "F/a.txt")
	if _, err := s.RemoveFile(ctx, "F", 1, "", files.Scope{}); err != nil {
		t.Fatalf("RemoveFile at the version read = %v", err)
	}
	if got := ops(rec); got != "begin exec exec query query commit exec" {
		t.Errorf("ops with a version = %q, want the hold before the begin", got)
	}
	if hold := rec.Calls()[1]; !strings.Contains(hold.SQL, "SET updated_at = updated_at") || !strings.Contains(hold.SQL, "AND version = ") || hold.Args[0] != "F" || hold.Args[1] != int64(1) {
		t.Errorf("the first exec is not the hold at the version: %s %v", hold.SQL, hold.Args)
	}
	if held(t, fake, "F/a.txt") {
		t.Error("the object is still in the store after the rm at the version read")
	}

	s, rec, fake = writeStore(t, &opens, sqltest.Response{Affected: 0}, file("F", "a.txt", blobfs.StatusAvailable, 5))
	stored(t, fake, "F/a.txt")
	_, err = s.RemoveFile(ctx, "F", 1, "", files.Scope{})
	if !errors.Is(err, query.ErrVersionMismatch) || !strings.Contains(err.Error(), "rm file F") {
		t.Errorf("RemoveFile at a stale version = %v, want ErrVersionMismatch naming the file", err)
	}
	if got := ops(rec); got != "begin exec query rollback" {
		t.Errorf("ops after the stale version = %q, want the hold, the read that classifies it, and a rollback", got)
	}
	if !held(t, fake, "F/a.txt") {
		t.Error("the refused rm deleted the object")
	}

	s, rec, _ = writeStore(t, &opens, sqltest.Response{Affected: 0}, file("F", "a.txt", blobfs.StatusDeleting, 2), sqltest.Response{Affected: 0}, file("F", "a.txt", blobfs.StatusDeleting, 2), bookmarkTotal(0), affected())
	if f, err := s.RemoveFile(ctx, "F", 1, "", files.Scope{}); err != nil || f.Status != blobfs.StatusDeleting {
		t.Errorf("RemoveFile of a deleting row at an earlier version = %+v, %v; want the delete resumed", f, err)
	}
	if got := ops(rec); got != "begin exec query exec query query commit exec" {
		t.Errorf("ops for the deleting row = %q, want the hold's read, then the begin", got)
	}

	s, rec, fake = writeStore(t, &opens, fileIn("F", "D", "a.txt", blobfs.StatusAvailable, 1), owner("A", unit), within(1), affected(), file("F", "a.txt", blobfs.StatusDeleting, 2), bookmarkTotal(0), affected())
	stored(t, fake, "F/a.txt")
	if _, err := s.RemoveFile(ctx, "F", 0, "", files.Scope{Unit: unit, DirectoryID: "A"}); err != nil {
		t.Fatalf("RemoveFile in scope = %v", err)
	}
	if got := ops(rec); got != "query query query begin exec query query commit exec" {
		t.Errorf("ops in scope = %q, want the read and the check on the pool before the steps", got)
	}
	opens = 0
	s, rec, fake = writeStore(t, &opens, fileIn("F", "D", "a.txt", blobfs.StatusAvailable, 1), owner("A", blobfs.NewID()))
	stored(t, fake, "F/a.txt")
	if _, err := s.RemoveFile(ctx, "F", 0, "", files.Scope{Unit: unit, DirectoryID: "A"}); !errors.Is(err, files.ErrNotOwned) {
		t.Errorf("RemoveFile outside the scope = %v, want ErrNotOwned", err)
	}
	if got := ops(rec); got != "query query" || opens != 0 || !held(t, fake, "F/a.txt") {
		t.Errorf("after the refusal ops = %q, %d store opens, object held %v; want nothing begun", got, opens, held(t, fake, "F/a.txt"))
	}

	s, rec, _ = writeStore(t, &opens, affected(), file("F", "a.txt", blobfs.StatusDeleting, 2), bookmarkTotal(0))
	_, err = s.RemoveFile(ctx, "F", 0, files.StepBegin, files.Scope{})
	var stop *files.StopError
	if !errors.As(err, &stop) || stop.Path != "file F" || !strings.Contains(err.Error(), "rm file F: stopped after step begin") {
		t.Errorf("RemoveFile --fail-after begin = %v, want a StopError naming the file by id", err)
	}
	if got := ops(rec); got != "begin exec query query commit" {
		t.Errorf("ops after the stop = %q", got)
	}

	s, rec, _ = writeStore(t, &opens, sqltest.Response{Affected: 0}, noFile())
	if _, err := s.RemoveFile(ctx, "missing", 0, "", files.Scope{}); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("RemoveFile of a missing id = %v, want ErrNotFound", err)
	}
	if got := ops(rec); got != "begin exec query rollback" {
		t.Errorf("ops after the missing id = %q", got)
	}
}

// TestPutFileByID proves the put by directory id: the first transaction
// holds the lookup by name in the directory, the pending insert, and its
// read-back, with no resolution before them, and the object write and
// the completion follow as in Put; with a scope the check runs first in
// that transaction and a directory outside the scope is ErrNotOwned with
// nothing inserted and nothing stored; a stop names the file by its name
// and directory; and a taken name is ErrNameTaken.
func TestPutFileByID(t *testing.T) {
	ctx := context.Background()
	unit := blobfs.NewID()
	opens := 0
	s, rec, fake := writeStore(t, &opens, noFile(), affected(), fileIn("F", "D", "a.txt", blobfs.StatusPending, 1), affected(), fileIn("F", "D", "a.txt", blobfs.StatusAvailable, 2))
	res, err := s.PutFile(ctx, "D", "a.txt", files.PutRequest{ContentType: "text/plain", Body: strings.NewReader("hello"), Size: 5}, files.Scope{})
	if err != nil || res.Resumed || res.File.Status != blobfs.StatusAvailable || res.File.DirectoryID != "D" {
		t.Fatalf("PutFile = %+v, %v", res, err)
	}
	if got := ops(rec); got != "begin query exec query commit exec query" {
		t.Errorf("ops = %q, want the first step without a resolution", got)
	}
	calls := rec.Calls()
	if calls[1].Args[0] != "D" || calls[1].Args[1] != "a.txt" {
		t.Errorf("the lookup bound %v, want the directory id and the name", calls[1].Args)
	}
	if !strings.HasPrefix(calls[2].SQL, "INSERT INTO blobfs_file") || calls[2].Args[1] != "D" {
		t.Errorf("the insert is %s with %v, want the directory id", calls[2].SQL, calls[2].Args)
	}
	if _, err := fake.Stat(ctx, "F/a.txt"); err != nil || opens != 1 {
		t.Errorf("the object is not under the row's key (%v) or the store was opened %d times", err, opens)
	}

	s, rec, _ = writeStore(t, &opens, owner("A", unit), within(1), noFile(), affected(), fileIn("F", "D", "a.txt", blobfs.StatusPending, 1), affected(), fileIn("F", "D", "a.txt", blobfs.StatusAvailable, 2))
	if _, err := s.PutFile(ctx, "D", "a.txt", files.PutRequest{Body: strings.NewReader("x")}, files.Scope{Unit: unit, DirectoryID: "A"}); err != nil {
		t.Fatalf("PutFile in scope = %v", err)
	}
	if got := ops(rec); got != "begin query query query exec query commit exec query" {
		t.Errorf("ops in scope = %q, want the check first in the transaction", got)
	}
	s, rec, fake = writeStore(t, &opens, owner("A", blobfs.NewID()))
	if _, err := s.PutFile(ctx, "D", "a.txt", files.PutRequest{Body: strings.NewReader("x")}, files.Scope{Unit: unit, DirectoryID: "A"}); !errors.Is(err, files.ErrNotOwned) {
		t.Errorf("PutFile outside the scope = %v, want ErrNotOwned", err)
	}
	if got := ops(rec); got != "begin query rollback" || fake.Puts() != 0 {
		t.Errorf("after the refusal ops = %q and %d puts; want nothing inserted or stored", got, fake.Puts())
	}

	s, _, _ = writeStore(t, &opens, noFile(), affected(), fileIn("F", "D", "a.txt", blobfs.StatusPending, 1))
	_, err = s.PutFile(ctx, "D", "a.txt", files.PutRequest{Body: strings.NewReader("x"), StopAfter: files.StepInsert}, files.Scope{})
	var stop *files.StopError
	if !errors.As(err, &stop) || stop.Path != "a.txt in directory D" || !strings.Contains(err.Error(), "put a.txt in directory D: stopped after step insert") {
		t.Errorf("PutFile --fail-after insert = %v, want a StopError naming the name and the directory", err)
	}
	s, _, _ = writeStore(t, &opens, fileIn("F", "D", "a.txt", blobfs.StatusAvailable, 2))
	if _, err := s.PutFile(ctx, "D", "a.txt", files.PutRequest{Body: strings.NewReader("x")}, files.Scope{}); !errors.Is(err, blobfs.ErrNameTaken) {
		t.Errorf("PutFile over an available row = %v, want ErrNameTaken", err)
	}
	s, rec = newStore(t)
	if _, err := s.PutFile(ctx, "D", "a.txt", files.PutRequest{Body: strings.NewReader("x")}, files.Scope{}); !errors.Is(err, files.ErrNoStorage) || len(rec.Calls()) != 0 {
		t.Errorf("PutFile without an object store = %v after %d calls, want ErrNoStorage before any SQL", err, len(rec.Calls()))
	}
}

// TestMoveEntryByID proves the move by id in one transaction: the source
// row is read, the paths of its parent and of the destination are
// computed by one upward walk each, the scope rule is checked on them,
// and then the library's move runs, a file's as the guarded update and a
// directory's under the lock with the cycle check; the version read
// guards the update unless the request carries one, and a stale one is
// query.ErrVersionMismatch with nothing changed; an empty name keeps the
// source's; the result carries the paths as a move by path does; a move
// across two top-level directories is ErrMoveAcrossScopes before any
// update; with a scope both the source's parent and the destination are
// checked; and the root, a bad kind, and a missing source are refused.
func TestMoveEntryByID(t *testing.T) {
	ctx := context.Background()
	unit := blobfs.NewID()
	s, rec := newStore(t,
		fileIn("F", "S", "a.txt", blobfs.StatusAvailable, 1),
		ancestors("s"), ancestors("s", "t"),
		affected(), fileIn("F", "T", "a.txt", blobfs.StatusAvailable, 2),
	)
	res, err := s.MoveEntry(ctx, files.MoveRequest{Kind: files.EntryFile, ID: "F", DirectoryID: "T"}, files.Scope{})
	if err != nil {
		t.Fatalf("MoveEntry: %v", err)
	}
	if res.Kind != files.EntryFile || res.ID != "F" || res.From != "/s/a.txt" || res.To != "/s/t/a.txt" {
		t.Errorf("MoveEntry = %+v", res)
	}
	if got := ops(rec); got != "begin query query query exec query commit" {
		t.Errorf("ops = %q", got)
	}
	calls := rec.Calls()
	if !strings.HasPrefix(calls[2].SQL, "WITH RECURSIVE ancestors") || calls[2].Args[0] != "S" || calls[3].Args[0] != "T" {
		t.Errorf("the walks bound %v and %v, want the source's parent and the destination", calls[2].Args, calls[3].Args)
	}
	if update := calls[4]; !strings.HasPrefix(update.SQL, "UPDATE blobfs_file") || update.Args[0] != "T" || update.Args[1] != "a.txt" || update.Args[2] != "F" || update.Args[3] != int64(1) {
		t.Errorf("the update bound %v, want the destination, the name kept, the id, and the version read", update.Args)
	}

	// The guarded update matches nothing at the caller's version; the
	// guard then reads the row's version to classify, and the library
	// reads the row once more to tell a mismatch from a deleting row.
	current := sqltest.Response{Columns: []string{"version"}, Rows: [][]driver.Value{{int64(1)}}}
	s, rec = newStore(t, fileIn("F", "S", "a.txt", blobfs.StatusAvailable, 1), ancestors("s"), ancestors("s"), sqltest.Response{Affected: 0}, current, fileIn("F", "S", "a.txt", blobfs.StatusAvailable, 1))
	_, err = s.MoveEntry(ctx, files.MoveRequest{Kind: files.EntryFile, ID: "F", DirectoryID: "S", Name: "b.txt", Version: 7}, files.Scope{})
	if !errors.Is(err, query.ErrVersionMismatch) {
		t.Errorf("MoveEntry at a stale version = %v, want ErrVersionMismatch", err)
	}
	if got := ops(rec); got != "begin query query query exec query query rollback" || rec.Calls()[4].Args[3] != int64(7) || rec.Calls()[4].Args[1] != "b.txt" {
		t.Errorf("ops = %q and the update bound %v; want the caller's version and name", got, rec.Calls()[4].Args)
	}

	s, rec = newStore(t,
		directory("X", "A", "x"),
		ancestors("a"), ancestors("a", "y"),
		within(0), affected(), directory("X", "Y", "x"),
	)
	res, err = s.MoveEntry(ctx, files.MoveRequest{Kind: files.EntryDirectory, ID: "X", DirectoryID: "Y"}, files.Scope{})
	if err != nil || res.Kind != files.EntryDirectory || res.From != "/a/x" || res.To != "/a/y/x" {
		t.Errorf("MoveEntry of a directory = %+v, %v", res, err)
	}
	if got := ops(rec); got != "begin query query query query exec query commit" {
		t.Errorf("ops for a directory = %q", got)
	}
	if check := rec.Calls()[4]; !strings.HasPrefix(check.SQL, "WITH RECURSIVE up") || check.Args[0] != "Y" || check.Args[1] != "X" {
		t.Errorf("the cycle check ran %s with %v", check.SQL, check.Args)
	}

	s, rec = newStore(t, fileIn("F", "A", "a.txt", blobfs.StatusAvailable, 1), ancestors("a"), ancestors("b"))
	_, err = s.MoveEntry(ctx, files.MoveRequest{Kind: files.EntryFile, ID: "F", DirectoryID: "B"}, files.Scope{})
	if !errors.Is(err, files.ErrMoveAcrossScopes) || !strings.Contains(err.Error(), "/a/a.txt is under /a and /b/a.txt under /b") {
		t.Errorf("a move across two top-level directories = %v, want ErrMoveAcrossScopes naming both", err)
	}
	if execs := rec.SQL(sqltest.OpExec); len(execs) != 0 {
		t.Errorf("the refused move ran %v", execs)
	}

	scope := files.Scope{Unit: unit, DirectoryID: "A"}
	s, rec = newStore(t,
		fileIn("F", "S", "a.txt", blobfs.StatusAvailable, 1),
		owner("A", unit), within(1), within(1),
		ancestors("a", "s"), ancestors("a", "t"),
		affected(), fileIn("F", "T", "a.txt", blobfs.StatusAvailable, 2),
	)
	if _, err := s.MoveEntry(ctx, files.MoveRequest{Kind: files.EntryFile, ID: "F", DirectoryID: "T"}, scope); err != nil {
		t.Fatalf("MoveEntry in scope = %v", err)
	}
	if got := ops(rec); got != "begin query query query query query query exec query commit" {
		t.Errorf("ops in scope = %q", got)
	}
	if calls := rec.Calls(); calls[3].Args[0] != "S" || calls[4].Args[0] != "T" {
		t.Errorf("the checks bound %v and %v, want the source's parent and the destination", calls[3].Args, calls[4].Args)
	}
	s, rec = newStore(t, fileIn("F", "S", "a.txt", blobfs.StatusAvailable, 1), owner("A", unit), within(1), within(0))
	if _, err := s.MoveEntry(ctx, files.MoveRequest{Kind: files.EntryFile, ID: "F", DirectoryID: "T"}, scope); !errors.Is(err, files.ErrNotOwned) {
		t.Errorf("MoveEntry to a destination outside the scope = %v, want ErrNotOwned", err)
	}
	if got := ops(rec); got != "begin query query query query rollback" {
		t.Errorf("ops after the refusal = %q", got)
	}

	s, rec = newStore(t)
	if _, err := s.MoveEntry(ctx, files.MoveRequest{Kind: files.EntryDirectory, ID: blobfs.RootID, DirectoryID: "A"}, files.Scope{}); !errors.Is(err, blobfs.ErrRootDirectory) {
		t.Errorf("MoveEntry of the root = %v, want ErrRootDirectory", err)
	}
	if _, err := s.MoveEntry(ctx, files.MoveRequest{Kind: "link", ID: "X", DirectoryID: "A"}, files.Scope{}); err == nil || !strings.Contains(err.Error(), "the kind is directory or file") {
		t.Errorf("MoveEntry of an unknown kind = %v", err)
	}
	if n := len(rec.Calls()); n != 0 {
		t.Errorf("the refusals reached the driver with %d calls", n)
	}
	s, rec = newStore(t, noFile())
	if _, err := s.MoveEntry(ctx, files.MoveRequest{Kind: files.EntryFile, ID: "F", DirectoryID: "A"}, files.Scope{}); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("MoveEntry of a missing source = %v, want ErrNotFound", err)
	}
	if got := ops(rec); got != "begin query rollback" {
		t.Errorf("ops after the missing source = %q", got)
	}
}
