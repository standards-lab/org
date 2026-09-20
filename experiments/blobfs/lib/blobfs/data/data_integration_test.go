//go:build integration

package data_test

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/migrate"
	"github.com/standards-lab/sqlate/postgres"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/migrations"
)

// env is one test's throwaway database with blobfs's migration set applied,
// its DSN, and the store compiled against the consumer's catalog.
type env struct {
	ctx   context.Context
	db    *sqlate.DB
	dsn   string
	store *data.Store
}

// open builds the environment and proves Verify against the migrated
// schema on the way, so every test starts from a store whose statements
// and listing renderings the schema satisfies.
func open(t *testing.T) env {
	t.Helper()
	ctx := context.Background()
	db, dsn := livetest.OpenDSN(t)
	set, err := migrations.Migrations(db.Dialect())
	if err != nil {
		t.Fatalf("Migrations: %v", err)
	}
	m, err := migrate.New(db, set, migrate.Options{Table: migrations.Table})
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	if err := m.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	c, err := query.NewCatalog(query.Patterns(), data.Patterns())
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	s, err := data.New(c, db.Dialect())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Verify(ctx, db); err != nil {
		t.Fatalf("Verify against the migrated schema: %v", err)
	}
	return env{ctx: ctx, db: db, dsn: dsn, store: s}
}

// second opens a second pool to the same database, so a write through it
// runs on a connection of its own.
func (e env) second(t *testing.T) *sqlate.DB {
	t.Helper()
	pool, err := sql.Open("pgx", e.dsn)
	if err != nil {
		t.Fatalf("open a second pool: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	return sqlate.Wrap(pool, postgres.Dialect{})
}

// mkdir creates a directory and fails the test on any error.
func (e env) mkdir(t *testing.T, parentID, name string) blobfs.Directory {
	t.Helper()
	d, err := e.store.Mkdir(e.ctx, e.db, parentID, name)
	if err != nil {
		t.Fatalf("Mkdir(%q): %v", name, err)
	}
	return d
}

// insertFile inserts an available file row directly through sess, since
// the write path is a later stage, and returns its id.
func insertFile(ctx context.Context, t *testing.T, sess sqlate.Session, dir, name string) string {
	t.Helper()
	id := blobfs.NewID()
	_, err := sess.ExecContext(ctx,
		"INSERT INTO blobfs_file (id, directory_id, name, status, key, content_type, size) VALUES ($1, $2, $3, 'available', $4, 'text/plain', $5)",
		id, dir, name, id+"/"+blobfs.SanitizeFilename(name), len(name))
	if err != nil {
		t.Fatalf("insert file %s: %v", name, err)
	}
	return id
}

// strings1 runs a one-column text query and returns the column.
func (e env) strings1(t *testing.T, sql string, args ...any) []string {
	t.Helper()
	rows, err := e.db.QueryContext(e.ctx, sql, args...)
	if err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return out
}

// The two spellings of one name, built from code points so that no editor
// can normalize the fixtures.
var (
	composed   = "caf" + string(rune(0x00E9))  // é as one code point
	decomposed = "cafe" + string(rune(0x0301)) // e followed by a combining acute
)

// TestRoot proves the root as the store reads it: the seeded row with
// RootID, no parent, and no name, reached by Root, by Directory under its
// id, and by ResolveDirectory at /, and its path is /.
func TestRoot(t *testing.T) {
	e := open(t)
	root, err := e.store.Root(e.ctx, e.db)
	if err != nil {
		t.Fatalf("Root: %v", err)
	}
	if root.ID != blobfs.RootID || root.ParentID != nil || root.Name != nil || !root.IsRoot() || root.Version != 1 || root.CreatedAt.IsZero() {
		t.Errorf("root = %+v, want the seeded row", root)
	}
	if d, err := e.store.Directory(e.ctx, e.db, blobfs.RootID); err != nil || d != root {
		t.Errorf("Directory(RootID) = %+v, %v, want the root", d, err)
	}
	if d, err := e.store.ResolveDirectory(e.ctx, e.db, "/"); err != nil || d != root {
		t.Errorf("ResolveDirectory(/) = %+v, %v, want the root", d, err)
	}
	if p, err := e.store.DirectoryPath(e.ctx, e.db, blobfs.RootID); err != nil || p != "/" {
		t.Errorf("DirectoryPath(RootID) = %q, %v, want /", p, err)
	}
}

// TestNotFound proves ErrNotFound on every single-row read of a row that
// does not exist.
func TestNotFound(t *testing.T) {
	e := open(t)
	id := blobfs.NewID()
	if _, err := e.store.Directory(e.ctx, e.db, id); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("Directory(missing) = %v, want ErrNotFound", err)
	}
	if _, err := e.store.ResolveDirectory(e.ctx, e.db, "/missing"); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("ResolveDirectory(/missing) = %v, want ErrNotFound", err)
	}
	if _, err := e.store.DirectoryPath(e.ctx, e.db, id); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("DirectoryPath(missing) = %v, want ErrNotFound", err)
	}
}

