//go:build integration

package files

// This file is the cost-shape assertion for the bookmark read model, in
// the package itself so it can run the projection through a recording
// session as the evidence measurement does. It runs under mise run
// integration; it measures nothing and asserts plan shapes only.

import (
	"context"
	"strings"
	"testing"

	"github.com/standards-lab/sqlate"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// TestBookmarkListingShapes proves, on the engine, that the default
// bookmark listing runs no recursion and the listing with paths runs
// the per-row walk on its page only: the count and the page of each are
// captured as the store composes them and explained; the default plans
// hold no Recursive Union, the page with paths holds one, and the count
// with paths holds none, because the planner drops the scalar subquery
// that nothing outside the derived table references. The statement text
// is checked too, so the assertion does not depend on the planner alone.
func TestBookmarkListingShapes(t *testing.T) {
	ctx := context.Background()
	db, _ := livetest.OpenDSN(t)
	evMigrate(ctx, t, db)
	store, err := New(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	unit := blobfs.NewID()
	dir, err := store.Mkdir(ctx, "/d", "")
	if err != nil {
		t.Fatal(err)
	}
	fileID := blobfs.NewID()
	if _, err := db.ExecContext(ctx, "INSERT INTO blobfs_file (id, directory_id, name, status, key, content_type) VALUES ($1, $2, 'a.txt', 'available', $3, 'text/plain')", fileID, dir.ID, fileID+"/a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddBookmark(ctx, "/d/a.txt", unit, false); err != nil {
		t.Fatal(err)
	}

	shipped := func(l Listing) []evCall {
		rec := &recorder{DB: db}
		if _, err := store.bookmarksOf(ctx, rec, unit, l); err != nil {
			t.Fatalf("bookmarksOf(%+v): %v", l, err)
		}
		if len(rec.calls) != 2 {
			t.Fatalf("bookmarksOf ran %d queries, want the count and the page", len(rec.calls))
		}
		return rec.calls
	}
	for _, tc := range []struct {
		label     string
		l         Listing
		recursive bool
	}{
		{"default", Listing{Page: 1, Size: 20}, false},
		{"with paths", Listing{Page: 1, Size: 20, Paths: true}, true},
	} {
		for i, c := range shipped(tc.l) {
			if strings.Contains(c.sql, "WITH RECURSIVE") != tc.recursive {
				t.Errorf("%s, query %d: the statement's recursion is %v, want %v:\n%s", tc.label, i, !tc.recursive, tc.recursive, c.sql)
			}
			// Only the page (query 1) walks; the count twin never does.
			want := tc.recursive && i == 1
			plan := explain(ctx, t, db, c)
			if strings.Contains(plan, "Recursive Union") != want {
				t.Errorf("%s, query %d: the plan's recursion is %v, want %v:\n%s", tc.label, i, !want, want, plan)
			}
		}
	}
}

// explain returns the plan text of c, without running it.
func explain(ctx context.Context, t *testing.T, db *sqlate.DB, c evCall) string {
	t.Helper()
	rows, err := db.QueryContext(ctx, "EXPLAIN "+c.sql, c.args...)
	if err != nil {
		t.Fatalf("explain: %v\n%s", err, c.sql)
	}
	defer func() { _ = rows.Close() }()
	var lines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return strings.Join(lines, "\n")
}
