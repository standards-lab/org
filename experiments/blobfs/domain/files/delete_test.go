package files_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/go-storage/storagetest"
	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/domain/files"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// bookmarkTotal scripts the bookmark count of a file.
func bookmarkTotal(n int64) sqltest.Response {
	return sqltest.Response{Columns: []string{"bookmarks"}, Rows: [][]driver.Value{{n}}}
}

// refusedBy scripts an exec a constraint refused, as the dialect maps it.
func refusedBy(constraint string) sqltest.Response {
	return sqltest.Response{Err: &sqlate.ConstraintError{Constraint: constraint, Class: sqlate.ErrForeignKeyViolation, Err: errors.New("driver")}}
}

// stored puts an object under key straight into the fake so a test can
// see whether a delete removed it.
func stored(t *testing.T, fake *storagetest.Fake, key string) {
	t.Helper()
	if _, err := fake.Put(context.Background(), key, strings.NewReader("bytes"), storage.PutOptions{}); err != nil {
		t.Fatalf("Put(%s): %v", key, err)
	}
}

// held reports whether the fake still holds an object under key.
func held(t *testing.T, fake *storagetest.Fake, key string) bool {
	t.Helper()
	_, err := fake.Stat(context.Background(), key)
	switch {
	case err == nil:
		return true
	case errors.Is(err, storage.ErrNotFound):
		return false
	}
	t.Fatalf("Stat(%s): %v", key, err)
	return false
}

// TestRemoveIsThreeStepsWithTwoBoundaries proves the transaction
// boundaries of a full rm on the baseline variant: the parent and the
// name are resolved on the pool; one transaction holds the bookmark count,
// the begin's update, and its read-back, and commits before the store is
// touched; the object delete happens outside any transaction; and the
// removal is one exec on the pool. The store is opened once, before the
// transaction, and the key the row carries is the key the object was
// deleted at.
func TestRemoveIsThreeStepsWithTwoBoundaries(t *testing.T) {
	opens := 0
	s, rec, fake := writeStore(t, &opens,
		root(), file("F", "a.txt", blobfs.StatusAvailable, 1),
		affected(), file("F", "a.txt", blobfs.StatusDeleting, 2), bookmarkTotal(0),
		affected(),
	)
	stored(t, fake, "F/a.txt")
	f, err := s.Remove(context.Background(), "/a.txt", "")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if f.ID != "F" || f.Status != blobfs.StatusDeleting || f.Version != 2 {
		t.Errorf("Remove returned %+v, want the deleting row the begin returned", f)
	}
	if got := ops(rec); got != "query query begin exec query query commit exec" {
		t.Errorf("ops = %q", got)
	}
	execs := rec.SQL(sqltest.OpExec)
	if !strings.HasPrefix(execs[0], "UPDATE blobfs_file") || !strings.HasPrefix(execs[1], "DELETE FROM blobfs_file") {
		t.Errorf("execs = %q, want the begin's update and the removal", execs)
	}
	if queries := rec.SQL(sqltest.OpQuery); !strings.HasPrefix(queries[2], "SELECT f.id, f.directory_id") || !strings.Contains(queries[3], "FROM bookmark") {
		t.Errorf("the transaction's reads are %q, want the begin's read-back and then the bookmark count", queries[2:])
	}
	if held(t, fake, "F/a.txt") {
		t.Error("the object is still in the store after the rm")
	}
	if opens != 1 {
		t.Errorf("the store was opened %d times, want once", opens)
	}
}

