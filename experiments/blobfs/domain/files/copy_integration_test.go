//go:build integration

package files_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/standards-lab/org/experiments/blobfs/domain/files"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// cp runs Copy and fails the test on any error.
func (e env) cp(t *testing.T, src, dst string) files.CopyResult {
	t.Helper()
	res, err := e.store.Copy(e.ctx, files.CopyRequest{Source: src, Destination: dst})
	if err != nil {
		t.Fatalf("Copy(%s, %s): %v", src, dst, err)
	}
	return res
}

// sameRow reports whether two reads of a file row agree on everything a
// copy must leave alone: the id, the key, the version, the size, the
// etag, the content type, and the timestamps.
func sameRow(a, b blobfs.File) bool {
	return a.ID == b.ID && a.Key == b.Key && a.Version == b.Version && a.ContentType == b.ContentType &&
		a.Size != nil && b.Size != nil && *a.Size == *b.Size && a.ETag != nil && b.ETag != nil && *a.ETag == *b.ETag &&
		a.UpdatedAt.Equal(b.UpdatedAt) && a.CreatedAt.Equal(b.CreatedAt)
}

// TestCopy is cp through the store on Postgres and Azurite, per variant:
// a file into an existing directory keeps its name, and the copy is a new
// row with its own key, the source's content type and size, the size and
// etag the service reports for the new blob, and the same bytes; a copy
// to a new name, across two top-level directories, into the root, and
// into an owned directory, with no owner row and no bookmark following;
// a file wins over a directory of the same name as the source; the
// source is untouched throughout; and the refusals: the taken name, the
// source itself by its path and by its directory, a directory as the
// source, a missing source and a missing parent, the root, and a pending
// and a deleting source.
func TestCopy(t *testing.T) {
	perVariant(t, func(t *testing.T, e env, _ variant) {
		unit := blobfs.NewID()
		st := e.storage(t)
		e.mkdir(t, "/a", "")
		e.mkdir(t, "/a/x", "")
		y := e.mkdir(t, "/a/y", "")
		e.mkdir(t, "/b", "")
		e.mkdir(t, "/owned", unit)
		e.put(t, "/a/x/f.txt", "text/plain", "content")
		e.bookmark(t, "/a/x/f.txt", unit, true)
		src := e.stat(t, "/a/x/f.txt")

		// Into an existing directory, under the source's name.
		res := e.cp(t, "/a/x/f.txt", "/a/y")
		if res.From != "/a/x/f.txt" || res.To != "/a/y/f.txt" || res.Resumed || res.File.Status != blobfs.StatusAvailable || res.File.DirectoryID != y.ID || res.File.Version != 2 {
			t.Errorf("Copy into a directory = %+v", res)
		}
		copied := e.stat(t, "/a/y/f.txt")
		if copied.ID == src.ID || copied.Key == src.Key || copied.Name != "f.txt" || copied.ContentType != src.ContentType || copied.Size == nil || *copied.Size != *src.Size {
			t.Errorf("the copy's row = %+v; the source's = %+v", copied, src)
		}
		obj, err := st.Stat(e.ctx, copied.Key)
		if err != nil {
			t.Fatalf("the service's Stat of the copy: %v", err)
		}
		if *copied.Size != obj.Size || *copied.ETag != obj.ETag || copied.ContentType != obj.ContentType {
			t.Errorf("the copy's row says size %d, etag %s, type %s; the service says %+v", *copied.Size, *copied.ETag, copied.ContentType, obj)
		}
		if got := e.read(t, "/a/y/f.txt"); got != "content" {
			t.Errorf("cat of the copy = %q", got)
		}
		if after := e.stat(t, "/a/x/f.txt"); !sameRow(after, src) || e.read(t, "/a/x/f.txt") != "content" {
			t.Errorf("the source after the copy = %+v, want %+v untouched", after, src)
		}

		// To a new name, across two top-level directories, into the root, and
		// into an owned directory; neither the owner row nor the bookmark
		// follows.
		if res := e.cp(t, "/a/x/f.txt", "/a/x/g.txt"); res.To != "/a/x/g.txt" || e.read(t, "/a/x/g.txt") != "content" {
			t.Errorf("Copy to a new name = %+v", res)
		}
		if res := e.cp(t, "/a/x/f.txt", "/b/f.txt"); res.To != "/b/f.txt" || e.read(t, "/b/f.txt") != "content" {
			t.Errorf("Copy across top-level directories = %+v", res)
		}
		if res := e.cp(t, "/a/x/f.txt", "/"); res.To != "/f.txt" || e.stat(t, "/f.txt").DirectoryID != blobfs.RootID {
			t.Errorf("Copy into the root = %+v", res)
		}
		if res := e.cp(t, "/a/x/f.txt", "/owned"); res.To != "/owned/f.txt" {
			t.Errorf("Copy into an owned directory = %+v", res)
		}
		if n := e.count(t, "SELECT COUNT(*) FROM directory_owner"); n != 1 {
			t.Errorf("%d owner rows after the copies, want the one", n)
		}
		if n := e.count(t, "SELECT COUNT(*) FROM bookmark"); n != 1 {
			t.Errorf("%d bookmark rows after the copies, want the one", n)
		}
		if p := paths(e.bookmarksOf(t, unit, files.Listing{Page: 1, Size: 10}).Rows); strings.Join(p, " ") != "/a/x/f.txt" {
			t.Errorf("bookmark ls after the copies = %v, want the source alone", p)
		}
		if n := e.count(t, "SELECT COUNT(*) FROM blobfs_file WHERE content_type = 'text/plain' AND size = 7 AND status = 'available'"); n != 6 {
			t.Errorf("%d available rows carry the source's type and size, want the source and five copies", n)
		}

		// The refusals: the taken name with the status named, the source
		// itself both ways, a directory, the missing source and parent, and
		// the root; each leaves the row count as it was.
		for _, tc := range []struct {
			label    string
			src, dst string
			want     error
		}{
			{"a taken name", "/a/x/f.txt", "/a/y", blobfs.ErrNameTaken},
			{"the source's own path", "/a/x/f.txt", "/a/x/f.txt", blobfs.ErrNameTaken},
			{"the source's own directory", "/a/x/f.txt", "/a/x", blobfs.ErrNameTaken},
			{"a directory as the source", "/a/x", "/a/y", files.ErrNotAFile},
			{"a missing source", "/a/missing.txt", "/a/y", blobfs.ErrNotFound},
			{"a missing parent", "/a/x/f.txt", "/a/nope/f.txt", blobfs.ErrNotFound},
			{"the root as the source", "/", "/a", blobfs.ErrRootDirectory},
			{"a relative destination", "/a/x/f.txt", "b", blobfs.ErrInvalidPath},
		} {
			if _, err := e.store.Copy(e.ctx, files.CopyRequest{Source: tc.src, Destination: tc.dst}); !errors.Is(err, tc.want) {
				t.Errorf("%s: Copy(%s, %s) = %v, want %v", tc.label, tc.src, tc.dst, err, tc.want)
			}
		}
		if _, err := e.store.Copy(e.ctx, files.CopyRequest{Source: "/a/x/f.txt", Destination: "/a/y"}); err == nil || !strings.Contains(err.Error(), `a file named "f.txt" is available`) {
			t.Errorf("the taken name's message = %v", err)
		}
		if n := e.count(t, "SELECT COUNT(*) FROM blobfs_file"); n != 6 {
			t.Errorf("%d file rows after the refusals, want 6", n)
		}

		// A pending and a deleting source, each refused with the status
		// named and nothing inserted; the deleting one is then removed.
		var stop *files.StopError
		if _, err := e.store.Put(e.ctx, files.PutRequest{Path: "/a/pend.txt", Body: strings.NewReader("never"), StopAfter: files.StepInsert}); !errors.As(err, &stop) {
			t.Fatalf("Put --fail-after insert = %v", err)
		}
		if _, err := e.store.Copy(e.ctx, files.CopyRequest{Source: "/a/pend.txt", Destination: "/a/y"}); !errors.Is(err, files.ErrNotAvailable) || !strings.Contains(err.Error(), "the file is pending") {
			t.Errorf("Copy of a pending source = %v, want ErrNotAvailable naming pending", err)
		}
		e.put(t, "/a/del.txt", "text/plain", "d")
		if _, err := e.store.Remove(e.ctx, "/a/del.txt", files.StepBegin); !errors.As(err, &stop) {
			t.Fatalf("Remove --fail-after begin = %v", err)
		}
		if _, err := e.store.Copy(e.ctx, files.CopyRequest{Source: "/a/del.txt", Destination: "/a/y"}); !errors.Is(err, files.ErrNotAvailable) || !strings.Contains(err.Error(), "the file is deleting") {
			t.Errorf("Copy of a deleting source = %v, want ErrNotAvailable naming deleting", err)
		}
		if _, err := e.store.Remove(e.ctx, "/a/del.txt", ""); err != nil {
			t.Fatalf("the finishing Remove: %v", err)
		}
		if _, err := e.store.Stat(e.ctx, "/a/y/pend.txt"); !errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("the refused copy of the pending source left a row: %v", err)
		}

		// A file and a directory may share a name; cp copies the file.
		e.mkdir(t, "/a/x/same", "")
		e.put(t, "/a/x/same", "text/plain", "the file")
		if res := e.cp(t, "/a/x/same", "/a/y"); res.To != "/a/y/same" || e.read(t, "/a/y/same") != "the file" {
			t.Errorf("Copy of a file that shares its name with a directory = %+v", res)
		}
		if n := e.count(t, "SELECT COUNT(*) FROM blobfs_file WHERE status <> 'available'"); n != 1 {
			t.Errorf("%d rows are not available, want the abandoned put alone", n)
		}
	})
}

