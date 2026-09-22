//go:build integration

package data_test

// This file is proof V3, the cost of the shipped listing. It runs only
// under BLOBFS_EVIDENCE=1; mise run evidence sets the variable and writes
// the transcript to evidence/read-model.txt. mise run integration skips
// it. It seeds one fixture in a throwaway database, captures the SQL the
// store composes by running the store through a recording session, and
// runs EXPLAIN (ANALYZE, BUFFERS) on that SQL and on the hand-written
// forms it is compared with. Nothing here changes the shipped DDL.
//
// The measurement-only SQL in this file (the seeding through unnest with
// ::text[] casts, VACUUM, ANALYZE, and the hand-written baseline forms)
// uses native Postgres forms freely: it is test code, not a statement the
// library ships, and the standard-tier rules apply to shipped SQL only.

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"os"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/migrate"
	"github.com/standards-lab/sqlate/postgres"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
	blobfspostgres "github.com/standards-lab/org/experiments/blobfs/lib/blobfs/postgres"
)

// The fixture's size in file rows, the page size the forms list, the
// timed runs per query, the depth limit, and the number of trees under
// the root.
const (
	fixtureFiles = 100_000
	pageSize     = 20
	runs         = 5
	maxDepth     = 6
	trees        = 3
)

// recorder is a sqlate.Session over the pool that keeps every query's text
// and arguments, so the measurement explains exactly what the store ran.
// The embedded *sqlate.DB keeps MapError reachable.
type recorder struct {
	*sqlate.DB
	calls []call
}

// call is one query as the engine received it.
type call struct {
	sql  string
	args []any
}

func (r *recorder) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	r.calls = append(r.calls, call{sql: q, args: args})
	return r.DB.QueryContext(ctx, q, args...)
}

// dirRow is one generated directory.
type dirRow struct {
	id, parent, name string
	depth            int
	path             string
	files            int
}

// forest is the seeded fixture: every directory, the small directory the
// forms list, and the biggest one.
type forest struct {
	dirs    []dirRow
	small   dirRow
	biggest dirRow
}

// result is one measured query: the median run's times and buffers.
type result struct {
	section, label, dir string
	planning, execution float64
	buffers             int
	shape               string
}

// measurement collects the transcript: the body as it is written and the
// summary rows.
type measurement struct {
	body    strings.Builder
	results []result
}

func (m *measurement) note(format string, args ...any) {
	fmt.Fprintf(&m.body, format+"\n", args...)
}