// TestRemoveStopsAfterTheStepNamed proves --fail-after: after begin the
// transaction has committed with the row deleting, the object is still in
// the store, and the error is a StopError naming rm, the step, and the
// deleting row; after object the object is gone and the row was not
// removed. A failed object delete leaves the same state as a stop after
// begin, with the store's error and the deleting row named.
func TestRemoveStopsAfterTheStepNamed(t *testing.T) {
	ctx := context.Background()
	opens := 0
	s, rec, fake := writeStore(t, &opens, root(), file("F", "a.txt", blobfs.StatusAvailable, 1), affected(), file("F", "a.txt", blobfs.StatusDeleting, 2), bookmarkTotal(0))
	stored(t, fake, "F/a.txt")
	f, err := s.Remove(ctx, "/a.txt", files.StepBegin)
	var stop *files.StopError
	if !errors.As(err, &stop) || !errors.Is(err, files.ErrStopped) || stop.Command != "rm" || stop.Step != files.StepBegin || stop.File.Status != blobfs.StatusDeleting {
		t.Fatalf("Remove --fail-after begin = %v", err)
	}
	if f.Status != blobfs.StatusDeleting || !held(t, fake, "F/a.txt") {
		t.Errorf("after the stop the result is %+v and the object held is %v", f, held(t, fake, "F/a.txt"))
	}
	if got := ops(rec); got != "query query begin exec query query commit" {
		t.Errorf("ops = %q, want the committed first step and nothing more", got)
	}
	if !strings.Contains(err.Error(), "rm /a.txt") || !strings.Contains(err.Error(), "deleting (id F)") || !strings.Contains(err.Error(), "rerun rm") {
		t.Errorf("the message %q does not say how to finish", err)
	}

	s, rec, fake = writeStore(t, &opens, root(), file("F", "a.txt", blobfs.StatusDeleting, 2), affected(), file("F", "a.txt", blobfs.StatusDeleting, 2), bookmarkTotal(0))
	stored(t, fake, "F/a.txt")
	_, err = s.Remove(ctx, "/a.txt", files.StepObject)
	if !errors.As(err, &stop) || stop.Step != files.StepObject {
		t.Fatalf("Remove --fail-after object = %v", err)
	}
	if held(t, fake, "F/a.txt") {
		t.Error("after the stop after object the object is still held")
	}
	if got := ops(rec); got != "query query begin exec query query commit" {
		t.Errorf("ops = %q, want no removal after the stop", got)
	}

	s, rec, fake = writeStore(t, &opens, root(), file("F", "a.txt", blobfs.StatusAvailable, 1), affected(), file("F", "a.txt", blobfs.StatusDeleting, 2), bookmarkTotal(0))
	fake.Down.Store(true)
	_, err = s.Remove(ctx, "/a.txt", "")
	if !errors.Is(err, files.ErrStorageUnavailable) || !strings.Contains(err.Error(), "the row stays deleting") {
		t.Errorf("Remove during an outage = %v, want ErrStorageUnavailable and the row's state named", err)
	}
	if got := ops(rec); got != "query query begin exec query query commit" {
		t.Errorf("ops = %q, want the committed first step and no removal", got)
	}
}

// TestRemoveRefusesABookmarkedFile proves the bookmark check sits after
// the begin in the same transaction: a file any unit bookmarks is
// ErrBookmarked, the transaction rolls back, which undoes the begin's
// update, and the object is untouched. A bookmark the foreign key reports
// at the removal, after the object is gone, is ErrBookmarked too, with
// the sqlate.ConstraintError reachable and the message saying the row
// stays deleting.
func TestRemoveRefusesABookmarkedFile(t *testing.T) {
	ctx := context.Background()
	opens := 0
	s, rec, fake := writeStore(t, &opens, root(), file("F", "a.txt", blobfs.StatusAvailable, 1), affected(), file("F", "a.txt", blobfs.StatusDeleting, 2), bookmarkTotal(2))
	stored(t, fake, "F/a.txt")
	_, err := s.Remove(ctx, "/a.txt", "")
	if !errors.Is(err, files.ErrBookmarked) || !strings.Contains(err.Error(), "2 unit(s) bookmark the file") {
		t.Fatalf("Remove of a bookmarked file = %v, want ErrBookmarked naming the count", err)
	}
	if got := ops(rec); got != "query query begin exec query query rollback" {
		t.Errorf("ops = %q, want the begin, the check, and a rollback", got)
	}
	if !held(t, fake, "F/a.txt") {
		t.Error("the refused rm deleted the object")
	}

	s, rec, _ = writeStore(t, &opens, root(), file("F", "a.txt", blobfs.StatusAvailable, 1), affected(), file("F", "a.txt", blobfs.StatusDeleting, 2), bookmarkTotal(0), refusedBy(files.ConstraintForeignKeyBookmarkFile))
	_, err = s.Remove(ctx, "/a.txt", "")
	var ce *sqlate.ConstraintError
	if !errors.Is(err, files.ErrBookmarked) || !errors.Is(err, blobfs.ErrReferenced) || !errors.As(err, &ce) || ce.Constraint != files.ConstraintForeignKeyBookmarkFile {
		t.Fatalf("Remove refused at the removal = %v, want ErrBookmarked over blobfs.ErrReferenced with the constraint reachable", err)
	}
	if !strings.Contains(err.Error(), "the row stays deleting and its object is gone") {
		t.Errorf("the message %q does not say what state the file is in", err)
	}
	if !strings.HasSuffix(err.Error(), "files: the file is bookmarked (constraint fk_bookmark_file)") || strings.Contains(err.Error(), "driver") {
		t.Errorf("the message %q does not end with the consumer's sentinel and the constraint, or carries the driver's text", err)
	}
	if got := ops(rec); got != "query query begin exec query query commit exec" {
		t.Errorf("ops = %q", got)
	}
}

