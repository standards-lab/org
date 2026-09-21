//go:build integration

package postgres_test

// This file holds the cost regression assertions of the Postgres
// variant's variation points (adjustment 19 of DECISIONS.md): the
// row-value keyset predicate, the one-statement path resolution, and the
// write steps that return the row. Each test seeds a fixture in its own
// throwaway database, captures a statement as the store composes it or
// takes it from the variant's inventory, explains it with EXPLAIN
// (ANALYZE, BUFFERS) through internal/livetest, and asserts a plan shape
// and a buffer bound, never a time. The bounds are set with a wide margin
// over the value measured on this fixture and well below what the
// regression the test guards against would read; each test's comment
// gives the measured values on the evidence fixture (100,000 files, the
// biggest directory 10,009) and on this one. The round trips of each
// variation point are asserted hermetically in this package and not here.

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/postgres"
)

// listingSizes is the fixture of the keyset assertion: the big directory
// holds a tenth of the files, as on the evidence fixture, and its files
// sit on contiguous heap pages. stepSizes is the fixture of the per-row
// assertions: enough rows that a lookup by primary key or by the unique
// constraint is the planner's choice over a sequential scan.
var (
	listingSizes = livetest.Sizes{Depth: 8, Directories: 3000, BigFiles: 8000, OtherFiles: 72000}
	stepSizes    = livetest.Sizes{Depth: 8, Directories: 3000, BigFiles: 2000, OtherFiles: 8000}
)

// costPageSize is the page size of the keyset assertion, and middlePage
// is the number of the offset page whose cursor continues from the
// middle of the big directory.
const costPageSize = 20

var middlePage = listingSizes.BigFiles / costPageSize / 2

// sortIndexDDL creates the index a consumer's own migration set adds for
// a listing sorted by created_at (decision 5); the library's set ships
// none, and the keyset assertion creates it in its own database.
const sortIndexDDL = "CREATE INDEX blobfs_ix_file_directory_created ON blobfs_file (directory_id, created_at)"

// recorder is a sqlate.Session over the pool that keeps every query's
// text and arguments, so a test explains exactly what the store ran. The
// embedded *sqlate.DB keeps MapError reachable.
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

// one runs op through a recorder over db and returns the one query it
// ran; more than one fails the test, since the point of each variation
// point is one statement.
func one(t *testing.T, db *sqlate.DB, op func(sess sqlate.Session) error) call {
	t.Helper()
	rec := &recorder{DB: db}
	if err := op(rec); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("the operation ran %d queries, want 1", len(rec.calls))
	}
	return rec.calls[0]
}