// TestListingCost is proof V3. It seeds three trees under the root with
// about one directory per ten files and 100,000 files, a tenth of them in
// one directory at depth two, then measures, as labeled sections: the
// biggest directory's exact-total page before and after VACUUM; the
// whole-forest baseline (a recursion from the root computing every
// directory's path, joined to the files, filtered after, in the
// derived-table shape the volume-era projection produced); the shipped
// listing with the exact total and with none, for a small directory and
// the biggest; the last page of the biggest directory by offset and by
// cursor; and the derived-table wrap of the shipped base in the shapes a
// query.Projection would produce, plus the wrap over a base that contains
// WITH RECURSIVE. Every query is EXPLAIN (ANALYZE, BUFFERS) once to warm
// the cache and then five times; the median run by execution time is
// reported with its plan.
func TestListingCost(t *testing.T) {
	if os.Getenv("BLOBFS_EVIDENCE") == "" {
		t.Skip("set BLOBFS_EVIDENCE=1 to run the listing cost measurement")
	}
	ctx := context.Background()
	db, dsn := livetest.OpenDSN(t)
	set, err := blobfspostgres.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	mig, err := migrate.New(db, set.Migrations, migrate.Options{Table: set.Table})
	if err != nil {
		t.Fatal(err)
	}
	if err := mig.Up(ctx); err != nil {
		t.Fatal(err)
	}
	version := scalar(ctx, t, db, "SELECT version()")
	c, err := query.NewCatalog(query.Patterns(), data.Patterns())
	if err != nil {
		t.Fatal(err)
	}
	store, err := data.New(c, db.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Verify(ctx, db); err != nil {
		t.Fatal(err)
	}
	explainer := openSimple(t, dsn)
	m := &measurement{}

	fx := seedForest(ctx, t, db)
	m.note("")
	m.note("==================== fixture ====================")
	m.note("directories: %d under the root in %d trees; files: %d; depth reached: %d", len(fx.dirs), trees, fixtureFiles, maxDepthOf(fx.dirs))
	m.note("small directory: %s (depth %d, %d files)", fx.small.path, fx.small.depth, fx.small.files)
	m.note("biggest directory: %s (depth %d, %d files)", fx.biggest.path, fx.biggest.depth, fx.biggest.files)

	// The listings as the store composes them, captured once per shape.
	exact := func(dir string, l data.Listing) call { return captureFiles(ctx, t, store, db, dir, l) }
	page1 := data.Listing{Page: 1, Size: pageSize}
	none1 := data.Listing{Page: 1, Size: pageSize, Total: data.TotalNone}

	m.note("")
	m.note("==================== 0. the biggest directory's count before and after VACUUM ====================")
	m.note("The fixture is analyzed and not yet vacuumed, as the volume-era fixture was; the visibility map is empty, so an index-only scan must read the heap.")
	m.note("Two counts: the shipped exact-total page (the window count in the page statement) and a plain count twin over the same anchor.")
	countTwin := call{sql: "SELECT COUNT(*) FROM blobfs_file q WHERE q.directory_id = CAST($1 AS uuid)", args: []any{fx.biggest.id}}
	m.note("count twin: %s", countTwin.sql)
	measure(ctx, t, m, explainer, "0", "shipped exact total, not vacuumed", "biggest", exact(fx.biggest.id, page1))
	measure(ctx, t, m, explainer, "0", "count twin, not vacuumed", "biggest", countTwin)
	for _, stmt := range []string{"VACUUM ANALYZE blobfs_directory", "VACUUM ANALYZE blobfs_file"} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	m.note("")
	m.note("VACUUM ANALYZE ran on both tables; every section below is measured after it.")
	measure(ctx, t, m, explainer, "0", "shipped exact total, vacuumed", "biggest", exact(fx.biggest.id, page1))
	measure(ctx, t, m, explainer, "0", "count twin, vacuumed", "biggest", countTwin)

	m.note("")
	m.note("==================== a. the whole-forest baseline ====================")
	m.note("The volume-era shape: a projection whose base recurses from the root over every directory computing its path, joins the files, and takes the directory as a directive outside the derived table; the count twin and the page.")
	m.note("count: %s", forestCount)
	m.note("page:  %s", forestPage)
	for _, target := range []struct {
		name string
		dir  dirRow
	}{{"small", fx.small}, {"biggest", fx.biggest}} {
		measure(ctx, t, m, explainer, "a", "forest count", target.name, call{sql: forestCount, args: []any{target.dir.id}})
		measure(ctx, t, m, explainer, "a", "forest page", target.name, call{sql: forestPage, args: []any{target.dir.id, 0, pageSize}})
	}

	m.note("")
	m.note("==================== b. the shipped listing, exact total ====================")
	m.note("ListFiles under TotalExact, page 1 of %d by name: one statement, the window count in the select list, one row beyond the page fetched.", pageSize)
	shippedExact := exact(fx.small.id, page1)
	m.note("SQL as run: %s", shippedExact.sql)
	measure(ctx, t, m, explainer, "b", "shipped exact", "small", shippedExact)
	measure(ctx, t, m, explainer, "b", "shipped exact", "biggest", exact(fx.biggest.id, page1))

	m.note("")
	m.note("==================== c. the shipped listing, no total ====================")
	m.note("ListFiles under TotalNone, page 1 of %d by name.", pageSize)
	shippedNone := exact(fx.small.id, none1)
	m.note("SQL as run: %s", shippedNone.sql)
	measure(ctx, t, m, explainer, "c", "shipped none", "small", shippedNone)
	measure(ctx, t, m, explainer, "c", "shipped none", "biggest", exact(fx.biggest.id, none1))

	m.note("")
	m.note("==================== d. the last page of the biggest directory, by offset and by cursor ====================")
	lastPage := (fx.biggest.files + pageSize - 1) / pageSize
	m.note("The biggest directory has %d files, so its last page is page %d of %d (offset %d).", fx.biggest.files, lastPage, pageSize, (lastPage-1)*pageSize)
	lastNone := data.Listing{Page: lastPage, Size: pageSize, Total: data.TotalNone}
	lastExact := data.Listing{Page: lastPage, Size: pageSize}
	before, err := store.ListFiles(ctx, db, fx.biggest.id, data.Listing{Page: lastPage - 1, Size: pageSize, Total: data.TotalNone})
	if err != nil || before.Next == "" {
		t.Fatalf("the page before the last = %+v, %v; want a cursor", before, err)
	}
	byCursor := exact(fx.biggest.id, data.Listing{Size: pageSize, After: before.Next})
	m.note("offset, no total: %s", exact(fx.biggest.id, lastNone).sql)
	m.note("cursor (the Next of page %d, a name), no total by construction: %s", lastPage-1, byCursor.sql)
	measure(ctx, t, m, explainer, "d", "last page by offset, none", "biggest", exact(fx.biggest.id, lastNone))
	measure(ctx, t, m, explainer, "d", "last page by offset, exact", "biggest", exact(fx.biggest.id, lastExact))
	measure(ctx, t, m, explainer, "d", "last page by cursor", "biggest", byCursor)
	offsetRows, err := store.ListFiles(ctx, db, fx.biggest.id, lastNone)
	if err != nil {
		t.Fatal(err)
	}
	cursorRows, err := store.ListFiles(ctx, db, fx.biggest.id, data.Listing{Size: pageSize, After: before.Next})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(offsetRows.Rows), ids(cursorRows.Rows)) || cursorRows.Next != "" || offsetRows.Next != "" {
		t.Errorf("the last page differs by offset and by cursor, or is not the last:\n%v\n%v", ids(offsetRows.Rows), ids(cursorRows.Rows))
	}
	m.note("cross-check: the last page holds the same %d ids by offset and by cursor, and neither issues a next cursor.", len(offsetRows.Rows))

	m.note("")
	m.note("==================== e. the derived-table wrap of the same base ====================")
	m.note("The shapes a query.Projection produces over the shipped statement's base, for the biggest directory, against the flat statement of sections b and c:")
	m.note("  e1  the base without its anchor, the directory as a directive outside the wrap, the count twin outside (a projection base cannot bind a parameter)")
	m.note("  e2  a parameterized base with the anchor bound inside, wrapped, sorted and paged outside; and the same with the window count inside the base")
	m.note("  e3  a base that contains WITH RECURSIVE (the upward walk that computes the listed directory's path, joined to the files), flat and wrapped")
	e1Page := call{sql: wrapUnanchoredPage, args: []any{fx.biggest.id, 0, pageSize}}
	e1Count := call{sql: wrapUnanchoredCount, args: []any{fx.biggest.id}}
	e2Page := call{sql: wrapAnchoredPage, args: []any{fx.biggest.id, 0, pageSize}}
	e2Window := call{sql: wrapAnchoredWindowPage, args: []any{fx.biggest.id, 0, pageSize}}
	e3Flat := call{sql: walkFlatPage, args: []any{fx.biggest.id, 0, pageSize}}
	e3Wrap := call{sql: walkWrappedPage, args: []any{fx.biggest.id, 0, pageSize}}
	for _, q := range []struct {
		label string
		c     call
	}{
		{"e1 wrap, unanchored base, count twin", e1Count},
		{"e1 wrap, unanchored base, page", e1Page},
		{"e2 wrap, anchored base, page", e2Page},
		{"e2 wrap, anchored base with the window count, page", e2Window},
		{"e3 recursive base, flat, page", e3Flat},
		{"e3 recursive base, wrapped, page", e3Wrap},
	} {
		m.note("%s: %s", q.label, q.c.sql)
		measure(ctx, t, m, explainer, "e", q.label, "biggest", q.c)
	}
	// The wrapped forms return what the flat one returns.
	flat := column(ctx, t, db, 0, exact(fx.biggest.id, none1).sql, fx.biggest.id, 0, pageSize)
	for _, q := range []struct {
		label string
		c     call
	}{{"e1", e1Page}, {"e2", e2Page}, {"e2 window", e2Window}, {"e3 flat", e3Flat}, {"e3 wrapped", e3Wrap}} {
		if got := column(ctx, t, db, 0, q.c.sql, q.c.args...); !slices.Equal(got, flat[:len(got)]) || len(got) != pageSize {
			t.Errorf("%s returns a different first page:\n%v\n%v", q.label, got, flat)
		}
	}
	for _, q := range []call{e1Count, {sql: forestCount, args: []any{fx.biggest.id}}} {
		if n := scalar(ctx, t, db, q.sql, q.args...); n != strconv.Itoa(fx.biggest.files) {
			t.Errorf("a count twin returns %s, the fixture has %d", n, fx.biggest.files)
		}
	}
	smallExact, err := store.ListFiles(ctx, db, fx.small.id, page1)
	if err != nil || smallExact.Total != fx.small.files {
		t.Errorf("the shipped listing's total for the small directory is %d, %v; the fixture has %d", smallExact.Total, err, fx.small.files)
	}
	m.note("cross-check: every wrapped form returns the flat form's first page, the count twins count %d for the biggest directory, and the shipped exact total for the small directory is %d.", fx.biggest.files, smallExact.Total)

	var out strings.Builder
	fmt.Fprintf(&out, "# blobfs proof V3: the cost of the shipped listing\n")
	fmt.Fprintf(&out, "# date: %s\n", time.Now().UTC().Format("2006-01-02"))
	fmt.Fprintf(&out, "# engine: %s\n", version)
	fmt.Fprintf(&out, "# machine: %s/%s, %d cpus; go %s. Timings are machine-dependent (a laptop, everything in shared buffers); plan shapes and buffer counts are not.\n", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.Version())
	fmt.Fprintf(&out, "# fixture: %d trees under the root up to depth %d, about one directory per ten files, %d file rows; a tenth of the files in one directory, at depth two (the biggest), the rest spread uniformly over every directory.\n", trees, maxDepth, fixtureFiles)
	fmt.Fprintf(&out, "#   The small directory is at the deepest level reached, with the file count nearest ten.\n")
	fmt.Fprintf(&out, "# method: each query is EXPLAIN (ANALYZE, BUFFERS) once to warm the cache, then %d times; the run with the median execution time is reported with its plan.\n", runs)
	fmt.Fprintf(&out, "#   EXPLAIN runs over pgx's simple protocol, so each run is planned with its literal values, as a custom plan is; a prepared statement's generic plan could differ.\n")
	fmt.Fprintf(&out, "#   Buffers are the top plan node's shared hit+read, in 8 KB pages. The tables were VACUUM ANALYZEd after seeding, except where section 0 says otherwise.\n")
	fmt.Fprintf(&out, "#\n# summary (median run; times in ms; buffers as hit+read pages):\n")
	fmt.Fprintf(&out, "# %-3s %-52s %-8s %9s %9s %8s  %s\n", "sec", "query", "dir", "plan ms", "exec ms", "buffers", "plan shape")
	for _, r := range m.results {
		fmt.Fprintf(&out, "# %-3s %-52s %-8s %9.3f %9.3f %8d  %s\n", r.section, r.label, r.dir, r.planning, r.execution, r.buffers, r.shape)
	}
	out.WriteString(m.body.String())
	fmt.Print(out.String())
}

