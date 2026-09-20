//go:build integration

package volume

// This file is proof V1, the read-model cost by form. It is an internal
// test so it can run the store's own file_view projection, the exact
// statement ls runs, through a recording session and measure the SQL it
// composes. It runs only under BLOBFS_EVIDENCE=1; mise run evidence sets
// the variable and writes the transcript to evidence/v1-read-model.txt.
// Nothing here changes the shipped DDL: the one ALTER TABLE runs against
// the throwaway database of the fixture.

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
	"testing/fstest"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/postgres"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
	blobfsmigrations "github.com/standards-lab/org/experiments/blobfs/lib/blobfs/migrations"
	"github.com/standards-lab/org/experiments/blobfs/lib/migrator"
	appmigrations "github.com/standards-lab/org/experiments/blobfs/migrations"
)

// The fixture sizes in file rows, and the page the forms list.
var (
	sizes    = []int{1_000, 10_000, 100_000}
	pageSize = 20
	runs     = 5
	maxDepth = 6
	volumes  = 3
)

// recorder is a sqlate.Session over the pool that keeps every query's text
// and arguments, so the measurement explains exactly what the projection
// ran. The embedded *sqlate.DB keeps MapError reachable.
type recorder struct {
	*sqlate.DB
	calls []call
}

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
	volume, depth    int
	path             string
	files            int
}

// fixture is one size's seeded database and the directories the forms list.
type fixture struct {
	size    int
	dirs    []dirRow // the roots first, then the generated directories
	volumes []string // volume ids, one per root
	units   []string
	listed  dirRow // a deep directory with a modest file count
	biggest dirRow // the directory a tenth of the files go to
}

