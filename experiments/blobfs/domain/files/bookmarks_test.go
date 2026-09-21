package files_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/domain/files"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// bookmarkColumns is the scan contract of the bookmark read model, in the
// order the projection's base selects them.
var bookmarkColumns = []string{"unit_id", "file_id", "active", "path", "name", "status", "size", "content_type", "created_at", "updated_at"}

// bookmarks scripts one page of the bookmark read model: one row per
// path, none active, each available with a size.
func bookmarks(unit string, paths ...string) sqltest.Response {
	resp := sqltest.Response{Columns: bookmarkColumns}
	now := time.Now()
	for i, p := range paths {
		resp.Rows = append(resp.Rows, []driver.Value{unit, "F" + path.Base(p), false, p, path.Base(p), "available", int64(i + 1), "text/plain", now, now})
	}
	return resp
}

// counted scripts the projection's count statement.
func counted(n int64) sqltest.Response {
	return sqltest.Response{Columns: []string{"count"}, Rows: [][]driver.Value{{n}}}
}

// violation scripts a constraint violation as the dialect classifies it.
func violation(constraint string, class error) sqltest.Response {
	return sqltest.Response{Err: &sqlate.ConstraintError{Constraint: constraint, Class: class, Err: errors.New("driver")}}
}

// TestListBookmarksRunsInOneReadOnlyRepeatableReadTransaction is the
// stage gate's hermetic proof that the projection's total agrees with its
// page: one bookmark ls is one transaction, begun read-only at repeatable
// read, holding the count statement and the page statement, then
// committed. Both statements read the bookmark read model filtered by the
// unit, the page in path order with file_id as the tie-breaker.
func TestListBookmarksRunsInOneReadOnlyRepeatableReadTransaction(t *testing.T) {
	unit := blobfs.NewID()
	s, rec := newStore(t, counted(3), bookmarks(unit, "/a/b.txt", "/c.txt"))
	p, err := s.ListBookmarks(context.Background(), unit, files.Listing{Page: 1, Size: 2})
	if err != nil {
		t.Fatalf("ListBookmarks: %v", err)
	}
	if got := ops(rec); got != "begin query query commit" {
		t.Errorf("ops = %q, want one transaction around the count and the page", got)
	}
	begin := rec.Calls()[0]
	if !begin.TxOptions.ReadOnly {
		t.Errorf("the transaction is not read-only: %+v", begin.TxOptions)
	}
	if begin.TxOptions.Isolation != driver.IsolationLevel(sql.LevelRepeatableRead) {
		t.Errorf("the transaction's isolation is %v, want repeatable read", sql.IsolationLevel(begin.TxOptions.Isolation))
	}
	count, page := rec.Calls()[1], rec.Calls()[2]
	for _, q := range []sqltest.Call{count, page} {
		if !strings.Contains(q.SQL, "FROM bookmark b") || !strings.Contains(q.SQL, "JOIN blobfs_file f ON f.id = b.file_id") || !strings.Contains(q.SQL, "WITH RECURSIVE up") {
			t.Errorf("query lacks the bookmark join or the upward walk:\n%s", q.SQL)
		}
		if !strings.Contains(q.SQL, "WHERE q.unit_id = CAST($1 AS uuid)") || q.Args[0] != unit {
			t.Errorf("query does not filter by the unit outside the base:\n%s\n%v", q.SQL, q.Args)
		}
	}
	if !strings.HasPrefix(count.SQL, "SELECT COUNT(*) FROM (") {
		t.Errorf("the first statement is not the count:\n%s", count.SQL)
	}
	if !strings.HasSuffix(page.SQL, " ORDER BY q.path, q.file_id OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY") {
		t.Errorf("the page is not in path order with the key as the tie-breaker:\n%s", page.SQL)
	}
	if args := page.Args; len(args) != 3 || args[1] != 0 || args[2] != 2 {
		t.Errorf("the page bound %v, want the unit, offset 0, fetch 2", args)
	}
	if p.Total != 3 || len(p.Rows) != 2 || p.Rows[0].Path != "/a/b.txt" || p.Rows[1].Name != "c.txt" || *p.Rows[1].Size != 2 || p.Rows[0].UnitID != unit {
		t.Errorf("page = %+v", p)
	}
	if rec.Pending() != 0 || rec.RowsLeaked() != 0 {
		t.Errorf("%d responses pending, %d row sets leaked", rec.Pending(), rec.RowsLeaked())
	}

	// A failure inside rolls the transaction back and reaches the caller.
	boom := errors.New("boom")
	s, rec = newStore(t, sqltest.Response{Err: boom})
	if _, err := s.ListBookmarks(context.Background(), unit, files.Listing{Page: 1, Size: 2}); !errors.Is(err, boom) {
		t.Errorf("ListBookmarks with a failing count = %v, want the failure", err)
	}
	if got := ops(rec); got != "begin query rollback" {
		t.Errorf("ops after a failure = %q, want begin query rollback", got)
	}
}