// The whole-forest baseline, the volume-era form 1 without the volume: a
// recursion from the root over every directory computing its path, the
// files joined, wrapped as a derived table with the directory as a
// directive outside, in the shape the query library's projection
// produced. The count twin and the page.
const forestBase = `WITH RECURSIVE tree (id, parent_id, path) AS (
    SELECT d.id, d.parent_id, CAST('' AS text)
    FROM blobfs_directory d
    WHERE d.parent_id IS NULL
  UNION ALL
    SELECT d.id, d.parent_id, t.path || '/' || d.name
    FROM blobfs_directory d
    JOIN tree t ON t.id = d.parent_id
)
SELECT f.id, f.directory_id, f.name, f.status, f.key, f.size, f.content_type, f.etag, f.version, f.created_at, f.updated_at, t.path || '/' || f.name AS path
FROM blobfs_file f
JOIN tree t ON t.id = f.directory_id`

const (
	forestCount = "SELECT COUNT(*) FROM (" + forestBase + ") q WHERE q.directory_id = CAST($1 AS uuid)"
	forestPage  = "SELECT * FROM (" + forestBase + ") q WHERE q.directory_id = CAST($1 AS uuid) ORDER BY q.name OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY"
)

// The shipped base without its anchor, as a projection base has to be
// (it cannot bind a parameter): the directory becomes a directive outside
// the wrap, and the total a count twin.
const shippedColumns = "q.id, q.directory_id, q.name, q.status, q.key, q.size, q.content_type, q.etag, q.version, q.created_at, q.updated_at"

