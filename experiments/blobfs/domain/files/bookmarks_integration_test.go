//go:build integration

package files_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/domain/files"
	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// bookmark adds a bookmark and fails the test on any error.
func (e env) bookmark(t *testing.T, path, unit string, active bool) blobfs.File {
	t.Helper()
	f, err := e.store.AddBookmark(e.ctx, path, unit, active)
	if err != nil {
		t.Fatalf("AddBookmark(%s, %s, %v): %v", path, unit, active, err)
	}
	return f
}

// bookmarksOf lists a unit's bookmarks and fails the test on any error.
func (e env) bookmarksOf(t *testing.T, unit string, l files.Listing) files.Page[files.BookmarkedFile] {
	t.Helper()
	p, err := e.store.ListBookmarks(e.ctx, unit, l)
	if err != nil {
		t.Fatalf("ListBookmarks(%s, %+v): %v", unit, l, err)
	}
	return p
}

// paths returns the paths of a bookmark page.
func paths(rows []files.BookmarkedFile) []string {
	out := make([]string, 0, len(rows))
	for _, b := range rows {
		out = append(out, b.Path)
	}
	return out
}

// TestBookmarks is the stage gate's engine proof of the bookmark
// commands: add, ls, and rm through the store; the duplicate add; the
// one-active refusal with its classifiable error and the constraint
// named; the missing file; the deleting file; the pending file, which can
// be bookmarked and lists with its status; the per-row path at depth six
// and at the root; paging and sorting with the total on every page; the
// unit filter, under which another unit's bookmarks never appear; and rm
// of the active bookmark, after which another can be made active.
func TestBookmarks(t *testing.T) {
	e := open(t)
	unit, other := blobfs.NewID(), blobfs.NewID()
	e.mkdir(t, "/reports", "")
	reports := e.mkdir(t, "/reports/2026", "")
	dir := "/d1"
	deep := e.mkdir(t, dir, "")
	for _, name := range []string{"d2", "d3", "d4", "d5", "d6"} {
		dir += "/" + name
		deep = e.mkdir(t, dir, "")
	}
	insertFile(e.ctx, t, e.db, deep.ID, "plan.txt")
	insertFile(e.ctx, t, e.db, blobfs.RootID, "root.txt")
	for _, name := range []string{"x.txt", "y.txt", "z.txt", "w.txt", "gone.txt"} {
		insertFile(e.ctx, t, e.db, reports.ID, name)
	}
	if _, err := e.db.ExecContext(e.ctx, "UPDATE blobfs_file SET status = 'deleting' WHERE name = 'gone.txt'"); err != nil {
		t.Fatal(err)
	}
	_, err := e.store.Put(e.ctx, files.PutRequest{Path: "/reports/2026/draft.bin", Body: strings.NewReader("x"), StopAfter: files.StepInsert})
	if !errors.Is(err, files.ErrStopped) {
		t.Fatalf("Put --fail-after insert = %v", err)
	}

	// add: the file's row comes back; a second add of the file is refused
	// under the primary key, active or not.
	deepPath := dir + "/plan.txt"
	f := e.bookmark(t, deepPath, unit, false)
	if f.Name != "plan.txt" || f.DirectoryID != deep.ID {
		t.Errorf("AddBookmark returned %+v", f)
	}
	for _, active := range []bool{false, true} {
		_, err := e.store.AddBookmark(e.ctx, deepPath, unit, active)
		if !errors.Is(err, files.ErrAlreadyBookmarked) {
			t.Errorf("a second add (active %v) = %v, want ErrAlreadyBookmarked", active, err)
		}
		if name := livetest.Constraint(t, err, sqlate.ErrUniqueViolation); name != files.ConstraintPrimaryKeyBookmark {
			t.Errorf("a second add violated %q, want the primary key", name)
		}
	}

	// The one-active rule: a second active bookmark is refused with the
	// classifiable error and the index named; the first stays active; an
	// inactive bookmark of the same file is fine, and so is the same file
	// active for another unit.
	e.bookmark(t, "/reports/2026/x.txt", unit, true)
	_, err = e.store.AddBookmark(e.ctx, "/reports/2026/y.txt", unit, true)
	if !errors.Is(err, files.ErrActiveBookmark) {
		t.Errorf("a second active add = %v, want ErrActiveBookmark", err)
	}
	if name := livetest.Constraint(t, err, sqlate.ErrUniqueViolation); name != files.ConstraintUniqueBookmarkActive {
		t.Errorf("a second active add violated %q, want uq_bookmark_active", name)
	}
	if n := e.count(t, "SELECT COUNT(*) FROM bookmark WHERE unit_id = $1", unit); n != 2 {
		t.Errorf("after the refusal the unit has %d bookmarks, want 2", n)
	}
	e.bookmark(t, "/reports/2026/y.txt", unit, false)
	e.bookmark(t, "/reports/2026/x.txt", other, true)
	e.bookmark(t, "/root.txt", unit, false)
	e.bookmark(t, "/reports/2026/draft.bin", unit, false)

	// The refusals: a missing file, a missing parent, a directory, a
	// deleting file; and a path the store refuses before any I/O.
	for path, want := range map[string]error{
		"/reports/2026/missing.txt": blobfs.ErrNotFound,
		"/nowhere/x.txt":            blobfs.ErrNotFound,
		"/reports":                  blobfs.ErrNotFound,
		"/reports/2026/gone.txt":    files.ErrNotAvailable,
		"/":                         blobfs.ErrRootDirectory,
		"/reports/2026/":            blobfs.ErrInvalidPath,
	} {
		if _, err := e.store.AddBookmark(e.ctx, path, unit, false); !errors.Is(err, want) {
			t.Errorf("AddBookmark(%s) = %v, want %v", path, err, want)
		}
	}
	if n := e.count(t, "SELECT COUNT(*) FROM bookmark"); n != 6 {
		t.Errorf("%d bookmarks exist, want 6: the refusals wrote nothing", n)
	}

	// ls: the unit's bookmarks in path order with their full paths, the
	// active marker on the active one, the pending file with its status
	// and no size, and nothing of the other unit's.
	p := e.bookmarksOf(t, unit, files.Listing{Page: 1, Size: 10})
	want := []string{deepPath, "/reports/2026/draft.bin", "/reports/2026/x.txt", "/reports/2026/y.txt", "/root.txt"}
	if !slices.Equal(paths(p.Rows), want) || p.Total != 5 {
		t.Errorf("ListBookmarks = %v, total %d; want %v, total 5", paths(p.Rows), p.Total, want)
	}
	for _, b := range p.Rows {
		if b.UnitID != unit {
			t.Errorf("row %+v belongs to another unit", b)
		}
		switch b.Path {
		case "/reports/2026/x.txt":
			if !b.Active || b.Size == nil || b.Status != blobfs.StatusAvailable || b.Name != "x.txt" {
				t.Errorf("the active bookmark = %+v", b)
			}
		case "/reports/2026/draft.bin":
			if b.Active || b.Size != nil || b.Status != blobfs.StatusPending {
				t.Errorf("the pending file's bookmark = %+v", b)
			}
		default:
			if b.Active || b.Status != blobfs.StatusAvailable || b.CreatedAt.IsZero() {
				t.Errorf("row %+v", b)
			}
		}
	}
	p = e.bookmarksOf(t, other, files.Listing{Page: 1, Size: 10})
	if !slices.Equal(paths(p.Rows), []string{"/reports/2026/x.txt"}) || p.Total != 1 || !p.Rows[0].Active {
		t.Errorf("ListBookmarks of the other unit = %+v", p)
	}
	p = e.bookmarksOf(t, blobfs.NewID(), files.Listing{Page: 1, Size: 10})
	if len(p.Rows) != 0 || p.Total != 0 {
		t.Errorf("ListBookmarks of a unit with none = %+v", p)
	}

	// Paging and sorting: pages of two concatenate to the whole listing
	// with the total on each, a descending path sort is the exact reverse,
	// a sort by name and by size agree with an independent query, an empty
	// later page keeps the exact total (the count is a statement of its
	// own), TotalNone reports NoTotal, and an unknown field is refused.
	var paged []string
	for page := 1; page <= 3; page++ {
		p := e.bookmarksOf(t, unit, files.Listing{Page: page, Size: 2})
		if p.Total != 5 {
			t.Errorf("page %d total = %d, want 5", page, p.Total)
		}
		// Five bookmarks at size 2: More is derived from the count, so
		// pages 1 and 2 have more and page 3 does not.
		if p.More != (page < 3) {
			t.Errorf("page %d more = %v, want %v", page, p.More, page < 3)
		}
		paged = append(paged, paths(p.Rows)...)
	}
	if !slices.Equal(paged, want) {
		t.Errorf("pages concatenated = %v, want %v", paged, want)
	}
	p = e.bookmarksOf(t, unit, files.Listing{Page: 1, Size: 10, Sort: []files.Sort{{Field: "path", Descending: true}}})
	reversed := slices.Clone(want)
	slices.Reverse(reversed)
	if !slices.Equal(paths(p.Rows), reversed) {
		t.Errorf("path desc = %v, want %v", paths(p.Rows), reversed)
	}
	for _, field := range []string{"name", "size"} {
		wantNames := e.strings1(t, fmt.Sprintf("SELECT f.name FROM bookmark b JOIN blobfs_file f ON f.id = b.file_id WHERE b.unit_id = $1 ORDER BY f.%s DESC, b.file_id", field), unit)
		p := e.bookmarksOf(t, unit, files.Listing{Page: 1, Size: 10, Sort: []files.Sort{{Field: field, Descending: true}}})
		var got []string
		for _, b := range p.Rows {
			got = append(got, b.Name)
		}
		if !slices.Equal(got, wantNames) {
			t.Errorf("sort by %s desc = %v, want %v", field, got, wantNames)
		}
	}
	p = e.bookmarksOf(t, unit, files.Listing{Page: 4, Size: 2})
	if len(p.Rows) != 0 || p.Total != 5 || p.More {
		t.Errorf("an empty later page = %+v, want no rows, the exact total, and no More", p)
	}
	p = e.bookmarksOf(t, unit, files.Listing{Page: 1, Size: 2, Total: files.TotalNone})
	if p.Total != files.NoTotal || len(p.Rows) != 2 || !p.More {
		t.Errorf("TotalNone = %+v, want NoTotal, 2 rows, and More from the count that still runs", p)
	}
	if _, err := e.store.ListBookmarks(e.ctx, unit, files.Listing{Page: 1, Size: 2, Sort: []files.Sort{{Field: "key"}}}); !errors.Is(err, query.ErrDirectives) {
		t.Errorf("a sort by an undeclared field = %v, want ErrDirectives", err)
	}

	// rm: the active bookmark can be removed, after which another can be
	// made active; a second rm is ErrNoBookmark; a missing file is
	// ErrNotFound; the other unit's bookmark of the same file survives.
	if f, err := e.store.RemoveBookmark(e.ctx, "/reports/2026/x.txt", unit); err != nil || f.Name != "x.txt" {
		t.Fatalf("RemoveBookmark of the active bookmark = %+v, %v", f, err)
	}
	e.bookmark(t, "/reports/2026/z.txt", unit, true)
	if _, err := e.store.RemoveBookmark(e.ctx, "/reports/2026/x.txt", unit); !errors.Is(err, files.ErrNoBookmark) {
		t.Errorf("a second rm = %v, want ErrNoBookmark", err)
	}
	if _, err := e.store.RemoveBookmark(e.ctx, "/reports/2026/missing.txt", unit); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("rm of a missing file = %v, want ErrNotFound", err)
	}
	p = e.bookmarksOf(t, unit, files.Listing{Page: 1, Size: 10})
	if !slices.Equal(paths(p.Rows), []string{deepPath, "/reports/2026/draft.bin", "/reports/2026/y.txt", "/reports/2026/z.txt", "/root.txt"}) || !p.Rows[3].Active {
		t.Errorf("after rm and a new active add = %v", paths(p.Rows))
	}
	p = e.bookmarksOf(t, other, files.Listing{Page: 1, Size: 10})
	if !slices.Equal(paths(p.Rows), []string{"/reports/2026/x.txt"}) {
		t.Errorf("the other unit's bookmark after the unit's rm = %v", paths(p.Rows))
	}

	// The foreign key holds the file: a bookmarked file cannot be deleted
	// from under its bookmark, which stage 12's rm will classify.
	_, err = e.db.ExecContext(e.ctx, "DELETE FROM blobfs_file WHERE id = $1", p.Rows[0].FileID)
	if name := livetest.Constraint(t, err, sqlate.ErrForeignKeyViolation); name != files.ConstraintForeignKeyBookmarkFile {
		t.Errorf("deleting a bookmarked file violated %q, want fk_bookmark_file", name)
	}
}