// TestCopyStopsAndResumes is the stop between the steps of a copy on the
// real services, per variant: a copy that stops after the insert leaves a
// pending row that stat shows, with no object under its key and cat
// refusing it, and a cp of the same paths resumes the row, streams the
// source, and completes it; a copy that stops after the write leaves the
// row pending with the object stored, and the retry completes it; and a
// source whose object the store no longer holds leaves the copy's row
// pending with the cause reachable, which rm then removes.
func TestCopyStopsAndResumes(t *testing.T) {
	perVariant(t, func(t *testing.T, e env, _ variant) {
		e.mkdir(t, "/docs", "")
		st := e.storage(t)
		e.put(t, "/docs/a.txt", "text/plain", "first")

		_, err := e.store.Copy(e.ctx, files.CopyRequest{Source: "/docs/a.txt", Destination: "/docs/b.txt", StopAfter: files.StepInsert})
		var stop *files.StopError
		if !errors.As(err, &stop) || stop.Command != "cp" || stop.Step != files.StepInsert || !strings.Contains(err.Error(), "rerun cp") {
			t.Fatalf("Copy --fail-after insert = %v", err)
		}
		pending := stop.File
		if row := e.stat(t, "/docs/b.txt"); row.Status != blobfs.StatusPending || row.ID != pending.ID || row.ContentType != "text/plain" {
			t.Errorf("Stat after the stop = %+v; want the pending row with the source's type", row)
		}
		if _, err := st.Stat(e.ctx, pending.Key); !errors.Is(err, files.ErrObjectMissing) {
			t.Errorf("the store holds %v under the pending key; want nothing", err)
		}
		if _, _, err := e.store.Open(e.ctx, "/docs/b.txt"); !errors.Is(err, files.ErrNotAvailable) {
			t.Errorf("Open of the pending copy = %v, want ErrNotAvailable", err)
		}
		res := e.cp(t, "/docs/a.txt", "/docs/b.txt")
		if !res.Resumed || res.File.ID != pending.ID || res.File.Status != blobfs.StatusAvailable || res.File.Version != 2 {
			t.Errorf("the retry returned %+v; want the pending row resumed and completed", res)
		}
		if got := e.read(t, "/docs/b.txt"); got != "first" {
			t.Errorf("cat after the retry = %q", got)
		}

		_, err = e.store.Copy(e.ctx, files.CopyRequest{Source: "/docs/a.txt", Destination: "/docs/c.txt", StopAfter: files.StepWrite})
		if !errors.As(err, &stop) || stop.Step != files.StepWrite {
			t.Fatalf("Copy --fail-after write = %v", err)
		}
		if obj, err := st.Stat(e.ctx, stop.File.Key); err != nil || obj.Size != 5 {
			t.Errorf("after the stop the store holds %+v, %v; want the 5-byte object", obj, err)
		}
		if row := e.stat(t, "/docs/c.txt"); row.Status != blobfs.StatusPending || row.Size != nil {
			t.Errorf("after the stop the row is %+v, want pending with no size", row)
		}
		res = e.cp(t, "/docs/a.txt", "/docs/c.txt")
		if !res.Resumed || res.File.Size == nil || *res.File.Size != 5 || res.File.Status != blobfs.StatusAvailable {
			t.Errorf("the retry returned %+v", res)
		}
		if got := e.read(t, "/docs/c.txt"); got != "first" {
			t.Errorf("cat after the retry = %q", got)
		}
		if n := e.count(t, "SELECT COUNT(*) FROM blobfs_file WHERE status = 'pending'"); n != 0 {
			t.Errorf("%d rows are still pending", n)
		}

		// The source's object is gone from under an available row.
		gone := e.put(t, "/docs/gone.txt", "text/plain", "gone").File
		if err := st.Delete(e.ctx, gone.Key); err != nil {
			t.Fatalf("delete the source's object: %v", err)
		}
		_, err = e.store.Copy(e.ctx, files.CopyRequest{Source: "/docs/gone.txt", Destination: "/docs/g2.txt"})
		if !errors.Is(err, files.ErrObjectMissing) || !strings.Contains(err.Error(), "stays pending") {
			t.Errorf("Copy of a source with no object = %v, want ErrObjectMissing and the pending row named", err)
		}
		if row := e.stat(t, "/docs/g2.txt"); row.Status != blobfs.StatusPending {
			t.Errorf("the copy's row after the missing object = %+v, want pending", row)
		}
		if _, err := e.store.Remove(e.ctx, "/docs/g2.txt", ""); err != nil {
			t.Errorf("rm of the abandoned copy = %v", err)
		}
	})
}