// TestMkdir proves directory creation under the one root: the row as the
// database holds it, a taken name in either spelling, an invalid name, a
// missing parent as ErrNotFound through the foreign key, the same name
// allowed under another parent, and Children listing one parent's
// directories with the total, the root itself never among them.
func TestMkdir(t *testing.T) {
	e := open(t)
	docs := e.mkdir(t, blobfs.RootID, "docs")
	if docs.ParentID == nil || *docs.ParentID != blobfs.RootID || docs.Name == nil || *docs.Name != "docs" || docs.IsRoot() || docs.Version != 1 || docs.CreatedAt.IsZero() {
		t.Errorf("docs = %+v", docs)
	}
	e.mkdir(t, blobfs.RootID, composed)
	for _, name := range []string{"docs", composed, decomposed} {
		_, err := e.store.Mkdir(e.ctx, e.db, blobfs.RootID, name)
		if !errors.Is(err, blobfs.ErrNameTaken) {
			t.Errorf("Mkdir(%q) again = %v, want ErrNameTaken", name, err)
		}
	}
	if _, err := e.store.Mkdir(e.ctx, e.db, blobfs.RootID, "no/slash"); !errors.Is(err, blobfs.ErrInvalidName) {
		t.Errorf("Mkdir(no/slash) = %v, want ErrInvalidName", err)
	}
	_, err := e.store.Mkdir(e.ctx, e.db, blobfs.NewID(), "orphan")
	if !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("Mkdir under a missing parent = %v, want ErrNotFound", err)
	}
	var ce *sqlate.ConstraintError
	if !errors.As(err, &ce) || ce.Constraint != blobfs.ConstraintForeignKeyDirectoryParent {
		t.Errorf("Mkdir under a missing parent does not carry the foreign key: %v", err)
	}
	e.mkdir(t, docs.ID, "docs")
	d, err := e.store.Directory(e.ctx, e.db, docs.ID)
	if err != nil || d.ID != docs.ID || *d.ParentID != blobfs.RootID || *d.Name != "docs" || d.Version != 1 {
		t.Errorf("Directory = %+v, %v, want %+v", d, err, docs)
	}

	page, err := e.store.Children(e.ctx, e.db, blobfs.RootID, data.Listing{Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	var names []string
	for _, d := range page.Rows {
		if d.Name == nil || d.IsRoot() {
			t.Fatalf("Children of the root listed a row with no name or no parent: %+v", d)
		}
		names = append(names, *d.Name)
	}
	if want := e.strings1(t, "SELECT name FROM blobfs_directory WHERE parent_id = $1 ORDER BY name", blobfs.RootID); !slices.Equal(names, want) || page.Total != 2 {
		t.Errorf("Children of the root = %v, total %d, want %v, total 2", names, page.Total, want)
	}
	page, err = e.store.Children(e.ctx, e.db, blobfs.NewID(), data.Listing{Page: 1, Size: 10})
	if err != nil || len(page.Rows) != 0 || page.Total != 0 {
		t.Errorf("Children of a missing parent = %d rows, total %d, %v; want none, total 0", len(page.Rows), page.Total, err)
	}
	if n := e.strings1(t, "SELECT CAST(id AS text) FROM blobfs_directory WHERE parent_id IS NULL"); !slices.Equal(n, []string{blobfs.RootID}) {
		t.Errorf("roots after the mkdirs = %v, want the one seeded root", n)
	}
}

// TestResolveDirectory proves iterative resolution: / is the root, /a and
// /a/b walk one child per segment, a decomposed segment resolves to the
// composed row, and a missing segment at any depth is ErrNotFound naming
// the prefix that failed. DirectoryPath inverts every resolution.
func TestResolveDirectory(t *testing.T) {
	e := open(t)
	a := e.mkdir(t, blobfs.RootID, "a")
	b := e.mkdir(t, a.ID, "b")
	cafe := e.mkdir(t, b.ID, composed)
	for path, want := range map[string]string{
		"/":                  blobfs.RootID,
		"/a":                 a.ID,
		"/a/b":               b.ID,
		"/a/b/" + composed:   cafe.ID,
		"/a/b/" + decomposed: cafe.ID,
	} {
		d, err := e.store.ResolveDirectory(e.ctx, e.db, path)
		if err != nil || d.ID != want {
			t.Errorf("ResolveDirectory(%q) = %s, %v, want %s", path, d.ID, err, want)
			continue
		}
		back, err := e.store.DirectoryPath(e.ctx, e.db, d.ID)
		if wantPath := blobfs.NormalizeName(path); err != nil || back != wantPath {
			t.Errorf("DirectoryPath(%s) = %q, %v, want %q", d.ID, back, err, wantPath)
		}
	}
	for _, path := range []string{"/missing", "/a/missing", "/a/b/c/d"} {
		_, err := e.store.ResolveDirectory(e.ctx, e.db, path)
		if !errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("ResolveDirectory(%q) = %v, want ErrNotFound", path, err)
		}
	}
	_, err := e.store.ResolveDirectory(e.ctx, e.db, "/a/missing/deeper")
	if err == nil || !strings.Contains(err.Error(), `at /a/missing`) {
		t.Errorf("ResolveDirectory(/a/missing/deeper) = %v, want the failing prefix named", err)
	}
}

