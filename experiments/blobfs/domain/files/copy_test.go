package files_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/go-storage/storagetest"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/domain/files"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// sourceObject seeds the fake with the source's object under key, the
// bytes a copy streams, as text/plain.
func sourceObject(t *testing.T, fake *storagetest.Fake, key, content string) {
	t.Helper()
	if _, err := fake.Put(context.Background(), key, strings.NewReader(content), storage.PutOptions{ContentType: "text/plain"}); err != nil {
		t.Fatalf("seed the fake with %s: %v", key, err)
	}
}

// objectBytes reads the object the fake holds under key.
func objectBytes(t *testing.T, fake *storagetest.Fake, key string) string {
	t.Helper()
	blob, err := fake.Get(context.Background(), key, storage.GetOptions{})
	if err != nil {
		t.Fatalf("Get(%s): %v", key, err)
	}
	defer func() { _ = blob.Body.Close() }()
	got, err := io.ReadAll(blob.Body)
	if err != nil {
		t.Fatalf("read %s: %v", key, err)
	}
	return string(got)
}

// copyToNewName scripts the first transaction of cp /a.txt /b.txt up to
// the begin-or-resume step's lookup: the source's parent and row, the
// destination tried as a directory and then its parent.
func copyToNewName(source sqltest.Response) []sqltest.Response {
	return []sqltest.Response{root(), source, root(), noDirectory(), root()}
}

// TestCopyIsPutOverTheSourcesObject proves the transaction boundaries of
// a full copy to a new name: one transaction holds the source's
// resolution, the destination's, the lookup by name, the pending insert
// with the source's content type, and its read-back, and commits before
// the store is touched; the source's object is then read and written
// under the new row's key with the source's type and size, outside any
// transaction; and the completion on the pool binds what the store
// reported for the new object. The result names both paths.
func TestCopyIsPutOverTheSourcesObject(t *testing.T) {
	ctx := context.Background()
	opens := 0
	s, rec, fake := writeStore(t, &opens, append(copyToNewName(file("A", "a.txt", blobfs.StatusAvailable, 2)),
		noFile(), affected(), file("F", "b.txt", blobfs.StatusPending, 1),
		affected(), file("F", "b.txt", blobfs.StatusAvailable, 2),
	)...)
	sourceObject(t, fake, "A/a.txt", "hello")
	res, err := s.Copy(ctx, files.CopyRequest{Source: "/a.txt", Destination: "/b.txt"})
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if res.From != "/a.txt" || res.To != "/b.txt" || res.Resumed || res.File.ID != "F" || res.File.Status != blobfs.StatusAvailable || res.File.Version != 2 {
		t.Errorf("Copy = %+v", res)
	}
	if got := ops(rec); got != "begin query query query query query query exec query commit exec query" {
		t.Errorf("ops = %q", got)
	}
	calls := rec.Calls()
	if insert := calls[7]; !strings.HasPrefix(insert.SQL, "INSERT INTO blobfs_file") || insert.Args[1] != blobfs.RootID || insert.Args[2] != "b.txt" || insert.Args[4] != "text/plain" {
		t.Errorf("the insert ran %s with %v, want the root, the new name, and the source's content type", insert.SQL, insert.Args)
	}
	if got := objectBytes(t, fake, "F/b.txt"); got != "hello" {
		t.Errorf("the copy's object holds %q, want the source's bytes", got)
	}
	if opts, n := fake.LastPut(); opts.ContentType != "text/plain" || opts.Size != 5 || n != 5 {
		t.Errorf("the store received %+v after %d bytes, want the source's type and size", opts, n)
	}
	stored, err := fake.Stat(ctx, "F/b.txt")
	if err != nil {
		t.Fatalf("Stat of the copy's object: %v", err)
	}
	if complete := calls[10].Args; !strings.HasPrefix(calls[10].SQL, "UPDATE blobfs_file") || complete[0] != int64(5) || complete[1] != "text/plain" || complete[2] != stored.ETag || complete[3] != "F" || complete[4] != int64(1) {
		t.Errorf("the completion bound %v, want the store's size, type, and etag %q, the id, and version 1", complete, stored.ETag)
	}
	if got := objectBytes(t, fake, "A/a.txt"); got != "hello" || opens != 1 {
		t.Errorf("after the copy the source holds %q and the store was opened %d times", got, opens)
	}
}