// TestAddBookmarkOfAFileRemovedMeanwhile proves the hold's refusal of a
// file that goes away on the engine: a second connection holds an
// uncommitted delete of the file's row, the add resolves the file (the
// delete is not visible yet) and blocks on the row at its hold, and once
// the delete commits the hold matches nothing, reads the row gone, and
// refuses with blobfs.ErrNotFound before the insert runs, so the foreign
// key fk_bookmark_file is never reached and no bookmark is written.
func TestAddBookmarkOfAFileRemovedMeanwhile(t *testing.T) {
	e := open(t)
	unit := blobfs.NewID()
	fileID := insertFile(e.ctx, t, e.db, blobfs.RootID, "doomed.txt")
	writer := e.second(t)
	deleted, commit := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := writer.Transact(e.ctx, func(tx *sqlate.Tx) (struct{}, error) {
			if _, err := tx.ExecContext(e.ctx, "DELETE FROM blobfs_file WHERE id = $1", fileID); err != nil {
				return struct{}{}, err
			}
			close(deleted)
			<-commit
			return struct{}{}, nil
		})
		done <- err
	}()
	<-deleted
	added := make(chan error, 1)
	go func() {
		_, err := e.store.AddBookmark(e.ctx, "/doomed.txt", unit, true)
		added <- err
	}()
	select {
	case err := <-added:
		t.Fatalf("the add returned %v before the delete committed; it should block on the row", err)
	case <-time.After(500 * time.Millisecond):
	}
	close(commit)
	if err := <-done; err != nil {
		t.Fatalf("the delete: %v", err)
	}
	err := <-added
	if !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("AddBookmark of the file removed meanwhile = %v, want ErrNotFound", err)
	}
	var ce *sqlate.ConstraintError
	if errors.As(err, &ce) {
		t.Errorf("the add reached the foreign key (%s); the hold refuses it first", ce.Constraint)
	}
	if n := e.count(t, "SELECT COUNT(*) FROM bookmark"); n != 0 {
		t.Errorf("%d bookmarks exist after the refusal", n)
	}
}