// fixture is the listing fixture: directories at four depths with file
// names repeated across directories, so a listing that leaked past its
// directory would show, and the path of every directory.
type fixture struct {
	paths map[string]string // directory path to id
}

// seed builds the fixture.
func seed(t *testing.T, e env) fixture {
	t.Helper()
	f := fixture{paths: map[string]string{"/": blobfs.RootID}}
	dir := func(parent, name string) string {
		d := e.mkdir(t, f.paths[parent], name)
		path := strings.TrimSuffix(parent, "/") + "/" + name
		f.paths[path] = d.ID
		return path
	}
	d1 := dir("/", "d1")
	d2 := dir("/", "d2")
	e1 := dir(d1, "e1")
	e2 := dir(d1, "e2")
	g := dir(e1, "g")
	files := map[string][]string{
		"/": {"r1", "r2", "a1"},
		d1:  {"a1", "a2", "a3", "b1"},
		e1:  {"a1", "x1", "a3"},
		e2:  {"y1"},
		g:   {"z1", "a1", "a2"},
		d2:  {"w1", "a1"},
	}
	for path, names := range files {
		for _, name := range names {
			insertFile(e.ctx, t, e.db, f.paths[path], name)
		}
	}
	return f
}

// forestBaseline is the test-only whole-forest query: the recursion over
// every directory from the root, each with its path, joined to the files
// and filtered on the path. It is the shape the shipped listing replaced,
// and the oracle the listing is compared against.
const forestBaseline = `WITH RECURSIVE tree (id, path) AS (
    SELECT d.id, CAST('' AS text) FROM blobfs_directory d WHERE d.parent_id IS NULL
  UNION ALL
    SELECT d.id, t.path || '/' || d.name FROM blobfs_directory d JOIN tree t ON t.id = d.parent_id
)
SELECT CAST(f.id AS text)
FROM blobfs_file f
JOIN tree t ON t.id = f.directory_id
WHERE CASE WHEN t.path = '' THEN '/' ELSE t.path END = $1 AND f.name LIKE $2
ORDER BY `

