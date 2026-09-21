//go:build integration

package files

// This file is the stage 11 measurement: the cost of the consumer-anchored
// read model, the bookmark projection. It runs only under
// BLOBFS_EVIDENCE=1; mise run evidence sets the variable and writes the
// transcript to evidence/bookmarks.txt, and mise run integration skips
// it. It seeds one fixture in a throwaway database, captures the SQL the
// store composes by running the projection through a recording session,
// and runs EXPLAIN (ANALYZE, BUFFERS) on that SQL and on the hand-written
// forms it is compared with: the projection over a top-level recursion,
// which is the shape the plan described and the only shape a projection
// base without parameters can take when the recursion is not correlated,
// and the anchored plain statement with the unit bound inside the
// recursion, which is the shape a parameterized projection base would
// give. The file is in the package itself rather than in files_test so it
// can run the projection through a session of its own.
//
// The measurement-only SQL in this file (the seeding through unnest with
// ::text[] casts, VACUUM, ANALYZE, and the hand-written forms) uses native
// Postgres forms freely: it is test code, not a statement the consumer
// ships, and the standard-tier rules apply to shipped SQL only.

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
	"github.com/standards-lab/sqlate/postgres"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	blobfsmigrations "github.com/standards-lab/org/experiments/blobfs/lib/blobfs/migrations"
	"github.com/standards-lab/org/experiments/blobfs/lib/migrator"
	appmigrations "github.com/standards-lab/org/experiments/blobfs/migrations"
)

// The fixture's size: the file rows, the directories per file, the depth
// limit, the trees under the root, the background units and their
// bookmarks each, the page size, and the timed runs per query.
const (
	evFiles    = 100_000
	evMaxDepth = 6
	evTrees    = 3
	evUnits    = 50
	evPerUnit  = 1_000
	evPageSize = 20
	evRuns     = 5
)

// recorder is a sqlate.Session over the pool that keeps every query's text
// and arguments, so the measurement explains exactly what the projection
// ran. The embedded *sqlate.DB keeps MapError reachable.
type recorder struct {
	*sqlate.DB
	calls []evCall
}

// evCall is one query as the engine received it.
type evCall struct {
	sql  string
	args []any
}

func (r *recorder) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	r.calls = append(r.calls, evCall{sql: q, args: args})
	return r.DB.QueryContext(ctx, q, args...)
}

// evDir is one generated directory.
type evDir struct {
	id, parent, name string
	depth            int
}

// evUnit is one measured unit: its id, how many bookmarks it holds, and
// the mean depth of the directories its bookmarked files sit in.
type evUnit struct {
	label string
	id    string
	n     int
	depth float64
}

// evResult is one measured query: the median run's times and buffers.
type evResult struct {
	section, label, unit string
	planning, execution  float64
	buffers              int
	shape                string
}

// evMeasurement collects the transcript: the body as it is written and
// the summary rows.
type evMeasurement struct {
	body    strings.Builder
	results []evResult
}

func (m *evMeasurement) note(format string, args ...any) {
	fmt.Fprintf(&m.body, format+"\n", args...)
}

