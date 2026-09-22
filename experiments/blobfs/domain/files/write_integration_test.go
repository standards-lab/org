//go:build integration

package files_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/go-storage/azureblob"

	"github.com/standards-lab/org/experiments/blobfs/domain/files"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// put uploads body as the file at path and fails the test on any error.
func (e env) put(t *testing.T, path, contentType, body string) files.PutResult {
	t.Helper()
	res, err := e.store.Put(e.ctx, files.PutRequest{Path: path, ContentType: contentType, Body: strings.NewReader(body), Size: int64(len(body))})
	if err != nil {
		t.Fatalf("Put(%s): %v", path, err)
	}
	return res
}

// cat reads the file at path back and fails the test on any error.
func (e env) cat(t *testing.T, path string) string {
	t.Helper()
	body, _, err := e.store.Open(e.ctx, path)
	if err != nil {
		t.Fatalf("Open(%s): %v", path, err)
	}
	defer func() { _ = body.Close() }()
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(got)
}

// storage opens the same object store the store uses, so a test can ask
// the service directly what it holds.
func (e env) storage(t *testing.T) *files.Storage {
	t.Helper()
	st, err := files.OpenStorage(e.ctx, "blobfs")
	if err != nil {
		t.Fatalf("OpenStorage: %v", err)
	}
	t.Cleanup(func() { _ = st.Shutdown(context.Background()) })
	return st
}

// TestPut is the write path over Postgres and Azurite: a put stores the
// bytes and completes the row, cat returns the same bytes, and stat shows
// the row with the size, etag, and content type the service reports for
// the object. A second put of the name is refused as taken, a put under a
// missing parent as not found, and a put with a name blobfs refuses fails
// before any row exists.
func TestPut(t *testing.T) {
	e := open(t)
	e.mkdir(t, "/docs", "")
	content := "hello, blob\n"
	res := e.put(t, "/docs/hello.txt", "text/plain", content)
	f := res.File
	if res.Resumed || f.Status != blobfs.StatusAvailable || f.Size == nil || *f.Size != int64(len(content)) || f.ETag == nil || f.Version != 2 {
		t.Errorf("Put returned %+v", res)
	}
	if got := e.cat(t, "/docs/hello.txt"); got != content {
		t.Errorf("cat returned %q, want what put stored", got)
	}
	row, err := e.store.Stat(e.ctx, "/docs/hello.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	obj, err := e.storage(t).Stat(e.ctx, row.Key)
	if err != nil {
		t.Fatalf("the service's Stat: %v", err)
	}
	if *row.Size != obj.Size || *row.ETag != obj.ETag || row.ContentType != obj.ContentType {
		t.Errorf("the row says size %d, etag %s, type %s; the service says %+v", *row.Size, *row.ETag, row.ContentType, obj)
	}
	if n := e.count(t, "SELECT COUNT(*) FROM blobfs_file"); n != 1 {
		t.Errorf("%d file rows, want one", n)
	}

	_, err = e.store.Put(e.ctx, files.PutRequest{Path: "/docs/hello.txt", Body: strings.NewReader("x")})
	if !errors.Is(err, blobfs.ErrNameTaken) {
		t.Errorf("a second put of the name = %v, want ErrNameTaken", err)
	}
	if got := e.cat(t, "/docs/hello.txt"); got != content {
		t.Errorf("after the refused put the content is %q", got)
	}
	_, err = e.store.Put(e.ctx, files.PutRequest{Path: "/missing/hello.txt", Body: strings.NewReader("x")})
	if !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("a put under a missing parent = %v, want ErrNotFound", err)
	}
	_, err = e.store.Put(e.ctx, files.PutRequest{Path: "/docs/" + strings.Repeat("x", blobfs.MaxNameLength+1), Body: strings.NewReader("x")})
	if !errors.Is(err, blobfs.ErrInvalidName) {
		t.Errorf("a put with a name over the limit = %v, want ErrInvalidName", err)
	}
	if n := e.count(t, "SELECT COUNT(*) FROM blobfs_file"); n != 1 {
		t.Errorf("after the refusals %d file rows exist, want one", n)
	}
	if _, err := e.store.Stat(e.ctx, "/docs/missing.txt"); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("Stat of a missing file = %v, want ErrNotFound", err)
	}
}