// pages runs ListFiles page by page until a short page, checking that
// every page reports the same total, and returns the ids concatenated and
// the total.
func pages(t *testing.T, e env, dir string, size int, l data.Listing) ([]string, int) {
	t.Helper()
	var ids []string
	total := data.NoTotal
	for page := 1; page <= 100; page++ {
		l.Page, l.Size = page, size
		p, err := e.store.ListFiles(e.ctx, e.db, dir, l)
		if err != nil {
			t.Fatalf("page %d of size %d: %v", page, size, err)
		}
		if l.Total == data.TotalExact && (len(p.Rows) > 0 || page == 1) {
			if total != data.NoTotal && p.Total != total {
				t.Errorf("page %d of size %d reports total %d, earlier pages %d", page, size, p.Total, total)
			}
			total = p.Total
		}
		for _, f := range p.Rows {
			ids = append(ids, f.ID)
		}
		if len(p.Rows) < size {
			return ids, total
		}
	}
	t.Fatalf("listing of size %d never ended", size)
	return nil, 0
}

// TestListingMatchesForest is the stage gate's fourth proof: for every
// directory of the fixture, sorted by name in both directions and by two
// other fields, at several page sizes, with and without a LIKE filter, the
// shipped listing's pages concatenated equal the rows the whole-forest
// baseline returns for that directory's path, and the exact total equals
// the baseline's count. TotalNone returns the same rows and NoTotal.
func TestListingMatchesForest(t *testing.T) {
	e := open(t)
	f := seed(t, e)
	sorts := []struct {
		label    string
		baseline string
		sort     []query.Sort
	}{
		{"name", "f.name", nil},
		{"name desc", "f.name DESC", []query.Sort{{Field: "name", Descending: true}}},
		{"size then name", "f.size, f.name", []query.Sort{{Field: "size"}}},
		{"created_at desc then name", "f.created_at DESC, f.name", []query.Sort{{Field: "created_at", Descending: true}}},
	}
	for path, dir := range f.paths {
		for _, like := range []string{"%", "a%"} {
			var filters []query.Filter
			if like != "%" {
				filters = []query.Filter{{Field: "name", Op: query.OpLike, Value: like}}
			}
			for _, s := range sorts {
				want := e.strings1(t, forestBaseline+s.baseline, path, like)
				for _, size := range []int{1, 2, 3, 10} {
					got, total := pages(t, e, dir, size, data.Listing{Sort: s.sort, Filters: filters})
					if !slices.Equal(got, want) {
						t.Errorf("%s like %q by %s, size %d: listing %v, forest %v", path, like, s.label, size, got, want)
					}
					if total != len(want) && !(len(want) == 0 && total == 0) {
						t.Errorf("%s like %q by %s, size %d: total %d, forest count %d", path, like, s.label, size, total, len(want))
					}
				}
				got, total := pages(t, e, dir, 2, data.Listing{Sort: s.sort, Filters: filters, Total: data.TotalNone})
				if !slices.Equal(got, want) || total != data.NoTotal {
					t.Errorf("%s like %q by %s without a total: listing %v, total %d; forest %v", path, like, s.label, got, total, want)
				}
			}
		}
	}
	// A directory that does not exist lists nothing, as the forest does.
	if got, total := pages(t, e, blobfs.NewID(), 5, data.Listing{}); len(got) != 0 || total != 0 {
		t.Errorf("listing of a missing directory = %v, total %d; want none, total 0", got, total)
	}
	// Children agree with the forest's directories under one parent.
	for path, dir := range f.paths {
		want := e.strings1(t, "SELECT CAST(d.id AS text) FROM blobfs_directory d WHERE d.parent_id = $1 ORDER BY d.name DESC", dir)
		page, err := e.store.Children(e.ctx, e.db, dir, data.Listing{Page: 1, Size: 10, Sort: []query.Sort{{Field: "name", Descending: true}}})
		if err != nil {
			t.Fatalf("Children of %s: %v", path, err)
		}
		var got []string
		for _, d := range page.Rows {
			got = append(got, d.ID)
		}
		if !slices.Equal(got, want) || page.Total != len(want) {
			t.Errorf("Children of %s = %v, total %d; want %v", path, got, page.Total, want)
		}
	}
}