// TestBookmarkCost is the stage 11 measurement. It seeds three trees
// under the root with about one directory per ten files and 100,000
// files, then 50 background units with 1,000 bookmarks each and the
// measured units: 10, 100, and 1,000 bookmarks of random files, and 1,000
// bookmarks of files at the shallowest depth that holds a thousand of
// them and at depth six. After VACUUM ANALYZE
// it measures, as labeled sections: the shipped projection's count and
// page as the store composes them, against the bookmark count; the same
// page sorted by the key instead of the path; the projection over a
// top-level recursion anchored on every bookmarked file with the unit as
// a directive outside the derived table; the anchored plain statement
// with the unit bound inside the recursion; the shipped page against the
// depth; and the shipped page and the top-level recursion with JIT
// compilation off. Every query is
// EXPLAIN (ANALYZE, BUFFERS) once to warm the cache and then five times;
// the median run by execution time is reported with its plan.
func TestBookmarkCost(t *testing.T) {
	if os.Getenv("BLOBFS_EVIDENCE") == "" {
		t.Skip("set BLOBFS_EVIDENCE=1 to run the bookmark cost measurement")
	}
	ctx := context.Background()
	db, dsn := livetest.OpenDSN(t)
	evMigrate(ctx, t, db)
	version := evScalar(ctx, t, db, "SELECT version()")
	store, err := New(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Verify(ctx); err != nil {
		t.Fatal(err)
	}
	explainer := evOpenSimple(t, dsn, "")
	noJIT := evOpenSimple(t, dsn, "jit=off")
	jit := evScalar(ctx, t, explainer, "SHOW jit")
	m := &evMeasurement{}

	dirs, units := evSeed(ctx, t, db)
	for _, stmt := range []string{"VACUUM ANALYZE blobfs_directory", "VACUUM ANALYZE blobfs_file", "VACUUM ANALYZE bookmark"} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	byLabel := map[string]evUnit{}
	for _, u := range units {
		byLabel[u.label] = u
	}
	bookmarks := evScalar(ctx, t, db, "SELECT COUNT(*) FROM bookmark")
	m.note("")
	m.note("==================== fixture ====================")
	m.note("directories: %d under the root in %d trees; files: %d; depth reached: %d; bookmarks: %s (%d background units with %d each, and the measured units below)", len(dirs), evTrees, evFiles, evMaxDepthOf(dirs), bookmarks, evUnits, evPerUnit)
	for _, u := range units {
		m.note("unit %-8s %s: %5d bookmarks, mean directory depth %.2f", u.label, u.id, u.n, u.depth)
	}
	m.note("VACUUM ANALYZE ran on the three tables after seeding; every section is measured after it.")

	// The projection as the store composes it, captured once per shape.
	shipped := func(u evUnit, l Listing) (count, page evCall) {
		rec := &recorder{DB: db}
		if _, err := store.bookmarksOf(ctx, rec, u.id, l); err != nil {
			t.Fatalf("bookmarksOf(%s): %v", u.label, err)
		}
		if len(rec.calls) != 2 {
			t.Fatalf("bookmarksOf ran %d queries, want the count and the page", len(rec.calls))
		}
		return rec.calls[0], rec.calls[1]
	}
	page1 := Listing{Page: 1, Size: evPageSize}
	byKey := Listing{Page: 1, Size: evPageSize, Sort: []Sort{{Field: "file_id"}}}

	m.note("")
	m.note("==================== a. the shipped projection against the bookmark count ====================")
	m.note("bookmarks as the store composes it under TotalExact, page 1 of %d in path order: the count twin, then the page. The base computes each row's path by a recursion correlated on the file's directory; the unit is a filter directive outside the derived table.", evPageSize)
	count, page := shipped(byLabel["u10"], page1)
	m.note("count as run: %s", count.sql)
	m.note("page as run:  %s", page.sql)
	for _, label := range []string{"u10", "u100", "u1000"} {
		u := byLabel[label]
		count, page := shipped(u, page1)
		evMeasure(ctx, t, m, explainer, "a", "shipped count", u, count)
		evMeasure(ctx, t, m, explainer, "a", "shipped page, by path", u, page)
	}

	m.note("")
	m.note("==================== b. the shipped page sorted by the key ====================")
	m.note("The same page with --sort file_id: the sort is the bookmark index's order, so the walk runs for the page's rows only.")
	_, keyPage := shipped(byLabel["u1000"], byKey)
	m.note("page as run: %s", keyPage.sql)
	for _, label := range []string{"u10", "u1000"} {
		u := byLabel[label]
		_, page := shipped(u, byKey)
		evMeasure(ctx, t, m, explainer, "b", "shipped page, by file_id", u, page)
	}

	m.note("")
	m.note("==================== c. the projection over a top-level recursion ====================")
	m.note("The shape the plan described: a base whose recursion is a common table expression anchored on every bookmarked file's directory, walking upward, joined to the bookmarks; the unit as a directive outside the derived table, because a projection base binds no parameter. The count twin and the page.")
	m.note("count: %s", walkAllCount)
	m.note("page:  %s", walkAllPage)
	for _, label := range []string{"u10", "u1000"} {
		u := byLabel[label]
		evMeasure(ctx, t, m, explainer, "c", "top-level recursion, count", u, evCall{sql: walkAllCount, args: []any{u.id}})
		evMeasure(ctx, t, m, explainer, "c", "top-level recursion, page", u, evCall{sql: walkAllPage, args: []any{u.id, 0, evPageSize}})
	}

	m.note("")
	m.note("==================== d. the anchored plain statement ====================")
	m.note("The shape a parameterized projection base would give: the unit bound inside the recursion's anchor, so the walk starts from the unit's bookmarks only. The count twin over the base, the page composed at the base's level, and the page wrapped as a derived table.")
	m.note("count:        %s", anchoredCount)
	m.note("page:         %s", anchoredPage)
	m.note("page wrapped: %s", anchoredWrappedPage)
	for _, label := range []string{"u10", "u1000"} {
		u := byLabel[label]
		evMeasure(ctx, t, m, explainer, "d", "anchored, count", u, evCall{sql: anchoredCount, args: []any{u.id}})
		evMeasure(ctx, t, m, explainer, "d", "anchored, page", u, evCall{sql: anchoredPage, args: []any{u.id, 0, evPageSize}})
	}
	evMeasure(ctx, t, m, explainer, "d", "anchored, page wrapped", byLabel["u1000"], evCall{sql: anchoredWrappedPage, args: []any{byLabel["u1000"].id, 0, evPageSize}})

	m.note("")
	m.note("==================== e. the shipped page and the anchored page against the depth ====================")
	m.note("Two units with %d bookmarks each: one of files at the shallowest depth that holds that many (mean depth %.2f), one of files at depth %d.", evPerUnit, byLabel["shallow"].depth, evMaxDepth)
	for _, label := range []string{"shallow", "deep"} {
		u := byLabel[label]
		_, page := shipped(u, page1)
		evMeasure(ctx, t, m, explainer, "e", "shipped page, by path", u, page)
		evMeasure(ctx, t, m, explainer, "e", "anchored, page", u, evCall{sql: anchoredPage, args: []any{u.id, 0, evPageSize}})
	}

	m.note("")
	m.note("==================== f. the same queries with JIT compilation off ====================")
	m.note("The planner's cost estimate of the correlated recursion crosses jit_above_cost at about a hundred and forty rows, and the top-level recursion's estimate crosses it for any unit, so those queries are JIT-compiled on a server with the default settings. The same queries on a session with jit = off isolate the compile time from the walk: the shipped page for the counted and the depth units, and the top-level recursion's count and page.")
	for _, label := range []string{"u100", "u1000", "shallow", "deep"} {
		u := byLabel[label]
		_, page := shipped(u, page1)
		evMeasure(ctx, t, m, noJIT, "f", "shipped page, by path, jit off", u, page)
	}
	for _, label := range []string{"u10", "u1000"} {
		u := byLabel[label]
		evMeasure(ctx, t, m, noJIT, "f", "top-level recursion, count, jit off", u, evCall{sql: walkAllCount, args: []any{u.id}})
		evMeasure(ctx, t, m, noJIT, "f", "top-level recursion, page, jit off", u, evCall{sql: walkAllPage, args: []any{u.id, 0, evPageSize}})
	}

	// Cross-checks: every form lists the same first page for the same unit,
	// and the counts agree with the fixture.
	for _, label := range []string{"u10", "u1000", "shallow", "deep"} {
		u := byLabel[label]
		_, page := shipped(u, page1)
		want := evColumn(ctx, t, db, 3, page.sql, page.args...)
		for _, q := range []struct {
			label string
			c     evCall
		}{
			{"top-level recursion", evCall{sql: walkAllPage, args: []any{u.id, 0, evPageSize}}},
			{"anchored", evCall{sql: anchoredPage, args: []any{u.id, 0, evPageSize}}},
			{"anchored wrapped", evCall{sql: anchoredWrappedPage, args: []any{u.id, 0, evPageSize}}},
		} {
			if got := evColumn(ctx, t, db, 3, q.c.sql, q.c.args...); !slices.Equal(got, want) {
				t.Errorf("%s: the %s form lists a different first page:\n%v\n%v", u.label, q.label, got, want)
			}
		}
		count, _ := shipped(u, page1)
		for _, q := range []evCall{count, {sql: walkAllCount, args: []any{u.id}}, {sql: anchoredCount, args: []any{u.id}}} {
			if n := evScalar(ctx, t, db, q.sql, q.args...); n != strconv.Itoa(u.n) {
				t.Errorf("%s: a count returns %s, the fixture has %d:\n%s", u.label, n, u.n, q.sql)
			}
		}
		for _, p := range want {
			if !strings.HasPrefix(p, "/t") || strings.Count(p, "/") < 2 {
				t.Errorf("%s: path %q is not a full path under a tree root", u.label, p)
			}
		}
	}
	m.note("")
	m.note("cross-check: for each measured unit every form lists the same first page of paths, every count equals the unit's bookmark count, and every path starts at a tree root.")

	var out strings.Builder
	fmt.Fprintf(&out, "# blobfs stage 11: the cost of the bookmark read model\n")
	fmt.Fprintf(&out, "# date: %s\n", time.Now().UTC().Format("2006-01-02"))
	fmt.Fprintf(&out, "# engine: %s\n", version)
	fmt.Fprintf(&out, "# machine: %s/%s, %d cpus; go %s. Timings are machine-dependent (a laptop, everything in shared buffers); plan shapes and buffer counts are not.\n", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.Version())
	fmt.Fprintf(&out, "# fixture: %d trees under the root up to depth %d, about one directory per ten files, %d file rows spread uniformly over every directory; %d background units with %d bookmarks each of random files, plus the measured units: 10, 100, and 1,000 bookmarks of random files, and 1,000 bookmarks of files at the shallowest depth that holds a thousand and at depth six.\n", evTrees, evMaxDepth, evFiles, evUnits, evPerUnit)
	fmt.Fprintf(&out, "# method: each query is EXPLAIN (ANALYZE, BUFFERS) once to warm the cache, then %d times; the run with the median execution time is reported with its plan.\n", evRuns)
	fmt.Fprintf(&out, "#   EXPLAIN runs over pgx's simple protocol, so each run is planned with its literal values, as a custom plan is; a prepared statement's generic plan could differ.\n")
	fmt.Fprintf(&out, "#   Buffers are the top plan node's shared hit+read, in 8 KB pages. The tables were VACUUM ANALYZEd after seeding. JIT is the server's default (%s) except in section f.\n", jit)
	fmt.Fprintf(&out, "#\n# summary (median run; times in ms; buffers as hit+read pages; n is the unit's bookmark count):\n")
	fmt.Fprintf(&out, "# %-3s %-38s %-8s %5s %9s %9s %8s  %s\n", "sec", "query", "unit", "n", "plan ms", "exec ms", "buffers", "plan shape")
	for _, r := range m.results {
		fmt.Fprintf(&out, "# %-3s %-38s %-8s %5d %9.3f %9.3f %8d  %s\n", r.section, r.label, r.unit, byLabel[r.unit].n, r.planning, r.execution, r.buffers, r.shape)
	}
	out.WriteString(m.body.String())
	fmt.Print(out.String())
}

// The columns every form selects, in the order the read model scans them.
const evColumns = "b.unit_id, b.file_id, b.active, up.path || '/' || f.name AS path, f.name, f.status, f.size, f.content_type, b.created_at, b.updated_at"

// The projection over a top-level recursion: the shape the plan described.
// The recursion is anchored on every bookmarked file's directory, because
// the base binds no parameter, and the unit is a directive outside the
// derived table. The count twin and the page in the shapes the query
// library's collection and count patterns produce.
const walkAllBase = `WITH RECURSIVE up (file_id, directory_id, path) AS (
    SELECT f.id, f.directory_id, CAST('' AS text)
    FROM blobfs_file f
    WHERE f.id IN (SELECT b.file_id FROM bookmark b)
  UNION ALL
    SELECT up.file_id, d.parent_id, COALESCE('/' || d.name, '') || up.path
    FROM up JOIN blobfs_directory d ON d.id = up.directory_id
)
SELECT ` + evColumns + `
FROM bookmark b
JOIN blobfs_file f ON f.id = b.file_id
JOIN up ON up.file_id = b.file_id AND up.directory_id IS NULL`

const (
	walkAllCount = "SELECT COUNT(*) FROM (" + walkAllBase + ") q WHERE q.unit_id = CAST($1 AS uuid)"
	walkAllPage  = "SELECT * FROM (" + walkAllBase + ") q WHERE q.unit_id = CAST($1 AS uuid) ORDER BY q.path, q.file_id OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY"
)

// The anchored plain statement: the unit bound inside the recursion's
// anchor and repeated on the outer join, the shape a parameterized
// projection base would give. The count twin over the base, the page
// composed at the base's level, and the page wrapped as a derived table.
const anchoredBase = `WITH RECURSIVE up (file_id, directory_id, path) AS (
    SELECT b.file_id, f.directory_id, CAST('' AS text)
    FROM bookmark b
    JOIN blobfs_file f ON f.id = b.file_id
    WHERE b.unit_id = CAST($1 AS uuid)
  UNION ALL
    SELECT up.file_id, d.parent_id, COALESCE('/' || d.name, '') || up.path
    FROM up JOIN blobfs_directory d ON d.id = up.directory_id
)
SELECT ` + evColumns + `
FROM bookmark b
JOIN blobfs_file f ON f.id = b.file_id
JOIN up ON up.file_id = b.file_id AND up.directory_id IS NULL
WHERE b.unit_id = CAST($1 AS uuid)`

const (
	anchoredCount       = "SELECT COUNT(*) FROM (" + anchoredBase + ") q"
	anchoredPage        = anchoredBase + " ORDER BY path, b.file_id OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY"
	anchoredWrappedPage = "SELECT * FROM (" + anchoredBase + ") q ORDER BY q.path, q.file_id OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY"
)

// evMigrate applies blobfs's set and then the consumer's, as schema up does.
func evMigrate(ctx context.Context, t *testing.T, db *sqlate.DB) {
	t.Helper()
	blobfsSet, err := blobfsmigrations.Migrations(db.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	consumerSet, err := appmigrations.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrator.New(db, []migrator.Set{
		{Name: blobfsmigrations.Source, Table: blobfsmigrations.Table, Migrations: blobfsSet},
		{Name: "consumer", Migrations: consumerSet},
	}, migrator.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Up(ctx); err != nil {
		t.Fatal(err)
	}
}

// evSeed builds the fixture: three depth-one directories as the tree
// roots, then the directories and the files in bulk through unnest,
// deterministic under a fixed seed, with directory i attached to a
// directory chosen uniformly among those above the depth limit that exist
// so far and every file to a directory chosen uniformly among all. Then
// the bookmarks: the background units over random files, the counted
// units over random files, and the depth units over files whose directory
// sits at the shallowest depth that holds enough of them and at depth six. It returns the directories and the
// measured units.
func evSeed(ctx context.Context, t *testing.T, db *sqlate.DB) ([]evDir, []evUnit) {
	t.Helper()
	rng := rand.New(rand.NewPCG(20260920, evFiles))
	var dirs []evDir
	for i := range evTrees {
		name := "t" + strconv.Itoa(i)
		dirs = append(dirs, evDir{id: blobfs.NewID(), parent: blobfs.RootID, name: name, depth: 1})
	}
	for i := range evFiles / 10 {
		var parent int
		for {
			parent = rng.IntN(len(dirs))
			if dirs[parent].depth < evMaxDepth {
				break
			}
		}
		p := dirs[parent]
		dirs = append(dirs, evDir{id: blobfs.NewID(), parent: p.id, name: "d" + strconv.Itoa(i), depth: p.depth + 1})
	}
	ids, parents, names := make([]string, 0, len(dirs)), make([]string, 0, len(dirs)), make([]string, 0, len(dirs))
	for _, d := range dirs {
		ids, parents, names = append(ids, d.id), append(parents, d.parent), append(names, d.name)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO blobfs_directory (id, parent_id, name) SELECT CAST(u.id AS uuid), CAST(u.parent_id AS uuid), u.name FROM unnest($1::text[], $2::text[], $3::text[]) AS u(id, parent_id, name)", ids, parents, names); err != nil {
		t.Fatalf("insert directories: %v", err)
	}

	fileIDs := make([]string, evFiles)
	fileDir := make([]int, evFiles)
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
	for i := range evFiles {
		dir := rng.IntN(len(dirs))
		id := blobfs.NewID()
		name := "f" + strconv.Itoa(i) + ".txt"
		fileIDs[i], fileDir[i] = id, dir
		fids, fdirs, fnames, fkeys, fsizes = append(fids, id), append(fdirs, dirs[dir].id), append(fnames, name), append(fkeys, id+"/"+name), append(fsizes, int64(i%4096))
		if len(fids) == batch {
			flush()
		}
	}
	flush()

	// The bookmarks: a unit's files are a prefix of a permutation, so no
	// file is bookmarked twice by one unit.
	var bunits, bfiles []string
	bookmark := func(unit string, files []int) {
		for _, f := range files {
			bunits, bfiles = append(bunits, unit), append(bfiles, fileIDs[f])
		}
	}
	for range evUnits {
		bookmark(blobfs.NewID(), rng.Perm(evFiles)[:evPerUnit])
	}
	atDepth := func(depth int) []int {
		var out []int
		for i, d := range fileDir {
			if dirs[d].depth == depth {
				out = append(out, i)
			}
		}
		rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
		return out
	}
	var units []evUnit
	unit := func(label string, files []int) {
		u := evUnit{label: label, id: blobfs.NewID(), n: len(files)}
		for _, f := range files {
			u.depth += float64(dirs[fileDir[f]].depth)
		}
		u.depth /= float64(len(files))
		bookmark(u.id, files)
		units = append(units, u)
	}
	unit("u10", rng.Perm(evFiles)[:10])
	unit("u100", rng.Perm(evFiles)[:100])
	unit("u1000", rng.Perm(evFiles)[:evPerUnit])
	// Uniform attachment fills the deep levels fast, so the shallow unit
	// takes the least depth that holds enough files; the fixture line
	// reports the mean depth reached.
	var shallow []int
	for depth := 1; depth < evMaxDepth && len(shallow) < evPerUnit; depth++ {
		shallow = atDepth(depth)
	}
	deep := atDepth(evMaxDepth)
	if len(shallow) < evPerUnit || len(deep) < evPerUnit {
		t.Fatalf("the fixture holds %d files at the shallowest usable depth and %d at depth %d; the depth units need %d each", len(shallow), len(deep), evMaxDepth, evPerUnit)
	}
	unit("shallow", shallow[:evPerUnit])
	unit("deep", deep[:evPerUnit])
	if _, err := db.ExecContext(ctx, "INSERT INTO bookmark (unit_id, file_id, active) SELECT CAST(u.unit_id AS uuid), CAST(u.file_id AS uuid), false FROM unnest($1::text[], $2::text[]) AS u(unit_id, file_id)", bunits, bfiles); err != nil {
		t.Fatalf("insert bookmarks: %v", err)
	}
	return dirs, units
}

func evMaxDepthOf(dirs []evDir) int {
	m := 0
	for _, d := range dirs {
		m = max(m, d.depth)
	}
	return m
}

// evMeasure explains one query, records the median run, and writes the
// section line and the plan to the transcript.
func evMeasure(ctx context.Context, t *testing.T, m *evMeasurement, explainer *sqlate.DB, section, label string, u evUnit, c evCall) {
	t.Helper()
	r, plan := evExplain(ctx, t, explainer, c)
	r.section, r.label, r.unit = section, label, u.label
	r.shape = strings.Join(evShapeOf(plan), ", ")
	m.results = append(m.results, r)
	m.note("")
	m.note("---- %s: %s, unit %s (%d bookmarks): median of %d runs: planning %.3f ms, execution %.3f ms, buffers %d ----", section, label, u.label, u.n, evRuns, r.planning, r.execution, r.buffers)
	m.note("%s", plan)
}

var (
	evPlanningRe  = regexp.MustCompile(`Planning Time: ([0-9.]+) ms`)
	evExecutionRe = regexp.MustCompile(`Execution Time: ([0-9.]+) ms`)
	evBuffersRe   = regexp.MustCompile(`Buffers: shared( hit=(\d+))?( read=(\d+))?`)
)

// evExplain runs EXPLAIN (ANALYZE, BUFFERS) on c once to warm the cache
// and then evRuns times, and returns the median run by execution time
// with its plan text.
func evExplain(ctx context.Context, t *testing.T, db *sqlate.DB, c evCall) (evResult, string) {
	t.Helper()
	type run struct {
		evResult
		plan string
	}
	var all []run
	for i := 0; i <= evRuns; i++ {
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
		r.planning = evNumber(evPlanningRe.FindStringSubmatch(plan), 1)
		r.execution = evNumber(evExecutionRe.FindStringSubmatch(plan), 1)
		if b := evBuffersRe.FindStringSubmatch(plan); b != nil {
			r.buffers = int(evNumber(b, 2) + evNumber(b, 4))
		}
		all = append(all, r)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].execution < all[j].execution })
	median := all[len(all)/2]
	return median.evResult, median.plan
}

func evNumber(m []string, i int) float64 {
	if len(m) <= i || m[i] == "" {
		return 0
	}
	f, _ := strconv.ParseFloat(m[i], 64)
	return f
}

// evShapeOf names the plan nodes that tell the forms apart, in a fixed
// order.
func evShapeOf(plan string) []string {
	var out []string
	for _, node := range []string{
		"Recursive Union", "WorkTable Scan", "CTE Scan", "SubPlan", "Subquery Scan",
		"Seq Scan on bookmark", "Index Scan using pk_bookmark", "Index Only Scan using pk_bookmark", "Bitmap Heap Scan on bookmark",
		"Seq Scan on blobfs_file", "Index Scan using blobfs_pk_file", "Index Only Scan using blobfs_pk_file",
		"Seq Scan on blobfs_directory", "Index Scan using blobfs_pk_directory",
		"Hash Join", "Nested Loop", "Merge Join", "HashAggregate", "Sort", "Aggregate", "Limit", "JIT",
	} {
		if strings.Contains(plan, node) {
			out = append(out, node)
		}
	}
	return out
}

// evOpenSimple opens a second pool on the fixture database over pgx's
// simple protocol, so EXPLAIN runs are planned with their literal values
// on every run, with extra runtime parameters appended to the DSN.
func evOpenSimple(t *testing.T, dsn, params string) *sqlate.DB {
	t.Helper()
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	dsn += sep + "default_query_exec_mode=simple_protocol"
	if params != "" {
		dsn += "&" + params
	}
	pool, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	return sqlate.Wrap(pool, postgres.Dialect{})
}

// evScalar runs a one-row, one-column query and returns the value as text.
func evScalar(ctx context.Context, t *testing.T, db *sqlate.DB, q string, args ...any) string {
	t.Helper()
	col := evColumn(ctx, t, db, 0, q, args...)
	if len(col) != 1 {
		t.Fatalf("%s: %d rows, want 1", q, len(col))
	}
	return col[0]
}

// evColumn runs a query and returns column i of every row as text.
func evColumn(ctx context.Context, t *testing.T, db *sqlate.DB, i int, q string, args ...any) []string {
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