// TestCopyIntoADirectoryKeepsTheName proves a destination that resolves
// as a directory receives the copy under the source's name, across two
// top-level directories, with no scope rule in the way: the insert binds
// the destination directory and the source's name.
func TestCopyIntoADirectoryKeepsTheName(t *testing.T) {
	opens := 0
	s, rec, fake := writeStore(t, &opens,
		root(), directory("A", blobfs.RootID, "a"), fileIn("X", "A", "x.txt", blobfs.StatusAvailable, 2),
		root(), directory("B", blobfs.RootID, "b"),
		noFile(), affected(), fileIn("F", "B", "x.txt", blobfs.StatusPending, 1),
		affected(), fileIn("F", "B", "x.txt", blobfs.StatusAvailable, 2),
	)
	sourceObject(t, fake, "X/x.txt", "cross")
	res, err := s.Copy(context.Background(), files.CopyRequest{Source: "/a/x.txt", Destination: "/b"})
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if res.From != "/a/x.txt" || res.To != "/b/x.txt" || res.File.DirectoryID != "B" {
		t.Errorf("Copy = %+v", res)
	}
	if got := ops(rec); got != "begin query query query query query query exec query commit exec query" {
		t.Errorf("ops = %q", got)
	}
	if insert := rec.Calls()[7]; insert.Args[1] != "B" || insert.Args[2] != "x.txt" {
		t.Errorf("the insert bound %v, want the destination directory and the source's name", insert.Args)
	}
	if got := objectBytes(t, fake, "F/x.txt"); got != "cross" {
		t.Errorf("the copy's object holds %q", got)
	}
}

// TestCopyRefusesTheSource proves the source's refusals, each with
// nothing inserted and nothing stored: a path with a directory and no
// file is ErrNotAFile, found after the file lookup fails; a path with
// neither is ErrNotFound; a pending or a deleting file is ErrNotAvailable
// naming the status, before the destination is read; the root, a
// relative source, and a relative destination fail before any I/O; and a
// store without an object store is ErrNoStorage before any SQL.
func TestCopyRefusesTheSource(t *testing.T) {
	ctx := context.Background()
	opens := 0
	s, rec, fake := writeStore(t, &opens, root(), noFile(), root(), directory("D", blobfs.RootID, "d"))
	_, err := s.Copy(ctx, files.CopyRequest{Source: "/d", Destination: "/x"})
	if !errors.Is(err, files.ErrNotAFile) || !strings.Contains(err.Error(), "cp copies files") {
		t.Errorf("Copy of a directory = %v, want ErrNotAFile", err)
	}
	if got := ops(rec); got != "begin query query query query rollback" || fake.Puts() != 0 {
		t.Errorf("ops = %q and %d puts after the directory refusal", got, fake.Puts())
	}

	s, rec, _ = writeStore(t, &opens, root(), noFile(), root(), noDirectory())
	if _, err := s.Copy(ctx, files.CopyRequest{Source: "/missing.txt", Destination: "/x"}); !errors.Is(err, blobfs.ErrNotFound) || errors.Is(err, files.ErrNotAFile) {
		t.Errorf("Copy of a missing source = %v, want ErrNotFound", err)
	}
	if got := ops(rec); got != "begin query query query query rollback" {
		t.Errorf("ops after the missing source = %q", got)
	}

	for _, status := range []blobfs.Status{blobfs.StatusPending, blobfs.StatusDeleting} {
		s, rec, fake = writeStore(t, &opens, root(), file("P", "a.txt", status, 1))
		_, err := s.Copy(ctx, files.CopyRequest{Source: "/a.txt", Destination: "/b.txt"})
		if !errors.Is(err, files.ErrNotAvailable) || !strings.Contains(err.Error(), "the file is "+string(status)) {
			t.Errorf("Copy of a %s file = %v, want ErrNotAvailable naming the status", status, err)
		}
		if got := ops(rec); got != "begin query query rollback" || fake.Puts() != 0 {
			t.Errorf("ops = %q and %d puts after the %s refusal; want the destination unread", got, fake.Puts(), status)
		}
	}

	opens = 0
	s, rec, _ = writeStore(t, &opens)
	for _, tc := range []struct {
		src, dst string
		want     error
	}{
		{"/", "/x", blobfs.ErrRootDirectory},
		{"a.txt", "/x", blobfs.ErrInvalidPath},
		{"/a/", "/x", blobfs.ErrInvalidPath},
		{"/a.txt", "x", blobfs.ErrInvalidPath},
	} {
		if _, err := s.Copy(ctx, files.CopyRequest{Source: tc.src, Destination: tc.dst}); !errors.Is(err, tc.want) {
			t.Errorf("Copy(%s, %s) = %v, want %v", tc.src, tc.dst, err, tc.want)
		}
	}
	if opens != 0 || len(rec.Calls()) != 0 {
		t.Errorf("the refusals opened the store %d times and reached the driver %d times", opens, len(rec.Calls()))
	}
	s, rec = newStore(t, root(), file("A", "a.txt", blobfs.StatusAvailable, 2))
	if _, err := s.Copy(ctx, files.CopyRequest{Source: "/a.txt", Destination: "/b.txt"}); !errors.Is(err, files.ErrNoStorage) || len(rec.Calls()) != 0 {
		t.Errorf("Copy without an object store = %v after %d calls, want ErrNoStorage before any SQL", err, len(rec.Calls()))
	}
}

