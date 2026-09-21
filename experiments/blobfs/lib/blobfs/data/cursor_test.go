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
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

// fileRowAt is fileRow with the timestamps fixed, so a cursor over
// created_at carries a known value.
func fileRowAt(id, name string, at time.Time) []driver.Value {
	return []driver.Value{id, blobfs.RootID, name, "available", id + "/" + name, nil, "text/plain", nil, int64(1), at, at}
}

// files scripts one response of the plain file listing with one row per
// name, all at the same time.
func files(at time.Time, names ...string) sqltest.Response {
	resp := sqltest.Response{Columns: fileColumns}
	for _, name := range names {
		resp.Rows = append(resp.Rows, fileRowAt("id-"+name, name, at))
	}
	return resp
}

// TestCursorContinuesThePage proves the cursor round trip on the scripted
// driver: an offset page under TotalExact fetches one row beyond its size,
// returns only the page, keeps its total, reports More, and fills Next;
// the page read with After runs the plain statement whatever Total says,
// carries the keyset predicate after the anchor and before ORDER BY,
// binds the last row's name and offset zero, ignores the page number, and
// reports NoTotal; the last page has no More and no Next.
func TestCursorContinuesThePage(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	dir := blobfs.NewID()
	at := time.Now()
	counted := append(slices.Clone(fileColumns), "total")
	pool, rec := sqltest.Open(t,
		sqltest.Response{Columns: counted, Rows: [][]driver.Value{
			append(fileRowAt("f1", "a.txt", at), int64(5)),
			append(fileRowAt("f2", "b.txt", at), int64(5)),
			append(fileRowAt("f3", "c.txt", at), int64(5)),
		}},
		files(at, "c.txt", "d.txt", "e.txt"),
		files(at, "e.txt"),
	)
	db := sqlate.Wrap(pool, sqltest.Dialect{})

	first, err := s.ListFiles(ctx, db, dir, data.Listing{Page: 1, Size: 2})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(first.Rows) != 2 || first.Rows[1].Name != "b.txt" || first.Total != 5 || !first.More || first.Next == "" {
		t.Fatalf("page 1 = %d rows, total %d, more %v, next %q; want 2 rows, total 5, More, and a cursor", len(first.Rows), first.Total, first.More, first.Next)
	}
	second, err := s.ListFiles(ctx, db, dir, data.Listing{Size: 2, After: first.Next})
	if err != nil {
		t.Fatalf("page after %q: %v", first.Next, err)
	}
	if len(second.Rows) != 2 || second.Rows[0].Name != "c.txt" || second.Rows[1].Name != "d.txt" || second.Total != data.NoTotal || !second.More || second.Next == "" || second.Next == first.Next {
		t.Errorf("page 2 = %+v; want c and d, NoTotal, More, and a new cursor", second)
	}
	third, err := s.ListFiles(ctx, db, dir, data.Listing{Page: 7, Size: 2, After: second.Next})
	if err != nil {
		t.Fatalf("page after %q: %v", second.Next, err)
	}
	if len(third.Rows) != 1 || third.Rows[0].Name != "e.txt" || third.Total != data.NoTotal || third.More || third.Next != "" {
		t.Errorf("last page = %+v; want e alone, NoTotal, no More, no cursor", third)
	}

	queries := rec.SQL(sqltest.OpQuery)
	if !strings.HasSuffix(queries[0], "WHERE q.directory_id = CAST($1 AS uuid) ORDER BY q.name OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY") || !strings.Contains(queries[0], "COUNT(*) OVER ()") {
		t.Errorf("page 1 composed:\n%s", queries[0])
	}
	for i, q := range queries[1:] {
		if !strings.HasSuffix(q, "WHERE q.directory_id = CAST($1 AS uuid) AND q.name > CAST($2 AS text) ORDER BY q.name OFFSET $3 ROWS FETCH NEXT $4 ROWS ONLY") || strings.Contains(q, "COUNT(*) OVER ()") {
			t.Errorf("cursor page %d composed:\n%s", i+2, q)
		}
	}
	calls := rec.Calls()
	if got := calls[0].Args; !slices.Equal(got, []any{dir, 0, 3}) {
		t.Errorf("page 1 bound %v, want the directory, offset 0, fetch 3", got)
	}
	if got := calls[1].Args; !slices.Equal(got, []any{dir, "b.txt", 0, 3}) {
		t.Errorf("cursor page bound %v, want the directory, the last name, offset 0, fetch 3", got)
	}
	if got := calls[2].Args; !slices.Equal(got, []any{dir, "d.txt", 0, 3}) {
		t.Errorf("cursor page bound %v, want the directory, the last name, offset 0, fetch 3 (the page number is ignored)", got)
	}
	if n := rec.RowsLeaked(); n != 0 {
		t.Errorf("%d row sets leaked", n)
	}
}

