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

// catalog builds the catalog a consumer builds: the library's patterns and
// blobfs's, and nothing else.
func catalog(t *testing.T) *query.Catalog {
	t.Helper()
	c, err := query.NewCatalog(query.Patterns(), data.Patterns())
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	return c
}

// newStore compiles the store against the consumer's catalog under the
// stub dialect.
func newStore(t *testing.T) *data.Store {
	t.Helper()
	s, err := data.New(catalog(t), sqltest.Dialect{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// fileColumns is the column list of the file listing statements, as the
// scripted driver must return them, and the total column the counted
// statement adds.
var fileColumns = []string{"id", "directory_id", "name", "status", "key", "size", "content_type", "etag", "version", "created_at", "updated_at"}

// fileRow is one scripted file row, its columns in fileColumns order.
func fileRow(id, name string) []driver.Value {
	now := time.Now()
	return []driver.Value{id, blobfs.RootID, name, "available", id + "/" + name, nil, "text/plain", nil, int64(1), now, now}
}

// TestNew proves the catalog builds with the two sources, every statement
// compiles and both listings construct, and reports the inventory the tier
// proof counts: fourteen statements, all standard tier, the baseline's
// file-delete begin the only one requiring a transaction. The default
// variant is the baseline, and it adds no statements of its own to the
// inventory.
func TestNew(t *testing.T) {
	s := newStore(t)
	stmts := s.Statements()
	var names []string
	for _, st := range stmts {
		names = append(names, st.Name())
		if st.Tier() != query.TierStandard {
			t.Errorf("%s is %s tier, want standard", st.Name(), st.Tier())
		}
		if st.TransactionRequired() != (st.Name() == "begin_file_delete") {
			t.Errorf("%s: TransactionRequired = %v; only begin_file_delete requires one", st.Name(), st.TransactionRequired())
		}
	}
	want := []string{
		"begin_file_delete", "begin_file_write", "children_of_directory", "children_of_directory_with_total",
		"complete_file_write", "create_directory", "directory_ancestors", "directory_by_id", "directory_child",
		"file_by_id", "file_by_name", "file_version", "files_in_directory", "files_in_directory_with_total",
	}
	if _, ok := s.Variant().(*data.Standard); !ok {
		t.Errorf("the default variant is %T, want *data.Standard", s.Variant())
	}
	if !slices.Equal(names, want) {
		t.Errorf("Statements = %v, want %v", names, want)
	}
	for _, p := range catalog(t).Patterns() {
		if p.Namespace == data.Namespace && p.Tier != query.TierStandard {
			t.Errorf("pattern %s.%s is %s tier, want standard", p.Namespace, p.Name, p.Tier)
		}
		if p.Namespace == data.Namespace && len(p.Slots) != 0 {
			t.Errorf("pattern %s.%s declares slots %v; published patterns are parameter-free", p.Namespace, p.Name, p.Slots)
		}
	}
}

// TestPatterns proves the published namespace and its inventory: the two
// column lists and nothing else.
func TestPatterns(t *testing.T) {
	var names []string
	for _, p := range catalog(t).Patterns() {
		if p.Namespace == data.Namespace {
			names = append(names, p.Name)
		}
	}
	if got := strings.Join(names, " "); got != "directory_columns file_columns" {
		t.Errorf("blobfs patterns = %q, want %q", got, "directory_columns file_columns")
	}
}

// TestNewWithoutPatterns proves a catalog that lacks either namespace is
// refused with an error naming what is missing, before any statement
// compiles: the blobfs namespace the statements include, and the query
// library's namespace the composer fills its clauses from.
func TestNewWithoutPatterns(t *testing.T) {
	c, err := query.NewCatalog(query.Patterns())
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	_, err = data.New(c, sqltest.Dialect{})
	if err == nil || !strings.Contains(err.Error(), `"blobfs"`) {
		t.Fatalf("New without the blobfs namespace = %v, want an error naming the namespace", err)
	}
	c, err = query.NewCatalog(data.Patterns())
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	_, err = data.New(c, sqltest.Dialect{})
	if err == nil || !strings.Contains(err.Error(), "sql.") {
		t.Fatalf("New without the query library's namespace = %v, want an error naming a missing sql pattern", err)
	}
}

// TestVerify proves Verify prepares every statement as authored and the
// canonical renderings per listing: twenty prepares against the
// scripted driver, none of which consumes a response. Four offset
// renderings carry every declared field as a predicate and a sort term
// and the paging clause, the two counted ones the window count; and two
// cursor renderings, one per listing, carry the keyset predicate in its
// expanded form over the fields a cursor can continue (id, directory_id
// or parent_id excluded as nullable, and name), with one row beyond the
// page fetched.
func TestVerify(t *testing.T) {
	s := newStore(t)
	pool, rec := sqltest.Open(t)
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	if err := s.Verify(context.Background(), db); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	prepared := rec.SQL(sqltest.OpPrepare)
	if len(prepared) != 20 {
		t.Fatalf("Verify prepared %d statements, want 20 (14 statements, 4 offset renderings, 2 cursor renderings)", len(prepared))
	}
	renderings, counted, cursors := 0, 0, 0
	for _, text := range prepared {
		if !strings.Contains(text, " ROWS FETCH NEXT ") {
			continue
		}
		renderings++
		if strings.Contains(text, "COUNT(*) OVER ()") {
			counted++
		}
		if strings.Contains(text, " OR (") {
			cursors++
			if strings.Contains(text, "COUNT(*) OVER ()") || strings.Contains(text, " IS NOT NULL") {
				t.Errorf("the cursor rendering carries the total or the filter probes:\n%s", text)
			}
			continue
		}
		if !strings.Contains(text, " AND q.name IS NOT NULL") || !strings.Contains(text, " ORDER BY q.id, ") {
			t.Errorf("rendering lacks the field probes:\n%s", text)
		}
	}
	if renderings != 6 || counted != 2 || cursors != 2 {
		t.Errorf("Verify prepared %d listing renderings of which %d counted and %d cursor, want 6, 2, and 2", renderings, counted, cursors)
	}
	wantFiles := "WHERE q.directory_id = CAST($1 AS uuid) AND (q.id > CAST($2 AS uuid) OR (q.id = CAST($3 AS uuid) AND q.directory_id > CAST($4 AS uuid)) OR (q.id = CAST($5 AS uuid) AND q.directory_id = CAST($6 AS uuid) AND q.name > CAST($7 AS text))) ORDER BY q.id, q.directory_id, q.name, q.status, q.content_type, q.version, q.created_at, q.updated_at OFFSET $8 ROWS FETCH NEXT $9 ROWS ONLY"
	wantChildren := "WHERE q.parent_id = CAST($1 AS uuid) AND (q.id > CAST($2 AS uuid) OR (q.id = CAST($3 AS uuid) AND q.name > CAST($4 AS text))) ORDER BY q.id, q.name, q.version, q.created_at, q.updated_at OFFSET $5 ROWS FETCH NEXT $6 ROWS ONLY"
	found := 0
	for _, text := range prepared {
		if strings.HasSuffix(text, wantFiles) || strings.HasSuffix(text, wantChildren) {
			found++
		}
	}
	if found != 2 {
		t.Errorf("the cursor renderings do not end with the expected predicates:\n%s", strings.Join(prepared, "\n"))
	}
}

// TestListingCarriesItsTotal is the stage gate's hermetic proof: ListFiles
// under TotalExact runs ONE statement, and that statement carries
// COUNT(*) OVER () in its select list, so the total and the page come from
// the same rows; under TotalNone the one statement omits the window count
// and the page reports NoTotal. The total is read from the rows: a row's
// total is the page's, and an empty first page has the exact total 0.
func TestListingCarriesItsTotal(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	dir := blobfs.NewID()
	counted := append(slices.Clone(fileColumns), "total")

	pool, rec := sqltest.Open(t,
		sqltest.Response{Columns: counted, Rows: [][]driver.Value{
			append(fileRow("f1", "a.txt"), int64(7)),
			append(fileRow("f2", "b.txt"), int64(7)),
		}},
		sqltest.Response{Columns: counted},
		sqltest.Response{Columns: fileColumns, Rows: [][]driver.Value{fileRow("f1", "a.txt")}},
	)
	db := sqlate.Wrap(pool, sqltest.Dialect{})

	page, err := s.ListFiles(ctx, db, dir, data.Listing{Page: 2, Size: 2})
	if err != nil {
		t.Fatalf("ListFiles exact: %v", err)
	}
	if page.Total != 7 || len(page.Rows) != 2 || page.Rows[1].Name != "b.txt" || page.Rows[1].DirectoryID != blobfs.RootID || page.Next != "" {
		t.Errorf("exact page = total %d, %d rows, next %q; want total 7 from the rows, 2 rows, no cursor", page.Total, len(page.Rows), page.Next)
	}
	page, err = s.ListFiles(ctx, db, dir, data.Listing{Page: 1, Size: 2})
	if err != nil {
		t.Fatalf("ListFiles exact, empty: %v", err)
	}
	if page.Total != 0 || len(page.Rows) != 0 {
		t.Errorf("empty first page = total %d, %d rows; want the exact total 0", page.Total, len(page.Rows))
	}
	page, err = s.ListFiles(ctx, db, dir, data.Listing{Page: 1, Size: 2, Total: data.TotalNone})
	if err != nil {
		t.Fatalf("ListFiles none: %v", err)
	}
	if page.Total != data.NoTotal || len(page.Rows) != 1 {
		t.Errorf("page without a total = total %d, %d rows; want NoTotal and 1 row", page.Total, len(page.Rows))
	}

	queries := rec.SQL(sqltest.OpQuery)
	if len(queries) != 3 {
		t.Fatalf("three listings ran %d queries, want 3 (one statement per listing, no count twin)", len(queries))
	}
	const window = ", COUNT(*) OVER () AS total"
	const tail = "\nFROM blobfs_file q\nWHERE q.directory_id = CAST($1 AS uuid) ORDER BY q.name OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY"
	for i, want := range []bool{true, true, false} {
		if got := strings.Contains(queries[i], window); got != want {
			t.Errorf("query %d carries the window count: %v, want %v\n%s", i, got, want, queries[i])
		}
		if !strings.HasSuffix(queries[i], tail) {
			t.Errorf("query %d does not end with the anchor, the default sort, and the paging clause:\n%s", i, queries[i])
		}
	}
	calls := rec.Calls()
	if got := calls[0].Args; len(got) != 3 || got[0] != dir || got[1] != 2 || got[2] != 3 {
		t.Errorf("page 2 of size 2 bound %v, want the directory, offset 2, fetch 3 (one row beyond the page tells whether a next page exists)", got)
	}
	if n := rec.RowsLeaked(); n != 0 {
		t.Errorf("%d row sets leaked", n)
	}
}

// TestListingComposesClauses proves the composer appends the caller's
// predicates and sort terms in the query library's spelling, at the
// statement's own level: each filter after AND with its value cast to the
// field's declared type, the sort terms in order with name as the
// tie-breaker unless the caller sorted by it, and the paging placeholders
// after every value. The caller's Listing is left as it was.
func TestListingComposesClauses(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	pool, rec := sqltest.Open(t,
		sqltest.Response{Columns: fileColumns},
		sqltest.Response{Columns: fileColumns},
	)
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	l := data.Listing{
		Page: 3, Size: 5, Total: data.TotalNone,
		Filters: []query.Filter{
			{Field: "name", Op: query.OpLike, Value: "a%"},
			{Field: "size", Op: query.OpIsNotNull},
			{Field: "status", Op: query.OpIn, Value: []any{"pending", "available"}},
			{Field: "created_at", Op: query.OpGe, Value: "2026-01-01T00:00:00Z"},
		},
		Sort: []query.Sort{{Field: "created_at", Descending: true}, {Field: "size"}},
	}
	if _, err := s.ListFiles(ctx, db, "dir", l); err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if _, err := s.ListFiles(ctx, db, "dir", data.Listing{Page: 1, Size: 5, Total: data.TotalNone, Sort: []query.Sort{{Field: "name", Descending: true}}}); err != nil {
		t.Fatalf("ListFiles by name desc: %v", err)
	}
	if len(l.Filters) != 4 || len(l.Sort) != 2 {
		t.Errorf("the composer modified the caller's Listing: %+v", l)
	}
	queries := rec.SQL(sqltest.OpQuery)
	wantTail := "WHERE q.directory_id = CAST($1 AS uuid)" +
		" AND q.name LIKE CAST($2 AS text)" +
		" AND q.size IS NOT NULL" +
		" AND q.status IN (CAST($3 AS text), CAST($4 AS text))" +
		" AND q.created_at >= CAST($5 AS timestamp with time zone)" +
		" ORDER BY q.created_at DESC, q.size, q.name" +
		" OFFSET $6 ROWS FETCH NEXT $7 ROWS ONLY"
	if !strings.HasSuffix(queries[0], wantTail) {
		t.Errorf("composed query ends with\n%s\nwant\n%s", queries[0], wantTail)
	}
	if !strings.HasSuffix(queries[1], " ORDER BY q.name DESC OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY") {
		t.Errorf("a sort by name gained a tie-breaker:\n%s", queries[1])
	}
	args := rec.Calls()[0].Args
	want := []any{"dir", "a%", "pending", "available", "2026-01-01T00:00:00Z", 10, 5}
	if !slices.Equal(args, want) {
		t.Errorf("bound %v, want %v", args, want)
	}
}

// TestChildrenSQL proves the directory listing composes over its own
// statement: the parent anchor, the directory columns, and the window
// count, with the same clause handling as the file listing.
func TestChildrenSQL(t *testing.T) {
	s := newStore(t)
	cols := []string{"id", "parent_id", "name", "version", "created_at", "updated_at", "total"}
	pool, rec := sqltest.Open(t, sqltest.Response{Columns: cols})
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	page, err := s.Children(context.Background(), db, blobfs.RootID, data.Listing{Page: 1, Size: 3, Filters: []query.Filter{{Field: "name", Op: query.OpGt, Value: "m"}}})
	if err != nil || page.Total != 0 || len(page.Rows) != 0 {
		t.Fatalf("Children = %+v, %v; want an empty first page with total 0", page, err)
	}
	q := rec.SQL(sqltest.OpQuery)[0]
	if !strings.Contains(q, "COUNT(*) OVER () AS total\nFROM blobfs_directory q\nWHERE q.parent_id = CAST($1 AS uuid) AND q.name > CAST($2 AS text) ORDER BY q.name OFFSET $3 ROWS FETCH NEXT $4 ROWS ONLY") {
		t.Errorf("Children composed:\n%s", q)
	}
}

// TestListingRefusals proves every refusal happens before any SQL reaches
// the driver, with the query library's own error types, each unwrapping to
// query.ErrDirectives: an unknown filter or sort field
// (UnknownFieldError, naming the use), an unknown operator, an in filter
// without a list, a page or size below one, and a cursor the listing did
// not issue (the cursor's own refusals are in cursor_test.go).
func TestListingRefusals(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	pool, rec := sqltest.Open(t)
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	cases := []struct {
		label string
		l     data.Listing
	}{
		{"unknown filter field", data.Listing{Page: 1, Size: 1, Filters: []query.Filter{{Field: "unit_id", Op: query.OpEq, Value: "x"}}}},
		{"unknown sort field", data.Listing{Page: 1, Size: 1, Sort: []query.Sort{{Field: "path"}}}},
		{"unknown operator", data.Listing{Page: 1, Size: 1, Filters: []query.Filter{{Field: "name", Op: "between", Value: "x"}}}},
		{"in without a list", data.Listing{Page: 1, Size: 1, Filters: []query.Filter{{Field: "name", Op: query.OpIn, Value: "x"}}}},
		{"page zero", data.Listing{Page: 0, Size: 1}},
		{"size zero", data.Listing{Page: 1, Size: 0}},
		{"cursor", data.Listing{Page: 1, Size: 1, After: "opaque"}},
	}
	for _, c := range cases {
		_, err := s.ListFiles(ctx, db, "dir", c.l)
		if !errors.Is(err, query.ErrDirectives) {
			t.Errorf("%s = %v, want ErrDirectives", c.label, err)
		}
		if _, err := s.Children(ctx, db, "dir", c.l); !errors.Is(err, query.ErrDirectives) {
			t.Errorf("Children, %s = %v, want ErrDirectives", c.label, err)
		}
	}
	var unknown *query.UnknownFieldError
	_, err := s.ListFiles(ctx, db, "dir", cases[0].l)
	if !errors.As(err, &unknown) || unknown.Field != "unit_id" || unknown.Use != query.FieldUseFilter {
		t.Errorf("unknown filter field = %v, want UnknownFieldError naming unit_id as a filter", err)
	}
	_, err = s.ListFiles(ctx, db, "dir", cases[1].l)
	if !errors.As(err, &unknown) || unknown.Field != "path" || unknown.Use != query.FieldUseSort {
		t.Errorf("unknown sort field = %v, want UnknownFieldError naming path as a sort", err)
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("the refusals reached the driver with %d calls", len(calls))
	}
}

// TestRootReadsByID proves Root is a read of the well-known id and no
// search: one query bound to RootID, and a database without the row (the
// schema not applied) is ErrNotFound.
func TestRootReadsByID(t *testing.T) {
	s := newStore(t)
	pool, rec := sqltest.Open(t, sqltest.Response{Columns: []string{"id", "parent_id", "name", "version", "created_at", "updated_at"}})
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	if _, err := s.Root(context.Background(), db); !errors.Is(err, blobfs.ErrNotFound) {
		t.Fatalf("Root without the seed = %v, want ErrNotFound", err)
	}
	calls := rec.Calls()
	if len(calls) != 1 || len(calls[0].Args) != 1 || calls[0].Args[0] != blobfs.RootID {
		t.Errorf("Root ran %d calls with %v, want one query bound to RootID", len(calls), calls)
	}
}

// TestDirectoryPathComposes proves the path is composed from the ancestor
// chain root first: the root alone is /, a chain is the names below the
// root joined by slashes, and no chain is ErrNotFound. The statement runs
// once per call, whatever the depth.
func TestDirectoryPathComposes(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	cols := []string{"parent_id", "name"}
	pool, rec := sqltest.Open(t,
		sqltest.Response{Columns: cols, Rows: [][]driver.Value{{nil, nil}}},
		sqltest.Response{Columns: cols, Rows: [][]driver.Value{{nil, nil}, {blobfs.RootID, "a"}, {"a", "b"}, {"b", "café"}}},
		sqltest.Response{Columns: cols},
	)
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	for i, want := range []string{"/", "/a/b/café"} {
		got, err := s.DirectoryPath(ctx, db, "id")
		if err != nil || got != want {
			t.Errorf("DirectoryPath %d = %q, %v, want %q", i, got, err, want)
		}
	}
	if _, err := s.DirectoryPath(ctx, db, "missing"); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("DirectoryPath of a missing directory = %v, want ErrNotFound", err)
	}
	if n := len(rec.SQL(sqltest.OpQuery)); n != 3 {
		t.Errorf("three paths ran %d queries, want 3 (one recursive statement each)", n)
	}
}

// TestResolveDirectoryPaths proves the path checks that run before any
// SQL: a relative path and an empty segment are ErrInvalidPath, a segment
// ValidateName refuses matches ErrInvalidName as well, and nothing reaches
// the driver.
func TestResolveDirectoryPaths(t *testing.T) {
	s := newStore(t)
	pool, rec := sqltest.Open(t)
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	for _, path := range []string{"", "a/b", "/a//b", "/a/", "/a/../b"} {
		_, err := s.ResolveDirectory(context.Background(), db, path)
		if !errors.Is(err, blobfs.ErrInvalidPath) {
			t.Errorf("ResolveDirectory(%q) = %v, want ErrInvalidPath", path, err)
		}
	}
	if _, err := s.ResolveDirectory(context.Background(), db, "/a/../b"); !errors.Is(err, blobfs.ErrInvalidName) {
		t.Errorf("ResolveDirectory(/a/../b) = %v, want ErrInvalidName as well", err)
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("path checks reached the driver with %d calls", len(calls))
	}
}

// TestMkdirInvalidNames proves Mkdir normalizes and validates before any
// SQL: an empty name, a slash, a control character, and .. are
// ErrInvalidName, and nothing reaches the driver. The empty name is the
// stage gate's API half: no call of Mkdir writes a row without a name, so
// no call creates a root.
func TestMkdirInvalidNames(t *testing.T) {
	s := newStore(t)
	pool, rec := sqltest.Open(t)
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	ctx := context.Background()
	for _, name := range []string{"", "a/b", "tab\there", ".."} {
		if _, err := s.Mkdir(ctx, db, blobfs.RootID, name); !errors.Is(err, blobfs.ErrInvalidName) {
			t.Errorf("Mkdir(%q) = %v, want ErrInvalidName", name, err)
		}
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("name checks reached the driver with %d calls", len(calls))
	}
}
