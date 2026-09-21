package files_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/domain/files"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// newStore builds the store over the scripted driver under the stub
// dialect, with responses queued for the calls a test expects.
func newStore(t *testing.T, responses ...sqltest.Response) (*files.Store, *sqltest.Recorder) {
	t.Helper()
	pool, rec := sqltest.Open(t, responses...)
	s, err := files.New(sqlate.Wrap(pool, sqltest.Dialect{}), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, rec
}

var (
	directoryColumns = []string{"id", "parent_id", "name", "version", "created_at", "updated_at"}
	fileColumns      = []string{"id", "directory_id", "name", "status", "key", "size", "content_type", "etag", "version", "created_at", "updated_at"}
	ownerColumns     = []string{"directory_id", "unit_id", "version", "created_at", "updated_at"}
	ownedColumns     = []string{"id", "parent_id", "name", "version", "created_at", "updated_at", "unit_id"}
)

// directory scripts one directory row: the root when parent and name are
// nil, so the row scans into blobfs.Directory as the engine returns it.
func directory(id string, parent, name any) sqltest.Response {
	now := time.Now()
	return sqltest.Response{Columns: directoryColumns, Rows: [][]driver.Value{{id, parent, name, int64(1), now, now}}}
}

// root scripts the seeded root row.
func root() sqltest.Response { return directory(blobfs.RootID, nil, nil) }

// affected scripts one successful exec.
func affected() sqltest.Response { return sqltest.Response{Affected: 1} }

// owner scripts the owner row of a directory, or no row when unit is
// empty.
func owner(directoryID, unit string) sqltest.Response {
	if unit == "" {
		return sqltest.Response{Columns: ownerColumns}
	}
	now := time.Now()
	return sqltest.Response{Columns: ownerColumns, Rows: [][]driver.Value{{directoryID, unit, int64(1), now, now}}}
}

// listing scripts one page of a library listing: the columns, the total
// column when counted, and one row per name carrying total.
func listing(columns []string, counted bool, total int64, names ...string) sqltest.Response {
	cols := slices.Clone(columns)
	if counted {
		cols = append(cols, "total")
	}
	resp := sqltest.Response{Columns: cols}
	now := time.Now()
	for i, name := range names {
		var row []driver.Value
		id := "id-" + name
		if len(columns) == len(directoryColumns) {
			row = []driver.Value{id, blobfs.RootID, name, int64(1), now, now}
		} else {
			row = []driver.Value{id, blobfs.RootID, name, "available", id + "/" + name, int64(i + 1), "text/plain", nil, int64(1), now, now}
		}
		if counted {
			row = append(row, total)
		}
		resp.Rows = append(resp.Rows, row)
	}
	return resp
}

func ops(rec *sqltest.Recorder) string {
	var out []string
	for _, op := range rec.Ops() {
		out = append(out, string(op))
	}
	return strings.Join(out, " ")
}

// TestNew proves the catalog builds from the two pattern sources and every
// statement compiles and the projection constructs, blobfs's and the
// consumer's, with no I/O: the recorder sees no call.
func TestNew(t *testing.T) {
	_, rec := newStore(t)
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("New reached the driver with %d calls", len(calls))
	}
}

// TestVerify proves Verify prepares the whole inventory, the consumer's
// three statements and one projection and blobfs's fourteen statements
// and six listing renderings (four offset, two cursor), twenty-four
// prepares,
// and wraps a failure in ErrVerify with the failing statement named.
func TestVerify(t *testing.T) {
	s, rec := newStore(t)
	if err := s.Verify(context.Background()); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if n := len(rec.SQL(sqltest.OpPrepare)); n != 24 {
		t.Errorf("Verify prepared %d statements, want 24", n)
	}

	s, rec = newStore(t)
	rec.FailPrepare = func(q string) error {
		if strings.Contains(q, "directory_owner") {
			return errors.New(`relation "directory_owner" does not exist`)
		}
		return nil
	}
	err := s.Verify(context.Background())
	if !errors.Is(err, files.ErrVerify) {
		t.Fatalf("Verify against a missing table = %v, want ErrVerify", err)
	}
	for _, want := range []string{"blobfs schema up", "owned_directories", "create_directory_owner", "owner_of_directory"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Verify's error lacks %q:\n%v", want, err)
		}
	}
}