// TestCursorTwoTermSort proves the expanded form and the tie-breaker's
// direction: a descending sort by created_at orders by created_at DESC and
// name DESC, the cursor carries the last row's timestamp at full precision
// in UTC and its name, and the replay binds them in the expanded predicate
// (a < x) OR (a = x AND b < y) with every occurrence bound on its own.
// A term after the key is not a cursor term: a sort by name then size
// continues by name alone, and size's nullability does not matter.
func TestCursorTwoTermSort(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 20, 10, 30, 0, 123456000, time.FixedZone("plus2", 2*3600))
	utc := "2026-09-20T08:30:00.123456Z"
	pool, rec := sqltest.Open(t,
		files(at, "c.txt", "b.txt", "a.txt"),
		files(at, "a.txt"),
		files(at, "a.txt", "b.txt", "c.txt"),
		files(at, "c.txt"),
	)
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	newest := data.Listing{Size: 2, Page: 1, Total: data.TotalNone, Sort: []query.Sort{{Field: "created_at", Descending: true}}}
	page, err := s.ListFiles(ctx, db, "dir", newest)
	if err != nil {
		t.Fatalf("newest first: %v", err)
	}
	newest.After = page.Next
	if _, err := s.ListFiles(ctx, db, "dir", newest); err != nil {
		t.Fatalf("newest first, continued: %v", err)
	}
	byNameThenSize := data.Listing{Size: 2, Page: 1, Total: data.TotalNone, Sort: []query.Sort{{Field: "name"}, {Field: "size"}}}
	page, err = s.ListFiles(ctx, db, "dir", byNameThenSize)
	if err != nil {
		t.Fatalf("by name then size: %v", err)
	}
	byNameThenSize.After = page.Next
	if _, err := s.ListFiles(ctx, db, "dir", byNameThenSize); err != nil {
		t.Fatalf("by name then size, continued: %v", err)
	}

	queries := rec.SQL(sqltest.OpQuery)
	if !strings.HasSuffix(queries[0], "WHERE q.directory_id = CAST($1 AS uuid) ORDER BY q.created_at DESC, q.name DESC OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY") {
		t.Errorf("the tie-breaker did not take the sort's direction:\n%s", queries[0])
	}
	want := "WHERE q.directory_id = CAST($1 AS uuid) AND (q.created_at < CAST($2 AS timestamp with time zone) OR (q.created_at = CAST($3 AS timestamp with time zone) AND q.name < CAST($4 AS text))) ORDER BY q.created_at DESC, q.name DESC OFFSET $5 ROWS FETCH NEXT $6 ROWS ONLY"
	if !strings.HasSuffix(queries[1], want) {
		t.Errorf("the two-term cursor composed:\n%s\nwant the suffix\n%s", queries[1], want)
	}
	if got := rec.Calls()[1].Args; !slices.Equal(got, []any{"dir", utc, utc, "b.txt", 0, 3}) {
		t.Errorf("the two-term cursor bound %v, want the timestamp twice in UTC at full precision, the name, offset 0, fetch 3", got)
	}
	if !strings.HasSuffix(queries[3], "WHERE q.directory_id = CAST($1 AS uuid) AND q.name > CAST($2 AS text) ORDER BY q.name, q.size OFFSET $3 ROWS FETCH NEXT $4 ROWS ONLY") {
		t.Errorf("a term after the key became a cursor term:\n%s", queries[3])
	}
}

