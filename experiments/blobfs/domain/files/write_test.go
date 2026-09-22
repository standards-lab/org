package files_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/go-storage/storagetest"
	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/domain/files"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// file scripts one file row in the root with a status and version.
func file(id, name string, status blobfs.Status, version int64) sqltest.Response {
	now := time.Now()
	var size, etag any
	if status == blobfs.StatusAvailable {
		size, etag = int64(5), `"e"`
	}
	row := []driver.Value{id, blobfs.RootID, name, string(status), id + "/" + name, size, "text/plain", etag, version, now, now}
	return sqltest.Response{Columns: fileColumns, Rows: [][]driver.Value{row}}
}

// noFile scripts the read of a name no row holds.
func noFile() sqltest.Response { return sqltest.Response{Columns: fileColumns} }

// writeStore builds the store over the scripted driver with an opener over
// the fake, counting how often the opener runs.
func writeStore(t *testing.T, opens *int, responses ...sqltest.Response) (*files.Store, *sqltest.Recorder, *storagetest.Fake) {
	t.Helper()
	st, fake := fakeStorage(t)
	pool, rec := sqltest.Open(t, responses...)
	s, err := files.New(sqlate.Wrap(pool, sqltest.Dialect{}), func(context.Context) (*files.Storage, error) {
		*opens++
		return st, nil
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, rec, fake
}

// TestPutIsThreeStepsWithTwoBoundaries proves the transaction boundaries
// of a full put: one transaction holds the parent's resolution, the
// lookup by name, the pending insert, and its read-back, and commits
// before the object is stored; the object write happens outside any
// transaction; and the completion is the guarded update and its
// read-back on the pool. The store is opened once, before the first
// transaction, and the key the row was inserted under is the key the
// object was stored at.
func TestPutIsThreeStepsWithTwoBoundaries(t *testing.T) {
	opens := 0
	s, rec, fake := writeStore(t, &opens,
		root(), noFile(), affected(), file("F", "a.txt", blobfs.StatusPending, 1),
		affected(), file("F", "a.txt", blobfs.StatusAvailable, 2),
	)
	res, err := s.Put(context.Background(), files.PutRequest{Path: "/a.txt", ContentType: "text/plain", Body: strings.NewReader("hello"), Size: 5})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if res.Resumed || res.File.ID != "F" || res.File.Status != blobfs.StatusAvailable || res.File.Version != 2 {
		t.Errorf("Put = %+v", res)
	}
	if got := ops(rec); got != "begin query query exec query commit exec query" {
		t.Errorf("ops = %q", got)
	}
	execs := rec.SQL(sqltest.OpExec)
	if !strings.HasPrefix(execs[0], "INSERT INTO blobfs_file") || !strings.HasPrefix(execs[1], "UPDATE blobfs_file") {
		t.Errorf("execs = %q", execs)
	}
	calls := rec.Calls()
	if key, _ := calls[3].Args[3].(string); !strings.HasSuffix(key, "/a.txt") || calls[4].Args[0] != key[:36] {
		t.Errorf("the insert bound the key %q and the read-back the id %v; want the minted id under both", key, calls[4].Args)
	}
	if _, err := fake.Stat(context.Background(), "F/a.txt"); err != nil {
		t.Errorf("the object is not at the key of the row read back: %v", err)
	}
	if opts, n := fake.LastPut(); opts.ContentType != "text/plain" || opts.Size != 5 || n != 5 {
		t.Errorf("the store received %+v after %d bytes", opts, n)
	}
	if complete := calls[6].Args; complete[0] != int64(5) || complete[1] != "text/plain" || complete[3] != "F" || complete[4] != int64(1) {
		t.Errorf("the completion bound %v, want the store's size and type, the id, and version 1", complete)
	}
	if opens != 1 {
		t.Errorf("the store was opened %d times, want once", opens)
	}
}

// TestPutStopsAfterTheStepNamed proves --fail-after: after insert the
// transaction has committed, nothing reached the store, and the error is a
// StopError naming the step and the pending row; after write the object is
// in the store and the row was not completed. A failed object write leaves
// the same state as a stop after insert, with the store's error and the
// pending row named.
func TestPutStopsAfterTheStepNamed(t *testing.T) {
	ctx := context.Background()
	opens := 0
	s, rec, fake := writeStore(t, &opens, root(), noFile(), affected(), file("F", "a.txt", blobfs.StatusPending, 1))
	res, err := s.Put(ctx, files.PutRequest{Path: "/a.txt", Body: strings.NewReader("x"), StopAfter: files.StepInsert})
	var stop *files.StopError
	if !errors.As(err, &stop) || !errors.Is(err, files.ErrStopped) || stop.Step != files.StepInsert || stop.File.ID != "F" {
		t.Fatalf("Put --fail-after insert = %v", err)
	}
	if res.File.Status != blobfs.StatusPending || fake.Puts() != 0 {
		t.Errorf("after the stop the result is %+v and the store saw %d puts", res, fake.Puts())
	}
	if got := ops(rec); got != "begin query query exec query commit" {
		t.Errorf("ops = %q, want the committed first step and nothing more", got)
	}
	if !strings.Contains(err.Error(), "pending (id F)") || !strings.Contains(err.Error(), "rerun put") {
		t.Errorf("the message %q does not say how to finish", err)
	}

	s, rec, fake = writeStore(t, &opens, root(), noFile(), affected(), file("F", "a.txt", blobfs.StatusPending, 1))
	_, err = s.Put(ctx, files.PutRequest{Path: "/a.txt", Body: strings.NewReader("x"), StopAfter: files.StepWrite})
	if !errors.As(err, &stop) || stop.Step != files.StepWrite {
		t.Fatalf("Put --fail-after write = %v", err)
	}
	if fake.Puts() != 1 || len(rec.SQL(sqltest.OpExec)) != 1 {
		t.Errorf("after the stop the store saw %d puts and the driver %d execs; want 1 and the insert only", fake.Puts(), len(rec.SQL(sqltest.OpExec)))
	}

	s, rec, fake = writeStore(t, &opens, root(), noFile(), affected(), file("F", "a.txt", blobfs.StatusPending, 1))
	cause := errors.New("the store failed")
	fake.FailPut(cause)
	res, err = s.Put(ctx, files.PutRequest{Path: "/a.txt", Body: strings.NewReader("x")})
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "stays pending") {
		t.Errorf("Put with a failing store = %v, want the cause and the pending row named", err)
	}
	if res.File.ID != "F" || len(rec.SQL(sqltest.OpExec)) != 1 {
		t.Errorf("after the failure the result is %+v and the driver saw %d execs", res, len(rec.SQL(sqltest.OpExec)))
	}
}

// TestPutResumesAPendingRow proves a put of a name a pending row holds
// inserts nothing: the row is taken up at its version, the object is
// stored under its key, and the completion runs; a name an available or a
// deleting row holds is ErrNameTaken with the status in the message, with
// nothing stored and the transaction rolled back.
func TestPutResumesAPendingRow(t *testing.T) {
	ctx := context.Background()
	opens := 0
	s, rec, fake := writeStore(t, &opens, root(), file("P", "a.txt", blobfs.StatusPending, 1), affected(), file("P", "a.txt", blobfs.StatusAvailable, 2))
	res, err := s.Put(ctx, files.PutRequest{Path: "/a.txt", ContentType: "text/plain", Body: strings.NewReader("again")})
	if err != nil || !res.Resumed || res.File.Version != 2 {
		t.Fatalf("Put over a pending row = %+v, %v; want a resumed, completed row", res, err)
	}
	if got := ops(rec); got != "begin query query commit exec query" {
		t.Errorf("ops = %q, want no insert", got)
	}
	if _, err := fake.Stat(ctx, "P/a.txt"); err != nil {
		t.Errorf("the object is not under the pending row's key: %v", err)
	}
	if args := rec.Calls()[4].Args; args[3] != "P" || args[4] != int64(1) {
		t.Errorf("the completion bound %v, want the pending row's id and version", args)
	}

	for _, status := range []blobfs.Status{blobfs.StatusAvailable, blobfs.StatusDeleting} {
		s, rec, fake = writeStore(t, &opens, root(), file("A", "a.txt", status, 1))
		_, err := s.Put(ctx, files.PutRequest{Path: "/a.txt", Body: strings.NewReader("x")})
		if !errors.Is(err, blobfs.ErrNameTaken) || !strings.Contains(err.Error(), string(status)) {
			t.Errorf("Put over a %s row = %v, want ErrNameTaken naming the status", status, err)
		}
		if got := ops(rec); got != "begin query query rollback" || fake.Puts() != 0 {
			t.Errorf("ops = %q and %d puts after the refusal", got, fake.Puts())
		}
	}
}

// TestPutRefusalsBeforeAnyStep proves a bad path fails before the store
// is opened and before any SQL, and a store built without an object store
// fails with ErrNoStorage before any SQL.
func TestPutRefusalsBeforeAnyStep(t *testing.T) {
	ctx := context.Background()
	opens := 0
	s, rec, _ := writeStore(t, &opens)
	for path, want := range map[string]error{"/": blobfs.ErrRootDirectory, "/a/": blobfs.ErrInvalidPath, "a": blobfs.ErrInvalidPath} {
		if _, err := s.Put(ctx, files.PutRequest{Path: path, Body: strings.NewReader("x")}); !errors.Is(err, want) {
			t.Errorf("Put(%s) = %v, want %v", path, err, want)
		}
	}
	if opens != 0 || len(rec.Calls()) != 0 {
		t.Errorf("the refusals opened the store %d times and reached the driver %d times", opens, len(rec.Calls()))
	}
	s, rec = newStore(t, root(), file("A", "a.txt", blobfs.StatusAvailable, 2))
	if _, err := s.Put(ctx, files.PutRequest{Path: "/a.txt", Body: strings.NewReader("x")}); !errors.Is(err, files.ErrNoStorage) {
		t.Errorf("Put without an object store = %v, want ErrNoStorage", err)
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("Put without an object store reached the driver: %v", calls)
	}
	if _, _, err := s.Open(ctx, "/a.txt"); !errors.Is(err, files.ErrNoStorage) {
		t.Errorf("Open without an object store = %v, want ErrNoStorage after the row was read", err)
	}
}

// TestStatAndOpen proves stat reads the parent and then the row on the
// pool with no transaction and no store access, and that Open refuses a
// pending or deleting row before the store is asked and streams an
// available row's object.
func TestStatAndOpen(t *testing.T) {
	ctx := context.Background()
	opens := 0
	s, rec, _ := writeStore(t, &opens, root(), file("P", "a.txt", blobfs.StatusPending, 1))
	f, err := s.Stat(ctx, "/a.txt")
	if err != nil || f.ID != "P" || f.Status != blobfs.StatusPending {
		t.Fatalf("Stat = %+v, %v", f, err)
	}
	if got := ops(rec); got != "query query" || opens != 0 {
		t.Errorf("ops = %q with %d store opens; want two pool reads and no store", got, opens)
	}
	s, _, _ = writeStore(t, &opens, root(), noFile())
	if _, err := s.Stat(ctx, "/missing.txt"); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("Stat of a missing file = %v, want ErrNotFound", err)
	}

	for _, status := range []blobfs.Status{blobfs.StatusPending, blobfs.StatusDeleting} {
		s, _, _ = writeStore(t, &opens, root(), file("P", "a.txt", status, 1))
		_, _, err := s.Open(ctx, "/a.txt")
		if !errors.Is(err, files.ErrNotAvailable) || !strings.Contains(err.Error(), string(status)) || opens != 0 {
			t.Errorf("Open of a %s file = %v with %d store opens; want ErrNotAvailable naming the status and no store", status, err, opens)
		}
	}

	s, _, fake := writeStore(t, &opens, root(), file("A", "a.txt", blobfs.StatusAvailable, 2))
	if _, err := fake.Put(ctx, "A/a.txt", strings.NewReader("stored"), storage.PutOptions{ContentType: "text/plain"}); err != nil {
		t.Fatalf("seed the fake: %v", err)
	}
	body, f, err := s.Open(ctx, "/a.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = body.Close() }()
	if got, _ := io.ReadAll(body); string(got) != "stored" || f.ID != "A" {
		t.Errorf("Open streamed %q for %+v", got, f)
	}
	s, _, _ = writeStore(t, &opens, root(), file("G", "gone.txt", blobfs.StatusAvailable, 2))
	if _, _, err := s.Open(ctx, "/gone.txt"); !errors.Is(err, files.ErrObjectMissing) {
		t.Errorf("Open of an available row with no object = %v, want ErrObjectMissing", err)
	}
}