const (
	wrapUnanchoredCount = "SELECT COUNT(*) FROM (SELECT " + shippedColumns + " FROM blobfs_file q) q WHERE q.directory_id = CAST($1 AS uuid)"
	wrapUnanchoredPage  = "SELECT * FROM (SELECT " + shippedColumns + " FROM blobfs_file q) q WHERE q.directory_id = CAST($1 AS uuid) ORDER BY q.name OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY"
)

// The shipped base with its anchor bound inside, wrapped: the shape a
// parameterized projection base would produce, with the sort and the
// paging outside; and the same with the window count inside the base,
// which is where the total has to be for the page and the total to come
// from one statement.
const (
	wrapAnchoredPage       = "SELECT * FROM (SELECT " + shippedColumns + " FROM blobfs_file q WHERE q.directory_id = CAST($1 AS uuid)) q ORDER BY q.name OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY"
	wrapAnchoredWindowPage = "SELECT * FROM (SELECT " + shippedColumns + ", COUNT(*) OVER () AS total FROM blobfs_file q WHERE q.directory_id = CAST($1 AS uuid)) q ORDER BY q.name OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY"
)

// A base that contains WITH RECURSIVE: the upward walk from the listed
// directory that computes its path (the volume-era form 2 without the
// owner), joined to the files of that directory. Flat, with the sort and
// paging at the base's level; and wrapped as a derived table.
const walkBase = "WITH RECURSIVE up (id, parent_id, path) AS (" +
	"SELECT d.id, d.parent_id, CAST(CASE WHEN d.parent_id IS NULL THEN '' ELSE '/' || d.name END AS text) FROM blobfs_directory d WHERE d.id = CAST($1 AS uuid) " +
	"UNION ALL SELECT p.id, p.parent_id, CASE WHEN p.parent_id IS NULL THEN '' ELSE '/' || p.name END || up.path FROM blobfs_directory p JOIN up ON p.id = up.parent_id) " +
	"SELECT f.id, f.directory_id, f.name, f.status, f.key, f.size, f.content_type, f.etag, f.version, f.created_at, f.updated_at, up.path || '/' || f.name AS path " +
	"FROM blobfs_file f JOIN up ON up.parent_id IS NULL WHERE f.directory_id = CAST($1 AS uuid)"

