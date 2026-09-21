// Package datatest is the conformance suite of the persistence package's
// variation points: the checks every data.Variant must pass, run through a
// data.Store built over the variant, against a live database. The
// persistence package's own tests run it over the standard baseline,
// the postgres package's tests run it over the Postgres variant, and a consumer that
// supplies a variant of its own runs it over that. It lives in a package of
// its own because a test helper in a _test.go file cannot be imported by
// another package's tests.
//
// The suite takes the database from the caller: a throwaway database with
// blobfs's migration set applied, opened however the caller's test tier
// opens one, and Run is called once per database. It seeds directories
// through the store and file rows through plain SQL, so it needs no key
// validator, and it creates one table of its own that references
// blobfs_file, to stand in for a consumer's foreign key.
package datatest

import (
	"context"
	"database/sql"
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
// blobfs.ErrNotFound); the whole delete protocol through the variant's
// begin and the shared complete step (the deleting row removed, a retry
// of every step converging, a pending row deletable, a row that is not
// deleting refused, and a row a consumer's foreign key references left
// deleting until the reference goes); the tree lock's (a pool session
// refused, and the lock held to commit and to rollback when the store
// serializes, or a documented no-op that never blocks when it does not);
// and the directory move's through the lock (the sequential contract on
// every variant, then two opposing concurrent moves, which leave no
// cycle and refuse one of the two when the store serializes, and form a
// cycle when it does not, which the suite asserts and then repairs); and
// the hold's against the variant's begin (the pool refused, the held row
// unchanged, a deleting row and a stale version refused, and the two
// interleavings serialized on the row: a hold makes the begin wait until
// the holder's transaction ends, and a begin makes the hold wait and then
// refuse once the begin commits, or succeed once it rolls back). db is a
// migrated throwaway database and store was built over the variant under
// test with db's dialect. The move group builds a second store of its
// own, over a wrapper of the store's variant that pauses each move after
// its lock, so the two moves interleave deterministically.
func Run(t *testing.T, db *sqlate.DB, store *data.Store) {
	t.Helper()
	ctx := context.Background()
	s := suite{t: t, ctx: ctx, db: db, store: store}
	t.Run("BeginFileDelete", s.beginFileDelete)
	t.Run("FileDelete", s.fileDelete)
	t.Run("HoldFile", s.holdFile)
	t.Run("LockTree", s.lockTree)
	t.Run("MoveDirectory", s.moveDirectory)
}

// suite is one run's state.
type suite struct {
	t     *testing.T
	ctx   context.Context
	db    *sqlate.DB
	store *data.Store
	// referenced records that createReference ran, so the groups that
	// need the reference table share one.
	referenced bool
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

// fileDelete checks the delete protocol end to end: the variant's begin
// followed by the shared complete step, and the idempotence that lets a
// retry finish after a stop at any step.
func (s *suite) fileDelete(t *testing.T) {
	dir := s.mkdir(t, "protocol-"+t.Name())

	t.Run("CompleteRemovesTheDeletingRow", func(t *testing.T) {
		id := s.insertFile(t, dir.ID, "available.txt", blobfs.StatusAvailable)
		s.begin(t, id)
		s.complete(t, id)
		s.wantGone(t, id)
	})
	t.Run("PendingIsDeletable", func(t *testing.T) {
		id := s.insertFile(t, dir.ID, "pending.txt", blobfs.StatusPending)
		if f := s.begin(t, id); f.Status != blobfs.StatusDeleting {
			t.Fatalf("begin from pending left the row %s", f.Status)
		}
		s.complete(t, id)
		s.wantGone(t, id)
	})
	t.Run("RetryAtEachStepConverges", func(t *testing.T) {
		id := s.insertFile(t, dir.ID, "retried.txt", blobfs.StatusAvailable)
		// A stop after the begin: the retry begins again and sees the same
		// row, then completes.
		once := s.begin(t, id)
		again := s.begin(t, id)
		if !equalFile(once, again) {
			t.Errorf("the retried begin returned\n%+v\nwant the same row\n%+v", again, once)
		}
		// A stop after the object delete: the complete step runs once, and
		// a retry that runs it again finds the row gone and succeeds.
		s.complete(t, id)
		s.complete(t, id)
		s.wantGone(t, id)
		// A retry that begins again after the complete step reports the
		// file gone, which is how a caller learns the delete finished.
		_, err := s.db.Transact(s.ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
			return s.store.BeginFileDelete(s.ctx, tx, id)
		})
		if !errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("a begin after the complete = %v, want ErrNotFound", err)
		}
		// A complete of an id that never existed is the same success.
		s.complete(t, blobfs.NewID())
	})
	t.Run("NotDeletingIsRefused", func(t *testing.T) {
		for _, status := range []blobfs.Status{blobfs.StatusAvailable, blobfs.StatusPending} {
			id := s.insertFile(t, dir.ID, "untouched-"+status.String()+".txt", status)
			before := s.file(t, id)
			err := s.store.CompleteFileDelete(s.ctx, s.db, id)
			if !errors.Is(err, blobfs.ErrNotDeleting) || !strings.Contains(err.Error(), status.String()) {
				t.Errorf("CompleteFileDelete of a %s row = %v, want ErrNotDeleting naming the status", status, err)
			}
			if after := s.file(t, id); !equalFile(before, after) {
				t.Errorf("the refused complete changed the %s row to\n%+v\nfrom\n%+v", status, after, before)
			}
		}
	})
	t.Run("ReferencedRowStaysDeleting", func(t *testing.T) {
		s.createReference(t)
		id := s.insertFile(t, dir.ID, "referenced.txt", blobfs.StatusAvailable)
		s.reference(t, id)
		deleting := s.begin(t, id)
		err := s.store.CompleteFileDelete(s.ctx, s.db, id)
		var ce *sqlate.ConstraintError
		if !errors.Is(err, blobfs.ErrReferenced) || !errors.As(err, &ce) || !errors.Is(ce.Class, sqlate.ErrForeignKeyViolation) || ce.Constraint != referenceConstraint {
			t.Fatalf("CompleteFileDelete of a referenced row = %v, want ErrReferenced over the consumer's constraint %q", err, referenceConstraint)
		}
		if errors.Is(err, blobfs.ErrNotEmpty) {
			t.Errorf("the consumer's key classified as blobfs's own: %v", err)
		}
		if after := s.file(t, id); !equalFile(deleting, after) {
			t.Errorf("the refused complete changed the row to\n%+v\nfrom\n%+v", after, deleting)
		}
		s.unreference(t, id)
		s.complete(t, id)
		s.wantGone(t, id)
	})
}