// TestRemoveRefusalsBeforeAnyStep proves a bad path fails before the
// store is opened and before any SQL, that a store built without an
// object store fails with ErrNoStorage after the resolution and before
// the row is touched, and that a missing file is ErrNotFound with nothing
// begun.
func TestRemoveRefusalsBeforeAnyStep(t *testing.T) {
	ctx := context.Background()
	opens := 0
	s, rec, _ := writeStore(t, &opens)
	for path, want := range map[string]error{"/": blobfs.ErrRootDirectory, "a.txt": blobfs.ErrInvalidPath, "/docs/": blobfs.ErrInvalidPath} {
		if _, err := s.Remove(ctx, path, ""); !errors.Is(err, want) {
			t.Errorf("Remove(%q) = %v, want %v", path, err, want)
		}
	}
	if calls := rec.Calls(); len(calls) != 0 || opens != 0 {
		t.Errorf("the refusals reached the driver (%d calls) or the store (%d opens)", len(calls), opens)
	}

	s, rec = newStore(t, root(), file("F", "a.txt", blobfs.StatusAvailable, 1))
	if _, err := s.Remove(ctx, "/a.txt", ""); !errors.Is(err, files.ErrNoStorage) {
		t.Errorf("Remove without an object store = %v, want ErrNoStorage", err)
	}
	if got := ops(rec); got != "query query" {
		t.Errorf("ops = %q, want the resolution and nothing begun", got)
	}

	s, rec, _ = writeStore(t, &opens, root(), noFile())
	if _, err := s.Remove(ctx, "/a.txt", ""); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("Remove of a missing file = %v, want ErrNotFound", err)
	}
	if got := ops(rec); got != "query query" {
		t.Errorf("ops = %q", got)
	}
}

// TestRemoveDirectoryIsOneTransaction proves rmdir resolves the path on
// the pool and then removes the owner row and the directory in one
// transaction, the owner row first; that the root and a trailing slash
// are refused before any SQL; and that the library's refusal of a
// directory with contents reaches the caller as ErrNotEmpty with the
// transaction rolled back.
func TestRemoveDirectoryIsOneTransaction(t *testing.T) {
	ctx := context.Background()
	s, rec := newStore(t, root(), directory("D", blobfs.RootID, "docs"), sqltest.Response{Affected: 0}, affected())
	dir, err := s.RemoveDirectory(ctx, "/docs")
	if err != nil || dir.ID != "D" {
		t.Fatalf("RemoveDirectory = %+v, %v", dir, err)
	}
	if got := ops(rec); got != "query query begin exec exec commit" {
		t.Errorf("ops = %q", got)
	}
	execs := rec.SQL(sqltest.OpExec)
	if !strings.HasPrefix(execs[0], "DELETE FROM directory_owner") || !strings.HasPrefix(execs[1], "DELETE FROM blobfs_directory") {
		t.Errorf("execs = %q, want the owner row then the directory", execs)
	}

	s, rec = newStore(t)
	for path, want := range map[string]error{"/": blobfs.ErrRootDirectory, "/docs/": blobfs.ErrInvalidPath} {
		if _, err := s.RemoveDirectory(ctx, path); !errors.Is(err, want) {
			t.Errorf("RemoveDirectory(%q) = %v, want %v", path, err, want)
		}
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("the refusals reached the driver: %+v", calls)
	}

	s, rec = newStore(t, root(), directory("D", blobfs.RootID, "docs"), affected(), refusedBy(blobfs.ConstraintForeignKeyFileDirectory))
	if _, err := s.RemoveDirectory(ctx, "/docs"); !errors.Is(err, blobfs.ErrNotEmpty) {
		t.Errorf("RemoveDirectory of a directory with files = %v, want ErrNotEmpty", err)
	}
	if got := ops(rec); got != "query query begin exec exec rollback" {
		t.Errorf("ops = %q, want the refused removal rolled back", got)
	}
}

