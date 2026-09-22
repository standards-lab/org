package files_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/go-storage/storagetest"

	"github.com/standards-lab/org/experiments/blobfs/domain/files"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// fakeStorage builds the adapter over a started storage.Store wrapping the
// storage package's in-memory fake, with the given options, and returns
// the fake so a test can toggle it.
func fakeStorage(t *testing.T, opts ...storagetest.Option) (*files.Storage, *storagetest.Fake) {
	t.Helper()
	fake := storagetest.NewFake(opts...)
	cfg := storage.Config{Container: "test", MaxObjectSize: 64}
	if err := cfg.Finalize(""); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	st := files.NewStorage(storage.New(fake, cfg))
	if err := st.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return st, fake
}

// TestStorageMapsTheStoresErrors is the adapter's error mapping: a missing
// object is ErrObjectMissing, an outage ErrStorageUnavailable, a store that
// was never started ErrStorageUnavailable too, and a body over the bound
// ErrObjectTooLarge, each with the store's own sentinel still reachable.
func TestStorageMapsTheStoresErrors(t *testing.T) {
	ctx := context.Background()
	st, fake := fakeStorage(t)

	if _, err := st.Stat(ctx, "nope"); !errors.Is(err, files.ErrObjectMissing) || !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("Stat of a missing key = %v, want ErrObjectMissing over storage.ErrNotFound", err)
	}
	if _, err := st.Get(ctx, "nope"); !errors.Is(err, files.ErrObjectMissing) {
		t.Errorf("Get of a missing key = %v, want ErrObjectMissing", err)
	}
	if _, err := st.Put(ctx, "big", strings.NewReader(strings.Repeat("x", 65)), "text/plain", 0); !errors.Is(err, files.ErrObjectTooLarge) || !errors.Is(err, storage.ErrTooLarge) {
		t.Errorf("Put over the bound = %v, want ErrObjectTooLarge over storage.ErrTooLarge", err)
	}

	fake.Down.Store(true)
	if _, err := st.Put(ctx, "k", strings.NewReader("x"), "text/plain", 1); !errors.Is(err, files.ErrStorageUnavailable) || !errors.Is(err, storagetest.ErrDown) {
		t.Errorf("Put during an outage = %v, want ErrStorageUnavailable over the fake's cause", err)
	}
	if err := st.Start(ctx); !errors.Is(err, files.ErrStorageUnavailable) {
		t.Errorf("Start during an outage = %v, want ErrStorageUnavailable", err)
	}
	fake.Down.Store(false)

	cfg := storage.Config{Container: "test"}
	if err := cfg.Finalize(""); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	cold := files.NewStorage(storage.New(storagetest.NewFake(), cfg))
	if _, err := cold.Stat(ctx, "k"); !errors.Is(err, files.ErrStorageUnavailable) || !errors.Is(err, storage.ErrNotReady) {
		t.Errorf("Stat before Start = %v, want ErrStorageUnavailable over storage.ErrNotReady", err)
	}
}

// TestStoragePutGetStat proves the adapter passes the body, the content
// type, and the declared size through and returns what the store
// reported, and that Get streams the bytes Put stored.
func TestStoragePutGetStat(t *testing.T) {
	ctx := context.Background()
	st, fake := fakeStorage(t)
	obj, err := st.Put(ctx, "k/name.txt", strings.NewReader("hello"), "text/plain", 5)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if obj.Size != 5 || obj.ContentType != "text/plain" || obj.ETag == "" {
		t.Errorf("Put returned %+v", obj)
	}
	if opts, n := fake.LastPut(); opts.ContentType != "text/plain" || opts.Size != 5 || n != 5 {
		t.Errorf("the fake received %+v after %d bytes", opts, n)
	}
	stat, err := st.Stat(ctx, "k/name.txt")
	if err != nil || stat != obj {
		t.Errorf("Stat = %+v, %v; want what Put returned", stat, err)
	}
	body, err := st.Get(ctx, "k/name.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = body.Close() }()
	if got, _ := io.ReadAll(body); string(got) != "hello" {
		t.Errorf("Get streamed %q", got)
	}
	if st.Container() != "test" {
		t.Errorf("Container() = %q", st.Container())
	}
}

// TestStorageValidatesKeysWithTheProvidersRule proves the adapter is the
// provider's ValidateKey and nothing more: a rule the fake declares is what
// blobfs's key construction sees, reason included, and the adapter adds no
// length rule of its own, so a key the provider accepts at any length is
// accepted.
func TestStorageValidatesKeysWithTheProvidersRule(t *testing.T) {
	refused := errors.New("the provider refuses dots")
	st, _ := fakeStorage(t, storagetest.WithCapabilities(storage.Capabilities{
		MaxKeyLength: 8,
		ValidateKey: func(key string) error {
			if strings.Contains(key, ".") {
				return refused
			}
			return nil
		},
	}))
	var _ blobfs.KeyValidator = st
	if _, err := blobfs.NewKey(st, "0123456789", "a.txt"); !errors.Is(err, blobfs.ErrInvalidKey) || !errors.Is(err, refused) {
		t.Errorf("NewKey with a dotted name = %v, want ErrInvalidKey over the provider's reason", err)
	}
	key, err := blobfs.NewKey(st, "0123456789", strings.Repeat("x", 100))
	if err != nil || len(key) != 111 {
		t.Errorf("NewKey with a long name = %q, %v; the adapter should apply no length of its own", key, err)
	}
}