// TestRowValueCursorCost proves that a cursor page sorted by created_at
// over the consumer's (directory_id, created_at) index costs the same
// wherever the cursor stands: the plan is an index scan on that index
// whose index condition carries the created_at bound of the row-value
// comparison, with no sequential scan; a cursor in the middle of the
// directory reads at most twice the buffers of a cursor at its start,
// and each reads at most 100. The regression it catches is the expanded
// OR chain, which the planner applies as a filter over the directory
// from its start, so the cost grows with the cursor's position.
// Measured on the evidence fixture: 35 buffers at the start and 35 in
// the middle for the row value, against 51 and 5,055 for the OR chain;
// on this fixture 25 to 26 at both positions against 43 and 5,031.
func TestRowValueCursorCost(t *testing.T) {
	e := open(t)
	tr := livetest.SeedTree(e.ctx, t, e.db, listingSizes)
	for _, stmt := range []string{sortIndexDDL, "ANALYZE blobfs_file"} {
		if _, err := e.db.ExecContext(e.ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	ex := livetest.NewExplainer(t, e.dsn)
	const bound = 100
	explainAfter := func(sort []query.Sort, page int) livetest.Plan {
		t.Helper()
		offset, err := e.native.ListFiles(e.ctx, e.db, tr.Big.ID, data.Listing{Sort: sort, Page: page, Size: costPageSize, Total: data.TotalNone})
		if err != nil || offset.Next == "" {
			t.Fatalf("page %d = %+v, %v; want a cursor", page, offset, err)
		}
		c := one(t, e.db, func(sess sqlate.Session) error {
			_, err := e.native.ListFiles(e.ctx, sess, tr.Big.ID, data.Listing{Sort: sort, Size: costPageSize, After: offset.Next})
			return err
		})
		return ex.Explain(e.ctx, t, c.sql, c.args...)
	}
	for _, tc := range []struct {
		label   string
		sort    []query.Sort
		compare string
	}{
		{"created_at ascending", []query.Sort{{Field: "created_at"}}, "created_at >="},
		{"created_at descending", []query.Sort{{Field: "created_at", Descending: true}}, "created_at <="},
	} {
		start, middle := explainAfter(tc.sort, 1), explainAfter(tc.sort, middlePage)
		t.Logf("%s: the cursor page costs %d buffers at the start and %d in the middle", tc.label, start.Buffers, middle.Buffers)
		for _, p := range []struct {
			position string
			plan     livetest.Plan
		}{{"start", start}, {"middle", middle}} {
			switch {
			case p.plan.Has("Seq Scan"), !p.plan.Has("using blobfs_ix_file_directory_created"):
				t.Errorf("%s, %s: the cursor page is not an index scan on the created_at index:\n%s", tc.label, p.position, p.plan.Text)
			case !indexCondHas(p.plan, tc.compare):
				t.Errorf("%s, %s: the bound %q is not an index condition:\n%s", tc.label, p.position, tc.compare, p.plan.Text)
			case p.plan.Buffers > bound:
				t.Errorf("%s, %s: the cursor page reads %d buffers, more than %d:\n%s", tc.label, p.position, p.plan.Buffers, bound, p.plan.Text)
			}
		}
		if middle.Buffers > 2*start.Buffers {
			t.Errorf("%s: the cursor page reads %d buffers in the middle of the directory and %d at its start; the cost depends on the position:\n%s", tc.label, middle.Buffers, start.Buffers, middle.Text)
		}
	}
}

// TestResolvePathCost proves that the one-statement path resolution
// costs the depth and not the table: resolving the chain's depth-6 path
// plans a Recursive Union whose anchor is an index scan on
// blobfs_pk_directory and whose step is an index scan on
// blobfs_uq_directory_parent_name, with no sequential scan on
// blobfs_directory, and it reads at most 8 buffers per segment. The
// regression it catches is a step the unique index cannot serve, which
// scans the directory table once per level. Measured on the evidence
// fixture: 24 buffers at depth 6 and 36 at depth 10; on this fixture 21
// at depth 6, three for the anchor and three per segment.
func TestResolvePathCost(t *testing.T) {
	e := open(t)
	tr := livetest.SeedTree(e.ctx, t, e.db, stepSizes)
	ex := livetest.NewExplainer(t, e.dsn)
	const depth = 6
	bound := 8 * depth
	c := one(t, e.db, func(sess sqlate.Session) error {
		d, err := e.native.ResolveDirectory(e.ctx, sess, tr.Path(depth))
		if err == nil && d.ID != tr.Chain[depth-1].ID {
			t.Fatalf("resolved %s to %s, want %s", tr.Path(depth), d.ID, tr.Chain[depth-1].ID)
		}
		return err
	})
	p := ex.Explain(e.ctx, t, c.sql, c.args...)
	t.Logf("the resolution of a depth-%d path costs %d buffers; the bound is %d", depth, p.Buffers, bound)
	switch {
	case p.Has("Seq Scan"), !p.Has("Recursive Union"), !p.Has("Index Scan using blobfs_pk_directory"), !p.Has("Index Scan using blobfs_uq_directory_parent_name"):
		t.Errorf("the resolution is not a recursion over the two indexes:\n%s", p.Text)
	case !indexCondHas(p, "parent_id ="), !indexCondHas(p, "name ="):
		t.Errorf("the step's index condition does not carry both columns:\n%s", p.Text)
	case p.Buffers > bound:
		t.Errorf("the resolution of a depth-%d path reads %d buffers, more than %d:\n%s", depth, p.Buffers, bound, p.Text)
	}
}

// TestNativeProtocolStepPlans proves that the variant's write steps that
// return the row find it through blobfs_pk_file: the begin of a delete
// and the complete of a write each plan an index scan on the primary key
// with the id as the index condition under the Update node, with no
// sequential scan, and each reads at most 32 buffers. The regression it
// catches is a predicate the primary key cannot serve, which scans the
// table on every step. The inserts with RETURNING are left out: an
// insert has no lookup to plan, and its buffers are the heap and index
// maintenance of the new row (68 on the evidence fixture for a file and
// 89 for a directory), which vary with the fixture. Measured on the
// evidence fixture: 16 buffers for the complete step; on this fixture 6
// for each step.
func TestNativeProtocolStepPlans(t *testing.T) {
	e := open(t)
	tr := livetest.SeedTree(e.ctx, t, e.db, stepSizes)
	ex := livetest.NewExplainer(t, e.dsn)
	const bound = 32
	id := e.insertFile(t, tr.Big.ID, "step.txt")
	pending := e.insertFile(t, tr.Big.ID, "pending.txt")
	if _, err := e.db.ExecContext(e.ctx, "UPDATE blobfs_file SET status = 'pending' WHERE id = $1", pending); err != nil {
		t.Fatal(err)
	}
	stmts := map[string]query.Statement{}
	for _, st := range e.native.Variant().(*postgres.Variant).Statements() {
		stmts[st.Name()] = st
	}
	for _, tc := range []struct {
		name string
		args query.Args
	}{
		{"begin_file_delete", query.Args{"id": id}},
		{"complete_file_write", query.Args{"id": pending, "version": int64(1), "size": int64(3), "content_type": "text/plain", "etag": "etag"}},
	} {
		st, ok := stmts[tc.name]
		if !ok {
			t.Fatalf("the variant has no %s statement", tc.name)
		}
		if !strings.Contains(st.Text(), "RETURNING") {
			t.Fatalf("%s does not return the row:\n%s", tc.name, st.Text())
		}
		args := make([]any, 0, len(st.Params()))
		for _, name := range st.Params() {
			args = append(args, tc.args[name])
		}
		p := ex.Explain(e.ctx, t, st.Text(), args...)
		t.Logf("%s costs %d buffers", tc.name, p.Buffers)
		switch {
		case p.Has("Seq Scan"), !p.Has("Update on blobfs_file"), !p.Has("Index Scan using blobfs_pk_file"), !indexCondHas(p, "id ="):
			t.Errorf("%s does not find the row through the primary key:\n%s", tc.name, p.Text)
		case p.Buffers > bound:
			t.Errorf("%s reads %d buffers, more than %d:\n%s", tc.name, p.Buffers, bound, p.Text)
		}
	}
}

// indexCondHas reports whether an Index Cond line of the plan contains s,
// so a comparison the planner applies as a filter instead does not pass.
func indexCondHas(p livetest.Plan, s string) bool {
	for _, line := range p.Lines("Index Cond:") {
		if strings.Contains(line, s) {
			return true
		}
	}
	return false
}