// TestCopyRefusesATakenDestination proves a destination name an
// available or a deleting file holds is ErrNameTaken with the status
// named, the transaction rolled back, and nothing stored; and that a copy
// onto the source itself, by its own path or by its directory, is refused
// the same way, since the source's row holds the name.
func TestCopyRefusesATakenDestination(t *testing.T) {
	ctx := context.Background()
	opens := 0
	for _, status := range []blobfs.Status{blobfs.StatusAvailable, blobfs.StatusDeleting} {
		s, rec, fake := writeStore(t, &opens, append(copyToNewName(file("A", "a.txt", blobfs.StatusAvailable, 2)), file("B", "b.txt", status, 3))...)
		_, err := s.Copy(ctx, files.CopyRequest{Source: "/a.txt", Destination: "/b.txt"})
		if !errors.Is(err, blobfs.ErrNameTaken) || !strings.Contains(err.Error(), string(status)) {
			t.Errorf("Copy over a %s row = %v, want ErrNameTaken naming the status", status, err)
		}
		if got := ops(rec); got != "begin query query query query query query rollback" || fake.Puts() != 0 {
			t.Errorf("ops = %q and %d puts after the refusal", got, fake.Puts())
		}
	}

	source := file("A", "a.txt", blobfs.StatusAvailable, 2)
	s, rec, _ := writeStore(t, &opens, append(copyToNewName(source), source)...)
	if _, err := s.Copy(ctx, files.CopyRequest{Source: "/a.txt", Destination: "/a.txt"}); !errors.Is(err, blobfs.ErrNameTaken) {
		t.Errorf("Copy onto the source's own path = %v, want ErrNameTaken", err)
	}
	if execs := rec.SQL(sqltest.OpExec); len(execs) != 0 {
		t.Errorf("the refused copy ran %v", execs)
	}
	s, rec, _ = writeStore(t, &opens, root(), source, root(), source)
	if _, err := s.Copy(ctx, files.CopyRequest{Source: "/a.txt", Destination: "/"}); !errors.Is(err, blobfs.ErrNameTaken) {
		t.Errorf("Copy into the source's own directory = %v, want ErrNameTaken", err)
	}
	if got := ops(rec); got != "begin query query query query rollback" {
		t.Errorf("ops = %q", got)
	}
}