// TestBookmarkTotalAgreesUnderConcurrentWrites is the stage gate's engine
// proof that the projection's total agrees with its page: a second
// connection adds and removes the unit's bookmarks as fast as it can, one
// statement at a time, while the listing runs repeatedly with a page big
// enough to hold every row. The count and the page are separate
// statements, so on the pool the total could differ from the rows by the
// bookmarks committed between them; under the read-only repeatable-read
// transaction every result has a total equal to its own rows.
func TestBookmarkTotalAgreesUnderConcurrentWrites(t *testing.T) {
	e := open(t)
	unit := blobfs.NewID()
	dir := e.mkdir(t, "/pool", "")
	const pool = 100
	fileIDs := make([]string, pool)
	for i := range fileIDs {
		fileIDs[i] = insertFile(e.ctx, t, e.db, dir.ID, fmt.Sprintf("f%03d.txt", i))
	}
	writer := e.second(t)
	ctx, cancel := context.WithCancel(e.ctx)
	done := make(chan error, 1)
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			for _, id := range fileIDs {
				if _, err := writer.ExecContext(ctx, "INSERT INTO bookmark (unit_id, file_id, active) VALUES ($1, $2, false)", unit, id); err != nil && ctx.Err() == nil {
					done <- err
					return
				}
			}
			for _, id := range fileIDs {
				if _, err := writer.ExecContext(ctx, "DELETE FROM bookmark WHERE unit_id = $1 AND file_id = $2", unit, id); err != nil && ctx.Err() == nil {
					done <- err
					return
				}
			}
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	runs, nonEmpty := 0, 0
	for time.Now().Before(deadline) {
		p := e.bookmarksOf(t, unit, files.Listing{Page: 1, Size: 10 * pool})
		runs++
		if p.Total != len(p.Rows) {
			t.Fatalf("run %d: total %d but %d rows; the count and the page saw different snapshots", runs, p.Total, len(p.Rows))
		}
		if len(p.Rows) > 0 {
			nonEmpty++
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("writer: %v", err)
	}
	if runs < 10 || nonEmpty < 5 {
		t.Errorf("%d listings, %d with rows; too few to have interleaved", runs, nonEmpty)
	}
	t.Logf("%d listings agreed with their own rows while the writer added and removed bookmarks", runs)
}