// TestCopyFileByID proves the copy by id against the path form on the
// engine: a copy into a directory by id keeps the source's name and lands
// where a copy by path would, a name given is used, the result carries no
// paths, a taken name and a missing source or directory are refused, the
// scope check covers the source's directory and the destination, and a
// stop by id resumes on the retry.
func TestCopyFileByID(t *testing.T) {
	e := open(t)
	unit, other := blobfs.NewID(), blobfs.NewID()
	docs := e.mkdir(t, "/docs", unit)
	y2026 := e.mkdir(t, "/docs/2026", "")
	beta := e.mkdir(t, "/beta", other)
	plan := e.put(t, "/docs/plan.txt", "text/plain", "plan").File
	theirs := e.put(t, "/beta/b.txt", "text/plain", "b").File
	mine := files.Scope{Unit: unit, DirectoryID: docs.ID}

	res, err := e.store.CopyFile(e.ctx, plan.ID, y2026.ID, "", "", files.Scope{})
	if err != nil || res.File.DirectoryID != y2026.ID || res.File.Name != "plan.txt" || res.File.Status != blobfs.StatusAvailable || res.From != "" || res.To != "" {
		t.Fatalf("CopyFile = %+v, %v", res, err)
	}
	if f := e.stat(t, "/docs/2026/plan.txt"); f.ID != res.File.ID || e.read(t, "/docs/2026/plan.txt") != "plan" {
		t.Errorf("the file copied by id is not at its path: %+v", f)
	}
	if res, err := e.store.CopyFile(e.ctx, plan.ID, docs.ID, "copy.txt", "", files.Scope{}); err != nil || res.File.Name != "copy.txt" || e.read(t, "/docs/copy.txt") != "plan" {
		t.Errorf("CopyFile as a name = %+v, %v", res, err)
	}
	if _, err := e.store.CopyFile(e.ctx, plan.ID, y2026.ID, "", "", files.Scope{}); !errors.Is(err, blobfs.ErrNameTaken) {
		t.Errorf("CopyFile over a taken name = %v, want ErrNameTaken", err)
	}
	if _, err := e.store.CopyFile(e.ctx, plan.ID, docs.ID, "", "", files.Scope{}); !errors.Is(err, blobfs.ErrNameTaken) {
		t.Errorf("CopyFile into the source's own directory = %v, want ErrNameTaken", err)
	}
	if _, err := e.store.CopyFile(e.ctx, blobfs.NewID(), y2026.ID, "", "", files.Scope{}); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("CopyFile of a missing source = %v, want ErrNotFound", err)
	}
	if _, err := e.store.CopyFile(e.ctx, plan.ID, blobfs.NewID(), "x.txt", "", files.Scope{}); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("CopyFile into a missing directory = %v, want ErrNotFound", err)
	}

	// The scope: both directories must lie within it.
	if res, err := e.store.CopyFile(e.ctx, plan.ID, y2026.ID, "scoped.txt", "", mine); err != nil || res.File.Name != "scoped.txt" {
		t.Errorf("CopyFile in scope = %+v, %v", res, err)
	}
	if _, err := e.store.CopyFile(e.ctx, plan.ID, beta.ID, "", "", mine); !errors.Is(err, files.ErrNotOwned) {
		t.Errorf("CopyFile to a destination outside the scope = %v, want ErrNotOwned", err)
	}
	if _, err := e.store.CopyFile(e.ctx, theirs.ID, y2026.ID, "", "", mine); !errors.Is(err, files.ErrNotOwned) {
		t.Errorf("CopyFile of a source outside the scope = %v, want ErrNotOwned", err)
	}
	if _, err := e.store.Stat(e.ctx, "/beta/plan.txt"); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("the refused copy left a row: %v", err)
	}

	// A stop by id, then the resume.
	var stop *files.StopError
	if _, err := e.store.CopyFile(e.ctx, plan.ID, y2026.ID, "stopped.txt", files.StepInsert, files.Scope{}); !errors.As(err, &stop) || stop.Path != "file "+plan.ID+" into directory "+y2026.ID {
		t.Fatalf("CopyFile --fail-after insert = %v", err)
	}
	if res, err := e.store.CopyFile(e.ctx, plan.ID, y2026.ID, "stopped.txt", "", files.Scope{}); err != nil || !res.Resumed || res.File.ID != stop.File.ID {
		t.Errorf("the retry by id = %+v, %v; want the pending row resumed", res, err)
	}
	if got := e.read(t, "/docs/2026/stopped.txt"); got != "plan" {
		t.Errorf("cat of the resumed copy = %q", got)
	}
}