// holdFile checks the hold against its contract and against the variant's
// begin: the two must take the same row lock, whatever statement the
// variant's begin runs.
func (s *suite) holdFile(t *testing.T) {
	dir := s.mkdir(t, "hold-"+t.Name())

	t.Run("PoolRefused", func(t *testing.T) {
		id := s.insertFile(t, dir.ID, "pool.txt", blobfs.StatusAvailable)
		before := s.file(t, id)
		if err := s.store.HoldFile(s.ctx, s.db, id); !errors.Is(err, query.ErrTransactionRequired) {
			t.Errorf("HoldFile on the pool = %v, want ErrTransactionRequired", err)
		}
		if after := s.file(t, id); !equalFile(before, after) {
			t.Errorf("the refused hold changed the row to\n%+v\nfrom\n%+v", after, before)
		}
	})
	t.Run("HeldRowIsUnchanged", func(t *testing.T) {
		for _, status := range []blobfs.Status{blobfs.StatusAvailable, blobfs.StatusPending} {
			id := s.insertFile(t, dir.ID, "held-"+status.String()+".txt", status)
			before := s.file(t, id)
			if _, err := s.db.Transact(s.ctx, func(tx *sqlate.Tx) (struct{}, error) {
				return struct{}{}, s.store.HoldFile(s.ctx, tx, id)
			}); err != nil {
				t.Fatalf("HoldFile of a %s row: %v", status, err)
			}
			if _, err := s.db.Transact(s.ctx, func(tx *sqlate.Tx) (struct{}, error) {
				return struct{}{}, s.store.HoldFile(s.ctx, tx, id, data.AtVersion(before.Version))
			}); err != nil {
				t.Fatalf("HoldFile of a %s row at its version: %v", status, err)
			}
			if after := s.file(t, id); !equalFile(before, after) {
				t.Errorf("the holds changed the %s row to\n%+v\nfrom\n%+v", status, after, before)
			}
		}
	})
	t.Run("StaleVersionRefused", func(t *testing.T) {
		id := s.insertFile(t, dir.ID, "stale.txt", blobfs.StatusAvailable)
		before := s.file(t, id)
		_, err := s.db.Transact(s.ctx, func(tx *sqlate.Tx) (struct{}, error) {
			return struct{}{}, s.store.HoldFile(s.ctx, tx, id, data.AtVersion(before.Version+1))
		})
		if !errors.Is(err, query.ErrVersionMismatch) || errors.Is(err, blobfs.ErrDeleting) {
			t.Errorf("HoldFile at a stale version = %v, want ErrVersionMismatch", err)
		}
		if after := s.file(t, id); !equalFile(before, after) {
			t.Errorf("the refused hold changed the row to\n%+v\nfrom\n%+v", after, before)
		}
	})
	t.Run("DeletingRefused", func(t *testing.T) {
		id := s.insertFile(t, dir.ID, "deleting.txt", blobfs.StatusAvailable)
		before := s.file(t, id)
		deleting := s.begin(t, id)
		for _, opts := range [][]data.HoldOption{nil, {data.AtVersion(before.Version)}, {data.AtVersion(deleting.Version)}} {
			_, err := s.db.Transact(s.ctx, func(tx *sqlate.Tx) (struct{}, error) {
				return struct{}{}, s.store.HoldFile(s.ctx, tx, id, opts...)
			})
			if !errors.Is(err, blobfs.ErrDeleting) || errors.Is(err, query.ErrVersionMismatch) {
				t.Errorf("HoldFile of a deleting row with %d options = %v, want ErrDeleting and no version mismatch", len(opts), err)
			}
		}
		if after := s.file(t, id); !equalFile(deleting, after) {
			t.Errorf("the refused holds changed the row to\n%+v\nfrom\n%+v", after, deleting)
		}
	})
	t.Run("MissingIsNotFound", func(t *testing.T) {
		_, err := s.db.Transact(s.ctx, func(tx *sqlate.Tx) (struct{}, error) {
			return struct{}{}, s.store.HoldFile(s.ctx, tx, blobfs.NewID())
		})
		if !errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("HoldFile(missing) = %v, want ErrNotFound", err)
		}
	})
	t.Run("HoldMakesTheBeginWait", func(t *testing.T) {
		// The holder inserts its reference under the hold; the begin waits
		// for the holder to commit and then runs, and the reference is
		// there for the delete's own check to see.
		s.createReference(t)
		id := s.insertFile(t, dir.ID, "held-then-deleted.txt", blobfs.StatusAvailable)
		holder := s.beginTx(t)
		ended := false
		defer func() {
			if !ended {
				_ = holder.Rollback()
			}
		}()
		if err := s.store.HoldFile(s.ctx, holder, id); err != nil {
			t.Fatalf("HoldFile: %v", err)
		}
		if _, err := holder.ExecContext(s.ctx, "INSERT INTO datatest_reference (file_id) VALUES ("+s.db.Dialect().Placeholder(1)+")", id); err != nil {
			t.Fatalf("reference under the hold: %v", err)
		}
		begun := s.beginIn(id)
		select {
		case r := <-begun:
			t.Fatalf("the begin returned (%+v, %v) while the hold's transaction was open", r.file, r.err)
		case <-time.After(500 * time.Millisecond):
		}
		ended = true
		if err := holder.Commit(); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		r := s.await(t, begun)
		if r.err != nil || r.file.Status != blobfs.StatusDeleting {
			t.Fatalf("the begin after the hold committed = %+v, %v; want the deleting row", r.file, r.err)
		}
		if n := s.references(t, id); n != 1 {
			t.Errorf("%d references exist after the begin, want the one the holder committed", n)
		}
		s.unreference(t, id)
		s.complete(t, id)
		s.wantGone(t, id)
	})
	t.Run("BeginMakesTheHoldWait", func(t *testing.T) {
		// The begin runs first and the hold waits; a commit of the begin
		// leaves the hold refusing the deleting row, and a rollback leaves
		// it holding the row.
		for _, end := range []struct {
			name string
			end  func(*sqlate.Tx) error
			want error
		}{
			{"Commit", (*sqlate.Tx).Commit, blobfs.ErrDeleting},
			{"Rollback", (*sqlate.Tx).Rollback, nil},
		} {
			t.Run(end.name, func(t *testing.T) {
				id := s.insertFile(t, dir.ID, "begun-then-held-"+end.name+".txt", blobfs.StatusAvailable)
				deleter := s.beginTx(t)
				ended := false
				defer func() {
					if !ended {
						_ = deleter.Rollback()
					}
				}()
				if _, err := s.store.BeginFileDelete(s.ctx, deleter, id); err != nil {
					t.Fatalf("BeginFileDelete: %v", err)
				}
				holder := s.beginTx(t)
				defer func() { _ = holder.Rollback() }()
				held := make(chan error, 1)
				go func() { held <- s.store.HoldFile(s.ctx, holder, id) }()
				select {
				case err := <-held:
					t.Fatalf("the hold returned (%v) while the begin's transaction was open", err)
				case <-time.After(500 * time.Millisecond):
				}
				ended = true
				if err := end.end(deleter); err != nil {
					t.Fatalf("ending the begin's transaction: %v", err)
				}
				select {
				case err := <-held:
					if !errors.Is(err, end.want) {
						t.Errorf("the hold after the begin's %s = %v, want %v", end.name, err, end.want)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("the hold still blocks after the begin's transaction ended")
				}
			})
		}
	})
}