// TestReadModelCost is proof V1. For each fixture size it seeds three
// volumes, a directory tree up to depth six with about one directory per
// ten files, and the files, then runs EXPLAIN (ANALYZE, BUFFERS) on the
// count and page queries of four forms and prints the plans and the
// medians. See the transcript's header for the forms.
func TestReadModelCost(t *testing.T) {
	if os.Getenv("BLOBFS_EVIDENCE") == "" {
		t.Skip("set BLOBFS_EVIDENCE=1 to run the read-model cost measurement")
	}
	ctx := context.Background()
	var body strings.Builder
	var results []result
	var shapes = map[string]map[string]bool{}
	note := func(format string, args ...any) { fmt.Fprintf(&body, format+"\n", args...) }
	var version string

	for i, size := range sizes {
		db, dsn := livetest.OpenDSN(t)
		applyBoth(ctx, t, db)
		explainer := openSimple(t, dsn)
		if version == "" {
			version = scalar(ctx, t, db, "SELECT version()")
		}
		s, err := New(db)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Verify(ctx); err != nil {
			t.Fatal(err)
		}
		fx := seed(ctx, t, db, s, size)
		note("")
		note("==================== fixture: %d files ====================", size)
		note("directories: %d generated under %d roots; files: %d; depth reached: %d", len(fx.dirs)-volumes, volumes, size, maxDepthOf(fx.dirs))
		note("listed directory: %s (depth %d, %d files) in volume %d", fx.listed.path, fx.listed.depth, fx.listed.files, fx.listed.volume)
		note("biggest directory: %s (depth %d, %d files) in volume %d", fx.biggest.path, fx.biggest.depth, fx.biggest.files, fx.biggest.volume)

		forms := []form{form1(ctx, t, s, fx), form1b(ctx, t, s, fx), form2(fx)}
		if i == 0 {
			for _, f := range forms {
				note("")
				note("---- %s: SQL as run ----", f.name)
				note("count: %s", f.count.sql)
				note("page:  %s", f.page.sql)
			}
		}
		for _, f := range forms {
			results = append(results, measure(ctx, t, note, explainer, shapes, size, f, fx)...)
		}
		// Form 3 alters the throwaway schema, so it runs after the others.
		f3 := form3(ctx, t, db, fx)
		if i == 0 {
			note("")
			note("---- %s: SQL as run ----", f3.name)
			note("count: %s", f3.count.sql)
			note("page:  %s", f3.page.sql)
		}
		results = append(results, measure(ctx, t, note, explainer, shapes, size, f3, fx)...)
		crossCheck(ctx, t, note, db, s, fx, forms, f3)
	}

	var out strings.Builder
	fmt.Fprintf(&out, "# blobfs proof V1: read-model cost by form\n")
	fmt.Fprintf(&out, "# date: %s\n", time.Now().UTC().Format("2006-01-02"))
	fmt.Fprintf(&out, "# engine: %s\n", version)
	fmt.Fprintf(&out, "# machine: %s/%s, %d cpus; go %s. Timings are machine-dependent; plan shapes and buffer counts are not.\n", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.Version())
	fmt.Fprintf(&out, "# fixtures: %d volumes, one tree per volume up to depth %d, about one directory per ten files, file rows at %v.\n", volumes, maxDepth, sizes)
	fmt.Fprintf(&out, "#   A tenth of the files go to one depth-1 directory (the biggest); the rest spread uniformly over every directory, roots included.\n")
	fmt.Fprintf(&out, "#   The listed directory is a deep directory with a modest file count, named per fixture below.\n")
	fmt.Fprintf(&out, "# method: each query is EXPLAIN (ANALYZE, BUFFERS) once to warm the cache, then %d times; the run with the median execution time is reported.\n", runs)
	fmt.Fprintf(&out, "#   EXPLAIN runs over pgx's simple protocol, so each run is planned with its literal values, as a custom plan is.\n")
	fmt.Fprintf(&out, "#   Buffers are the top plan node's shared hit+read, in 8 KB pages.\n")
	fmt.Fprintf(&out, "# forms:\n")
	fmt.Fprintf(&out, "#   1  the shipped shape: file_view (blobfs.tree over the whole forest, then the files joined to the tree and to volume_owner), directory_id filter applied outside as a directive\n")
	fmt.Fprintf(&out, "#   1b the same without the volume_owner join, to isolate the owner hop\n")
	fmt.Fprintf(&out, "#   2  a plain parameterized statement for one directory: the path from a recursive walk up the directory's ancestors, owner joined through the walk's root\n")
	fmt.Fprintf(&out, "#   3  volume_id denormalized onto blobfs_file in the throwaway database only, with an index on (volume_id, directory_id, name); flat listing, no recursion, no path\n")
	fmt.Fprintf(&out, "# plan shapes (machine-independent), by fixture size, form, and query, the listed and the biggest directory together:\n")
	for _, key := range sortedKeys(shapes) {
		fmt.Fprintf(&out, "#   %s: %s\n", key, strings.Join(sortedKeys(shapes[key]), ", "))
	}
	fmt.Fprintf(&out, "#\n# summary (median run; times in ms; buffers as hit+read pages):\n")
	fmt.Fprintf(&out, "# %-7s %-5s %-8s %-6s %10s %10s %10s\n", "files", "form", "dir", "query", "plan ms", "exec ms", "buffers")
	for _, r := range results {
		fmt.Fprintf(&out, "# %-7d %-5s %-8s %-6s %10.3f %10.3f %10d\n", r.size, r.form, r.dir, r.query, r.planning, r.execution, r.buffers)
	}
	out.WriteString(body.String())
	fmt.Print(out.String())
}

// form is one shape of the listing: its name and its count and page
// queries with their arguments, as the engine receives them.
type form struct {
	name        string
	count, page call
}

// form1 runs the store's own file_view projection through the recorder,
// as ls does, and captures the two queries it composed.
func form1(ctx context.Context, t *testing.T, s *Store, fx fixture) form {
	t.Helper()
	rec := &recorder{DB: s.db}
	_, _, err := data.ListIn(ctx, rec, s.fileView, "directory_id", fx.listed.id, listing())
	if err != nil {
		t.Fatalf("form 1: %v", err)
	}
	if len(rec.calls) != 2 {
		t.Fatalf("form 1 ran %d queries, want 2", len(rec.calls))
	}
	return form{name: "1", count: rec.calls[0], page: rec.calls[1]}
}

