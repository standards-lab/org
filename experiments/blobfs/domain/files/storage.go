package files

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/go-storage/azureblob"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// This file is the only application file that names go-storage and its
// Azure Blob provider. It adapts the storage store to what the domain
// needs: the object operations the file commands make, the key validation
// blobfs asks for before it inserts a pending row, and the store's start
// and shutdown, which the composition root drives.

// Storage is the domain's object store: a started storage.Store over one
// container, the one the configuration names. It implements
// blobfs.KeyValidator over the provider's key rules, so blobfs validates a
// key against the real store before it inserts a pending row, and it maps
// the store's errors onto the domain's sentinels.
type Storage struct {
	store *storage.Store
}

var _ blobfs.KeyValidator = (*Storage)(nil)

// OpenStorage builds the object store from the environment and starts it.
// The settings are the storage package's, read by its own configuration
// under envPrefix: with the prefix blobfs they are BLOBFS_STORAGE_ENDPOINT,
// BLOBFS_STORAGE_CONTAINER, BLOBFS_STORAGE_ACCOUNT, and BLOBFS_STORAGE_KEY,
// plus the optional limits the package documents. The provider is Azure
// Blob Storage through the azureblob sub-module, the only place the
// experiment names it. Start ensures the container exists and probes the
// store, so a store that is unreachable, or a credential the service
// rejects, fails here as ErrStorageUnavailable and no pending row is ever
// inserted for an object that could not be stored. The caller shuts the
// store down with Shutdown when the run ends.
func OpenStorage(ctx context.Context, envPrefix string) (*Storage, error) {
	var cfg storage.Config
	if err := cfg.Finalize(envPrefix); err != nil {
		return nil, fmt.Errorf("files: storage configuration: %w", err)
	}
	client, err := azureblob.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("files: %w", err)
	}
	st := NewStorage(storage.New(client, cfg))
	if err := st.Start(ctx); err != nil {
		return nil, err
	}
	return st, nil
}

// NewStorage wraps a store the caller built, started or not. A service
// composes the object store through NewStorage: it builds the store from
// its own configuration, as it builds its other infrastructure, and hands
// it to the domain, so the domain never reads an environment prefix. A
// test hands it a store over the storage package's in-memory fake, and
// the command-line tool's composition root goes through OpenStorage, which
// reads the environment.
func NewStorage(store *storage.Store) *Storage {
	return &Storage{store: store}
}

// Start starts the store: the container is ensured and the provider
// probed, under the store's own request timeout. A failure is
// ErrStorageUnavailable with the store's error reachable; the store
// classifies every start failure that way, a rejected credential
// included.
func (s *Storage) Start(ctx context.Context) error {
	if err := s.store.Start(ctx); err != nil {
		return fmt.Errorf("files: start storage: %w", storageError(err))
	}
	return nil
}

// Shutdown stops the store and closes the provider. It is safe before
// Start and after a failed one.
func (s *Storage) Shutdown(ctx context.Context) error {
	return s.store.Shutdown(ctx)
}

// Container returns the name of the container the store writes to.
func (s *Storage) Container() string {
	return s.store.Container()
}

// ValidateKey reports whether the provider accepts key, with the
// provider's reason when it does not. It is the provider's own rule, read
// from the store's capabilities, and the provider's rule already bounds the
// key's length, so nothing is added here.
func (s *Storage) ValidateKey(key string) error {
	return s.store.Capabilities().ValidateKey(key)
}

// Put stores body as the object at key with the content type declared, and
// size as the body's length when the caller knows it (0 asserts nothing),
// and returns what the store reported: the size it counted, the entity tag
// it assigned, and the content type as sent. A body longer than the
// configured bound is ErrObjectTooLarge, and a store that cannot be
// reached ErrStorageUnavailable.
func (s *Storage) Put(ctx context.Context, key string, body io.Reader, contentType string, size int64) (blobfs.Object, error) {
	obj, err := s.store.Put(ctx, key, body, storage.PutOptions{ContentType: contentType, Size: size})
	if err != nil {
		return blobfs.Object{}, fmt.Errorf("files: put object %s: %w", key, storageError(err))
	}
	return object(obj), nil
}

// Get opens the object at key for reading. The caller closes the reader.
// A key with no object is ErrObjectMissing: the row says the file is
// available and the store disagrees.
func (s *Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	blob, err := s.store.Get(ctx, key, storage.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("files: get object %s: %w", key, storageError(err))
	}
	return blob.Body, nil
}

// Stat returns what the store holds about the object at key without
// reading it, or ErrObjectMissing.
func (s *Storage) Stat(ctx context.Context, key string) (blobfs.Object, error) {
	obj, err := s.store.Stat(ctx, key)
	if err != nil {
		return blobfs.Object{}, fmt.Errorf("files: stat object %s: %w", key, storageError(err))
	}
	return object(obj), nil
}

// Delete removes the object at key. A key with no object is success: the
// provider answers a missing object with success, as the storage
// contract asks, so the step can be repeated after a stop. A missing
// container is not swallowed: the provider reports it as the store's
// not-found error, and the adapter maps it to ErrContainerMissing rather
// than count the object gone, since the configured target is gone and
// the object may well exist elsewhere. A store that cannot be reached is
// ErrStorageUnavailable.
func (s *Storage) Delete(ctx context.Context, key string) error {
	err := s.store.Delete(ctx, key)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, storage.ErrNotFound):
		return fmt.Errorf("files: delete object %s: %w: %w", key, ErrContainerMissing, err)
	}
	return fmt.Errorf("files: delete object %s: %w", key, storageError(err))
}

// object narrows the store's Object to the facts a file row keeps.
func object(o storage.Object) blobfs.Object {
	return blobfs.Object{Size: o.Size, ContentType: o.ContentType, ETag: o.ETag}
}

// storageError maps the store's sentinels onto the domain's, keeping the
// store's error reachable: a missing object is ErrObjectMissing, a store
// that is unreachable or not started ErrStorageUnavailable, and a body
// over the configured bound ErrObjectTooLarge. Any other error, a failure
// of the body included, passes through as it came.
func storageError(err error) error {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return fmt.Errorf("%w: %w", ErrObjectMissing, err)
	case errors.Is(err, storage.ErrUnavailable), errors.Is(err, storage.ErrNotReady):
		return fmt.Errorf("%w: %w", ErrStorageUnavailable, err)
	case errors.Is(err, storage.ErrTooLarge):
		return fmt.Errorf("%w: %w", ErrObjectTooLarge, err)
	}
	return err
}

// objects returns the store's object store, opening it on the first call
// through the opener New received. A store built without an opener has
// no object store, and the call is ErrNoStorage.
func (s *Store) objects(ctx context.Context) (*Storage, error) {
	if s.storage != nil {
		return s.storage, nil
	}
	if s.openStorage == nil {
		return nil, ErrNoStorage
	}
	st, err := s.openStorage(ctx)
	if err != nil {
		return nil, err
	}
	s.storage = st
	return st, nil
}
