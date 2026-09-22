//go:build integration

package files_test

import (
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/domain/files"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// fileNames returns the names of a file page.
func fileNames(rows []blobfs.File) []string {
	out := make([]string, 0, len(rows))
	for _, f := range rows {
		out = append(out, f.Name)
	}
	return out
}

// sameContents reports whether two listings hold the same rows and
// totals, ignoring the path each reports.
func sameContents(t *testing.T, label string, byPath, byID files.Contents) {
	t.Helper()
	if !slices.Equal(names(byPath.Directories.Rows), names(byID.Directories.Rows)) || byPath.Directories.Total != byID.Directories.Total {
		t.Errorf("%s: directories by path = %v (total %d), by id = %v (total %d)", label, names(byPath.Directories.Rows), byPath.Directories.Total, names(byID.Directories.Rows), byID.Directories.Total)
	}
	if !slices.Equal(fileNames(byPath.Files.Rows), fileNames(byID.Files.Rows)) || byPath.Files.Total != byID.Files.Total {
		t.Errorf("%s: files by path = %v (total %d), by id = %v (total %d)", label, fileNames(byPath.Files.Rows), byPath.Files.Total, fileNames(byID.Files.Rows), byID.Files.Total)
	}
	for i := range byPath.Files.Rows {
		if i < len(byID.Files.Rows) && byPath.Files.Rows[i].ID != byID.Files.Rows[i].ID {
			t.Errorf("%s: file %d differs by id: %s and %s", label, i, byPath.Files.Rows[i].ID, byID.Files.Rows[i].ID)
		}
	}
}

// TestIDForms proves each id-keyed method against its path form on the
// same data, on the engine: the listing, stat, and open by id return
// what the path forms return; put by directory id writes what put by
// path writes; a move by id moves what a move by path moves, with the
// same result paths and the same scope rule; and a delete by id removes
// the file. A version from a listing acts without a read and a stale
// one is refused, while a version-less call reads the row first.
func TestIDForms(t *testing.T) {
	e := open(t)
	docs := e.mkdir(t, "/docs", "")
	y2026 := e.mkdir(t, "/docs/2026", "")
	e.mkdir(t, "/docs/2025", "")
	beta := e.mkdir(t, "/beta", "")
	e.put(t, "/docs/2026/plan.txt", "text/plain", "plan")
	e.put(t, "/docs/top.txt", "text/plain", "top")
	e.put(t, "/beta/b.txt", "text/plain", "b")
	e.put(t, "/root.txt", "text/plain", "r")
	l := files.Listing{Page: 1, Size: 10}

	// The listing: the same rows with no path reported; the root by id
	// lists what / lists; a missing id is ErrNotFound.
	byID, err := e.store.ListDirectory(e.ctx, docs.ID, l, files.Scope{})
	if err != nil {
		t.Fatalf("ListDirectory(/docs): %v", err)
	}
	sameContents(t, "/docs", e.list(t, "/docs", l), byID)
	if byID.Path != "" {
		t.Errorf("a listing by id reports the path %q, want none", byID.Path)
	}
	byID, err = e.store.ListDirectory(e.ctx, blobfs.RootID, l, files.Scope{})
	if err != nil {
		t.Fatalf("ListDirectory(root): %v", err)
	}
	sameContents(t, "/", e.list(t, "/", l), byID)
	if _, err := e.store.ListDirectory(e.ctx, blobfs.NewID(), l, files.Scope{}); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("ListDirectory of a missing id = %v, want ErrNotFound", err)
	}

	// Stat and open: the same row and the same bytes.
	plan := e.stat(t, "/docs/2026/plan.txt")
	f, err := e.store.StatFile(e.ctx, plan.ID, files.Scope{})
	if err != nil || f.ID != plan.ID || f.Name != plan.Name || f.DirectoryID != y2026.ID || f.Version != plan.Version || f.Key != plan.Key {
		t.Errorf("StatFile = %+v, %v; Stat = %+v", f, err, plan)
	}
	body, _, err := e.store.OpenFile(e.ctx, plan.ID, files.Scope{})
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	got, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || string(got) != "plan" || string(got) != e.read(t, "/docs/2026/plan.txt") {
		t.Errorf("OpenFile read %q, %v", got, err)
	}
	if _, err := e.store.StatFile(e.ctx, blobfs.NewID(), files.Scope{}); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("StatFile of a missing id = %v, want ErrNotFound", err)
	}

	// Put by directory id: the row lands where put by path would put it,
	// a taken name and a missing directory are refused.
	res, err := e.store.PutFile(e.ctx, y2026.ID, "notes.txt", files.PutRequest{ContentType: "text/plain", Body: strings.NewReader("notes"), Size: 5}, files.Scope{})
	if err != nil || res.File.DirectoryID != y2026.ID || res.File.Status != blobfs.StatusAvailable {
		t.Fatalf("PutFile = %+v, %v", res, err)
	}
	if f := e.stat(t, "/docs/2026/notes.txt"); f.ID != res.File.ID || e.read(t, "/docs/2026/notes.txt") != "notes" {
		t.Errorf("the file put by id is not at its path: %+v", f)
	}
	if _, err := e.store.PutFile(e.ctx, y2026.ID, "notes.txt", files.PutRequest{Body: strings.NewReader("x")}, files.Scope{}); !errors.Is(err, blobfs.ErrNameTaken) {
		t.Errorf("PutFile over a taken name = %v, want ErrNameTaken", err)
	}
	if _, err := e.store.PutFile(e.ctx, blobfs.NewID(), "x.txt", files.PutRequest{Body: strings.NewReader("x")}, files.Scope{}); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("PutFile into a missing directory = %v, want ErrNotFound", err)
	}

	// Move by id: a file renamed into another directory with the version
	// from the read, then a stale version refused, then a directory moved
	// under the lock, a cycle, and the scope rule.
	mv, err := e.store.MoveEntry(e.ctx, files.MoveRequest{Kind: files.EntryFile, ID: plan.ID, DirectoryID: docs.ID, Name: "plan-moved.txt"}, files.Scope{})
	if err != nil || mv.From != "/docs/2026/plan.txt" || mv.To != "/docs/plan-moved.txt" || mv.Kind != files.EntryFile {
		t.Fatalf("MoveEntry of a file = %+v, %v", mv, err)
	}
	moved := e.stat(t, "/docs/plan-moved.txt")
	if moved.ID != plan.ID || moved.Version != plan.Version+1 || moved.Key != plan.Key {
		t.Errorf("the moved file = %+v", moved)
	}
	if _, err := e.store.MoveEntry(e.ctx, files.MoveRequest{Kind: files.EntryFile, ID: plan.ID, DirectoryID: y2026.ID, Version: plan.Version}, files.Scope{}); !errors.Is(err, query.ErrVersionMismatch) {
		t.Errorf("MoveEntry at the version before the move = %v, want ErrVersionMismatch", err)
	}
	if mv, err := e.store.MoveEntry(e.ctx, files.MoveRequest{Kind: files.EntryFile, ID: plan.ID, DirectoryID: y2026.ID, Version: moved.Version}, files.Scope{}); err != nil || mv.To != "/docs/2026/plan-moved.txt" {
		t.Errorf("MoveEntry at the version read = %+v, %v", mv, err)
	}
	mv, err = e.store.MoveEntry(e.ctx, files.MoveRequest{Kind: files.EntryDirectory, ID: y2026.ID, DirectoryID: docs.ID, Name: "archive"}, files.Scope{})
	if err != nil || mv.From != "/docs/2026" || mv.To != "/docs/archive" {
		t.Fatalf("MoveEntry renaming a directory = %+v, %v", mv, err)
	}
	e.stat(t, "/docs/archive/plan-moved.txt")
	sub := e.mkdir(t, "/docs/archive/sub", "")
	if _, err := e.store.MoveEntry(e.ctx, files.MoveRequest{Kind: files.EntryDirectory, ID: y2026.ID, DirectoryID: sub.ID}, files.Scope{}); !errors.Is(err, blobfs.ErrCycle) {
		t.Errorf("MoveEntry of a directory under its descendant = %v, want ErrCycle", err)
	}
	// A top-level directory under its own descendant meets the scope rule
	// first, by id as by path.
	if _, err := e.store.MoveEntry(e.ctx, files.MoveRequest{Kind: files.EntryDirectory, ID: docs.ID, DirectoryID: sub.ID}, files.Scope{}); !errors.Is(err, files.ErrMoveAcrossScopes) {
		t.Errorf("MoveEntry of a top-level directory under its descendant = %v, want ErrMoveAcrossScopes", err)
	}
	if _, err := e.store.Move(e.ctx, "/docs", "/docs/archive/sub"); !errors.Is(err, files.ErrMoveAcrossScopes) {
		t.Errorf("Move of a top-level directory under its descendant = %v, want ErrMoveAcrossScopes", err)
	}
	if _, err := e.store.MoveEntry(e.ctx, files.MoveRequest{Kind: files.EntryFile, ID: moved.ID, DirectoryID: beta.ID}, files.Scope{}); !errors.Is(err, files.ErrMoveAcrossScopes) {
		t.Errorf("MoveEntry across two top-level directories = %v, want ErrMoveAcrossScopes", err)
	}
	if _, err := e.store.Move(e.ctx, "/docs/archive/plan-moved.txt", "/beta/plan-moved.txt"); !errors.Is(err, files.ErrMoveAcrossScopes) {
		t.Errorf("Move across two top-level directories = %v, want ErrMoveAcrossScopes", err)
	}
	if mv, err := e.store.MoveEntry(e.ctx, files.MoveRequest{Kind: files.EntryDirectory, ID: beta.ID, DirectoryID: blobfs.RootID, Name: "gamma"}, files.Scope{}); err != nil || mv.To != "/gamma" {
		t.Errorf("MoveEntry renaming a top-level directory = %+v, %v", mv, err)
	}

	// Remove by id: (id, version) from a listing acts without a read; a
	// stale version is refused with the row untouched; without a version
	// the delete runs; a deleting row resumes at any version.
	c := e.list(t, "/docs/archive", l)
	if len(c.Files.Rows) != 2 {
		t.Fatalf("/docs/archive lists %v", fileNames(c.Files.Rows))
	}
	row := c.Files.Rows[0]
	if f, err := e.store.RemoveFile(e.ctx, row.ID, row.Version, "", files.Scope{}); err != nil || f.ID != row.ID {
		t.Fatalf("RemoveFile at the listed version = %+v, %v", f, err)
	}
	if _, err := e.store.StatFile(e.ctx, row.ID, files.Scope{}); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("the removed file still stats: %v", err)
	}
	row = c.Files.Rows[1]
	if _, err := e.store.MoveEntry(e.ctx, files.MoveRequest{Kind: files.EntryFile, ID: row.ID, DirectoryID: row.DirectoryID, Name: "bumped.txt"}, files.Scope{}); err != nil {
		t.Fatalf("the rename that bumps the version: %v", err)
	}
	if _, err := e.store.RemoveFile(e.ctx, row.ID, row.Version, "", files.Scope{}); !errors.Is(err, query.ErrVersionMismatch) {
		t.Errorf("RemoveFile at the version before the rename = %v, want ErrVersionMismatch", err)
	}
	if f := e.stat(t, "/docs/archive/bumped.txt"); f.Status != blobfs.StatusAvailable {
		t.Errorf("the refused rm changed the row: %+v", f)
	}
	var stop *files.StopError
	if _, err := e.store.RemoveFile(e.ctx, row.ID, 0, files.StepBegin, files.Scope{}); !errors.As(err, &stop) || stop.File.Status != blobfs.StatusDeleting {
		t.Fatalf("RemoveFile --fail-after begin = %v", err)
	}
	if f, err := e.store.RemoveFile(e.ctx, row.ID, row.Version, "", files.Scope{}); err != nil || f.Status != blobfs.StatusDeleting {
		t.Errorf("RemoveFile of the deleting row at an earlier version = %+v, %v; want the delete resumed", f, err)
	}
	if _, err := e.store.Stat(e.ctx, "/docs/archive/bumped.txt"); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("after the resumed rm the file still stats: %v", err)
	}
	if _, err := e.store.RemoveFile(e.ctx, blobfs.NewID(), 0, "", files.Scope{}); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("RemoveFile of a missing id = %v, want ErrNotFound", err)
	}
}