// TestListingPagesPastTheEnd fixes the convention for a page beyond the
// last: no row carries the window count, so the page is empty and its
// Total is NoTotal, while an empty first page has the exact total 0.
func TestListingPagesPastTheEnd(t *testing.T) {
	e := open(t)
	insertFile(e.ctx, t, e.db, blobfs.RootID, "only")
	page, err := e.store.ListFiles(e.ctx, e.db, blobfs.RootID, data.Listing{Page: 3, Size: 5})
	if err != nil || len(page.Rows) != 0 || page.Total != data.NoTotal {
		t.Errorf("page past the end = %d rows, total %d, %v; want none and NoTotal", len(page.Rows), page.Total, err)
	}
	page, err = e.store.ListFiles(e.ctx, e.db, blobfs.RootID, data.Listing{Page: 1, Size: 5, Filters: []query.Filter{{Field: "name", Op: query.OpEq, Value: "none"}}})
	if err != nil || len(page.Rows) != 0 || page.Total != 0 {
		t.Errorf("empty first page = %d rows, total %d, %v; want none and the exact total 0", len(page.Rows), page.Total, err)
	}
}

// TestExactTotalUnderConcurrentInserts is the stage gate's second proof:
// the total a page carries agrees with the rows of that same statement
// while a second connection inserts between calls. On the pool, each call
// sees its own snapshot, and its total equals the rows it returns. Inside
// a read-only repeatable-read transaction, an insert committed between two
// pages changes neither the total nor the rows: both pages come from the
// transaction's snapshot, and the file inserted meanwhile appears, with the
// larger total, only to a listing after the transaction.
func TestExactTotalUnderConcurrentInserts(t *testing.T) {
	e := open(t)
	other := e.second(t)
	dir := e.mkdir(t, blobfs.RootID, "busy").ID
	for _, name := range []string{"a", "b", "c"} {
		insertFile(e.ctx, t, e.db, dir, name)
	}
	all := func(sess sqlate.Session, page, size int) data.Page[blobfs.File] {
		t.Helper()
		p, err := e.store.ListFiles(e.ctx, sess, dir, data.Listing{Page: page, Size: size})
		if err != nil {
			t.Fatalf("ListFiles page %d: %v", page, err)
		}
		return p
	}

	before := all(e.db, 1, 10)
	if before.Total != 3 || len(before.Rows) != 3 {
		t.Fatalf("before the insert: total %d, %d rows; want 3 and 3", before.Total, len(before.Rows))
	}
	insertFile(e.ctx, t, other, dir, "d")
	after := all(e.db, 1, 10)
	if after.Total != 4 || len(after.Rows) != 4 {
		t.Errorf("after the insert: total %d, %d rows; want 4 and 4 (the total agrees with its own rows)", after.Total, len(after.Rows))
	}

	tx, err := e.db.Begin(e.ctx, sqlate.ReadOnly(), sqlate.Isolation(sql.LevelRepeatableRead))
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	first := all(tx, 1, 2)
	insertFile(e.ctx, t, other, dir, "aa")
	second := all(tx, 2, 2)
	if first.Total != 4 || second.Total != 4 || len(first.Rows) != 2 || len(second.Rows) != 2 {
		t.Errorf("under repeatable read: totals %d and %d, rows %d and %d; want 4, 4, 2, 2", first.Total, second.Total, len(first.Rows), len(second.Rows))
	}
	var names []string
	for _, f := range append(first.Rows, second.Rows...) {
		names = append(names, f.Name)
	}
	if !slices.Equal(names, []string{"a", "b", "c", "d"}) {
		t.Errorf("the two pages under one snapshot = %v, want a b c d (the row inserted meanwhile excluded)", names)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	final := all(e.db, 1, 10)
	if final.Total != 5 || len(final.Rows) != 5 || final.Rows[0].Name != "a" || final.Rows[1].Name != "aa" {
		t.Errorf("after the transaction: total %d, %d rows, first %q %q; want 5, 5, a, aa", final.Total, len(final.Rows), final.Rows[0].Name, final.Rows[1].Name)
	}
}

// TestListingEngineRefusal proves a filter value the engine cannot read as
// the field's type is the request's fault: a data exception becomes a
// query.InvalidValueError, which unwraps to query.ErrDirectives.
func TestListingEngineRefusal(t *testing.T) {
	e := open(t)
	_, err := e.store.ListFiles(e.ctx, e.db, blobfs.RootID, data.Listing{Page: 1, Size: 1, Filters: []query.Filter{{Field: "id", Op: query.OpEq, Value: "not a uuid"}}})
	var invalid *query.InvalidValueError
	if !errors.As(err, &invalid) || !errors.Is(err, query.ErrDirectives) {
		t.Errorf("filter on a malformed uuid = %v, want InvalidValueError under ErrDirectives", err)
	}
}