const (
	walkFlatPage    = walkBase + " ORDER BY f.name OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY"
	walkWrappedPage = "SELECT * FROM (" + walkBase + ") q ORDER BY q.name OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY"
)

// captureFiles runs ListFiles through the recorder and returns the one
// query it composed, as the engine received it.
func captureFiles(ctx context.Context, t *testing.T, store *data.Store, db *sqlate.DB, dir string, l data.Listing) call {
	t.Helper()
	rec := &recorder{DB: db}
	if _, err := store.ListFiles(ctx, rec, dir, l); err != nil {
		t.Fatalf("ListFiles(%+v): %v", l, err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("ListFiles ran %d queries, want 1", len(rec.calls))
	}
	return rec.calls[0]
}

// ids returns the ids of a page's rows.
func ids(rows []blobfs.File) []string {
	out := make([]string, len(rows))
	for i, f := range rows {
		out[i] = f.ID
	}
	return out
}

// measure explains one query, records the median run, and writes the
// section line and the plan to the transcript.
func measure(ctx context.Context, t *testing.T, m *measurement, explainer *sqlate.DB, section, label, dir string, c call) {
	t.Helper()
	r, plan := explain(ctx, t, explainer, c)
	r.section, r.label, r.dir = section, label, dir
	r.shape = strings.Join(shapeOf(plan), ", ")
	m.results = append(m.results, r)
	m.note("")
	m.note("---- %s: %s, %s directory: median of %d runs: planning %.3f ms, execution %.3f ms, buffers %d ----", section, label, dir, runs, r.planning, r.execution, r.buffers)
	m.note("%s", plan)
}

var (
	planningRe  = regexp.MustCompile(`Planning Time: ([0-9.]+) ms`)
	executionRe = regexp.MustCompile(`Execution Time: ([0-9.]+) ms`)
	buffersRe   = regexp.MustCompile(`Buffers: shared( hit=(\d+))?( read=(\d+))?`)
)

// explain runs EXPLAIN (ANALYZE, BUFFERS) on c once to warm the cache and
// then runs times, and returns the median run by execution time with its
// plan text.
func explain(ctx context.Context, t *testing.T, db *sqlate.DB, c call) (result, string) {
	t.Helper()
	type run struct {
		result
		plan string
	}
	var all []run
	for i := 0; i <= runs; i++ {
		rows, err := db.QueryContext(ctx, "EXPLAIN (ANALYZE, BUFFERS) "+c.sql, c.args...)
		if err != nil {
			t.Fatalf("explain: %v\n%s", err, c.sql)
		}
		var lines []string
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				t.Fatal(err)
			}
			lines = append(lines, line)
		}
		_ = rows.Close()
		if i == 0 {
			continue
		}
		plan := strings.Join(lines, "\n")
		r := run{plan: plan}
		r.planning = number(planningRe.FindStringSubmatch(plan), 1)
		r.execution = number(executionRe.FindStringSubmatch(plan), 1)
		if b := buffersRe.FindStringSubmatch(plan); b != nil {
			r.buffers = int(number(b, 2) + number(b, 4))
		}
		all = append(all, r)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].execution < all[j].execution })
	median := all[len(all)/2]
	return median.result, median.plan
}