// beginResult is what a begin on a goroutine reports.
type beginResult struct {
	file blobfs.File
	err  error
}

// beginIn runs BeginFileDelete in a transaction of its own on a
// goroutine, commits it, and reports the result on the channel.
func (s *suite) beginIn(id string) <-chan beginResult {
	done := make(chan beginResult, 1)
	go func() {
		f, err := s.db.Transact(s.ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
			return s.store.BeginFileDelete(s.ctx, tx, id)
		})
		done <- beginResult{file: f, err: err}
	}()
	return done
}

// await returns the begin's result, failing the test when it does not
// arrive within a bound.
func (s *suite) await(t *testing.T, begun <-chan beginResult) beginResult {
	t.Helper()
	select {
	case r := <-begun:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("the begin still blocks after the hold's transaction ended")
		return beginResult{}
	}
}

// references counts the rows that reference the file with id.
func (s *suite) references(t *testing.T, id string) int {
	t.Helper()
	rows, err := s.db.QueryContext(s.ctx, "SELECT COUNT(*) FROM datatest_reference WHERE file_id = "+s.db.Dialect().Placeholder(1), id)
	if err != nil {
		t.Fatalf("count references of %s: %v", id, err)
	}
	defer func() { _ = rows.Close() }()
	var n int
	if !rows.Next() || rows.Scan(&n) != nil {
		t.Fatalf("count references of %s: no row: %v", id, rows.Err())
	}
	return n
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

// moveDirectory checks the directory move against its contract: the
// sequential outcomes, which are the same on every variant, then the two
// opposing concurrent moves, whose outcome is what Serializes promises.
func (s *suite) moveDirectory(t *testing.T) {
	t.Run("RootRefused", func(t *testing.T) {
		root := s.directory(t, blobfs.RootID)
		_, err := s.db.Transact(s.ctx, func(tx *sqlate.Tx) (blobfs.Directory, error) {
			return s.store.MoveDirectory(s.ctx, tx, blobfs.RootID, root.ID, "root", root.Version)
		})
		if !errors.Is(err, blobfs.ErrRootDirectory) {
			t.Errorf("MoveDirectory(root) = %v, want ErrRootDirectory", err)
		}
		if after := s.directory(t, blobfs.RootID); !equalDirectory(after, root) {
			t.Errorf("the root changed to %+v from %+v", after, root)
		}
	})
	t.Run("PoolRefused", func(t *testing.T) {
		d := s.mkdir(t, "pool-"+t.Name())
		if _, err := s.store.MoveDirectory(s.ctx, s.db, d.ID, blobfs.RootID, "renamed", d.Version); !errors.Is(err, query.ErrTransactionRequired) {
			t.Errorf("MoveDirectory on the pool = %v, want ErrTransactionRequired", err)
		}
		if after := s.directory(t, d.ID); !equalDirectory(after, d) {
			t.Errorf("the refused move changed the row to %+v from %+v", after, d)
		}
	})
	t.Run("IntoOwnSubtreeRefused", func(t *testing.T) {
		a := s.mkdir(t, "cycle-"+t.Name())
		b := s.mkdirUnder(t, a.ID, "b")
		c := s.mkdirUnder(t, b.ID, "c")
		for _, target := range []blobfs.Directory{a, b, c} {
			_, err := s.move(t, a.ID, target.ID, "a", a.Version)
			if !errors.Is(err, blobfs.ErrCycle) {
				t.Errorf("MoveDirectory of a under %s = %v, want ErrCycle", target.Name, err)
			}
		}
		if after := s.directory(t, a.ID); !equalDirectory(after, a) {
			t.Errorf("the refused moves changed the row to %+v from %+v", after, a)
		}
	})
	t.Run("MovesTheSubtree", func(t *testing.T) {
		src := s.mkdir(t, "src-"+t.Name())
		dst := s.mkdir(t, "dst-"+t.Name())
		x := s.mkdirUnder(t, src.ID, "x")
		y := s.mkdirUnder(t, x.ID, "y")
		fx := s.insertFile(t, x.ID, "in-x.txt", blobfs.StatusAvailable)
		fy := s.insertFile(t, y.ID, "in-y.txt", blobfs.StatusAvailable)
		fileBefore := s.file(t, fx)
		moved, err := s.move(t, x.ID, dst.ID, "x", x.Version)
		if err != nil {
			t.Fatalf("MoveDirectory: %v", err)
		}
		if *moved.ParentID != dst.ID || moved.Name != "x" || moved.Version != x.Version+1 || !moved.UpdatedAt.After(x.UpdatedAt) || !moved.CreatedAt.Equal(x.CreatedAt) {
			t.Errorf("the moved row is %+v, want it under dst at the next version", moved)
		}
		if after := s.directory(t, x.ID); !equalDirectory(after, moved) {
			t.Errorf("MoveDirectory returned %+v but the database holds %+v", moved, after)
		}
		s.wantPath(t, x.ID, "/"+dst.Name+"/x")
		s.wantPath(t, y.ID, "/"+dst.Name+"/x/y")
		if after := s.directory(t, y.ID); !equalDirectory(after, y) {
			t.Errorf("the child changed to %+v from %+v; it follows its parent by id", after, y)
		}
		if after := s.file(t, fx); !equalFile(after, fileBefore) {
			t.Errorf("the file changed to %+v from %+v; it follows its directory by id", after, fileBefore)
		}
		if f := s.file(t, fy); f.DirectoryID != y.ID {
			t.Errorf("the deep file is in %s, want %s", f.DirectoryID, y.ID)
		}
		if _, err := s.store.ResolveDirectory(s.ctx, s.db, "/"+src.Name+"/x"); !errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("the old path still resolves: %v", err)
		}
	})
	t.Run("Renames", func(t *testing.T) {
		d := s.mkdir(t, "rename-"+t.Name())
		child := s.mkdirUnder(t, d.ID, "child")
		renamed, err := s.move(t, d.ID, blobfs.RootID, s.name("renamed-"+t.Name()), d.Version)
		if err != nil {
			t.Fatalf("MoveDirectory as a rename: %v", err)
		}
		if *renamed.ParentID != blobfs.RootID || renamed.Name != s.name("renamed-"+t.Name()) || renamed.Version != d.Version+1 {
			t.Errorf("the renamed row is %+v", renamed)
		}
		s.wantPath(t, child.ID, "/"+renamed.Name+"/child")
	})
	t.Run("NameTaken", func(t *testing.T) {
		p := s.mkdir(t, "taken-"+t.Name())
		s.mkdirUnder(t, p.ID, "held")
		d := s.mkdir(t, "mover-"+t.Name())
		_, err := s.move(t, d.ID, p.ID, "held", d.Version)
		if !errors.Is(err, blobfs.ErrNameTaken) {
			t.Errorf("MoveDirectory under a taken name = %v, want ErrNameTaken", err)
		}
		var ce *sqlate.ConstraintError
		if !errors.As(err, &ce) || ce.Constraint != blobfs.ConstraintUniqueDirectoryParentName {
			t.Errorf("the refusal does not carry blobfs_uq_directory_parent_name: %v", err)
		}
		// A file of the same name is no conflict: the name spaces are
		// separate.
		s.insertFile(t, p.ID, "shared", blobfs.StatusAvailable)
		if _, err := s.move(t, d.ID, p.ID, "shared", d.Version); err != nil {
			t.Errorf("MoveDirectory under the name of a file = %v, want the move to succeed", err)
		}
	})
	t.Run("MissingParent", func(t *testing.T) {
		d := s.mkdir(t, "orphan-"+t.Name())
		_, err := s.move(t, d.ID, blobfs.NewID(), "d", d.Version)
		if !errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("MoveDirectory under a missing parent = %v, want ErrNotFound", err)
		}
		var ce *sqlate.ConstraintError
		if !errors.As(err, &ce) || ce.Constraint != blobfs.ConstraintForeignKeyDirectoryParent {
			t.Errorf("the refusal does not carry blobfs_fk_directory_parent: %v", err)
		}
		if after := s.directory(t, d.ID); !equalDirectory(after, d) {
			t.Errorf("the refused move changed the row to %+v from %+v", after, d)
		}
	})
	t.Run("MissingDirectory", func(t *testing.T) {
		if _, err := s.move(t, blobfs.NewID(), blobfs.RootID, "ghost", 1); !errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("MoveDirectory of a missing directory = %v, want ErrNotFound", err)
		}
	})
	t.Run("StaleVersion", func(t *testing.T) {
		d := s.mkdir(t, "stale-"+t.Name())
		if _, err := s.move(t, d.ID, blobfs.RootID, s.name("stale-once-"+t.Name()), d.Version); err != nil {
			t.Fatalf("the first rename: %v", err)
		}
		if _, err := s.move(t, d.ID, blobfs.RootID, s.name("stale-twice-"+t.Name()), d.Version); !errors.Is(err, query.ErrVersionMismatch) {
			t.Errorf("MoveDirectory at the version before the rename = %v, want ErrVersionMismatch", err)
		}
	})
	t.Run("OpposingConcurrentMoves", s.opposingMoves)
	t.Run("OpposingSerializableMoves", s.opposingSerializableMoves)
}