// form1b compiles file_view without the owner join against the store's
// catalog and captures its two queries the same way.
func form1b(ctx context.Context, t *testing.T, s *Store, fx fixture) form {
	t.Helper()
	base := "--| tier: standard\n--| key: id\n--| field: id uuid\n--| field: volume_id uuid\n--| field: directory_id uuid\n--| field: name text\n--| field: path text\n" +
		"{{> blobfs.tree}}\nSELECT {{> blobfs.file_columns}}, {{> blobfs.file_path}} AS path, t.volume_id\nFROM blobfs_file f\nJOIN tree t ON t.id = f.directory_id"
	stmts, err := s.catalog.Compile(fstest.MapFS{"statements/file_view_no_owner.sql": {Data: []byte(base)}}, "statements", s.db.Dialect())
	if err != nil {
		t.Fatalf("form 1b: %v", err)
	}
	type row struct {
		ID          string        `json:"id"`
		DirectoryID string        `json:"directory_id"`
		Name        string        `json:"name"`
		Status      blobfs.Status `json:"status"`
		Key         string        `json:"key"`
		Size        *int64        `json:"size"`
		ContentType string        `json:"content_type"`
		ETag        *string       `json:"etag"`
		Version     int64         `json:"version"`
		CreatedAt   time.Time     `json:"created_at"`
		UpdatedAt   time.Time     `json:"updated_at"`
		Path        string        `json:"path"`
		VolumeID    string        `json:"volume_id"`
	}
	p := stmts.Statement("file_view_no_owner").Project(query.Scanner[row]())
	rec := &recorder{DB: s.db}
	if _, _, err := data.ListIn(ctx, rec, p, "directory_id", fx.listed.id, listing()); err != nil {
		t.Fatalf("form 1b: %v", err)
	}
	return form{name: "1b", count: rec.calls[0], page: rec.calls[1]}
}

// form2 is the hand-written plain statement: the path from a walk up the
// listed directory's ancestors, whose last row is the root and carries the
// volume and the whole path, the owner joined through it, the same sort
// and paging written by hand. Standard SQL throughout.
func form2(fx fixture) form {
	walk := "WITH RECURSIVE up (id, parent_id, volume_id, path) AS (" +
		"SELECT d.id, d.parent_id, d.volume_id, CAST(COALESCE('/' || d.name, '') AS text) FROM blobfs_directory d WHERE d.id = $1 " +
		"UNION ALL SELECT p.id, p.parent_id, p.volume_id, COALESCE('/' || p.name, '') || up.path FROM blobfs_directory p JOIN up ON p.id = up.parent_id) "
	from := "FROM blobfs_file f JOIN up ON up.parent_id IS NULL JOIN volume_owner o ON o.volume_id = up.volume_id WHERE f.directory_id = $1"
	return form{
		name:  "2",
		count: call{sql: walk + "SELECT COUNT(*) " + from, args: []any{fx.listed.id}},
		page: call{sql: walk + "SELECT f.id, f.directory_id, f.name, f.status, f.key, f.size, f.content_type, f.etag, f.version, f.created_at, f.updated_at, " +
			"up.path || '/' || f.name AS path, up.volume_id, o.unit_id " + from + " ORDER BY f.name, f.id OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY",
			args: []any{fx.listed.id, 0, pageSize}},
	}
}

// form3 denormalizes volume_id onto blobfs_file in the fixture database,
// backfills it through the tree, indexes it, and returns the flat listing:
// the volume and the directory as equality predicates, the owner joined by
// volume_id, no recursion. The path is not computed: this form has no
// way to, short of a stored path.
func form3(ctx context.Context, t *testing.T, db *sqlate.DB, fx fixture) form {
	t.Helper()
	for _, stmt := range []string{
		"ALTER TABLE blobfs_file ADD COLUMN volume_id uuid",
		"UPDATE blobfs_file f SET volume_id = t.volume_id FROM (" +
			"WITH RECURSIVE tree (id, volume_id) AS (SELECT d.id, d.volume_id FROM blobfs_directory d WHERE d.volume_id IS NOT NULL " +
			"UNION ALL SELECT d.id, t.volume_id FROM blobfs_directory d JOIN tree t ON t.id = d.parent_id) SELECT id, volume_id FROM tree) t " +
			"WHERE t.id = f.directory_id",
		"ALTER TABLE blobfs_file ALTER COLUMN volume_id SET NOT NULL",
		"CREATE INDEX ix_evidence_file_volume_directory_name ON blobfs_file (volume_id, directory_id, name)",
		"ANALYZE blobfs_file",
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("form 3: %s: %v", stmt, err)
		}
	}
	from := "FROM blobfs_file f JOIN volume_owner o ON o.volume_id = f.volume_id WHERE f.volume_id = $1 AND f.directory_id = $2"
	vol := fx.volumes[fx.listed.volume]
	return form{
		name:  "3",
		count: call{sql: "SELECT COUNT(*) " + from, args: []any{vol, fx.listed.id}},
		page: call{sql: "SELECT f.id, f.directory_id, f.name, f.status, f.key, f.size, f.content_type, f.etag, f.version, f.created_at, f.updated_at, f.volume_id, o.unit_id " +
			from + " ORDER BY f.name, f.id OFFSET $3 ROWS FETCH NEXT $4 ROWS ONLY", args: []any{vol, fx.listed.id, 0, pageSize}},
	}
}