// TestCopyStopsAfterTheStepNamed proves --fail-after on a copy: after
// insert the transaction has committed, the store was not read or
// written, and the error is a StopError naming cp, the step, and the
// pending row; after write the copy's object is stored and the row not
// completed; a cp of a name a pending row holds inserts nothing, streams
// the source under the pending row's key, and completes it; a source
// object the store no longer holds, and a failed object write, each leave
// the row pending with the cause reachable.
func TestCopyStopsAfterTheStepNamed(t *testing.T) {
	ctx := context.Background()
	opens := 0
	source := file("A", "a.txt", blobfs.StatusAvailable, 2)
	inserted := []sqltest.Response{noFile(), affected(), file("F", "b.txt", blobfs.StatusPending, 1)}
	s, rec, fake := writeStore(t, &opens, append(copyToNewName(source), inserted...)...)
	sourceObject(t, fake, "A/a.txt", "hello")
	res, err := s.Copy(ctx, files.CopyRequest{Source: "/a.txt", Destination: "/b.txt", StopAfter: files.StepInsert})
	var stop *files.StopError
	if !errors.As(err, &stop) || !errors.Is(err, files.ErrStopped) || stop.Command != "cp" || stop.Step != files.StepInsert || stop.File.ID != "F" {
		t.Fatalf("Copy --fail-after insert = %v", err)
	}
	if res.To != "/b.txt" || res.File.Status != blobfs.StatusPending || fake.Puts() != 1 {
		t.Errorf("after the stop the result is %+v and the store saw %d puts, want the seed only", res, fake.Puts())
	}
	if got := ops(rec); got != "begin query query query query query query exec query commit" {
		t.Errorf("ops = %q, want the committed first step and nothing more", got)
	}
	if !strings.Contains(err.Error(), "cp /a.txt /b.txt: stopped after step insert") || !strings.Contains(err.Error(), "pending (id F)") || !strings.Contains(err.Error(), "rerun cp") {
		t.Errorf("the message %q does not say how to finish", err)
	}

	s, rec, fake = writeStore(t, &opens, append(copyToNewName(source), inserted...)...)
	sourceObject(t, fake, "A/a.txt", "hello")
	_, err = s.Copy(ctx, files.CopyRequest{Source: "/a.txt", Destination: "/b.txt", StopAfter: files.StepWrite})
	if !errors.As(err, &stop) || stop.Command != "cp" || stop.Step != files.StepWrite {
		t.Fatalf("Copy --fail-after write = %v", err)
	}
	if fake.Puts() != 2 || len(rec.SQL(sqltest.OpExec)) != 1 || objectBytes(t, fake, "F/b.txt") != "hello" {
		t.Errorf("after the stop the store saw %d puts and the driver %d execs; want the copy's object stored and the insert only", fake.Puts(), len(rec.SQL(sqltest.OpExec)))
	}

	s, rec, fake = writeStore(t, &opens, append(copyToNewName(source), file("P", "b.txt", blobfs.StatusPending, 1), affected(), file("P", "b.txt", blobfs.StatusAvailable, 2))...)
	sourceObject(t, fake, "A/a.txt", "again")
	res, err = s.Copy(ctx, files.CopyRequest{Source: "/a.txt", Destination: "/b.txt"})
	if err != nil || !res.Resumed || res.File.ID != "P" || res.File.Version != 2 {
		t.Fatalf("Copy over a pending row = %+v, %v; want a resumed, completed row", res, err)
	}
	if got := ops(rec); got != "begin query query query query query query commit exec query" {
		t.Errorf("ops = %q, want no insert", got)
	}
	if got := objectBytes(t, fake, "P/b.txt"); got != "again" {
		t.Errorf("the object under the pending row's key holds %q", got)
	}
	if args := rec.Calls()[8].Args; args[3] != "P" || args[4] != int64(1) {
		t.Errorf("the completion bound %v, want the pending row's id and version", args)
	}

	s, rec, fake = writeStore(t, &opens, append(copyToNewName(source), inserted...)...)
	res, err = s.Copy(ctx, files.CopyRequest{Source: "/a.txt", Destination: "/b.txt"})
	if !errors.Is(err, files.ErrObjectMissing) || !strings.Contains(err.Error(), "stays pending") {
		t.Errorf("Copy of a source with no object = %v, want ErrObjectMissing and the pending row named", err)
	}
	if res.File.ID != "F" || len(rec.SQL(sqltest.OpExec)) != 1 || fake.Puts() != 0 {
		t.Errorf("after the missing object the result is %+v, the driver saw %d execs, and the store %d puts", res, len(rec.SQL(sqltest.OpExec)), fake.Puts())
	}

	s, rec, fake = writeStore(t, &opens, append(copyToNewName(source), inserted...)...)
	sourceObject(t, fake, "A/a.txt", "hello")
	cause := errors.New("the store failed")
	fake.FailPut(cause)
	res, err = s.Copy(ctx, files.CopyRequest{Source: "/a.txt", Destination: "/b.txt"})
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "cp /a.txt /b.txt") || !strings.Contains(err.Error(), "stays pending") {
		t.Errorf("Copy with a failing store = %v, want the cause and the pending row named", err)
	}
	if res.File.ID != "F" || len(rec.SQL(sqltest.OpExec)) != 1 {
		t.Errorf("after the failure the result is %+v and the driver saw %d execs", res, len(rec.SQL(sqltest.OpExec)))
	}
}

