package data_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// TestResolveDirectoryWalk proves the baseline's resolution against the
// script: a path of n segments is n+1 queries, the read of the root by
// id and then one directory_child per segment, each bound to the
// directory the last one returned and the normalized name; a walk that
// finds no child stops there and reports the prefix up to the missing
// segment; and the relative form reads the start first and refuses a
// missing start before any segment.
func TestResolveDirectoryWalk(t *testing.T) {
	ctx := context.Background()
	nfd := "cafe" + string(rune(0x0301))
	nfc := "caf" + string(rune(0x00E9))
	s, db, rec := openStore(t,
		directoryResponse(blobfs.RootID, "", "/", 1),
		directoryResponse("A", blobfs.RootID, "a", 1),
		directoryResponse("B", "A", nfc, 1),
		directoryResponse("C", "B", "c", 1),
	)
	d, err := s.ResolveDirectory(ctx, db, "/a/"+nfd+"/c")
	if err != nil || d.ID != "C" {
		t.Fatalf("ResolveDirectory = %+v, %v, want the last child", d, err)
	}
	if ops := rec.Ops(); !slices.Equal(ops, []sqltest.Op{sqltest.OpQuery, sqltest.OpQuery, sqltest.OpQuery, sqltest.OpQuery}) {
		t.Fatalf("ops = %v, want four queries for three segments", ops)
	}
	calls := rec.Calls()
	if !slices.Equal(calls[0].Args, []any{blobfs.RootID}) || !slices.Equal(calls[1].Args, []any{blobfs.RootID, "a"}) ||
		!slices.Equal(calls[2].Args, []any{"A", nfc}) || !slices.Equal(calls[3].Args, []any{"B", "c"}) {
		t.Errorf("the walk bound %v, %v, %v, %v", calls[0].Args, calls[1].Args, calls[2].Args, calls[3].Args)
	}
	if !strings.HasSuffix(calls[1].SQL, "WHERE d.parent_id = CAST($1 AS uuid) AND d.name = $2") {
		t.Errorf("a step is not directory_child:\n%s", calls[1].SQL)
	}

	// The second of ten segments missing: three queries, then the prefix.
	s, db, rec = openStore(t,
		directoryResponse(blobfs.RootID, "", "/", 1),
		directoryResponse("A", blobfs.RootID, "a", 1),
		sqltest.Response{Columns: directoryColumns},
	)
	_, err = s.ResolveDirectory(ctx, db, "/a/missing/c/d/e/f/g/h/i/j")
	if !errors.Is(err, blobfs.ErrNotFound) || !strings.HasSuffix(err.Error(), " at /a/missing: "+blobfs.ErrNotFound.Error()) {
		t.Errorf("ResolveDirectory with the second segment missing = %v, want ErrNotFound at /a/missing", err)
	}
	if n := len(rec.Ops()); n != 3 {
		t.Errorf("the walk ran %d queries, want 3: the root, a, and the missing child", n)
	}

	// The relative form: the start read first, then the segments; a
	// missing start is refused before any segment and names no prefix.
	s, db, rec = openStore(t, directoryResponse("S", blobfs.RootID, "s", 1), sqltest.Response{Columns: directoryColumns})
	_, err = s.ResolveDirectoryFrom(ctx, db, "S", "x/y")
	if !errors.Is(err, blobfs.ErrNotFound) || !strings.HasSuffix(err.Error(), " at x: "+blobfs.ErrNotFound.Error()) {
		t.Errorf("ResolveDirectoryFrom with the first segment missing = %v, want ErrNotFound at x", err)
	}
	if n := len(rec.Ops()); n != 2 {
		t.Errorf("the relative walk ran %d queries, want 2: the start and the missing child", n)
	}
	s, db, rec = openStore(t, sqltest.Response{Columns: directoryColumns})
	_, err = s.ResolveDirectoryFrom(ctx, db, "S", "x")
	if !errors.Is(err, blobfs.ErrNotFound) || strings.Contains(err.Error(), " at ") {
		t.Errorf("ResolveDirectoryFrom from a missing start = %v, want ErrNotFound with no prefix", err)
	}
	if n := len(rec.Ops()); n != 1 {
		t.Errorf("the refused walk ran %d queries, want the start read alone", n)
	}
}