// opposingMoves is the gate: transaction A moves X under Y while
// transaction B moves Y under X, each through the full MoveDirectory, and
// the two are interleaved through a wrapper variant that pauses each move
// once its lock call has returned, with the commits held by the suite. A
// starts first and reaches the pause with the lock held; B then starts.
// When the store serializes, B blocks inside the lock while A holds it,
// through A's check and update and until A commits; B's lock then
// returns, and B's check sees X under Y and refuses with ErrCycle: no
// cycle exists. When the store does not serialize, B passes the no-op
// lock at once, A's check and update run and stay uncommitted, B's check
// then runs against the same committed state and passes, B's update runs,
// both commit, and X and Y are each other's ancestor and unreachable from
// the root, which the suite asserts with a walk down from the root and a
// bounded walk up from each, and then repairs.
func (s *suite) opposingMoves(t *testing.T) {
	x := s.mkdir(t, "x-"+t.Name())
	y := s.mkdir(t, "y-"+t.Name())
	g := &gated{Variant: s.store.Variant(), arrived: make(chan chan struct{})}
	catalog, err := query.NewCatalog(query.Patterns(), data.Patterns())
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	store, err := data.New(catalog, s.db.Dialect(), data.WithVariant(g))
	if err != nil {
		t.Fatalf("data.New over the gated variant: %v", err)
	}
	const wait = 5 * time.Second
	arrived := func(who string) chan struct{} {
		t.Helper()
		select {
		case release := <-g.arrived:
			return release
		case <-time.After(wait):
			t.Fatalf("%s never reached the pause after its lock", who)
			return nil
		}
	}
	notArrived := func(who string) {
		t.Helper()
		select {
		case <-g.arrived:
			t.Fatalf("%s passed the lock while the other transaction held it; the store reports it serializes", who)
		case <-time.After(500 * time.Millisecond):
		}
	}

	a := s.startMove(store, x.ID, y.ID, x.Version)
	releaseA := arrived("A")
	b := s.startMove(store, y.ID, x.ID, y.Version)

	if s.store.Serializes() {
		notArrived("B")
		close(releaseA)
		if err := <-a.moved; err != nil {
			t.Fatalf("A's move: %v", err)
		}
		notArrived("B")
		a.commit <- struct{}{}
		if err := <-a.done; err != nil {
			t.Fatalf("A's commit: %v", err)
		}
		close(arrived("B"))
		if err := <-b.moved; !errors.Is(err, blobfs.ErrCycle) {
			t.Fatalf("B's move = %v, want ErrCycle: its check ran after A's commit", err)
		}
		if err := <-b.done; err != nil {
			t.Fatalf("B's rollback: %v", err)
		}
		s.wantPath(t, x.ID, "/"+y.Name+"/moved")
		s.wantPath(t, y.ID, "/"+y.Name)
		if n := s.reachable(t, x.ID, y.ID); n != 2 {
			t.Errorf("%d of the two directories are reachable from the root, want both", n)
		}
		if s.ownAncestor(t, x.ID) || s.ownAncestor(t, y.ID) {
			t.Error("a directory is its own ancestor: a cycle formed under a serializing lock")
		}
		return
	}

	releaseB := arrived("B")
	close(releaseA)
	if err := <-a.moved; err != nil {
		t.Fatalf("A's move on the baseline: %v", err)
	}
	// A's update is uncommitted, so B's check sees X still under the root
	// and passes. B's update then waits on the row locks A's update took
	// (an update of parent_id is a key update on Postgres, since the
	// column is in a unique constraint, and the foreign-key check on the
	// new parent shares the same rows), so B returns only once A commits;
	// a B that returned ErrCycle here would mean its check ran after A's
	// commit, which the no-op lock cannot cause.
	close(releaseB)
	select {
	case err := <-b.moved:
		if err != nil {
			t.Fatalf("B's move on the baseline = %v; its check ran before A committed, so nothing refused it", err)
		}
	case <-time.After(500 * time.Millisecond):
	}
	a.commit <- struct{}{}
	if err := <-a.done; err != nil {
		t.Fatalf("A's commit: %v", err)
	}
	select {
	case err := <-b.moved:
		if err != nil {
			t.Fatalf("B's move on the baseline = %v after A's commit; its check had passed already", err)
		}
	case <-time.After(wait):
		t.Fatal("B's move did not return after A committed")
	}
	b.commit <- struct{}{}
	if err := <-b.done; err != nil {
		t.Fatalf("B's commit: %v", err)
	}
	// The proof that the baseline forms a cycle: both moves committed, X is
	// under Y and Y is under X, neither is reachable from the root, and
	// each is its own ancestor.
	if xr, yr := s.directory(t, x.ID), s.directory(t, y.ID); *xr.ParentID != y.ID || *yr.ParentID != x.ID {
		t.Fatalf("after both commits x's parent is %s and y's is %s, want each the other", *xr.ParentID, *yr.ParentID)
	}
	if n := s.reachable(t, x.ID, y.ID); n != 0 {
		t.Errorf("%d of the two directories are reachable from the root, want none: they form a cycle detached from the tree", n)
	}
	if !s.ownAncestor(t, x.ID) || !s.ownAncestor(t, y.ID) {
		t.Error("the two directories are not each their own ancestor; the baseline formed no cycle")
	}
	// Repair, so the rest of the database stays walkable: Y goes back
	// under the root, and X stays under Y.
	if _, err := s.db.ExecContext(s.ctx, "UPDATE blobfs_directory SET parent_id = "+s.db.Dialect().Placeholder(1)+" WHERE id = "+s.db.Dialect().Placeholder(2), blobfs.RootID, y.ID); err != nil {
		t.Fatalf("repair: %v", err)
	}
	if n := s.reachable(t, x.ID, y.ID); n != 2 {
		t.Errorf("after the repair %d of the two directories are reachable, want both", n)
	}
}

