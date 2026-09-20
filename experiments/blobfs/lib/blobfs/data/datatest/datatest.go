// Package datatest is the conformance suite of the persistence package's
// variation points: the checks every data.Variant must pass, run through a
// data.Store built over the variant, against a live database. The
// persistence package's own tests run it over the standard baseline,
// pgnative's tests run it over the Postgres variant, and a consumer that
// supplies a variant of its own runs it over that. It lives in a package of
// its own because a test helper in a _test.go file cannot be imported by
// another package's tests.
//
// The suite takes the database from the caller: a throwaway database with
// blobfs's migration set applied, opened however the caller's test tier
// opens one. It seeds directories through the store and file rows through
// plain SQL, since the write path is a later stage.
package datatest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

// Run runs the suite as subtests of t: the file-delete begin's contract
// (the row returned and left deleting, the version advanced once, a retry
// converging, a rollback undoing it, a missing file as
// blobfs.ErrNotFound) and the tree lock's (a pool session refused, and the
// lock held to commit and to rollback when the store serializes, or a
// documented no-op that never blocks when it does not). db is a migrated
// throwaway database and store was built over the variant under test with
// db's dialect.
func Run(t *testing.T, db *sqlate.DB, store *data.Store) {
	t.Helper()
	ctx := context.Background()
	s := suite{t: t, ctx: ctx, db: db, store: store}
	t.Run("BeginFileDelete", s.beginFileDelete)
	t.Run("LockTree", s.lockTree)
}

// suite is one run's state.
type suite struct {
	t     *testing.T
	ctx   context.Context
	db    *sqlate.DB
	store *data.Store
}

// beginFileDelete checks the file-delete begin against its contract.
func (s *suite) beginFileDelete(t *testing.T) {
	dir := s.mkdir(t, "delete-"+t.Name())
	available := s.insertFile(t, dir.ID, "available.txt", blobfs.StatusAvailable)
	pending := s.insertFile(t, dir.ID, "pending.txt", blobfs.StatusPending)

	t.Run("ReturnsTheDeletingRow", func(t *testing.T) {
		before := s.file(t, available)
		got := s.begin(t, available)
		s.wantDeleting(t, before, got)
		after := s.file(t, available)
		if !equalFile(got, after) {
			t.Errorf("begin returned\n%+v\nbut the database holds\n%+v", got, after)
		}
	})
	t.Run("FromPending", func(t *testing.T) {
		before := s.file(t, pending)
		got := s.begin(t, pending)
		s.wantDeleting(t, before, got)
	})
	t.Run("RetryConverges", func(t *testing.T) {
		once := s.file(t, available)
		if once.Status != blobfs.StatusDeleting {
			t.Fatalf("the row is %s, want deleting from the earlier begin", once.Status)
		}
		again := s.begin(t, available)
		if !equalFile(once, again) {
			t.Errorf("a second begin returned\n%+v\nwant the row unchanged\n%+v", again, once)
		}
		stored := s.file(t, available)
		if !equalFile(once, stored) {
			t.Errorf("a second begin changed the row to\n%+v\nfrom\n%+v", stored, once)
		}
	})
	t.Run("RollbackUndoesIt", func(t *testing.T) {
		id := s.insertFile(t, dir.ID, "rolled-back.txt", blobfs.StatusAvailable)
		before := s.file(t, id)
		tx, err := s.db.Begin(s.ctx)
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		got, err := s.store.BeginFileDelete(s.ctx, tx, id)
		if err != nil {
			_ = tx.Rollback()
			t.Fatalf("BeginFileDelete: %v", err)
		}
		if got.Status != blobfs.StatusDeleting {
			t.Errorf("inside the transaction the row is %s, want deleting", got.Status)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatalf("Rollback: %v", err)
		}
		if after := s.file(t, id); !equalFile(before, after) {
			t.Errorf("after rollback the row is\n%+v\nwant it unchanged\n%+v", after, before)
		}
	})
	t.Run("MissingIsNotFound", func(t *testing.T) {
		_, err := s.db.Transact(s.ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
			return s.store.BeginFileDelete(s.ctx, tx, blobfs.NewID())
		})
		if !errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("BeginFileDelete(missing) = %v, want ErrNotFound", err)
		}
	})
}