// TestCopyFileByIDSkipsTheResolution proves the copy by id: the first
// transaction reads the source by id, with no resolution, and runs the
// lookup by name in the destination directory, the pending insert, and
// its read-back, and the object read and write and the completion follow
// as in Copy; an empty name keeps the source's; with a scope the check
// runs after the read on the source's directory and then the
// destination, and a directory outside the scope is ErrNotOwned with
// nothing inserted; a source that is not available is ErrNotAvailable; a
// stop names the copy by ids; a missing source is ErrNotFound; and the
// result carries no paths.
func TestCopyFileByIDSkipsTheResolution(t *testing.T) {
	ctx := context.Background()
	unit := blobfs.NewID()
	opens := 0
	completed := []sqltest.Response{noFile(), affected(), fileIn("F", "D", "a.txt", blobfs.StatusPending, 1), affected(), fileIn("F", "D", "a.txt", blobfs.StatusAvailable, 2)}
	s, rec, fake := writeStore(t, &opens, append([]sqltest.Response{file("A", "a.txt", blobfs.StatusAvailable, 2)}, completed...)...)
	sourceObject(t, fake, "A/a.txt", "by id")
	res, err := s.CopyFile(ctx, "A", "D", "", "", files.Scope{})
	if err != nil || res.Resumed || res.File.ID != "F" || res.File.DirectoryID != "D" || res.File.Status != blobfs.StatusAvailable || res.From != "" || res.To != "" {
		t.Fatalf("CopyFile = %+v, %v", res, err)
	}
	if got := ops(rec); got != "begin query query exec query commit exec query" {
		t.Errorf("ops = %q, want the first step without a resolution", got)
	}
	calls := rec.Calls()
	if !strings.Contains(calls[1].SQL, "FROM blobfs_file f\nWHERE f.id =") || calls[1].Args[0] != "A" {
		t.Errorf("the first read is not the source by id: %s %v", calls[1].SQL, calls[1].Args)
	}
	if calls[2].Args[0] != "D" || calls[2].Args[1] != "a.txt" || calls[3].Args[1] != "D" || calls[3].Args[2] != "a.txt" || calls[3].Args[4] != "text/plain" {
		t.Errorf("the lookup bound %v and the insert %v, want the destination, the source's name, and its content type", calls[2].Args, calls[3].Args)
	}
	if got := objectBytes(t, fake, "F/a.txt"); got != "by id" || opens != 1 {
		t.Errorf("the copy's object holds %q and the store was opened %d times", got, opens)
	}

	s, rec, fake = writeStore(t, &opens, file("A", "a.txt", blobfs.StatusAvailable, 2), noFile(), affected(), fileIn("F", "D", "copy.txt", blobfs.StatusPending, 1), affected(), fileIn("F", "D", "copy.txt", blobfs.StatusAvailable, 2))
	sourceObject(t, fake, "A/a.txt", "hello")
	if res, err := s.CopyFile(ctx, "A", "D", "copy.txt", "", files.Scope{}); err != nil || res.File.Name != "copy.txt" {
		t.Errorf("CopyFile as a name = %+v, %v", res, err)
	}
	if lookup := rec.Calls()[2].Args; lookup[1] != "copy.txt" {
		t.Errorf("the lookup bound %v, want the name given", lookup)
	}

	scope := files.Scope{Unit: unit, DirectoryID: "R"}
	s, rec, fake = writeStore(t, &opens, append([]sqltest.Response{fileIn("A", "S", "a.txt", blobfs.StatusAvailable, 2), owner("R", unit), within(1), within(1)}, completed...)...)
	sourceObject(t, fake, "A/a.txt", "hello")
	if _, err := s.CopyFile(ctx, "A", "D", "", "", scope); err != nil {
		t.Fatalf("CopyFile in scope = %v", err)
	}
	if got := ops(rec); got != "begin query query query query query exec query commit exec query" {
		t.Errorf("ops in scope = %q, want the read, the owner read, and the two walks first", got)
	}
	if calls := rec.Calls(); calls[3].Args[0] != "S" || calls[3].Args[1] != "R" || calls[4].Args[0] != "D" || calls[4].Args[1] != "R" {
		t.Errorf("the walks bound %v and %v, want the source's directory and the destination against the scope", calls[3].Args, calls[4].Args)
	}
	s, rec, fake = writeStore(t, &opens, fileIn("A", "S", "a.txt", blobfs.StatusAvailable, 2), owner("R", blobfs.NewID()))
	if _, err := s.CopyFile(ctx, "A", "D", "", "", scope); !errors.Is(err, files.ErrNotOwned) {
		t.Errorf("CopyFile outside the scope = %v, want ErrNotOwned", err)
	}
	if got := ops(rec); got != "begin query query rollback" || fake.Puts() != 0 {
		t.Errorf("after the refusal ops = %q and %d puts; want nothing inserted or stored", got, fake.Puts())
	}

	s, rec, _ = writeStore(t, &opens, file("A", "a.txt", blobfs.StatusPending, 1))
	if _, err := s.CopyFile(ctx, "A", "D", "", "", files.Scope{}); !errors.Is(err, files.ErrNotAvailable) || !strings.Contains(err.Error(), "cp file A into directory D: the file is pending") {
		t.Errorf("CopyFile of a pending source = %v, want ErrNotAvailable naming the copy and the status", err)
	}
	if got := ops(rec); got != "begin query rollback" {
		t.Errorf("ops after the pending source = %q", got)
	}
	s, _, _ = writeStore(t, &opens, file("A", "a.txt", blobfs.StatusAvailable, 2), noFile(), affected(), fileIn("F", "D", "a.txt", blobfs.StatusPending, 1))
	_, err = s.CopyFile(ctx, "A", "D", "", files.StepInsert, files.Scope{})
	var stop *files.StopError
	if !errors.As(err, &stop) || stop.Command != "cp" || stop.Path != "file A into directory D" || !strings.Contains(err.Error(), "cp file A into directory D: stopped after step insert") {
		t.Errorf("CopyFile --fail-after insert = %v, want a StopError naming the ids", err)
	}
	s, rec, _ = writeStore(t, &opens, noFile())
	if _, err := s.CopyFile(ctx, "missing", "D", "", "", files.Scope{}); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("CopyFile of a missing source = %v, want ErrNotFound", err)
	}
	if got := ops(rec); got != "begin query rollback" {
		t.Errorf("ops after the missing source = %q", got)
	}
	s, rec = newStore(t)
	if _, err := s.CopyFile(ctx, "A", "D", "", "", files.Scope{}); !errors.Is(err, files.ErrNoStorage) || len(rec.Calls()) != 0 {
		t.Errorf("CopyFile without an object store = %v after %d calls, want ErrNoStorage before any SQL", err, len(rec.Calls()))
	}
}