// TestRemoveTreeWalksChildrenFirst proves rm -r against the script: the
// target is resolved, each directory is listed until empty (directories
// then files), a file goes through the delete steps, a directory is
// removed after its contents with its owner row, the target last, and
// the observer sees each step in order: a file removed, a directory found
// empty, that directory removed. The root and a trailing slash are
// refused before any SQL.
func TestRemoveTreeWalksChildrenFirst(t *testing.T) {
	ctx := context.Background()
	opens := 0
	s, rec, _ := writeStore(t, &opens,
		root(), directory("D", blobfs.RootID, "docs"),
		// docs: one child directory, then no more.
		listing(directoryColumns, true, 1, "sub"),
		// sub: no directories, one file, then no more; then sub is removed.
		listing(directoryColumns, true, 0),
		listing(fileColumns, true, 1, "a.txt"),
		affected(), file("id-a.txt", "a.txt", blobfs.StatusDeleting, 2), bookmarkTotal(0), affected(),
		listing(fileColumns, true, 0),
		sqltest.Response{Affected: 0}, affected(),
		// docs again: no directories, no files; then docs is removed.
		listing(directoryColumns, true, 0),
		listing(fileColumns, true, 0),
		sqltest.Response{Affected: 1}, affected(),
	)
	var events []string
	res, err := s.RemoveTree(ctx, "/docs", func(ev files.RemovalEvent) {
		events = append(events, string(ev.Kind)+" "+ev.Path)
	})
	if err != nil {
		t.Fatalf("RemoveTree: %v", err)
	}
	if res.Files != 1 || res.Directories != 2 {
		t.Errorf("RemoveTree = %+v, want 1 file and 2 directories", res)
	}
	want := "file /docs/sub/a.txt|emptied /docs/sub|directory /docs/sub|emptied /docs|directory /docs"
	if got := strings.Join(events, "|"); got != want {
		t.Errorf("events = %q\nwant     %q", got, want)
	}
	if rec.Pending() != 0 {
		t.Errorf("%d scripted responses were not consumed", rec.Pending())
	}

	s, rec, _ = writeStore(t, &opens)
	for path, want := range map[string]error{"/": blobfs.ErrRootDirectory, "/docs/": blobfs.ErrInvalidPath} {
		if _, err := s.RemoveTree(ctx, path, nil); !errors.Is(err, want) {
			t.Errorf("RemoveTree(%q) = %v, want %v", path, err, want)
		}
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("the refusals reached the driver: %+v", calls)
	}
}

// TestStorageDelete proves the adapter's delete: the object goes, a
// second delete of the same key is success, a missing container is
// ErrContainerMissing with the store's not-found reachable rather than
// success, and an outage is ErrStorageUnavailable.
func TestStorageDelete(t *testing.T) {
	ctx := context.Background()
	st, fake := fakeStorage(t)
	stored(t, fake, "k")
	if err := st.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if held(t, fake, "k") {
		t.Error("the object is still held after Delete")
	}
	if err := st.Delete(ctx, "k"); err != nil {
		t.Errorf("Delete of a missing key = %v, want success", err)
	}
	fake.DropContainer()
	if err := st.Delete(ctx, "k"); !errors.Is(err, files.ErrContainerMissing) || !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("Delete with the container gone = %v, want ErrContainerMissing over storage.ErrNotFound", err)
	}
	fake.Down.Store(true)
	if err := st.Delete(ctx, "k"); !errors.Is(err, files.ErrStorageUnavailable) {
		t.Errorf("Delete during an outage = %v, want ErrStorageUnavailable", err)
	}
}