// lockTree checks the tree lock against its contract, by what Serializes
// reports.
func (s *suite) lockTree(t *testing.T) {
	t.Run("PoolRefused", func(t *testing.T) {
		if err := s.store.LockTree(s.ctx, s.db); !errors.Is(err, query.ErrTransactionRequired) {
			t.Errorf("LockTree on the pool = %v, want ErrTransactionRequired", err)
		}
	})
	if s.store.Serializes() {
		t.Run("HeldToCommit", func(t *testing.T) { s.heldUntil(t, (*sqlate.Tx).Commit) })
		t.Run("HeldToRollback", func(t *testing.T) { s.heldUntil(t, (*sqlate.Tx).Rollback) })
		return
	}
	t.Run("NoOpNeverBlocks", func(t *testing.T) {
		first := s.beginTx(t)
		defer func() { _ = first.Rollback() }()
		if err := s.store.LockTree(s.ctx, first); err != nil {
			t.Fatalf("first LockTree: %v", err)
		}
		second := s.beginTx(t)
		defer func() { _ = second.Rollback() }()
		select {
		case err := <-s.lockIn(second):
			if err != nil {
				t.Fatalf("second LockTree: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the second LockTree blocked while the first transaction held the lock; a variant that does not serialize must not block")
		}
	})
}

// heldUntil proves a second transaction's LockTree blocks while the first
// holds the lock and returns once the first ends through end.
func (s *suite) heldUntil(t *testing.T, end func(*sqlate.Tx) error) {
	first := s.beginTx(t)
	ended := false
	defer func() {
		if !ended {
			_ = first.Rollback()
		}
	}()
	if err := s.store.LockTree(s.ctx, first); err != nil {
		t.Fatalf("first LockTree: %v", err)
	}
	second := s.beginTx(t)
	defer func() { _ = second.Rollback() }()
	done := s.lockIn(second)
	select {
	case err := <-done:
		t.Fatalf("the second LockTree returned (%v) while the first transaction held the lock", err)
	case <-time.After(500 * time.Millisecond):
	}
	ended = true
	if err := end(first); err != nil {
		t.Fatalf("ending the first transaction: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("second LockTree after the first ended: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the second LockTree still blocks after the first transaction ended")
	}
}

// lockIn takes the tree lock in tx on a goroutine and reports the result
// on the channel.
func (s *suite) lockIn(tx *sqlate.Tx) <-chan error {
	done := make(chan error, 1)
	go func() { done <- s.store.LockTree(s.ctx, tx) }()
	return done
}

// beginTx opens a transaction on the pool, failing the test when it
// cannot.
func (s *suite) beginTx(t *testing.T) *sqlate.Tx {
	t.Helper()
	tx, err := s.db.Begin(s.ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	return tx
}

// begin runs BeginFileDelete in its own transaction and commits it.
func (s *suite) begin(t *testing.T, id string) blobfs.File {
	t.Helper()
	f, err := s.db.Transact(s.ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
		return s.store.BeginFileDelete(s.ctx, tx, id)
	})
	if err != nil {
		t.Fatalf("BeginFileDelete(%s): %v", id, err)
	}
	return f
}

// wantDeleting checks got against before: the same row moved to deleting,
// its version advanced by one, updated_at moved forward, and every other
// column unchanged.
func (s *suite) wantDeleting(t *testing.T, before, got blobfs.File) {
	t.Helper()
	if got.Status != blobfs.StatusDeleting {
		t.Errorf("status = %s, want deleting", got.Status)
	}
	if got.Version != before.Version+1 {
		t.Errorf("version = %d, want %d", got.Version, before.Version+1)
	}
	if got.UpdatedAt.Before(before.UpdatedAt) || got.UpdatedAt.Equal(before.UpdatedAt) {
		t.Errorf("updated_at = %v, want later than %v", got.UpdatedAt, before.UpdatedAt)
	}
	want := before
	want.Status, want.Version, want.UpdatedAt = got.Status, got.Version, got.UpdatedAt
	if !equalFile(want, got) {
		t.Errorf("begin changed more than the status, version, and updated_at:\n%+v\nfrom\n%+v", got, before)
	}
}

// file reads the row by id through the store, on the pool.
func (s *suite) file(t *testing.T, id string) blobfs.File {
	t.Helper()
	f, err := s.store.File(s.ctx, s.db, id)
	if err != nil {
		t.Fatalf("File(%s): %v", id, err)
	}
	return f
}

// mkdir creates a directory under the root.
func (s *suite) mkdir(t *testing.T, name string) blobfs.Directory {
	t.Helper()
	name = strings.NewReplacer("/", "-", "\\", "-").Replace(name)
	d, err := s.store.Mkdir(s.ctx, s.db, blobfs.RootID, name)
	if err != nil {
		t.Fatalf("Mkdir(%q): %v", name, err)
	}
	return d
}

// insertFile inserts a file row in status through plain SQL, with the
// dialect's placeholders, and returns its id. The write path is a later
// stage, so the suite seeds rows itself.
func (s *suite) insertFile(t *testing.T, dir, name string, status blobfs.Status) string {
	t.Helper()
	id := blobfs.NewID()
	p := s.db.Dialect().Placeholder
	text := fmt.Sprintf(
		"INSERT INTO blobfs_file (id, directory_id, name, status, key, content_type, size) VALUES (%s, %s, %s, %s, %s, 'text/plain', %s)",
		p(1), p(2), p(3), p(4), p(5), p(6))
	if _, err := s.db.ExecContext(s.ctx, text, id, dir, name, status.String(), id+"/"+name, len(name)); err != nil {
		t.Fatalf("insert file %s: %v", name, err)
	}
	return id
}

// equalFile compares two rows field by field, timestamps by instant.
func equalFile(a, b blobfs.File) bool {
	return a.ID == b.ID && a.DirectoryID == b.DirectoryID && a.Name == b.Name && a.Status == b.Status &&
		a.Key == b.Key && equalInt(a.Size, b.Size) && a.ContentType == b.ContentType && equalString(a.ETag, b.ETag) &&
		a.Version == b.Version && a.CreatedAt.Equal(b.CreatedAt) && a.UpdatedAt.Equal(b.UpdatedAt)
}

func equalInt(a, b *int64) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func equalString(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