// opposingSerializableMoves is the standard-tier alternative to the lock,
// measured on every variant: the same two opposing moves, each in a
// transaction the caller opened at serializable isolation, interleaved as
// on the baseline (both past their locks, A's update uncommitted when B's
// check runs). The engine then refuses one of the two, at its update or
// at its commit, with a serialization failure (SQLSTATE 40001, which
// sqlate leaves unmapped), and no cycle forms. On a serializing variant
// B's snapshot predates A's commit all the same, because the lock
// statement is B's first and takes the snapshot before it blocks, so B is
// refused at its update instead of at its check.
func (s *suite) opposingSerializableMoves(t *testing.T) {
	x := s.mkdir(t, "sx-"+t.Name())
	y := s.mkdir(t, "sy-"+t.Name())
	g := &gated{Variant: s.store.Variant(), arrived: make(chan chan struct{})}
	catalog, err := query.NewCatalog(query.Patterns(), data.Patterns())
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	store, err := data.New(catalog, s.db.Dialect(), data.WithVariant(g))
	if err != nil {
		t.Fatalf("data.New over the gated variant: %v", err)
	}
	const wait = 5 * time.Second
	serializable := sqlate.Isolation(sql.LevelSerializable)
	arrived := func(who string) chan struct{} {
		t.Helper()
		select {
		case release := <-g.arrived:
			return release
		case <-time.After(wait):
			t.Fatalf("%s never reached the pause after its lock", who)
			return nil
		}
	}
	a := s.startMove(store, x.ID, y.ID, x.Version, serializable)
	releaseA := arrived("A")
	b := s.startMove(store, y.ID, x.ID, y.Version, serializable)
	var releaseB chan struct{}
	if !s.store.Serializes() {
		releaseB = arrived("B")
	}
	close(releaseA)
	if err := <-a.moved; err != nil {
		t.Fatalf("A's move: %v", err)
	}
	if s.store.Serializes() {
		a.commit <- struct{}{}
		if err := <-a.done; err != nil {
			t.Fatalf("A's commit: %v", err)
		}
		releaseB = arrived("B")
	}
	close(releaseB)
	if !s.store.Serializes() {
		// B's update waits on A's row locks; A commits meanwhile.
		time.Sleep(200 * time.Millisecond)
		a.commit <- struct{}{}
		if err := <-a.done; err != nil {
			t.Fatalf("A's commit: %v", err)
		}
	}
	var refused error
	select {
	case refused = <-b.moved:
	case <-time.After(wait):
		t.Fatal("B's move did not return")
	}
	if refused == nil {
		b.commit <- struct{}{}
		refused = <-b.done
	} else if err := <-b.done; err != nil {
		t.Fatalf("B's rollback: %v", err)
	}
	if refused == nil {
		t.Fatal("B's move committed under serializable isolation; the engine did not refuse the second of two opposing moves")
	}
	var state interface{ SQLState() string }
	if !errors.As(refused, &state) || state.SQLState() != "40001" {
		t.Errorf("B was refused with %v, want a serialization failure (SQLSTATE 40001)", refused)
	}
	if n := s.reachable(t, x.ID, y.ID); n != 2 {
		t.Errorf("%d of the two directories are reachable from the root, want both", n)
	}
	if s.ownAncestor(t, x.ID) || s.ownAncestor(t, y.ID) {
		t.Error("a cycle formed under serializable isolation")
	}
	s.wantPath(t, x.ID, "/"+y.Name+"/moved")
}