func number(m []string, i int) float64 {
	if len(m) <= i || m[i] == "" {
		return 0
	}
	f, _ := strconv.ParseFloat(m[i], 64)
	return f
}

// shapeOf names the plan nodes that tell the forms apart, in a fixed order.
func shapeOf(plan string) []string {
	var out []string
	for _, node := range []string{
		"Recursive Union", "WorkTable Scan", "CTE Scan", "Subquery Scan",
		"Seq Scan on blobfs_directory", "Index Scan using blobfs_pk_directory", "Index Scan using blobfs_uq_directory",
		"Seq Scan on blobfs_file", "Index Scan using blobfs_uq_file_directory_name", "Index Only Scan using blobfs_uq_file_directory_name",
		"Index Scan Backward using blobfs_uq_file_directory_name", "Bitmap Heap Scan on blobfs_file", "Bitmap Index Scan on blobfs_uq_file_directory_name",
		"Index Scan using blobfs_ix_file_directory_created", "Index Scan Backward using blobfs_ix_file_directory_created", "Bitmap Index Scan on blobfs_ix_file_directory_created",
		"Hash Join", "Nested Loop", "Merge Join", "Sort", "Incremental Sort", "Aggregate", "WindowAgg", "Limit",
		"Heap Fetches: 0",
	} {
		if strings.Contains(plan, node) {
			out = append(out, node)
		}
	}
	return out
}

// openSimple opens a second pool on the fixture database over pgx's simple
// protocol, so EXPLAIN runs are planned with their literal values on every
// run and no prepared-statement cache switches to a generic plan mid-way.
func openSimple(t *testing.T, dsn string) *sqlate.DB {
	t.Helper()
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	pool, err := sql.Open("pgx", dsn+sep+"default_query_exec_mode=simple_protocol")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	return sqlate.Wrap(pool, postgres.Dialect{})
}