// listing is the directives every form lists under: page 1 of pageSize,
// sorted by name (the key is appended as the tie-breaker).
func listing() query.Directives {
	return query.Directives{Page: query.Page{Number: 1, Size: pageSize}, Sort: []query.Sort{{Field: "name"}}}
}

// result is one measured query: the median run's times and buffers.
type result struct {
	size                int
	form, dir, query    string
	planning, execution float64
	buffers             int
}

// measure explains a form's two queries for the listed directory and the
// biggest one, prints the median run's plan for the listed directory and
// the summary line for both, and records the plan shapes.
func measure(ctx context.Context, t *testing.T, note func(string, ...any), explainer *sqlate.DB, shapes map[string]map[string]bool, size int, f form, fx fixture) []result {
	t.Helper()
	var out []result
	for _, target := range []struct {
		name string
		dir  dirRow
	}{{"listed", fx.listed}, {"biggest", fx.biggest}} {
		for _, q := range []struct {
			name string
			c    call
		}{{"count", f.count}, {"page", f.page}} {
			c := q.c
			c.args = retarget(c.args, map[string]string{
				fx.listed.id:                 target.dir.id,
				fx.volumes[fx.listed.volume]: fx.volumes[target.dir.volume],
			})
			r, plan := explain(ctx, t, explainer, c)
			r.size, r.form, r.dir, r.query = size, f.name, target.name, q.name
			out = append(out, r)
			key := fmt.Sprintf("%-7d form %-2s %s", size, f.name, q.name)
			if shapes[key] == nil {
				shapes[key] = map[string]bool{}
			}
			for _, s := range shapeOf(plan) {
				shapes[key][s] = true
			}
			note("")
			note("---- form %s, %s directory (%d files), %s query: median of %d runs: planning %.3f ms, execution %.3f ms, buffers %d ----",
				f.name, target.name, target.dir.files, q.name, runs, r.planning, r.execution, r.buffers)
			if target.name == "listed" {
				note("%s", plan)
			} else {
				note("(plan shape: %s)", strings.Join(shapeOf(plan), ", "))
			}
		}
	}
	return out
}

// retarget replaces ids in a query's arguments, the listed directory's
// with another directory's and its volume's with that directory's volume,
// so the same query text lists the biggest directory.
func retarget(args []any, ids map[string]string) []any {
	out := slices.Clone(args)
	for i, a := range out {
		if s, ok := a.(string); ok {
			if to, ok := ids[s]; ok {
				out[i] = to
			}
		}
	}
	return out
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
		if m := buffersRe.FindStringSubmatch(plan); m != nil {
			r.buffers = int(number(m, 2) + number(m, 4))
		}
		all = append(all, r)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].execution < all[j].execution })
	m := all[len(all)/2]
	return m.result, m.plan
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
		"Recursive Union", "WorkTable Scan", "CTE Scan",
		"Seq Scan on blobfs_directory", "Index Scan using blobfs_pk_directory", "Index Scan using blobfs_uq_directory",
		"Seq Scan on blobfs_file", "Index Scan using blobfs_uq_file_directory_name", "Index Only Scan using blobfs_uq_file_directory_name",
		"Index Scan using ix_evidence", "Index Only Scan using ix_evidence", "Bitmap Heap Scan on blobfs_file",
		"Seq Scan on volume_owner", "Index Scan using pk_volume_owner", "Index Only Scan using pk_volume_owner",
		"Hash Join", "Nested Loop", "Merge Join", "Sort", "Incremental Sort", "Aggregate",
	} {
		if strings.Contains(plan, node) {
			out = append(out, node)
		}
	}
	return out
}