// TestListRunsInOneReadOnlyRepeatableReadTransaction is the stage gate's
// hermetic proof: one ls is one transaction, begun read-only at repeatable
// read, holding the address resolution and both halves, then committed.
// The recording shows one begin carrying both options, the root read, the
// directory half, the file half, and the commit, and nothing outside them.
func TestListRunsInOneReadOnlyRepeatableReadTransaction(t *testing.T) {
	s, rec := newStore(t,
		root(),
		listing(directoryColumns, true, 2, "a", "b"),
		listing(fileColumns, true, 1, "x.txt"),
	)
	c, err := s.List(context.Background(), "/", files.Listing{Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := ops(rec); got != "begin query query query commit" {
		t.Errorf("ops = %q, want one transaction around the root read and the two halves", got)
	}
	begin := rec.Calls()[0]
	if !begin.TxOptions.ReadOnly {
		t.Errorf("the transaction is not read-only: %+v", begin.TxOptions)
	}
	if begin.TxOptions.Isolation != driver.IsolationLevel(sql.LevelRepeatableRead) {
		t.Errorf("the transaction's isolation is %v, want repeatable read", sql.IsolationLevel(begin.TxOptions.Isolation))
	}
	queries := rec.SQL(sqltest.OpQuery)
	if !strings.Contains(queries[1], "FROM blobfs_directory q") || !strings.Contains(queries[2], "FROM blobfs_file q") {
		t.Errorf("the halves ran out of order or through the wrong statements:\n%s\n%s", queries[1], queries[2])
	}
	if c.Path != "/" || c.Directories.Total != 2 || len(c.Directories.Rows) != 2 || c.Files.Total != 1 || len(c.Files.Rows) != 1 {
		t.Errorf("contents = %+v", c)
	}
	if *c.Directories.Rows[1].Name != "b" || c.Files.Rows[0].Name != "x.txt" || *c.Files.Rows[0].Size != 1 {
		t.Errorf("rows = %+v %+v", c.Directories.Rows, c.Files.Rows)
	}
	if rec.Pending() != 0 || rec.RowsLeaked() != 0 {
		t.Errorf("%d responses pending, %d row sets leaked", rec.Pending(), rec.RowsLeaked())
	}

	// A failure inside rolls the transaction back and reaches the caller.
	s, rec = newStore(t, sqltest.Response{Columns: directoryColumns})
	if _, err := s.List(context.Background(), "/", files.Listing{Page: 1, Size: 10}); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("List without the root = %v, want ErrNotFound", err)
	}
	if got := ops(rec); got != "begin query rollback" {
		t.Errorf("ops after a failure = %q, want begin query rollback", got)
	}
}

// TestListLowersTheSort proves database.go's lowering of the sort terms:
// the file half takes every term, in order, and the directory half takes
// only the terms naming a directory field, so a sort by size sorts the
// files and leaves the directories in name order. TotalNone chooses the
// statements without the window count and reports NoTotal on both halves.
func TestListLowersTheSort(t *testing.T) {
	s, rec := newStore(t,
		root(),
		listing(directoryColumns, false, 0, "b", "a"),
		listing(fileColumns, false, 0, "big", "small"),
	)
	l := files.Listing{Page: 2, Size: 5, Total: files.TotalNone, Sort: []files.Sort{{Field: "size", Descending: true}, {Field: "created_at"}}}
	c, err := s.List(context.Background(), "/", l)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	queries := rec.SQL(sqltest.OpQuery)
	if !strings.HasSuffix(queries[1], " ORDER BY q.created_at, q.name OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY") {
		t.Errorf("the directory half took a file-only term or lost a shared one:\n%s", queries[1])
	}
	if !strings.HasSuffix(queries[2], " ORDER BY q.size DESC, q.created_at, q.name OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY") {
		t.Errorf("the file half did not take every term:\n%s", queries[2])
	}
	for _, q := range queries[1:] {
		if strings.Contains(q, "COUNT(*) OVER ()") {
			t.Errorf("TotalNone ran a counted statement:\n%s", q)
		}
	}
	if c.Directories.Total != files.NoTotal || c.Files.Total != files.NoTotal {
		t.Errorf("totals = %d, %d; want NoTotal on both halves", c.Directories.Total, c.Files.Total)
	}
	if args := rec.Calls()[2].Args; len(args) != 3 || args[1] != 5 || args[2] != 6 {
		t.Errorf("page 2 of size 5 bound %v, want the directory, offset 5, fetch 6 (one row beyond the page)", args)
	}

	// An unknown field is refused by the file half before any SQL and
	// unwraps to the query library's sentinel.
	s, rec = newStore(t, root(), listing(directoryColumns, true, 0))
	_, err = s.List(context.Background(), "/", files.Listing{Page: 1, Size: 5, Sort: []files.Sort{{Field: "path"}}})
	if !errors.Is(err, query.ErrDirectives) {
		t.Errorf("a sort by an undeclared field = %v, want ErrDirectives", err)
	}
	if got := ops(rec); got != "begin query query rollback" {
		t.Errorf("ops = %q, want the directory half to run and the file half to refuse", got)
	}
}

// TestDirectoryFieldsMatchTheLibrary pins the field set the directory
// half of a sort takes: the five fields both library listings declare all
// reach the directory half, and both halves accept them, so a field the
// library drops surfaces here. parent_id, which only the directory
// listing declares, is refused by the file half like any field it lacks:
// a sort term must be one the file half declares.
func TestDirectoryFieldsMatchTheLibrary(t *testing.T) {
	shared := []string{"id", "name", "version", "created_at", "updated_at"}
	s, rec := newStore(t, root(), listing(directoryColumns, true, 0), listing(fileColumns, true, 0))
	var terms []files.Sort
	for _, f := range shared {
		terms = append(terms, files.Sort{Field: f})
	}
	if _, err := s.List(context.Background(), "/", files.Listing{Page: 1, Size: 1, Sort: terms}); err != nil {
		t.Fatalf("List sorted by every shared field: %v", err)
	}
	queries := rec.SQL(sqltest.OpQuery)
	for _, q := range queries[1:] {
		if !strings.Contains(q, " ORDER BY q.id, q.name, q.version, q.created_at, q.updated_at OFFSET") {
			t.Errorf("a half dropped a shared field:\n%s", q)
		}
	}
	s, _ = newStore(t, root(), listing(directoryColumns, true, 0))
	_, err := s.List(context.Background(), "/", files.Listing{Page: 1, Size: 1, Sort: []files.Sort{{Field: "parent_id"}}})
	if !errors.Is(err, query.ErrDirectives) || !strings.Contains(err.Error(), "parent_id") {
		t.Errorf("a sort by parent_id = %v, want the file half's refusal naming it", err)
	}
}

// TestListScopedChecksTheAncestorOnce proves the ownership rehearsal: for
// /a/b under a unit, the depth-one ancestor /a is resolved and its owner
// row read once, before the rest of the path is resolved, and the two
// halves are then read with no owner predicate on any row. A unit that
// does not own the ancestor, and an ancestor with no owner row, are
// ErrNotOwned with nothing listed and the transaction rolled back.
func TestListScopedChecksTheAncestorOnce(t *testing.T) {
	unit, other := blobfs.NewID(), blobfs.NewID()
	s, rec := newStore(t,
		root(), directory("A", blobfs.RootID, "a"), // the ancestor
		owner("A", unit),
		root(), directory("A", blobfs.RootID, "a"), directory("B", "A", "b"), // the path
		listing(directoryColumns, true, 0),
		listing(fileColumns, true, 1, "x.txt"),
	)
	c, err := s.List(context.Background(), "/a/b", files.Listing{Page: 1, Size: 10, Unit: unit})
	if err != nil {
		t.Fatalf("List as the owner: %v", err)
	}
	if got := ops(rec); got != "begin query query query query query query query query commit" {
		t.Errorf("ops = %q", got)
	}
	queries := rec.SQL(sqltest.OpQuery)
	ownerReads := 0
	for _, q := range queries {
		if strings.Contains(q, "FROM directory_owner") {
			ownerReads++
		}
	}
	if ownerReads != 1 || !strings.Contains(queries[2], "FROM directory_owner") {
		t.Errorf("the owner row was read %d times, want once, third:\n%s", ownerReads, strings.Join(queries, "\n"))
	}
	if strings.Contains(queries[6], "directory_owner") || strings.Contains(queries[7], "directory_owner") {
		t.Errorf("a listing half joined or filtered by the owner:\n%s\n%s", queries[6], queries[7])
	}
	if c.Files.Total != 1 || c.Directories.Total != 0 || c.Path != "/a/b" {
		t.Errorf("contents = %+v", c)
	}

	for _, tc := range []struct {
		label string
		owner sqltest.Response
	}{
		{"another unit's directory", owner("A", other)},
		{"a directory with no owner", owner("A", "")},
	} {
		s, rec := newStore(t, root(), directory("A", blobfs.RootID, "a"), tc.owner)
		_, err := s.List(context.Background(), "/a/b", files.Listing{Page: 1, Size: 10, Unit: unit})
		if !errors.Is(err, files.ErrNotOwned) {
			t.Errorf("%s = %v, want ErrNotOwned", tc.label, err)
		}
		if got := ops(rec); got != "begin query query query rollback" {
			t.Errorf("%s: ops = %q, want the refusal before the path resolves further", tc.label, got)
		}
	}

	// At depth one the ancestor is the directory itself: no second
	// resolution.
	s, rec = newStore(t, root(), directory("A", blobfs.RootID, "a"), owner("A", unit), listing(directoryColumns, true, 0), listing(fileColumns, true, 0))
	if _, err := s.List(context.Background(), "/a", files.Listing{Page: 1, Size: 10, Unit: unit}); err != nil {
		t.Fatalf("List /a as the owner: %v", err)
	}
	if got := ops(rec); got != "begin query query query query query commit" {
		t.Errorf("ops for a depth-one path = %q", got)
	}
}

// TestListScopedAtRootUsesTheOwnerProjection proves ls / --unit reads the
// consumer's read model: the projection over blobfs_directory joined to
// directory_owner, filtered by the unit, counted and paged by the query
// library, with the rows returned as the library's Directory type and no
// file half read. Under TotalNone the count is still run, and dropped.
func TestListScopedAtRootUsesTheOwnerProjection(t *testing.T) {
	unit := blobfs.NewID()
	now := time.Now()
	s, rec := newStore(t,
		sqltest.Response{Columns: []string{"count"}, Rows: [][]driver.Value{{int64(3)}}},
		sqltest.Response{Columns: ownedColumns, Rows: [][]driver.Value{{"A", blobfs.RootID, "a", int64(1), now, now, unit}}},
	)
	c, err := s.List(context.Background(), "/", files.Listing{Page: 2, Size: 1, Unit: unit, Sort: []files.Sort{{Field: "size"}, {Field: "created_at", Descending: true}}})
	if err != nil {
		t.Fatalf("List / as a unit: %v", err)
	}
	if got := ops(rec); got != "begin query query commit" {
		t.Errorf("ops = %q, want the count and the page in one transaction", got)
	}
	count, page := rec.Calls()[1], rec.Calls()[2]
	for _, q := range []sqltest.Call{count, page} {
		if !strings.Contains(q.SQL, "JOIN directory_owner o ON o.directory_id = d.id") || !strings.Contains(q.SQL, "WHERE q.unit_id = CAST($1 AS uuid)") {
			t.Errorf("query lacks the owner join or the unit filter:\n%s", q.SQL)
		}
		if q.Args[0] != unit {
			t.Errorf("query binds %v first, want the unit", q.Args[0])
		}
	}
	if !strings.HasSuffix(page.SQL, " ORDER BY q.created_at DESC, q.name OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY") {
		t.Errorf("the page took a file-only term or lost the key:\n%s", page.SQL)
	}
	if c.Directories.Total != 3 || len(c.Directories.Rows) != 1 || *c.Directories.Rows[0].Name != "a" || *c.Directories.Rows[0].ParentID != blobfs.RootID {
		t.Errorf("directories = %+v", c.Directories)
	}
	if c.Files.Total != 0 || len(c.Files.Rows) != 0 {
		t.Errorf("files = %+v, want none: the root's files belong to no unit", c.Files)
	}

	s, rec = newStore(t,
		sqltest.Response{Columns: []string{"count"}, Rows: [][]driver.Value{{int64(3)}}},
		sqltest.Response{Columns: ownedColumns},
	)
	c, err = s.List(context.Background(), "/", files.Listing{Page: 1, Size: 1, Unit: unit, Total: files.TotalNone})
	if err != nil {
		t.Fatalf("List / as a unit without a total: %v", err)
	}
	if c.Directories.Total != files.NoTotal || c.Files.Total != files.NoTotal {
		t.Errorf("totals under TotalNone = %d, %d; want NoTotal", c.Directories.Total, c.Files.Total)
	}
	if n := len(rec.SQL(sqltest.OpQuery)); n != 2 {
		t.Errorf("the projection ran %d queries under TotalNone, want 2: it cannot skip its count", n)
	}
}

// TestMkdirWithUnitIsOneTransaction proves the two-row unit: with a unit,
// the parent's resolution, the directory's insert and read-back, and the
// owner's insert run in one transaction, committed together; a failing
// owner insert rolls the directory back; and a path below depth one is
// refused before any I/O. Without a unit, mkdir runs on the pool.
func TestMkdirWithUnitIsOneTransaction(t *testing.T) {
	unit := blobfs.NewID()
	s, rec := newStore(t,
		root(),
		affected(),
		directory("A", blobfs.RootID, "a"),
		affected(),
	)
	dir, err := s.Mkdir(context.Background(), "/a", unit)
	if err != nil || dir.ID != "A" {
		t.Fatalf("Mkdir(/a, unit) = %+v, %v", dir, err)
	}
	if got := ops(rec); got != "begin query exec query exec commit" {
		t.Errorf("ops = %q", got)
	}
	execs := rec.SQL(sqltest.OpExec)
	if !strings.HasPrefix(execs[0], "INSERT INTO blobfs_directory") || !strings.HasPrefix(execs[1], "INSERT INTO directory_owner") {
		t.Errorf("execs = %q", execs)
	}
	if args := rec.Calls()[4].Args; len(args) != 2 || args[0] != "A" || args[1] != unit {
		t.Errorf("the owner insert bound %v, want the directory and the unit", args)
	}

	refused := errors.New("owner refused")
	s, rec = newStore(t, root(), affected(), directory("A", blobfs.RootID, "a"), sqltest.Response{Err: refused})
	if _, err := s.Mkdir(context.Background(), "/a", unit); !errors.Is(err, refused) {
		t.Errorf("Mkdir with a refused owner = %v, want the owner's error", err)
	}
	if got := ops(rec); got != "begin query exec query exec rollback" {
		t.Errorf("ops after the refused owner = %q, want a rollback", got)
	}

	s, rec = newStore(t)
	for _, path := range []string{"/a/b", "/a/b/c"} {
		if _, err := s.Mkdir(context.Background(), path, unit); !errors.Is(err, files.ErrUnitDepth) {
			t.Errorf("Mkdir(%s, unit) = %v, want ErrUnitDepth", path, err)
		}
	}
	for path, want := range map[string]error{"/": blobfs.ErrRootDirectory, "/a/": blobfs.ErrInvalidPath, "a": blobfs.ErrInvalidPath} {
		if _, err := s.Mkdir(context.Background(), path, ""); !errors.Is(err, want) {
			t.Errorf("Mkdir(%s) = %v, want %v", path, err, want)
		}
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("the refusals reached the driver with %d calls", len(calls))
	}

	s, rec = newStore(t, root(), directory("A", blobfs.RootID, "a"), affected(), directory("B", "A", "b"))
	if dir, err := s.Mkdir(context.Background(), "/a/b", ""); err != nil || dir.ID != "B" {
		t.Fatalf("Mkdir(/a/b) = %+v, %v", dir, err)
	}
	if got := ops(rec); got != "query query exec query" {
		t.Errorf("ops without a unit = %q, want no transaction", got)
	}
}

// TestListContinuesEachHalfFromItsCursor proves database.go lowers each
// half's cursor on its own: the file half read after a cursor runs the
// plain statement with the keyset predicate and reports NoTotal and its
// own Next, while the directory half, with no cursor, is read by number
// under TotalExact with its total; and that ls / --unit with a cursor is
// ErrNoCursorAtRoot before any I/O.
func TestListContinuesEachHalfFromItsCursor(t *testing.T) {
	s, _ := newStore(t,
		root(),
		listing(directoryColumns, true, 1, "docs"),
		listing(fileColumns, true, 3, "a.txt", "b.txt", "c.txt"),
	)
	first, err := s.List(context.Background(), "/", files.Listing{Page: 1, Size: 2})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if first.Files.Next == "" || first.Directories.Next != "" || len(first.Files.Rows) != 2 || first.Files.Total != 3 {
		t.Fatalf("page 1 = files next %q, directories next %q, %d files, total %d; want a file cursor only, 2 files, total 3", first.Files.Next, first.Directories.Next, len(first.Files.Rows), first.Files.Total)
	}

	s, rec := newStore(t,
		root(),
		listing(directoryColumns, true, 1, "docs"),
		listing(fileColumns, false, 0, "c.txt"),
	)
	second, err := s.List(context.Background(), "/", files.Listing{Page: 1, Size: 2, After: files.After{Files: first.Files.Next}})
	if err != nil {
		t.Fatalf("List after the file cursor: %v", err)
	}
	queries := rec.SQL(sqltest.OpQuery)
	if !strings.Contains(queries[1], "COUNT(*) OVER ()") || strings.Contains(queries[1], " AND q.name > ") {
		t.Errorf("the directory half was not read by number with its total:\n%s", queries[1])
	}
	if strings.Contains(queries[2], "COUNT(*) OVER ()") || !strings.HasSuffix(queries[2], " AND q.name > CAST($2 AS text) ORDER BY q.name OFFSET $3 ROWS FETCH NEXT $4 ROWS ONLY") {
		t.Errorf("the file half was not read after the cursor:\n%s", queries[2])
	}
	if args := rec.Calls()[3].Args; len(args) != 4 || args[1] != "b.txt" || args[2] != 0 {
		t.Errorf("the file half bound %v, want the last name and offset 0", args)
	}
	if second.Files.Total != files.NoTotal || second.Directories.Total != 1 || second.Files.Next != "" || len(second.Files.Rows) != 1 {
		t.Errorf("contents = %+v", second)
	}

	s, rec = newStore(t)
	_, err = s.List(context.Background(), "/", files.Listing{Page: 1, Size: 2, Unit: blobfs.NewID(), After: files.After{Directories: first.Files.Next}})
	if !errors.Is(err, files.ErrNoCursorAtRoot) {
		t.Errorf("ls / --unit with a cursor = %v, want ErrNoCursorAtRoot", err)
	}
	if n := len(rec.Calls()); n != 0 {
		t.Errorf("the refusal reached the driver with %d calls", n)
	}
}