// TestListBookmarksLowersTheDirectives proves database.go's lowering of a
// bookmark listing: every sort term reaches the projection in order with
// file_id appended, a sort by file_id itself gains no second term, the
// page and size become the offset and the fetch count, TotalNone still
// runs the count (the projection cannot skip it) and reports NoTotal, and
// an unknown sort field is refused before any statement runs.
func TestListBookmarksLowersTheDirectives(t *testing.T) {
	ctx := context.Background()
	unit := blobfs.NewID()
	s, rec := newStore(t, counted(11), bookmarks(unit, "/z.txt"))
	l := files.Listing{Page: 3, Size: 5, Sort: []files.Sort{{Field: "size", Descending: true}, {Field: "created_at"}}}
	if _, err := s.ListBookmarks(ctx, unit, l); err != nil {
		t.Fatalf("ListBookmarks: %v", err)
	}
	page := rec.Calls()[2]
	if !strings.HasSuffix(page.SQL, " ORDER BY q.size DESC, q.created_at, q.file_id OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY") {
		t.Errorf("the page lost a term or the key:\n%s", page.SQL)
	}
	if args := page.Args; args[1] != 10 || args[2] != 5 {
		t.Errorf("page 3 of size 5 bound %v, want offset 10 and fetch 5", args)
	}

	s, rec = newStore(t, counted(11), bookmarks(unit))
	p, err := s.ListBookmarks(ctx, unit, files.Listing{Page: 1, Size: 5, Total: files.TotalNone, Sort: []files.Sort{{Field: "file_id"}}})
	if err != nil {
		t.Fatalf("ListBookmarks under TotalNone: %v", err)
	}
	if n := len(rec.SQL(sqltest.OpQuery)); n != 2 {
		t.Errorf("the projection ran %d queries under TotalNone, want 2: it cannot skip its count", n)
	}
	if p.Total != files.NoTotal || len(p.Rows) != 0 {
		t.Errorf("page under TotalNone = %+v, want NoTotal and no rows", p)
	}
	if !strings.HasSuffix(rec.Calls()[2].SQL, " ORDER BY q.file_id OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY") {
		t.Errorf("a sort by the key gained a second term:\n%s", rec.Calls()[2].SQL)
	}

	s, rec = newStore(t)
	_, err = s.ListBookmarks(ctx, unit, files.Listing{Page: 1, Size: 5, Sort: []files.Sort{{Field: "directory_id"}}})
	if !errors.Is(err, query.ErrDirectives) || !strings.Contains(err.Error(), "directory_id") {
		t.Errorf("a sort by an undeclared field = %v, want ErrDirectives naming it", err)
	}
	if got := ops(rec); got != "begin rollback" {
		t.Errorf("ops after the refusal = %q, want no statement", got)
	}
}