// crossCheck proves the four forms agree on the listed directory: the same
// total and the same first page, in order, so the timings compare like
// with like. Form 3 is compared on ids alone, since it computes no path.
func crossCheck(ctx context.Context, t *testing.T, note func(string, ...any), db *sqlate.DB, s *Store, fx fixture, forms []form, f3 form) {
	t.Helper()
	files, total, err := data.ListIn(ctx, db, s.fileView, "directory_id", fx.listed.id, listing())
	if err != nil {
		t.Fatal(err)
	}
	var ids, paths []string
	for _, f := range files {
		ids = append(ids, f.ID)
		paths = append(paths, f.Path)
	}
	if total != fx.listed.files {
		t.Errorf("form 1 total %d, fixture says %d", total, fx.listed.files)
	}
	for _, p := range paths {
		if !strings.HasPrefix(p, fx.listed.path+"/") {
			t.Errorf("form 1 path %q is not under %s", p, fx.listed.path)
		}
	}
	for _, f := range append(slices.Clone(forms[1:]), f3) {
		if n := scalar(ctx, t, db, f.count.sql, f.count.args...); n != strconv.Itoa(total) {
			t.Errorf("form %s counts %s, form 1 %d", f.name, n, total)
		}
		got := column(ctx, t, db, 0, f.page.sql, f.page.args...)
		if !slices.Equal(got, ids) {
			t.Errorf("form %s first page differs from form 1's:\n%v\n%v", f.name, got, ids)
		}
		if f.name == "2" {
			if gotPaths := column(ctx, t, db, 11, f.page.sql, f.page.args...); !slices.Equal(gotPaths, paths) {
				t.Errorf("form 2 paths differ from form 1's:\n%v\n%v", gotPaths, paths)
			}
		}
	}
	ids2 := map[string]string{fx.listed.id: fx.biggest.id, fx.volumes[fx.listed.volume]: fx.volumes[fx.biggest.volume]}
	_, bigTotal, err := data.ListIn(ctx, db, s.fileView, "directory_id", fx.biggest.id, listing())
	if err != nil {
		t.Fatal(err)
	}
	if bigTotal != fx.biggest.files {
		t.Errorf("form 1 total for the biggest directory %d, fixture says %d", bigTotal, fx.biggest.files)
	}
	for _, f := range append(slices.Clone(forms[1:]), f3) {
		if n := scalar(ctx, t, db, f.count.sql, retarget(f.count.args, ids2)...); n != strconv.Itoa(bigTotal) {
			t.Errorf("form %s counts %s for the biggest directory, form 1 %d", f.name, n, bigTotal)
		}
	}
	note("")
	note("cross-check: every form returns total %d and the same first page of %d ids for %s, form 2's paths equal form 1's, and every form counts %d for %s", total, len(ids), fx.listed.path, bigTotal, fx.biggest.path)
}

// applyBoth applies blobfs's set and then the consumer's, as schema up does.
func applyBoth(ctx context.Context, t *testing.T, db *sqlate.DB) {
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

// seed builds one fixture: the volumes through the store, then the
// directories and the files in bulk through unnest, deterministic under a
// fixed seed. Directory i attaches to a directory chosen uniformly among
// the roots and the directories above depth maxDepth that exist so far.
// Every tenth file goes to the first generated directory; the rest to a
// directory chosen uniformly among all, roots included.
func seed(ctx context.Context, t *testing.T, db *sqlate.DB, s *Store, size int) fixture {
	t.Helper()
	rng := rand.New(rand.NewPCG(20260920, uint64(size)))
	fx := fixture{size: size}
	for v := range volumes {
		unit := blobfs.NewID()
		entry, err := s.CreateVolume(ctx, fmt.Sprintf("vol%d", v), unit)
		if err != nil {
			t.Fatal(err)
		}
		root, err := s.blobfs.RootDirectory(ctx, db, entry.ID)
		if err != nil {
			t.Fatal(err)
		}
		fx.volumes = append(fx.volumes, entry.ID)
		fx.units = append(fx.units, unit)
		fx.dirs = append(fx.dirs, dirRow{id: root.ID, volume: v, depth: 0, path: ""})
	}
	n := max(size/10, 30)
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
		fx.dirs = append(fx.dirs, dirRow{id: blobfs.NewID(), parent: p.id, name: name, volume: p.volume, depth: p.depth + 1, path: p.path + "/" + name})
	}
	ids, parents, names := make([]string, 0, n), make([]string, 0, n), make([]string, 0, n)
	for _, d := range fx.dirs[volumes:] {
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
	for i := range size {
		dir := volumes // the first generated directory is the biggest
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
	if _, err := db.ExecContext(ctx, "ANALYZE blobfs_volume, blobfs_directory, blobfs_file, volume_owner"); err != nil {
		t.Fatal(err)
	}

	fx.biggest = fx.dirs[volumes]
	// The listed directory: at the deepest level reached, the directory
	// whose file count is nearest ten, ties to the first.
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
	fx.listed = fx.dirs[best]
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

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
