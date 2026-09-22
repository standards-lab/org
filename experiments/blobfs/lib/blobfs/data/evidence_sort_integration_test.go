//go:build integration

package data_test

// This file is the stage 14 measurement: what an index on blobfs_file
// (directory_id, created_at) buys a listing sorted by created_at. The
// library's migration set ships no such index; a consumer adds it in its
// own set, and this test creates it itself in its throwaway database. It
// runs only under BLOBFS_EVIDENCE=1; mise run evidence sets the variable
// and writes the transcript to evidence/sort-index.txt. It seeds the same
// fixture as the listing measurement, spreads the files' created_at over
// a year (bulk seeding gives every row of a batch the same timestamp,
// which would make a sort by created_at degenerate), measures the
// created_at listings without the index, creates the index, and measures
// them again, with the index's size. The helpers are the listing
// measurement's.

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/sqlate/migrate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
	blobfspostgres "github.com/standards-lab/org/experiments/blobfs/lib/blobfs/postgres"
)

// sortIndex is the index the measurement creates, under the name the
// listing measurement's plan shapes recognize, and sortIndexDDL is its
// statement.
const (
	sortIndex    = "blobfs_ix_file_directory_created"
	sortIndexDDL = "CREATE INDEX " + sortIndex + " ON blobfs_file (directory_id, created_at)"
)