// TestAddBookmarkIsOneTransaction proves the transaction boundaries of a
// bookmark add: one transaction holds the parent's resolution, the file's
// lookup by name, and the insert, then commits; the insert binds the
// unit, the file's id, and the active flag. A deleting file is refused
// with ErrNotAvailable and a missing one with ErrNotFound, each after the
// lookup and with the transaction rolled back; a pending file is
// accepted; and a bad path is refused before any I/O.
func TestAddBookmarkIsOneTransaction(t *testing.T) {
	ctx := context.Background()
	unit := blobfs.NewID()
	s, rec := newStore(t, root(), file("F", "a.txt", blobfs.StatusAvailable, 2), affected())
	f, err := s.AddBookmark(ctx, "/a.txt", unit, true)
	if err != nil || f.ID != "F" {
		t.Fatalf("AddBookmark = %+v, %v", f, err)
	}
	if got := ops(rec); got != "begin query query exec commit" {
		t.Errorf("ops = %q", got)
	}
	insert := rec.Calls()[3]
	if !strings.HasPrefix(insert.SQL, "INSERT INTO bookmark (unit_id, file_id, active)") {
		t.Errorf("the exec is not the bookmark insert:\n%s", insert.SQL)
	}
	if args := insert.Args; len(args) != 3 || args[0] != unit || args[1] != "F" || args[2] != true {
		t.Errorf("the insert bound %v, want the unit, the file, and active", args)
	}

	s, rec = newStore(t, root(), file("P", "a.txt", blobfs.StatusPending, 1), affected())
	if f, err := s.AddBookmark(ctx, "/a.txt", unit, false); err != nil || f.Status != blobfs.StatusPending {
		t.Errorf("AddBookmark of a pending file = %+v, %v; want the pending row bookmarked", f, err)
	}
	if args := rec.Calls()[3].Args; args[2] != false {
		t.Errorf("the insert bound active %v without --active", args[2])
	}

	s, rec = newStore(t, root(), file("D", "a.txt", blobfs.StatusDeleting, 1))
	if _, err := s.AddBookmark(ctx, "/a.txt", unit, false); !errors.Is(err, files.ErrNotAvailable) || !strings.Contains(err.Error(), "deleting") {
		t.Errorf("AddBookmark of a deleting file = %v, want ErrNotAvailable naming the status", err)
	}
	if got := ops(rec); got != "begin query query rollback" {
		t.Errorf("ops after the deleting refusal = %q", got)
	}

	s, rec = newStore(t, root(), noFile())
	if _, err := s.AddBookmark(ctx, "/a.txt", unit, false); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("AddBookmark of a missing file = %v, want ErrNotFound", err)
	}
	if got := ops(rec); got != "begin query query rollback" {
		t.Errorf("ops after the missing file = %q", got)
	}

	s, rec = newStore(t)
	for p, want := range map[string]error{"/": blobfs.ErrRootDirectory, "/a/": blobfs.ErrInvalidPath, "a": blobfs.ErrInvalidPath} {
		if _, err := s.AddBookmark(ctx, p, unit, false); !errors.Is(err, want) {
			t.Errorf("AddBookmark(%s) = %v, want %v", p, err, want)
		}
	}
	if n := len(rec.Calls()); n != 0 {
		t.Errorf("the refusals reached the driver with %d calls", n)
	}
}

