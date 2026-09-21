package postgres_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// resolvedColumns is the resolve_path row: the directory columns then the
// depth.
var resolvedColumns = []string{"id", "parent_id", "name", "version", "created_at", "updated_at", "depth"}

// resolvedResponse scripts the deepest row a walk reached, at depth.
func resolvedResponse(id, parent, name string, depth int64) sqltest.Response {
	now := time.Now()
	return sqltest.Response{Columns: resolvedColumns, Rows: [][]driver.Value{{id, parent, name, int64(1), now, now, depth}}}
}

// TestResolvePathIsOneStatement proves path resolution through the
// Postgres variant is one query whatever the depth: the recursive
// statement bound to the start id and the segments as one slice, in
// path order, normalized; that the root path binds an empty slice; that
// a walk that stops short reports the failing prefix as the baseline
// spells it; and that no row is ErrNotFound for the start.
func TestResolvePathIsOneStatement(t *testing.T) {
	ctx := context.Background()
	nfd := "cafe" + string(rune(0x0301))
	nfc := "caf" + string(rune(0x00E9))
	s, db, rec := openNative(t, resolvedResponse("D", "P", "z", 3))
	d, err := s.ResolveDirectory(ctx, db, "/a/"+nfd+"/z")
	if err != nil || d.ID != "D" || d.Name != "z" {
		t.Fatalf("ResolveDirectory = %+v, %v, want the resolved row", d, err)
	}
	if ops := rec.Ops(); !slices.Equal(ops, []sqltest.Op{sqltest.OpQuery}) {
		t.Fatalf("ops = %v, want one query for three segments", ops)
	}
	c := rec.Calls()[0]
	if !strings.HasPrefix(c.SQL, "WITH RECURSIVE walk (id, parent_id, name, version, created_at, updated_at, depth) AS (") ||
		!strings.Contains(c.SQL, "WHERE d.id = CAST($1 AS uuid)") ||
		!strings.Contains(c.SQL, "d.name = (CAST($2 AS text[]))[w.depth + 1]") ||
		!strings.HasSuffix(c.SQL, "WHERE w.depth = (SELECT max(x.depth) FROM walk x)") {
		t.Errorf("the statement is not resolve_path:\n%s", c.SQL)
	}
	if len(c.Args) != 2 || c.Args[0] != blobfs.RootID {
		t.Fatalf("resolve bound %v, want the root id and the segments", c.Args)
	}
	if segs, ok := c.Args[1].([]string); !ok || !slices.Equal(segs, []string{"a", nfc, "z"}) {
		t.Errorf("the segments bound as %#v, want the normalized names as one []string", c.Args[1])
	}

	s, db, rec = openNative(t, resolvedResponse(blobfs.RootID, "", "/", 0))
	if _, err := s.ResolveDirectory(ctx, db, "/"); err != nil {
		t.Fatalf("ResolveDirectory(/) = %v", err)
	}
	if segs, ok := rec.Calls()[0].Args[1].([]string); !ok || len(segs) != 0 {
		t.Errorf("the root path bound %#v, want an empty []string", rec.Calls()[0].Args[1])
	}

	// Ten segments, the walk stopping at the second: one query, and the
	// prefix names the segment that failed.
	s, db, rec = openNative(t, resolvedResponse("A", blobfs.RootID, "a", 1))
	_, err = s.ResolveDirectory(ctx, db, "/a/missing/c/d/e/f/g/h/i/j")
	if !errors.Is(err, blobfs.ErrNotFound) || !strings.HasSuffix(err.Error(), " at /a/missing: "+blobfs.ErrNotFound.Error()) {
		t.Errorf("ResolveDirectory with the second segment missing = %v, want ErrNotFound at /a/missing", err)
	}
	if ops := rec.Ops(); !slices.Equal(ops, []sqltest.Op{sqltest.OpQuery}) {
		t.Errorf("ops = %v, want one query for ten segments", ops)
	}

	// The relative form: bound to the start, and a missing prefix spelled
	// without a leading slash.
	s, db, rec = openNative(t, resolvedResponse("S", blobfs.RootID, "s", 0))
	_, err = s.ResolveDirectoryFrom(ctx, db, "S", "x/y")
	if !errors.Is(err, blobfs.ErrNotFound) || !strings.HasSuffix(err.Error(), " at x: "+blobfs.ErrNotFound.Error()) {
		t.Errorf("ResolveDirectoryFrom with the first segment missing = %v, want ErrNotFound at x", err)
	}
	if c := rec.Calls()[0]; c.Args[0] != "S" {
		t.Errorf("the relative resolve bound %v, want the start id first", c.Args)
	}

	// No row: the start does not exist, and no prefix is named.
	s, db, _ = openNative(t, sqltest.Response{Columns: resolvedColumns})
	_, err = s.ResolveDirectoryFrom(ctx, db, "S", "x")
	if !errors.Is(err, blobfs.ErrNotFound) || strings.Contains(err.Error(), " at ") {
		t.Errorf("ResolveDirectoryFrom from a missing start = %v, want ErrNotFound with no prefix", err)
	}
}