// TestSortIndexCost measures, for the biggest directory (about 10,000
// files), ListFiles sorted by created_at ascending and descending on page
// 1 with no total, the same ascending sort with the exact total, and the
// cursor page after page 1, each before and after the index is created,
// plus the index's size. The sort by name of section c of the listing
// measurement is the reference: it reads the unique constraint's index
// either way.
func TestSortIndexCost(t *testing.T) {
	if os.Getenv("BLOBFS_EVIDENCE") == "" {
		t.Skip("set BLOBFS_EVIDENCE=1 to run the sort index measurement")
	}
	ctx := context.Background()
	db, dsn := livetest.OpenDSN(t)
	set, err := blobfspostgres.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	installed, err := migrate.New(db, set.Migrations, migrate.Options{Table: set.Table})
	if err != nil {
		t.Fatal(err)
	}
	if err := installed.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if livetest.Exists(ctx, t, db, sortIndex) {
		t.Fatalf("the library's set created %s; the measurement creates the index itself", sortIndex)
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
	explainer := openSimple(t, dsn)
	m := &measurement{}

	fx := seedForest(ctx, t, db)
	if _, err := db.ExecContext(ctx, "UPDATE blobfs_file SET created_at = created_at - (random() * INTERVAL '365 days')"); err != nil {
		t.Fatalf("spread created_at: %v", err)
	}
	for _, stmt := range []string{"VACUUM ANALYZE blobfs_directory", "VACUUM ANALYZE blobfs_file"} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	m.note("")
	m.note("==================== fixture ====================")
	m.note("directories: %d under the root in %d trees; files: %d; biggest directory: %s (%d files).", len(fx.dirs), trees, fixtureFiles, fx.biggest.path, fx.biggest.files)
	m.note("created_at spread uniformly over the year before seeding; distinct created_at values in the biggest directory: %s.", scalar(ctx, t, db, "SELECT COUNT(DISTINCT created_at) FROM blobfs_file WHERE directory_id = CAST($1 AS uuid)", fx.biggest.id))
	m.note("VACUUM ANALYZE ran on both tables after seeding and again after the index was built.")

	asc := []query.Sort{{Field: "created_at"}}
	desc := []query.Sort{{Field: "created_at", Descending: true}}
	forms := []struct {
		label string
		l     data.Listing
	}{
		{"created_at asc, page 1, no total", data.Listing{Sort: asc, Page: 1, Size: pageSize, Total: data.TotalNone}},
		{"created_at desc, page 1, no total", data.Listing{Sort: desc, Page: 1, Size: pageSize, Total: data.TotalNone}},
		{"created_at asc, page 1, exact total", data.Listing{Sort: asc, Page: 1, Size: pageSize}},
	}
	first, err := store.ListFiles(ctx, db, fx.biggest.id, forms[0].l)
	if err != nil || first.Next == "" {
		t.Fatalf("page 1 by created_at = %+v, %v; want a cursor", first, err)
	}
	forms = append(forms, struct {
		label string
		l     data.Listing
	}{"created_at asc, cursor page after page 1", data.Listing{Sort: asc, Size: pageSize, After: first.Next}})

	// A page's rows at each schema version, to check the index changes the
	// plan and not the answer.
	var pagesBefore, pagesAfter [][]string
	section := func(name, heading string, pages *[][]string) {
		m.note("")
		m.note("==================== %s ====================", heading)
		for _, f := range forms {
			c := captureFiles(ctx, t, store, db, fx.biggest.id, f.l)
			m.note("%s: %s", f.label, c.sql)
			measure(ctx, t, m, explainer, name, f.label, "biggest", c)
			page, err := store.ListFiles(ctx, db, fx.biggest.id, f.l)
			if err != nil {
				t.Fatal(err)
			}
			*pages = append(*pages, ids(page.Rows))
		}
	}
	section("before", "1. before the index: the library's set alone, the sort by created_at is a sort of the directory", &pagesBefore)

	if _, err := db.ExecContext(ctx, sortIndexDDL); err != nil {
		t.Fatalf("create the index: %v", err)
	}
	if !livetest.Exists(ctx, t, db, sortIndex) {
		t.Fatalf("%s does not exist after its create", sortIndex)
	}
	if _, err := db.ExecContext(ctx, "VACUUM ANALYZE blobfs_file"); err != nil {
		t.Fatal(err)
	}
	m.note("")
	m.note("index created by the measurement, as a consumer's own migration would: %s", sortIndexDDL)
	indexSize := scalar(ctx, t, db, "SELECT pg_size_pretty(pg_relation_size('"+sortIndex+"'))")
	nameIndexSize := scalar(ctx, t, db, "SELECT pg_size_pretty(pg_relation_size('blobfs_uq_file_directory_name'))")
	tableSize := scalar(ctx, t, db, "SELECT pg_size_pretty(pg_relation_size('blobfs_file'))")
	m.note("sizes: %s %s; blobfs_uq_file_directory_name %s; blobfs_file heap %s (%d rows).", sortIndex, indexSize, nameIndexSize, tableSize, fixtureFiles)
	section("after", "2. after the index", &pagesAfter)

	for i, f := range forms {
		if !slices.Equal(pagesBefore[i], pagesAfter[i]) {
			t.Errorf("%s returns different rows before and after the index:\n%v\n%v", f.label, pagesBefore[i], pagesAfter[i])
		}
	}
	m.note("")
	m.note("cross-check: every form returns the same %d ids before and after the index.", pageSize)

	var out strings.Builder
	fmt.Fprintf(&out, "# blobfs stage 14: what the created_at index buys a sorted listing\n")
	fmt.Fprintf(&out, "# date: %s\n", time.Now().UTC().Format("2006-01-02"))
	fmt.Fprintf(&out, "# engine: %s\n", version)
	fmt.Fprintf(&out, "# machine: %s/%s, %d cpus; go %s. Timings are machine-dependent (a laptop, everything in shared buffers); plan shapes and buffer counts are not.\n", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.Version())
	fmt.Fprintf(&out, "# fixture: the listing measurement's (%d trees, %d file rows, a tenth of them in the biggest directory), with created_at spread uniformly over a year.\n", trees, fixtureFiles)
	fmt.Fprintf(&out, "# method: ListFiles on the biggest directory sorted by created_at, captured as the store composes it; each query is EXPLAIN (ANALYZE, BUFFERS) once to warm the cache, then %d times; the median run by execution time is reported with its plan.\n", runs)
	fmt.Fprintf(&out, "#   Section 1 is measured over the library's migration set, which ships no created_at index, and section 2 after the measurement creates %s (directory_id, created_at), as a consumer's own migration set would.\n", sortIndex)
	fmt.Fprintf(&out, "#   EXPLAIN runs over pgx's simple protocol; buffers are the top plan node's shared hit+read, in 8 KB pages. The tables were VACUUM ANALYZEd after seeding and after the index was built.\n")
	fmt.Fprintf(&out, "# index size: %s (the name index is %s; the heap is %s).\n", indexSize, nameIndexSize, tableSize)
	fmt.Fprintf(&out, "#\n# summary (median run; times in ms; buffers as hit+read pages):\n")
	fmt.Fprintf(&out, "# %-6s %-44s %-8s %9s %9s %8s  %s\n", "sec", "query", "dir", "plan ms", "exec ms", "buffers", "plan shape")
	for _, r := range m.results {
		fmt.Fprintf(&out, "# %-6s %-44s %-8s %9.3f %9.3f %8d  %s\n", r.section, r.label, r.dir, r.planning, r.execution, r.buffers, r.shape)
	}
	out.WriteString(m.body.String())
	fmt.Print(out.String())
}
