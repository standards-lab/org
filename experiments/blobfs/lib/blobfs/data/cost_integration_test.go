//go:build integration

package data_test

// This file holds the cost regression assertions of the standard tier
// (REVIEW.md, "Phase 3", adjustment 19): each test seeds a fixture in its own
// throwaway database, captures a statement as the store composes it or
// takes it from the store's inventory, explains it with EXPLAIN (ANALYZE,
// BUFFERS) through internal/livetest, and asserts a plan shape and a
// buffer bound. No test asserts a time. The bounds are set with a wide
// margin over the value measured on this fixture and well below what the
// regression the test guards against would read; the comment of each
// test gives the measured values on the evidence fixture (100,000 files,
// the biggest directory 10,000) and on this one. A plan shape is asserted
// only where the fixture is large enough for the index to be the
// planner's own choice, and the tests never disable a plan type.

import (
	"strings"
	"testing"

	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

// listingSizes is the fixture of the listing assertions: the big
// directory holds a tenth of the files, as on the evidence fixture, so a
// read of the whole directory is a bitmap scan over its own pages and
// not a sequential scan of the table, which the planner rightly prefers
// once a directory holds a quarter of the rows. Its files were inserted
// first, so they sit on contiguous heap pages and a scan of the whole
// table reads about ten times as many pages as a scan of the directory.
var listingSizes = livetest.Sizes{Depth: 8, Directories: 3000, BigFiles: 8000, OtherFiles: 72000}

// stepSizes is the fixture of the per-row assertions: enough rows that a
// lookup by primary key or by the unique constraint is the planner's
// choice over a sequential scan on both tables.
var stepSizes = livetest.Sizes{Depth: 8, Directories: 3000, BigFiles: 2000, OtherFiles: 8000}

// costPageSize is the page size of the listing assertions, and
// middlePage is the number of the offset page whose cursor continues
// from the middle of the big directory.
const costPageSize = 20

var middlePage = listingSizes.BigFiles / costPageSize / 2

// statementsByName indexes a store's inventory by name; a name the
// variant overrides keeps the last, native, statement, so a test of the
// baseline reads from a baseline store.
func statementsByName(store *data.Store) map[string]query.Statement {
	out := map[string]query.Statement{}
	for _, st := range store.Statements() {
		out[st.Name()] = st
	}
	return out
}

// bindArgs orders args by the statement's parameters, as the query
// library does before it runs the statement.
func bindArgs(t *testing.T, st query.Statement, args query.Args) []any {
	t.Helper()
	out := make([]any, 0, len(st.Params()))
	for _, name := range st.Params() {
		v, ok := args[name]
		if !ok {
			t.Fatalf("%s: no value for %s", st.Name(), name)
		}
		out = append(out, v)
	}
	return out
}

// TestCursorPageCost proves that a page continued by cursor from the
// middle of a directory, sorted by name in either direction, is bounded
// by the page size and not by the directory: the plan is an index scan
// on blobfs_uq_file_directory_name whose index condition carries the
// keyset comparison on name, with no sort and no sequential scan, and it
// reads at most 96 buffers. The regression it catches is a keyset
// predicate the index cannot serve, for example a comparison wrapped in
// an expression or hidden in a derived table, which reads the directory
// up to the cursor as a filter. Measured: 12 to 13 buffers at any cursor
// position on the evidence fixture; 22 to 24 on this fixture (three
// index pages and about one heap page per row of the page, since name
// order and heap order differ). A read of the whole directory costs 255 buffers here,
// which the test checks stays above the bound so the bound has teeth.
func TestCursorPageCost(t *testing.T) {
	e := open(t)
	tr := livetest.SeedTree(e.ctx, t, e.db, listingSizes)
	ex := livetest.NewExplainer(t, e.dsn)
	const bound = 96

	whole := captureFiles(e.ctx, t, e.store, e.db, tr.Big.ID, data.Listing{Page: 1, Size: costPageSize})
	if p := ex.Explain(e.ctx, t, whole.sql, whole.args...); p.Buffers <= bound {
		t.Fatalf("the fixture is too small: a read of the whole directory costs %d buffers, within the bound %d", p.Buffers, bound)
	} else {
		t.Logf("a read of the whole directory costs %d buffers; the bound is %d", p.Buffers, bound)
	}
	desc := []query.Sort{{Field: "name", Descending: true}}
	for _, tc := range []struct {
		label   string
		sort    []query.Sort
		compare string
	}{
		{"name ascending", nil, "name >"},
		{"name descending", desc, "name <"},
	} {
		middle, err := e.store.ListFiles(e.ctx, e.db, tr.Big.ID, data.Listing{Sort: tc.sort, Page: middlePage, Size: costPageSize, Total: data.TotalNone})
		if err != nil || middle.Next == "" {
			t.Fatalf("%s: page %d = %+v, %v; want a cursor", tc.label, middlePage, middle, err)
		}
		c := captureFiles(e.ctx, t, e.store, e.db, tr.Big.ID, data.Listing{Sort: tc.sort, Size: costPageSize, After: middle.Next})
		p := ex.Explain(e.ctx, t, c.sql, c.args...)
		t.Logf("%s: the cursor page from the middle costs %d buffers", tc.label, p.Buffers)
		switch {
		case p.Has("Seq Scan"), p.Has("Sort"), !p.Has("using blobfs_uq_file_directory_name"):
			t.Errorf("%s: the cursor page is not an index scan on the name index without a sort:\n%s", tc.label, p.Text)
		case !indexCondHas(p, tc.compare):
			t.Errorf("%s: the keyset comparison %q is not an index condition:\n%s", tc.label, tc.compare, p.Text)
		case p.Buffers > bound:
			t.Errorf("%s: the cursor page reads %d buffers, more than %d:\n%s", tc.label, p.Buffers, bound, p.Text)
		}
	}
}

// TestExactTotalPageCost proves that the page with the exact total reads
// the directory once: the plan holds one read of blobfs_file, through
// its index and not a sequential scan, under one WindowAgg, with no
// subplan that would count the rows a second time, and it reads at most
// three times the directory's own heap pages. The regression it catches
// is a total computed over the table instead of the directory, such as a
// lost anchor or a window over an unanchored derived table, and a second
// pass such as a count subquery. Measured: 2,434 buffers on the evidence
// fixture; 255 on this fixture, the directory's 187 heap pages and its
// share of the index, against 1,861 pages for the whole table, which the
// test checks stays above the bound so the bound has teeth.
func TestExactTotalPageCost(t *testing.T) {
	e := open(t)
	tr := livetest.SeedTree(e.ctx, t, e.db, listingSizes)
	ex := livetest.NewExplainer(t, e.dsn)
	bound := 3 * livetest.HeapBlocks(e.ctx, t, e.db, tr.Big.ID)
	if table := livetest.RelationPages(e.ctx, t, e.db, "blobfs_file"); table <= bound {
		t.Fatalf("the fixture is too small: the table has %d pages, within the bound %d", table, bound)
	}
	c := captureFiles(e.ctx, t, e.store, e.db, tr.Big.ID, data.Listing{Page: 1, Size: costPageSize})
	if !strings.Contains(c.sql, "COUNT(*) OVER ()") {
		t.Fatalf("the page with the exact total carries no window count:\n%s", c.sql)
	}
	p := ex.Explain(e.ctx, t, c.sql, c.args...)
	t.Logf("the exact-total page costs %d buffers; the bound is %d, the table has %d pages", p.Buffers, bound, livetest.RelationPages(e.ctx, t, e.db, "blobfs_file"))
	switch {
	case p.Has("Seq Scan"), p.Has("SubPlan"), p.Has("InitPlan"), !p.Has("WindowAgg"):
		t.Errorf("the exact-total page is not one index-backed read under a WindowAgg:\n%s", p.Text)
	case p.HeapScans("blobfs_file") != 1:
		t.Errorf("the exact-total page reads blobfs_file %d times, want once:\n%s", p.HeapScans("blobfs_file"), p.Text)
	case p.Buffers > bound:
		t.Errorf("the exact-total page reads %d buffers, more than %d:\n%s", p.Buffers, bound, p.Text)
	}
}

// TestPathWalkStepCost proves that each step of the baseline's path walk
// is one search of blobfs_uq_directory_parent_name: the directory_child
// statement's plan is an index scan whose index condition carries both
// the parent and the name, with no sequential scan, and it reads at most
// 8 buffers. The regression it catches is a predicate the unique index
// cannot serve on one of its columns, which reads every child of the
// parent or the whole table. The round trips of the walk, one per
// segment, are asserted hermetically in paths_test.go and not here.
// Measured: 3 buffers per step on the evidence fixture and on this one.
func TestPathWalkStepCost(t *testing.T) {
	e := open(t)
	tr := livetest.SeedTree(e.ctx, t, e.db, stepSizes)
	ex := livetest.NewExplainer(t, e.dsn)
	const bound = 8
	st, ok := statementsByName(e.store)["directory_child"]
	if !ok {
		t.Fatal("the store has no directory_child statement")
	}
	p := ex.Explain(e.ctx, t, st.Text(), bindArgs(t, st, query.Args{"parent_id": tr.Chain[4].ID, "name": tr.Chain[5].Name})...)
	t.Logf("the child read costs %d buffers", p.Buffers)
	switch {
	case p.Has("Seq Scan"), !p.Has("Index Scan using blobfs_uq_directory_parent_name"):
		t.Errorf("the child read is not an index scan on the unique constraint:\n%s", p.Text)
	case !indexCondHas(p, "parent_id ="), !indexCondHas(p, "name ="):
		t.Errorf("the child read's index condition does not carry both columns:\n%s", p.Text)
	case p.Buffers > bound:
		t.Errorf("the child read costs %d buffers, more than %d:\n%s", p.Buffers, bound, p.Text)
	}
}

// TestProtocolStepPlans proves that the baseline's guarded steps and
// read-backs on a file row find the row through blobfs_pk_file: the
// begin of a delete, the guarded complete of a write with its version
// check, the read by id, and the hold each plan an index scan on the
// primary key with the id as the index condition, with no sequential
// scan, and each reads at most 32 buffers. The regression it catches is
// a predicate the primary key cannot serve, which scans the table on
// every step. The inserts of the write protocol are left out: an insert
// has no lookup to plan, and its buffers are the heap and index
// maintenance of the new row (68 on the evidence fixture), which vary
// with the fixture. The round trips of each step are asserted
// hermetically and not here. Measured on the evidence fixture: 16
// buffers for the complete step and 4 for a read; on this fixture 6 for
// a step that updates the row and 3 for a read.
func TestProtocolStepPlans(t *testing.T) {
	e := open(t)
	tr := livetest.SeedTree(e.ctx, t, e.db, stepSizes)
	ex := livetest.NewExplainer(t, e.dsn)
	const bound = 32
	id := insertFile(e.ctx, t, e.db, tr.Big.ID, "step.txt")
	pending := insertFile(e.ctx, t, e.db, tr.Big.ID, "pending.txt")
	if _, err := e.db.ExecContext(e.ctx, "UPDATE blobfs_file SET status = 'pending' WHERE id = $1", pending); err != nil {
		t.Fatal(err)
	}
	stmts := statementsByName(e.store)
	for _, tc := range []struct {
		name string
		args query.Args
	}{
		{"begin_file_delete", query.Args{"id": id}},
		{"complete_file_write", query.Args{"id": pending, "version": int64(1), "size": int64(3), "content_type": "text/plain", "etag": "etag"}},
		{"file_version", query.Args{"id": id}},
		{"file_by_id", query.Args{"id": id}},
		{"hold_file", query.Args{"id": id}},
	} {
		st, ok := stmts[tc.name]
		if !ok {
			t.Fatalf("the store has no %s statement", tc.name)
		}
		p := ex.Explain(e.ctx, t, st.Text(), bindArgs(t, st, tc.args)...)
		t.Logf("%s costs %d buffers", tc.name, p.Buffers)
		switch {
		case p.Has("Seq Scan"), !p.Has("Index Scan using blobfs_pk_file"), !indexCondHas(p, "id ="):
			t.Errorf("%s does not find the row through the primary key:\n%s", tc.name, p.Text)
		case p.Buffers > bound:
			t.Errorf("%s reads %d buffers, more than %d:\n%s", tc.name, p.Buffers, bound, p.Text)
		}
	}
}

// indexCondHas reports whether an Index Cond line of the plan contains s,
// so a comparison that the planner applies as a filter instead does not
// pass.
func indexCondHas(p livetest.Plan, s string) bool {
	for _, line := range p.Lines("Index Cond:") {
		if strings.Contains(line, s) {
			return true
		}
	}
	return false
}