// mover is one move under way on a goroutine: moved reports
// MoveDirectory's result once it returns, commit tells the goroutine to
// commit, and done reports the commit, or the rollback that follows a
// refused move.
type mover struct {
	moved  chan error
	commit chan struct{}
	done   chan error
}

// startMove begins a transaction on a goroutine under opts, runs
// MoveDirectory through store in it, and waits for the suite before
// committing.
func (s *suite) startMove(store *data.Store, id, parentID string, version int64, opts ...sqlate.TxOption) *mover {
	m := &mover{moved: make(chan error, 1), commit: make(chan struct{}), done: make(chan error, 1)}
	go func() {
		tx, err := s.db.Begin(s.ctx, opts...)
		if err != nil {
			m.moved <- err
			m.done <- err
			return
		}
		_, err = store.MoveDirectory(s.ctx, tx, id, parentID, "moved", version)
		m.moved <- err
		if err != nil {
			m.done <- tx.Rollback()
			return
		}
		<-m.commit
		m.done <- tx.Commit()
	}()
	return m
}

// gated wraps a variant so that every LockTree, once the wrapped lock has
// returned, sends a release channel of its own on arrived and waits on
// it before it returns. The suite drives two moves through it and
// decides when each proceeds past its lock, in arrival order; on a
// serializing variant a second lock call blocks inside the wrapped lock
// and never reaches arrived until the first transaction ends.
type gated struct {
	data.Variant
	arrived chan chan struct{}
}