// TestCommands_RenderCopy proves cp prints one result line with both
// paths, the id, the size, and the etag, says when it resumed a pending
// row, and prints nothing after a stop.
func TestCommands_RenderCopy(t *testing.T) {
	opens := 0
	source := file("A", "a.txt", blobfs.StatusAvailable, 2)
	scripted := func() (*files.Store, error) {
		s, _, fake := writeStore(t, &opens, append(copyToNewName(source), noFile(), affected(), file("F", "b.txt", blobfs.StatusPending, 1), affected(), file("F", "b.txt", blobfs.StatusAvailable, 2))...)
		sourceObject(t, fake, "A/a.txt", "hello")
		return s, nil
	}
	out, err := run(t, scripted, "cp", "/a.txt", "/b.txt")
	if err != nil || out != "cp: /a.txt -> /b.txt (id F, 5 bytes, etag \"e\")\n" {
		t.Errorf("cp = %q, %v", out, err)
	}

	scripted = func() (*files.Store, error) {
		s, _, fake := writeStore(t, &opens, append(copyToNewName(source), file("P", "b.txt", blobfs.StatusPending, 1), affected(), file("P", "b.txt", blobfs.StatusAvailable, 2))...)
		sourceObject(t, fake, "A/a.txt", "hello")
		return s, nil
	}
	out, err = run(t, scripted, "cp", "/a.txt", "/b.txt")
	if err != nil || out != "cp: /a.txt -> /b.txt (id P, 5 bytes, etag \"e\", resumed the pending row)\n" {
		t.Errorf("cp over a pending row = %q, %v", out, err)
	}

	scripted = func() (*files.Store, error) {
		s, _, fake := writeStore(t, &opens, append(copyToNewName(source), noFile(), affected(), file("F", "b.txt", blobfs.StatusPending, 1))...)
		sourceObject(t, fake, "A/a.txt", "hello")
		return s, nil
	}
	out, err = run(t, scripted, "cp", "/a.txt", "/b.txt", "--fail-after", "insert")
	if !errors.Is(err, files.ErrStopped) || out != "" {
		t.Errorf("cp --fail-after insert = %q, %v; want the stop and no result line", out, err)
	}
}