// TestAddBookmarkClassifiesTheConstraints proves the consumer's mapping
// from the bookmark table's constraint names to its sentinels: the
// primary key is ErrAlreadyBookmarked, the partial unique index is
// ErrActiveBookmark, and the foreign key is blobfs.ErrNotFound, each with
// the sqlate.ConstraintError still reachable and the transaction rolled
// back. A constraint the mapping does not name, or a named one reported
// under another class, passes through unclassified.
func TestAddBookmarkClassifiesTheConstraints(t *testing.T) {
	ctx := context.Background()
	unit := blobfs.NewID()
	for _, tc := range []struct {
		constraint string
		class      error
		want       error
	}{
		{files.ConstraintPrimaryKeyBookmark, sqlate.ErrUniqueViolation, files.ErrAlreadyBookmarked},
		{files.ConstraintUniqueBookmarkActive, sqlate.ErrUniqueViolation, files.ErrActiveBookmark},
		{files.ConstraintForeignKeyBookmarkFile, sqlate.ErrForeignKeyViolation, blobfs.ErrNotFound},
	} {
		s, rec := newStore(t, root(), file("F", "a.txt", blobfs.StatusAvailable, 2), violation(tc.constraint, tc.class))
		_, err := s.AddBookmark(ctx, "/a.txt", unit, true)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: AddBookmark = %v, want %v", tc.constraint, err, tc.want)
		}
		var ce *sqlate.ConstraintError
		if !errors.As(err, &ce) || ce.Constraint != tc.constraint {
			t.Errorf("%s: the constraint error is not reachable: %v", tc.constraint, err)
		}
		var ve *blobfs.ViolationError
		if !errors.As(err, &ve) || ve.Sentinel != tc.want || ve.Constraint != tc.constraint {
			t.Errorf("%s: the wrapper is not reachable or does not carry the sentinel and the constraint: %v", tc.constraint, err)
		}
		if !strings.HasSuffix(err.Error(), tc.want.Error()+" (constraint "+tc.constraint+")") || strings.Contains(err.Error(), "driver") {
			t.Errorf("%s: message = %q, want it to end with the sentinel and the constraint and to hide the driver's text", tc.constraint, err)
		}
		if got := ops(rec); got != "begin query query exec rollback" {
			t.Errorf("%s: ops = %q", tc.constraint, got)
		}
	}
	for _, tc := range []struct {
		label      string
		constraint string
		class      error
	}{
		{"an unnamed constraint", "cc_bookmark_other", sqlate.ErrCheckViolation},
		{"a named constraint under another class", files.ConstraintPrimaryKeyBookmark, sqlate.ErrForeignKeyViolation},
	} {
		s, _ := newStore(t, root(), file("F", "a.txt", blobfs.StatusAvailable, 2), violation(tc.constraint, tc.class))
		_, err := s.AddBookmark(ctx, "/a.txt", unit, true)
		for _, sentinel := range []error{files.ErrAlreadyBookmarked, files.ErrActiveBookmark, blobfs.ErrNotFound} {
			if errors.Is(err, sentinel) {
				t.Errorf("%s classified as %v: %v", tc.label, sentinel, err)
			}
		}
		if !errors.Is(err, tc.class) {
			t.Errorf("%s lost its class: %v", tc.label, err)
		}
	}
}

// TestRemoveBookmark proves a bookmark rm resolves the file on the pool
// and deletes the unit's row for it in one statement, that no row
// affected is ErrNoBookmark, and that a missing file is ErrNotFound
// before any delete.
func TestRemoveBookmark(t *testing.T) {
	ctx := context.Background()
	unit := blobfs.NewID()
	s, rec := newStore(t, root(), file("F", "a.txt", blobfs.StatusAvailable, 2), affected())
	f, err := s.RemoveBookmark(ctx, "/a.txt", unit)
	if err != nil || f.ID != "F" {
		t.Fatalf("RemoveBookmark = %+v, %v", f, err)
	}
	if got := ops(rec); got != "query query exec" {
		t.Errorf("ops = %q, want the two reads and the delete on the pool", got)
	}
	del := rec.Calls()[2]
	if !strings.HasPrefix(del.SQL, "DELETE FROM bookmark") || len(del.Args) != 2 || del.Args[0] != unit || del.Args[1] != "F" {
		t.Errorf("the delete is %s with %v", del.SQL, del.Args)
	}

	s, _ = newStore(t, root(), file("F", "a.txt", blobfs.StatusAvailable, 2), sqltest.Response{Affected: 0})
	if _, err := s.RemoveBookmark(ctx, "/a.txt", unit); !errors.Is(err, files.ErrNoBookmark) {
		t.Errorf("RemoveBookmark of a file the unit has not bookmarked = %v, want ErrNoBookmark", err)
	}

	s, rec = newStore(t, root(), noFile())
	if _, err := s.RemoveBookmark(ctx, "/a.txt", unit); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("RemoveBookmark of a missing file = %v, want ErrNotFound", err)
	}
	if got := ops(rec); got != "query query" {
		t.Errorf("ops after the missing file = %q, want no delete", got)
	}
}