func (g *gated) LockTree(ctx context.Context, sess sqlate.Session) error {
	if err := g.Variant.LockTree(ctx, sess); err != nil {
		return err
	}
	release := make(chan struct{})
	g.arrived <- release
	<-release
	return nil
}

// move runs MoveDirectory in a transaction of its own and commits it.
func (s *suite) move(t *testing.T, id, parentID, name string, version int64) (blobfs.Directory, error) {
	t.Helper()
	return s.db.Transact(s.ctx, func(tx *sqlate.Tx) (blobfs.Directory, error) {
		return s.store.MoveDirectory(s.ctx, tx, id, parentID, name, version)
	})
}

// reachable counts how many of the given directories a walk down from the
// root reaches. A directory in a cycle is never reached, because its chain
// of parents never arrives at the root, so the walk terminates whatever
// the two directories' state.
func (s *suite) reachable(t *testing.T, ids ...string) int {
	t.Helper()
	p := s.db.Dialect().Placeholder
	text := "WITH RECURSIVE tree (id) AS (" +
		"SELECT d.id FROM blobfs_directory d WHERE d.parent_id IS NULL" +
		" UNION ALL SELECT d.id FROM blobfs_directory d JOIN tree t ON d.parent_id = t.id)" +
		" SELECT COUNT(*) FROM tree WHERE tree.id IN (" + p(1) + ", " + p(2) + ")"
	return s.count(t, text, ids[0], ids[1])
}