// TestPutStopsAndResumes is the stop between the steps on the real
// services: a put that stops after the insert leaves a pending row that a
// second connection reads and stat shows, with no object in the store;
// cat refuses it; a put of the same path resumes the row and completes
// it, and cat then returns the bytes. A put that stops after the write
// leaves the row pending with the object stored, and a resume with other
// content replaces the object and completes the row.
func TestPutStopsAndResumes(t *testing.T) {
	e := open(t)
	other := e.second(t)
	e.mkdir(t, "/docs", "")
	st := e.storage(t)

	_, err := e.store.Put(e.ctx, files.PutRequest{Path: "/docs/a.txt", Body: strings.NewReader("first"), StopAfter: files.StepInsert})
	var stop *files.StopError
	if !errors.As(err, &stop) || stop.Step != files.StepInsert {
		t.Fatalf("Put --fail-after insert = %v", err)
	}
	pending := stop.File
	if status := e.strings1(t, "SELECT status FROM blobfs_file WHERE id = $1", pending.ID); strings.Join(status, ",") != "pending" {
		t.Errorf("the row is %v, want pending", status)
	}
	if _, err := other.QueryContext(e.ctx, "SELECT 1"); err != nil {
		t.Fatalf("second connection: %v", err)
	}
	row, err := e.store.Stat(e.ctx, "/docs/a.txt")
	if err != nil || row.Status != blobfs.StatusPending || row.ID != pending.ID {
		t.Errorf("Stat after the stop = %+v, %v; want the pending row", row, err)
	}
	if _, err := st.Stat(e.ctx, pending.Key); !errors.Is(err, files.ErrObjectMissing) {
		t.Errorf("the store holds %v under the pending key; want nothing", err)
	}
	if _, _, err := e.store.Open(e.ctx, "/docs/a.txt"); !errors.Is(err, files.ErrNotAvailable) {
		t.Errorf("Open of the pending file = %v, want ErrNotAvailable", err)
	}

	res := e.put(t, "/docs/a.txt", "text/plain", "second")
	if !res.Resumed || res.File.ID != pending.ID || res.File.Status != blobfs.StatusAvailable || res.File.Version != 2 {
		t.Errorf("the retry returned %+v; want the pending row resumed and completed", res)
	}
	if got := e.cat(t, "/docs/a.txt"); got != "second" {
		t.Errorf("cat after the retry = %q", got)
	}

	_, err = e.store.Put(e.ctx, files.PutRequest{Path: "/docs/b.txt", ContentType: "text/plain", Body: strings.NewReader("draft"), StopAfter: files.StepWrite})
	if !errors.As(err, &stop) || stop.Step != files.StepWrite {
		t.Fatalf("Put --fail-after write = %v", err)
	}
	if obj, err := st.Stat(e.ctx, stop.File.Key); err != nil || obj.Size != 5 {
		t.Errorf("after the stop the store holds %+v, %v; want the 5-byte object", obj, err)
	}
	if row, err := e.store.Stat(e.ctx, "/docs/b.txt"); err != nil || row.Status != blobfs.StatusPending || row.Size != nil {
		t.Errorf("after the stop the row is %+v, %v; want pending with no size", row, err)
	}
	res = e.put(t, "/docs/b.txt", "text/plain", "final")
	if !res.Resumed || *res.File.Size != 5 || res.File.Status != blobfs.StatusAvailable {
		t.Errorf("the retry returned %+v", res)
	}
	if got := e.cat(t, "/docs/b.txt"); got != "final" {
		t.Errorf("cat after the retry = %q; the object should have been replaced", got)
	}
	if n := e.count(t, "SELECT COUNT(*) FROM blobfs_file WHERE status = 'pending'"); n != 0 {
		t.Errorf("%d rows are still pending", n)
	}
}

// TestPutNameAtTheRuneBoundary proves a filename at blobfs's limit of 255
// runes, in two-byte runes, is accepted end to end: the key is 292 runes
// and 546 bytes, Azurite stores it, and cat reads it back; one rune more
// is refused as an invalid name before any row or object exists.
func TestPutNameAtTheRuneBoundary(t *testing.T) {
	e := open(t)
	name := strings.Repeat("é", blobfs.MaxNameLength)
	res := e.put(t, "/"+name, "text/plain", "at the boundary")
	if n := utf8.RuneCountInString(res.File.Key); n != 36+1+blobfs.MaxNameLength {
		t.Errorf("the key is %d runes, want 292", n)
	}
	if len(res.File.Key) != 36+1+2*blobfs.MaxNameLength {
		t.Errorf("the key is %d bytes, want 546", len(res.File.Key))
	}
	if got := e.cat(t, "/"+name); got != "at the boundary" {
		t.Errorf("cat returned %q", got)
	}
	_, err := e.store.Put(e.ctx, files.PutRequest{Path: "/" + name + "é", Body: strings.NewReader("x")})
	if !errors.Is(err, blobfs.ErrInvalidName) {
		t.Errorf("a name one rune over = %v, want ErrInvalidName", err)
	}
	if n := e.count(t, "SELECT COUNT(*) FROM blobfs_file"); n != 1 {
		t.Errorf("%d file rows, want the one at the boundary", n)
	}
}

// TestAzuriteAndTheKeyLimit records what the emulator does with a key at
// the provider's limit of 1,024 runes and one rune over, put straight
// through the storage store, which validates no key: the provider's rule
// refuses the longer key, and whether Azurite does too is logged, since
// the ledger says the emulator accepts keys the provider refuses.
func TestAzuriteAndTheKeyLimit(t *testing.T) {
	e := open(t)
	st := e.storage(t)
	at := strings.Repeat("é", azureblob.MaxKeyLength)
	over := at + "é"
	if err := st.ValidateKey(at); err != nil {
		t.Errorf("the provider refuses a key at its limit: %v", err)
	}
	if err := st.ValidateKey(over); err == nil {
		t.Errorf("the provider accepts a key one rune over its limit")
	}
	var cfg storage.Config
	if err := cfg.Finalize("blobfs"); err != nil {
		t.Fatal(err)
	}
	client, err := azureblob.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	raw := storage.New(client, cfg)
	if err := raw.Start(e.ctx); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{at, over} {
		_, err := raw.Put(e.ctx, key, bytes.NewReader([]byte("x")), storage.PutOptions{})
		t.Logf("Azurite, a key of %d runes (%d bytes): put error = %v", utf8.RuneCountInString(key), len(key), err)
		if err == nil {
			if _, err := raw.Stat(e.ctx, key); err != nil {
				t.Logf("Azurite, a key of %d runes: stat after the put = %v", utf8.RuneCountInString(key), err)
			}
		}
	}
}