// seedForest builds the fixture: three depth-one directories as the tree
// roots, then the directories and the files in bulk through unnest,
// deterministic under a fixed seed. Directory i attaches to a directory
// chosen uniformly among the tree roots and the directories above depth
// maxDepth that exist so far. Every tenth file goes to the first
// generated directory (the biggest); the rest to a directory chosen
// uniformly among all. The tables are analyzed, and not vacuumed, so
// section 0 can measure the difference.
func seedForest(ctx context.Context, t *testing.T, db *sqlate.DB) forest {
	t.Helper()
	rng := rand.New(rand.NewPCG(20260920, fixtureFiles))
	var fx forest
	for i := range trees {
		name := "t" + strconv.Itoa(i)
		fx.dirs = append(fx.dirs, dirRow{id: blobfs.NewID(), parent: blobfs.RootID, name: name, depth: 1, path: "/" + name})
	}
	n := fixtureFiles / 10
	for i := range n {
		var parent int
		for {
			parent = rng.IntN(len(fx.dirs))
			if fx.dirs[parent].depth < maxDepth {
				break
			}
		}
		p := fx.dirs[parent]
		name := "d" + strconv.Itoa(i)
		fx.dirs = append(fx.dirs, dirRow{id: blobfs.NewID(), parent: p.id, name: name, depth: p.depth + 1, path: p.path + "/" + name})
	}
	ids, parents, names := make([]string, 0, len(fx.dirs)), make([]string, 0, len(fx.dirs)), make([]string, 0, len(fx.dirs))
	for _, d := range fx.dirs {
		ids, parents, names = append(ids, d.id), append(parents, d.parent), append(names, d.name)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO blobfs_directory (id, parent_id, name) SELECT CAST(u.id AS uuid), CAST(u.parent_id AS uuid), u.name FROM unnest($1::text[], $2::text[], $3::text[]) AS u(id, parent_id, name)", ids, parents, names); err != nil {
		t.Fatalf("insert directories: %v", err)
	}

	const batch = 20_000
	fids, fdirs, fnames, fkeys := make([]string, 0, batch), make([]string, 0, batch), make([]string, 0, batch), make([]string, 0, batch)
	fsizes := make([]int64, 0, batch)
	flush := func() {
		if len(fids) == 0 {
			return
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO blobfs_file (id, directory_id, name, status, key, size, content_type) "+
			"SELECT CAST(u.id AS uuid), CAST(u.directory_id AS uuid), u.name, 'available', u.key, u.size, 'text/plain' "+
			"FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::bigint[]) AS u(id, directory_id, name, key, size)",
			fids, fdirs, fnames, fkeys, fsizes); err != nil {
			t.Fatalf("insert files: %v", err)
		}
		fids, fdirs, fnames, fkeys, fsizes = fids[:0], fdirs[:0], fnames[:0], fkeys[:0], fsizes[:0]
	}
	for i := range fixtureFiles {
		dir := trees // the first generated directory is the biggest
		if i%10 != 0 {
			dir = rng.IntN(len(fx.dirs))
		}
		fx.dirs[dir].files++
		id := blobfs.NewID()
		name := "f" + strconv.Itoa(i) + ".txt"
		fids, fdirs, fnames, fkeys, fsizes = append(fids, id), append(fdirs, fx.dirs[dir].id), append(fnames, name), append(fkeys, id+"/"+name), append(fsizes, int64(i%4096))
		if len(fids) == batch {
			flush()
		}
	}
	flush()
	if _, err := db.ExecContext(ctx, "ANALYZE blobfs_directory, blobfs_file"); err != nil {
		t.Fatal(err)
	}

	fx.biggest = fx.dirs[trees]
	deepest := maxDepthOf(fx.dirs)
	best := -1
	for i, d := range fx.dirs {
		if d.depth != deepest || d.files == 0 {
			continue
		}
		if best < 0 || abs(d.files-10) < abs(fx.dirs[best].files-10) {
			best = i
		}
	}
	if best < 0 {
		t.Fatalf("no directory at depth %d holds a file", deepest)
	}
	fx.small = fx.dirs[best]
	return fx
}

func maxDepthOf(dirs []dirRow) int {
	m := 0
	for _, d := range dirs {
		m = max(m, d.depth)
	}
	return m
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// scalar runs a one-row, one-column query and returns the value as text.
func scalar(ctx context.Context, t *testing.T, db *sqlate.DB, q string, args ...any) string {
	t.Helper()
	col := column(ctx, t, db, 0, q, args...)
	if len(col) != 1 {
		t.Fatalf("%s: %d rows, want 1", q, len(col))
	}
	return col[0]
}

// column runs a query and returns column i of every row as text.
func column(ctx context.Context, t *testing.T, db *sqlate.DB, i int, q string, args ...any) []string {
	t.Helper()
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for rows.Next() {
		dest := make([]any, len(cols))
		for j := range dest {
			dest[j] = new(sql.NullString)
		}
		if err := rows.Scan(dest...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		out = append(out, dest[i].(*sql.NullString).String)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