// ownAncestor reports whether the directory with id is met again on a
// walk up from itself, bounded to eight steps so the walk terminates on a
// cycle.
func (s *suite) ownAncestor(t *testing.T, id string) bool {
	t.Helper()
	p := s.db.Dialect().Placeholder
	text := "WITH RECURSIVE up (id, parent_id, depth) AS (" +
		"SELECT d.id, d.parent_id, CAST(0 AS integer) FROM blobfs_directory d WHERE d.id = " + p(1) +
		" UNION ALL SELECT d.id, d.parent_id, up.depth + 1 FROM blobfs_directory d JOIN up ON up.parent_id = d.id WHERE up.depth < 8)" +
		" SELECT COUNT(*) FROM up WHERE up.id = " + p(2)
	return s.count(t, text, id, id) > 1
}

// count runs a one-value count query.
func (s *suite) count(t *testing.T, text string, args ...any) int {
	t.Helper()
	rows, err := s.db.QueryContext(s.ctx, text, args...)
	if err != nil {
		t.Fatalf("%s: %v", text, err)
	}
	defer func() { _ = rows.Close() }()
	var n int
	if !rows.Next() {
		t.Fatalf("%s: no row: %v", text, rows.Err())
	}
	if err := rows.Scan(&n); err != nil {
		t.Fatalf("%s: %v", text, err)
	}
	return n
}

// wantPath checks DirectoryPath of id.
func (s *suite) wantPath(t *testing.T, id, want string) {
	t.Helper()
	got, err := s.store.DirectoryPath(s.ctx, s.db, id)
	if err != nil || got != want {
		t.Errorf("DirectoryPath(%s) = %q, %v, want %q", id, got, err, want)
	}
}

// directory reads the row by id through the store, on the pool.
func (s *suite) directory(t *testing.T, id string) blobfs.Directory {
	t.Helper()
	d, err := s.store.Directory(s.ctx, s.db, id)
	if err != nil {
		t.Fatalf("Directory(%s): %v", id, err)
	}
	return d
}

// mkdirUnder creates a directory under a parent.
func (s *suite) mkdirUnder(t *testing.T, parentID, name string) blobfs.Directory {
	t.Helper()
	d, err := s.store.Mkdir(s.ctx, s.db, parentID, s.name(name))
	if err != nil {
		t.Fatalf("Mkdir(%q): %v", name, err)
	}
	return d
}

// name makes a subtest name usable as a directory name.
func (*suite) name(name string) string {
	return strings.NewReplacer("/", "-", "\\", "-").Replace(name)
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

// complete runs CompleteFileDelete on the pool and fails the test on any
// error.
func (s *suite) complete(t *testing.T, id string) {
	t.Helper()
	if err := s.store.CompleteFileDelete(s.ctx, s.db, id); err != nil {
		t.Fatalf("CompleteFileDelete(%s): %v", id, err)
	}
}

// wantGone checks that no row with id exists.
func (s *suite) wantGone(t *testing.T, id string) {
	t.Helper()
	if f, err := s.store.File(s.ctx, s.db, id); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("after the complete step the row is %+v, %v; want it gone", f, err)
	}
}

// referenceConstraint is the name of the suite's own foreign key into
// blobfs_file, which stands in for a consumer's.
const referenceConstraint = "datatest_fk_reference_file"

// createReference creates the suite's reference table, once per run: one
// column of the same type as blobfs_file.id, taken from the migrated
// table so the DDL names no engine type, under a foreign key to
// blobfs_file.
func (s *suite) createReference(t *testing.T) {
	t.Helper()
	if s.referenced {
		return
	}
	s.referenced = true
	for _, text := range []string{
		"CREATE TABLE datatest_reference AS SELECT f.id AS file_id FROM blobfs_file f WHERE 1 = 0",
		"ALTER TABLE datatest_reference ADD CONSTRAINT " + referenceConstraint + " FOREIGN KEY (file_id) REFERENCES blobfs_file (id)",
	} {
		if _, err := s.db.ExecContext(s.ctx, text); err != nil {
			t.Fatalf("%s: %v", text, err)
		}
	}
}

// reference inserts a row that references the file with id.
func (s *suite) reference(t *testing.T, id string) {
	t.Helper()
	if _, err := s.db.ExecContext(s.ctx, "INSERT INTO datatest_reference (file_id) VALUES ("+s.db.Dialect().Placeholder(1)+")", id); err != nil {
		t.Fatalf("reference file %s: %v", id, err)
	}
}

// unreference removes the rows that reference the file with id.
func (s *suite) unreference(t *testing.T, id string) {
	t.Helper()
	if _, err := s.db.ExecContext(s.ctx, "DELETE FROM datatest_reference WHERE file_id = "+s.db.Dialect().Placeholder(1), id); err != nil {
		t.Fatalf("unreference file %s: %v", id, err)
	}
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
	name = s.name(name)
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

// equalDirectory compares two rows field by field, timestamps by instant.
func equalDirectory(a, b blobfs.Directory) bool {
	return a.ID == b.ID && equalString(a.ParentID, b.ParentID) && a.Name == b.Name &&
		a.Version == b.Version && a.CreatedAt.Equal(b.CreatedAt) && a.UpdatedAt.Equal(b.UpdatedAt)
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