// TestCursorRefusals proves every refusal happens before any SQL reaches
// the driver, as a CursorError unwrapping to query.ErrDirectives with a
// reason a caller can read: a sort that mixes directions, a sort by a
// field that can be NULL, a cursor replayed under another sort, a cursor
// edited in transit, a truncated one, an arbitrary string, and a cursor
// issued by the other listing.
func TestCursorRefusals(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	pool, rec := sqltest.Open(t, files(time.Now(), "a.txt", "b.txt", "c.txt"))
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	page, err := s.ListFiles(ctx, db, "dir", data.Listing{Page: 1, Size: 2, Total: data.TotalNone})
	if err != nil || page.Next == "" {
		t.Fatalf("page 1 = %+v, %v; want a cursor", page, err)
	}
	byName := page.Next
	edited := []byte(byName)
	mid := len(edited) / 2
	if edited[mid] == 'A' {
		edited[mid] = 'B'
	} else {
		edited[mid] = 'A'
	}
	before := len(rec.Calls())

	cases := []struct {
		label  string
		l      data.Listing
		reason string
	}{
		{"mixed directions", data.Listing{Size: 2, After: byName, Sort: []query.Sort{{Field: "created_at", Descending: true}, {Field: "version"}}}, "mixes directions"},
		{"nullable field", data.Listing{Size: 2, After: byName, Sort: []query.Sort{{Field: "size"}}}, "size can be NULL"},
		{"another sort", data.Listing{Size: 2, After: byName, Sort: []query.Sort{{Field: "created_at"}}}, "issued under the sort name; this listing sorts by created_at, name"},
		{"another direction", data.Listing{Size: 2, After: byName, Sort: []query.Sort{{Field: "name", Descending: true}}}, "issued under the sort name; this listing sorts by name desc"},
		{"edited", data.Listing{Size: 2, After: string(edited)}, "not one this listing issued"},
		{"truncated", data.Listing{Size: 2, After: byName[:len(byName)-4]}, "not one this listing issued"},
		{"arbitrary", data.Listing{Size: 2, After: "opaque"}, "not one this listing issued"},
		{"size zero", data.Listing{Size: 0, After: byName}, "page size"},
	}
	for _, c := range cases {
		_, err := s.ListFiles(ctx, db, "dir", c.l)
		if !errors.Is(err, query.ErrDirectives) || err == nil || !strings.Contains(err.Error(), c.reason) {
			t.Errorf("%s = %v, want ErrDirectives with %q", c.label, err, c.reason)
		}
		var ce *data.CursorError
		if c.label != "size zero" && !errors.As(err, &ce) {
			t.Errorf("%s = %v, want a CursorError", c.label, err)
		}
	}
	_, err = s.Children(ctx, db, "dir", data.Listing{Size: 2, After: byName})
	if !errors.Is(err, query.ErrDirectives) || err == nil || !strings.Contains(err.Error(), "issued by files_in_directory, not by children_of_directory") {
		t.Errorf("the file cursor on Children = %v, want the refusal naming both listings", err)
	}
	if n := len(rec.Calls()); n != before {
		t.Errorf("the refusals reached the driver with %d calls", n-before)
	}
}

// TestNullableSortIssuesNoCursor proves a sort a cursor cannot continue
// still pages by number, still fetches one row beyond its size so More is
// reported, and issues no Next: a page with rows beyond it is the third
// state, More with an empty Next, and a caller reads the next page by
// number. It also proves the page number is still checked for an offset
// page.
func TestNullableSortIssuesNoCursor(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	pool, rec := sqltest.Open(t, files(time.Now(), "a.txt", "b.txt"), files(time.Now(), "a.txt", "b.txt", "c.txt"))
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	bySize := data.Listing{Page: 3, Size: 2, Total: data.TotalNone, Sort: []query.Sort{{Field: "size", Descending: true}}}
	page, err := s.ListFiles(ctx, db, "dir", bySize)
	if err != nil {
		t.Fatalf("by size: %v", err)
	}
	if len(page.Rows) != 2 || page.More || page.Next != "" {
		t.Errorf("by size = %d rows, more %v, next %q; want 2 rows, no More, and no cursor", len(page.Rows), page.More, page.Next)
	}
	if got := rec.Calls()[0].Args; !slices.Equal(got, []any{"dir", 4, 3}) {
		t.Errorf("by size bound %v, want the directory, offset 4, fetch 3 (one row beyond the page tells whether more remain)", got)
	}
	if !strings.HasSuffix(rec.SQL(sqltest.OpQuery)[0], " ORDER BY q.size DESC, q.name DESC OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY") {
		t.Errorf("by size composed:\n%s", rec.SQL(sqltest.OpQuery)[0])
	}
	page, err = s.ListFiles(ctx, db, "dir", bySize)
	if err != nil {
		t.Fatalf("by size with a row beyond: %v", err)
	}
	if len(page.Rows) != 2 || page.Rows[1].Name != "b.txt" || !page.More || page.Next != "" {
		t.Errorf("by size with a row beyond = %d rows, more %v, next %q; want 2 rows, More, and no cursor (page by number)", len(page.Rows), page.More, page.Next)
	}
	if _, err := s.ListFiles(ctx, db, "dir", data.Listing{Size: 2}); !errors.Is(err, query.ErrDirectives) {
		t.Errorf("an offset page without a page number = %v, want ErrDirectives", err)
	}
}