// TestScopeByID proves the ownership check by id on the engine: a
// directory within an owned scope is accepted at any depth, one outside
// it is refused, a scope directory the unit does not own is refused
// whether another unit owns it or none does, the root cannot be a scope,
// and the supplied scope id never stands in for the owner row. The
// id-keyed methods each run the check, and the root as a unit lists the
// unit's own top-level directories as ls / --unit does.
func TestScopeByID(t *testing.T) {
	e := open(t)
	unit, other := blobfs.NewID(), blobfs.NewID()
	docs := e.mkdir(t, "/docs", unit)
	deep := e.mkdir(t, "/docs/2026", "")
	beta := e.mkdir(t, "/beta", other)
	shared := e.mkdir(t, "/shared", "")
	e.mkdir(t, "/alpha", unit)
	planID := e.put(t, "/docs/2026/plan.txt", "text/plain", "plan").File.ID
	sharedID := e.put(t, "/shared/s.txt", "text/plain", "s").File.ID
	mine := files.Scope{Unit: unit, DirectoryID: docs.ID}
	l := files.Listing{Page: 1, Size: 10}

	for _, id := range []string{docs.ID, deep.ID} {
		if err := e.store.InScope(e.ctx, id, mine); err != nil {
			t.Errorf("InScope(%s) under the owned scope = %v", id, err)
		}
	}
	for label, tc := range map[string]struct {
		id    string
		scope files.Scope
	}{
		"a directory outside the scope":         {shared.ID, mine},
		"another unit's directory":              {beta.ID, mine},
		"the scope root of another unit":        {beta.ID, files.Scope{Unit: unit, DirectoryID: beta.ID}},
		"a scope root no unit owns":             {shared.ID, files.Scope{Unit: unit, DirectoryID: shared.ID}},
		"the root as the scope":                 {deep.ID, files.Scope{Unit: unit, DirectoryID: blobfs.RootID}},
		"a scope root below the top level":      {deep.ID, files.Scope{Unit: unit, DirectoryID: deep.ID}},
		"the other unit over the owned scope":   {deep.ID, files.Scope{Unit: other, DirectoryID: docs.ID}},
		"a scope root that does not exist":      {deep.ID, files.Scope{Unit: unit, DirectoryID: blobfs.NewID()}},
		"a directory that does not exist":       {blobfs.NewID(), mine},
		"a scope that names a unit alone":       {deep.ID, files.Scope{Unit: unit}},
		"a scope that names a directory alone":  {deep.ID, files.Scope{DirectoryID: docs.ID}},
		"a unit that owns nothing named as own": {deep.ID, files.Scope{Unit: blobfs.NewID(), DirectoryID: docs.ID}},
	} {
		if err := e.store.InScope(e.ctx, tc.id, tc.scope); !errors.Is(err, files.ErrNotOwned) {
			t.Errorf("%s: InScope = %v, want ErrNotOwned", label, err)
		}
	}

	// Each id-keyed method runs the check.
	byID, err := e.store.ListDirectory(e.ctx, deep.ID, l, mine)
	if err != nil || byID.Files.Total != 1 {
		t.Errorf("ListDirectory in scope = %+v, %v", byID, err)
	}
	if _, err := e.store.ListDirectory(e.ctx, shared.ID, l, mine); !errors.Is(err, files.ErrNotOwned) {
		t.Errorf("ListDirectory outside the scope = %v, want ErrNotOwned", err)
	}
	if _, err := e.store.StatFile(e.ctx, planID, mine); err != nil {
		t.Errorf("StatFile in scope = %v", err)
	}
	if _, err := e.store.StatFile(e.ctx, sharedID, mine); !errors.Is(err, files.ErrNotOwned) {
		t.Errorf("StatFile outside the scope = %v, want ErrNotOwned", err)
	}
	if _, _, err := e.store.OpenFile(e.ctx, sharedID, mine); !errors.Is(err, files.ErrNotOwned) {
		t.Errorf("OpenFile outside the scope = %v, want ErrNotOwned", err)
	}
	if _, err := e.store.PutFile(e.ctx, shared.ID, "x.txt", files.PutRequest{Body: strings.NewReader("x")}, mine); !errors.Is(err, files.ErrNotOwned) {
		t.Errorf("PutFile outside the scope = %v, want ErrNotOwned", err)
	}
	if _, err := e.store.Stat(e.ctx, "/shared/x.txt"); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("the refused put left a row: %v", err)
	}
	res, err := e.store.PutFile(e.ctx, deep.ID, "x.txt", files.PutRequest{Body: strings.NewReader("x")}, mine)
	if err != nil {
		t.Fatalf("PutFile in scope = %v", err)
	}
	if _, err := e.store.MoveEntry(e.ctx, files.MoveRequest{Kind: files.EntryFile, ID: res.File.ID, DirectoryID: shared.ID}, mine); !errors.Is(err, files.ErrNotOwned) {
		t.Errorf("MoveEntry to a destination outside the scope = %v, want ErrNotOwned", err)
	}
	if _, err := e.store.MoveEntry(e.ctx, files.MoveRequest{Kind: files.EntryFile, ID: sharedID, DirectoryID: deep.ID}, mine); !errors.Is(err, files.ErrNotOwned) {
		t.Errorf("MoveEntry of a source outside the scope = %v, want ErrNotOwned", err)
	}
	if mv, err := e.store.MoveEntry(e.ctx, files.MoveRequest{Kind: files.EntryFile, ID: res.File.ID, DirectoryID: docs.ID}, mine); err != nil || mv.To != "/docs/x.txt" {
		t.Errorf("MoveEntry within the scope = %+v, %v", mv, err)
	}
	if _, err := e.store.MoveEntry(e.ctx, files.MoveRequest{Kind: files.EntryDirectory, ID: docs.ID, DirectoryID: blobfs.RootID, Name: "renamed"}, mine); !errors.Is(err, files.ErrNotOwned) {
		t.Errorf("a scoped rename of the scope root = %v, want ErrNotOwned: its parent is the root", err)
	}
	if _, err := e.store.RemoveFile(e.ctx, sharedID, 0, "", mine); !errors.Is(err, files.ErrNotOwned) {
		t.Errorf("RemoveFile outside the scope = %v, want ErrNotOwned", err)
	}
	e.stat(t, "/shared/s.txt")
	if _, err := e.store.RemoveFile(e.ctx, res.File.ID, 0, "", mine); err != nil {
		t.Errorf("RemoveFile in scope = %v", err)
	}

	// The root as a unit, by id, is ls / --unit.
	c, err := e.store.ListDirectory(e.ctx, blobfs.RootID, l, files.Scope{Unit: unit})
	if err != nil {
		t.Fatalf("ListDirectory of the root as the unit: %v", err)
	}
	if !slices.Equal(names(c.Directories.Rows), []string{"alpha", "docs"}) || c.Files.Total != 0 || c.Path != "/" {
		t.Errorf("the root as the unit = %v, %d files, path %q", names(c.Directories.Rows), c.Files.Total, c.Path)
	}
	sameContents(t, "/ as the unit", e.list(t, "/", files.Listing{Page: 1, Size: 10, Unit: unit}), c)
}
