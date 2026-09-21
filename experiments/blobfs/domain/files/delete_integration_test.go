//go:build integration

package files_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/domain/files"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/postgres"
)

// variant is one of the two variants the consumer's delete tests run
// over: its name, its constructor, and the options a test passes New to
// run over it. The standard baseline is what New uses without an option,
// so its options are none; the Postgres variant goes through WithVariant.
// The Postgres variant is named only here, in a test file; the
// composition root chooses the binary's variant in a later stage.
type variant struct {
	name  string
	build files.VariantConstructor
	opts  []files.Option
}

// standardVariant and postgresVariant are the two constructors, wrapped
// to return the interface.
func standardVariant(c *query.Catalog, d sqlate.Dialect) (data.Variant, error) {
	return data.NewStandard(c, d)
}

func postgresVariant(c *query.Catalog, d sqlate.Dialect) (data.Variant, error) {
	return postgres.New(c, d)
}

var variants = []variant{
	{"standard", standardVariant, nil},
	{"postgres", postgresVariant, []files.Option{files.WithVariant(postgresVariant)}},
}

// perVariant runs f once per variant, each in its own database and
// container, as a subtest named for the variant.
func perVariant(t *testing.T, f func(t *testing.T, e env, v variant)) {
	t.Helper()
	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			f(t, openWith(t, v.opts...), v)
		})
	}
}

// pausing is a variant that pauses the consumer's rm inside the begin's
// transaction, once: after the base begin has run, and so after the row
// lock is taken, it reports begun and waits for release while the
// transaction holds the lock, which is where a bookmark add has to
// queue. A later begin passes through, so the finishing rm converges.
type pausing struct {
	data.Variant
	begun   chan struct{}
	release chan struct{}
	done    bool
}

func (p *pausing) BeginFileDelete(ctx context.Context, sess sqlate.Session, id string) (blobfs.File, error) {
	f, err := p.Variant.BeginFileDelete(ctx, sess, id)
	if err != nil || p.done {
		return f, err
	}
	p.done = true
	close(p.begun)
	<-p.release
	return f, nil
}

// library builds the persistence package's store over the test's
// database, as a service that follows the reference-then-delete rule
// would hold one, so a test can hold a file and insert a row that
// references it in a transaction of its own.
func (e env) library(t *testing.T) *data.Store {
	t.Helper()
	c, err := query.NewCatalog(query.Patterns(), data.Patterns())
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	s, err := data.New(c, e.db.Dialect())
	if err != nil {
		t.Fatalf("data.New: %v", err)
	}
	return s
}

// awaitLockWait returns once a session of the test's database waits on
// a lock inside a statement whose text contains text, which is how a
// test observes that the other side of an interleaving has queued on
// the row, and fails the test when none does within a bound.
func (e env) awaitLockWait(t *testing.T, text string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		n := e.count(t, "SELECT COUNT(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE $1", "%"+text+"%")
		if n > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no session waits on a lock in a statement containing %q", text)
}

// removal is what an rm on a goroutine reports.
type removal struct {
	file blobfs.File
	err  error
}

// remove runs Remove on a goroutine and reports its result on the
// channel.
func (e env) remove(path string, stopAfter files.Step) <-chan removal {
	done := make(chan removal, 1)
	go func() {
		f, err := e.store.Remove(e.ctx, path, stopAfter)
		done <- removal{file: f, err: err}
	}()
	return done
}

// await returns what the channel reports, failing the test when nothing
// arrives within a bound.
func await[T any](t *testing.T, done <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-done:
		return v
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not return", what)
		var zero T
		return zero
	}
}

// status reads a file row's status straight from the database, or "" when
// the row is gone.
func (e env) status(t *testing.T, id string) string {
	t.Helper()
	return strings.Join(e.strings1(t, "SELECT status FROM blobfs_file WHERE id = $1", id), ",")
}

// objectHeld reports whether the store holds an object under key.
func (e env) objectHeld(t *testing.T, st *files.Storage, key string) bool {
	t.Helper()
	_, err := st.Stat(e.ctx, key)
	switch {
	case err == nil:
		return true
	case errors.Is(err, files.ErrObjectMissing):
		return false
	}
	t.Fatalf("Stat(%s): %v", key, err)
	return false
}

// TestRemoveConvergesAtEachStep is the first gate item, per variant, on
// the real services: a stop after begin leaves the row deleting with its
// object in the store, where stat and ls show it, cat and put refuse it;
// a stop after the object delete leaves the row deleting with the object
// gone; and an rm after either finishes the delete, so the row and the
// object are gone and the path is not found. A pending row and an
// available row go the same way, and a repeated rm of a finished path is
// not found.
func TestRemoveConvergesAtEachStep(t *testing.T) {
	perVariant(t, func(t *testing.T, e env, _ variant) {
		e.mkdir(t, "/docs", "")
		st := e.storage(t)
		f := e.put(t, "/docs/a.txt", "text/plain", "to be removed").File

		// A stop after begin.
		_, err := e.store.Remove(e.ctx, "/docs/a.txt", files.StepBegin)
		var stop *files.StopError
		if !errors.As(err, &stop) || stop.Step != files.StepBegin || stop.File.ID != f.ID || stop.File.Status != blobfs.StatusDeleting {
			t.Fatalf("Remove --fail-after begin = %v", err)
		}
		if got := e.status(t, f.ID); got != "deleting" {
			t.Errorf("after the stop the row is %q, want deleting", got)
		}
		if !e.objectHeld(t, st, f.Key) {
			t.Error("after the stop after begin the object is gone")
		}
		if row, err := e.store.Stat(e.ctx, "/docs/a.txt"); err != nil || row.Status != blobfs.StatusDeleting || row.Version != 3 {
			t.Errorf("Stat after the stop = %+v, %v; want the deleting row at version 3", row, err)
		}
		c := e.list(t, "/docs", files.Listing{Page: 1, Size: 10})
		if len(c.Files.Rows) != 1 || c.Files.Rows[0].Status != blobfs.StatusDeleting {
			t.Errorf("ls after the stop = %+v, want the deleting row listed", c.Files.Rows)
		}
		if _, _, err := e.store.Open(e.ctx, "/docs/a.txt"); !errors.Is(err, files.ErrNotAvailable) {
			t.Errorf("Open of the deleting file = %v, want ErrNotAvailable", err)
		}
		if _, err := e.store.Put(e.ctx, files.PutRequest{Path: "/docs/a.txt", Body: strings.NewReader("x")}); !errors.Is(err, blobfs.ErrNameTaken) {
			t.Errorf("Put over the deleting name = %v, want ErrNameTaken", err)
		}

		// A retry that stops after the object delete: the begin is repeated
		// and changes nothing, the object goes, the row stays.
		_, err = e.store.Remove(e.ctx, "/docs/a.txt", files.StepObject)
		if !errors.As(err, &stop) || stop.Step != files.StepObject || stop.File.Version != 3 {
			t.Fatalf("Remove --fail-after object = %v", err)
		}
		if e.objectHeld(t, st, f.Key) {
			t.Error("after the stop after object the object is still held")
		}
		if got := e.status(t, f.ID); got != "deleting" {
			t.Errorf("after the stop after object the row is %q, want deleting", got)
		}

		// The retry that finishes: the object delete finds nothing and the
		// row is removed.
		done, err := e.store.Remove(e.ctx, "/docs/a.txt", "")
		if err != nil || done.ID != f.ID {
			t.Fatalf("the finishing rm = %+v, %v", done, err)
		}
		if got := e.status(t, f.ID); got != "" {
			t.Errorf("after the rm the row is %q, want it gone", got)
		}
		if _, err := e.store.Stat(e.ctx, "/docs/a.txt"); !errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("Stat after the rm = %v, want ErrNotFound", err)
		}
		if _, err := e.store.Remove(e.ctx, "/docs/a.txt", ""); !errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("a repeated rm = %v, want ErrNotFound", err)
		}

		// A pending row, an abandoned write, goes through the same steps
		// and its missing object is no obstacle.
		_, err = e.store.Put(e.ctx, files.PutRequest{Path: "/docs/pending.txt", Body: strings.NewReader("never stored"), StopAfter: files.StepInsert})
		if !errors.As(err, &stop) {
			t.Fatalf("Put --fail-after insert = %v", err)
		}
		if _, err := e.store.Remove(e.ctx, "/docs/pending.txt", ""); err != nil {
			t.Fatalf("rm of the pending row: %v", err)
		}
		if got := e.status(t, stop.File.ID); got != "" {
			t.Errorf("after the rm the pending row is %q, want it gone", got)
		}

		// An available row in one go.
		g := e.put(t, "/docs/b.txt", "text/plain", "whole").File
		if _, err := e.store.Remove(e.ctx, "/docs/b.txt", ""); err != nil {
			t.Fatalf("rm: %v", err)
		}
		if e.objectHeld(t, st, g.Key) || e.status(t, g.ID) != "" {
			t.Error("after a whole rm the object or the row remains")
		}
		if n := e.count(t, "SELECT COUNT(*) FROM blobfs_file"); n != 0 {
			t.Errorf("%d file rows remain", n)
		}
		if _, err := e.store.Remove(e.ctx, "/missing/a.txt", ""); !errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("rm under a missing parent = %v, want ErrNotFound", err)
		}
	})
}

// TestRemoveMeetsABookmark is the third gate item, per variant: an rm of
// a bookmarked file is refused before anything is touched, with
// ErrBookmarked and the begin rolled back, and converges once the
// bookmark is removed. Then a rerun's check: the delete began and
// stopped, a bookmark was inserted without the hold (the rule bypassed
// through plain SQL), and the rerun is refused by the count before the
// object delete, so a deleting row's object is protected too.
func TestRemoveMeetsABookmark(t *testing.T) {
	perVariant(t, func(t *testing.T, e env, _ variant) {
		unit := blobfs.NewID()
		st := e.storage(t)
		f := e.put(t, "/a.txt", "text/plain", "bookmarked").File
		if _, err := e.store.AddBookmark(e.ctx, "/a.txt", unit, true); err != nil {
			t.Fatalf("AddBookmark: %v", err)
		}
		_, err := e.store.Remove(e.ctx, "/a.txt", "")
		if !errors.Is(err, files.ErrBookmarked) {
			t.Fatalf("rm of a bookmarked file = %v, want ErrBookmarked", err)
		}
		if row, err := e.store.Stat(e.ctx, "/a.txt"); err != nil || row.Status != blobfs.StatusAvailable || row.Version != f.Version {
			t.Errorf("after the refusal the row is %+v, %v; want it untouched", row, err)
		}
		if !e.objectHeld(t, st, f.Key) {
			t.Error("the refused rm deleted the object")
		}
		if _, err := e.store.RemoveBookmark(e.ctx, "/a.txt", unit); err != nil {
			t.Fatalf("RemoveBookmark: %v", err)
		}
		if _, err := e.store.Remove(e.ctx, "/a.txt", ""); err != nil {
			t.Fatalf("rm after the bookmark went: %v", err)
		}
		if e.objectHeld(t, st, f.Key) || e.status(t, f.ID) != "" {
			t.Error("after the rm the object or the row remains")
		}

		// A rerun's check protects even a deleting row's object: the delete
		// began and stopped, a bookmark arrived, and the rerun is refused
		// before the object delete.
		g := e.put(t, "/b.txt", "text/plain", "stopped").File
		var stop *files.StopError
		if _, err := e.store.Remove(e.ctx, "/b.txt", files.StepBegin); !errors.As(err, &stop) {
			t.Fatalf("Remove --fail-after begin = %v", err)
		}
		if _, err := e.second(t).ExecContext(e.ctx, "INSERT INTO bookmark (unit_id, file_id, active) VALUES ($1, $2, false)", unit, g.ID); err != nil {
			t.Fatalf("the bookmark of the deleting row: %v", err)
		}
		if _, err := e.store.Remove(e.ctx, "/b.txt", ""); !errors.Is(err, files.ErrBookmarked) || errors.Is(err, blobfs.ErrReferenced) {
			t.Fatalf("rm of a deleting row with a bookmark = %v, want ErrBookmarked from the check, not the key", err)
		}
		if got := e.status(t, g.ID); got != "deleting" || !e.objectHeld(t, st, g.Key) {
			t.Errorf("after the refusal the row is %q and the object held is %v; want deleting with its object", got, e.objectHeld(t, st, g.Key))
		}
		if _, err := e.store.RemoveBookmark(e.ctx, "/b.txt", unit); err != nil {
			t.Fatal(err)
		}
		if _, err := e.store.Remove(e.ctx, "/b.txt", ""); err != nil || e.status(t, g.ID) != "" {
			t.Errorf("the rm after the bookmark went = %v; the row remains: %v", err, e.status(t, g.ID) != "")
		}
	})
}

// TestBookmarkAddedDuringTheDeleteIsRefused is the first interleaving of
// the reference-then-delete rule on the engine, per variant: the rm's
// first transaction has begun the delete and holds the row lock (the
// pausing variant keeps it open), a bookmark add starts, resolves the
// file, and queues on the row at its hold, and once the rm's transaction
// commits the deleting status the add is refused with ErrNotAvailable
// over blobfs.ErrDeleting and writes no bookmark. The rm, stopped after
// its begin, then finishes on a rerun with nothing in its way.
func TestBookmarkAddedDuringTheDeleteIsRefused(t *testing.T) {
	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			unit := blobfs.NewID()
			paused := &pausing{begun: make(chan struct{}), release: make(chan struct{})}
			e := openWith(t, files.WithVariant(func(c *query.Catalog, d sqlate.Dialect) (data.Variant, error) {
				base, err := v.build(c, d)
				paused.Variant = base
				return paused, err
			}))
			st := e.storage(t)
			g := e.put(t, "/b.txt", "text/plain", "raced").File

			removed := e.remove("/b.txt", files.StepBegin)
			<-paused.begun
			added := make(chan error, 1)
			go func() {
				_, err := e.store.AddBookmark(e.ctx, "/b.txt", unit, true)
				added <- err
			}()
			e.awaitLockWait(t, "SET updated_at = updated_at")
			select {
			case err := <-added:
				t.Fatalf("the add returned %v while the delete's transaction held the row", err)
			default:
			}
			close(paused.release)

			r := await(t, removed, "the rm")
			var stop *files.StopError
			if !errors.As(r.err, &stop) || stop.Step != files.StepBegin || r.file.Status != blobfs.StatusDeleting {
				t.Fatalf("Remove --fail-after begin = %+v, %v", r.file, r.err)
			}
			err := await(t, added, "the add")
			if !errors.Is(err, files.ErrNotAvailable) || !errors.Is(err, blobfs.ErrDeleting) {
				t.Errorf("the add after the delete began = %v, want ErrNotAvailable over ErrDeleting", err)
			}
			if n := e.count(t, "SELECT COUNT(*) FROM bookmark"); n != 0 {
				t.Errorf("%d bookmarks exist after the refused add", n)
			}
			if got := e.status(t, g.ID); got != "deleting" || !e.objectHeld(t, st, g.Key) {
				t.Errorf("after the stop the row is %q and the object held is %v; want deleting with its object", got, e.objectHeld(t, st, g.Key))
			}
			if _, err := e.store.Remove(e.ctx, "/b.txt", ""); err != nil {
				t.Fatalf("the finishing rm: %v", err)
			}
			if e.status(t, g.ID) != "" || e.objectHeld(t, st, g.Key) {
				t.Error("after the finishing rm the row or the object remains")
			}
		})
	}
}

// TestDeleteDuringTheBookmarkAddIsRefused is the second interleaving on
// the engine, per variant: a transaction has held the file through the
// library's HoldFile and inserted its bookmark, as AddBookmark does, and
// has not committed; an rm starts and its begin queues on the row; once
// the holder commits, the rm's check sees the bookmark and refuses with
// ErrBookmarked from the count, not the foreign key, rolling its begin
// back, so the row is available at its version and the object is held.
// When the holder rolls back instead, the rm proceeds and finishes.
func TestDeleteDuringTheBookmarkAddIsRefused(t *testing.T) {
	perVariant(t, func(t *testing.T, e env, _ variant) {
		unit := blobfs.NewID()
		lib := e.library(t)
		holder := e.second(t)
		st := e.storage(t)
		g := e.put(t, "/b.txt", "text/plain", "held").File

		// hold holds the file at the version put returned and inserts the
		// unit's bookmark, in a transaction left open.
		hold := func(t *testing.T, f blobfs.File) *sqlate.Tx {
			t.Helper()
			tx, err := holder.Begin(e.ctx)
			if err != nil {
				t.Fatalf("Begin: %v", err)
			}
			if err := lib.HoldFile(e.ctx, tx, f.ID, data.AtVersion(f.Version)); err != nil {
				_ = tx.Rollback()
				t.Fatalf("HoldFile: %v", err)
			}
			if _, err := tx.ExecContext(e.ctx, "INSERT INTO bookmark (unit_id, file_id, active) VALUES ($1, $2, false)", unit, f.ID); err != nil {
				_ = tx.Rollback()
				t.Fatalf("the bookmark under the hold: %v", err)
			}
			return tx
		}

		tx := hold(t, g)
		removed := e.remove("/b.txt", "")
		e.awaitLockWait(t, "SET status = 'deleting'")
		select {
		case r := <-removed:
			t.Fatalf("the rm returned (%+v, %v) while the hold's transaction was open", r.file, r.err)
		default:
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		r := await(t, removed, "the rm")
		if !errors.Is(r.err, files.ErrBookmarked) || errors.Is(r.err, blobfs.ErrReferenced) || !strings.Contains(r.err.Error(), "1 unit(s) bookmark the file") {
			t.Fatalf("the rm after the bookmark committed = %v, want ErrBookmarked from the check, not the key", r.err)
		}
		if row, err := e.store.Stat(e.ctx, "/b.txt"); err != nil || row.Status != blobfs.StatusAvailable || row.Version != g.Version {
			t.Errorf("after the refusal the row is %+v, %v; want it available at version %d", row, err, g.Version)
		}
		if !e.objectHeld(t, st, g.Key) {
			t.Error("the refused rm deleted the object")
		}
		page, err := e.store.ListBookmarks(e.ctx, unit, files.Listing{Page: 1, Size: 10})
		if err != nil || len(page.Rows) != 1 || page.Rows[0].Status != blobfs.StatusAvailable || page.Rows[0].Path != "/b.txt" {
			t.Errorf("bookmark ls = %+v, %v; want the bookmark of the available file", page.Rows, err)
		}
		if _, err := e.store.RemoveBookmark(e.ctx, "/b.txt", unit); err != nil {
			t.Fatalf("RemoveBookmark: %v", err)
		}
		if _, err := e.store.Remove(e.ctx, "/b.txt", ""); err != nil || e.status(t, g.ID) != "" {
			t.Errorf("the rm after the bookmark went = %v; the row remains: %v", err, e.status(t, g.ID) != "")
		}

		// The holder rolls back: the rm waits the same way and then finishes.
		h := e.put(t, "/c.txt", "text/plain", "released").File
		tx = hold(t, h)
		removed = e.remove("/c.txt", "")
		e.awaitLockWait(t, "SET status = 'deleting'")
		if err := tx.Rollback(); err != nil {
			t.Fatalf("Rollback: %v", err)
		}
		if r := await(t, removed, "the rm"); r.err != nil || r.file.ID != h.ID {
			t.Fatalf("the rm after the hold rolled back = %+v, %v", r.file, r.err)
		}
		if e.status(t, h.ID) != "" || e.objectHeld(t, st, h.Key) || e.count(t, "SELECT COUNT(*) FROM bookmark") != 0 {
			t.Error("after the rm the row, the object, or a bookmark remains")
		}
	})
}

// TestRemoveDirectory proves rmdir on the engine: an owned top-level
// directory goes with its owner row, a non-empty owned directory is
// refused with ErrNotEmpty and keeps its owner row, the root is refused,
// and a directory holding only a deleting file is still not empty.
func TestRemoveDirectory(t *testing.T) {
	e := open(t)
	unit := blobfs.NewID()
	owned := e.mkdir(t, "/owned", unit)
	if dir, err := e.store.RemoveDirectory(e.ctx, "/owned"); err != nil || dir.ID != owned.ID {
		t.Fatalf("rmdir of the owned directory = %+v, %v", dir, err)
	}
	if n := e.count(t, "SELECT COUNT(*) FROM directory_owner"); n != 0 {
		t.Errorf("%d owner rows remain after the rmdir", n)
	}
	if _, err := e.store.Stat(e.ctx, "/owned/x"); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("the directory remains: %v", err)
	}

	e.mkdir(t, "/kept", unit)
	e.mkdir(t, "/kept/child", "")
	if _, err := e.store.RemoveDirectory(e.ctx, "/kept"); !errors.Is(err, blobfs.ErrNotEmpty) {
		t.Errorf("rmdir of a directory with a child = %v, want ErrNotEmpty", err)
	}
	if n := e.count(t, "SELECT COUNT(*) FROM directory_owner"); n != 1 {
		t.Errorf("%d owner rows after the refused rmdir, want the one rolled back into place", n)
	}
	if _, err := e.store.RemoveDirectory(e.ctx, "/"); !errors.Is(err, blobfs.ErrRootDirectory) {
		t.Errorf("rmdir / = %v, want ErrRootDirectory", err)
	}
	if _, err := e.store.RemoveDirectory(e.ctx, "/missing"); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("rmdir of a missing directory = %v, want ErrNotFound", err)
	}

	e.put(t, "/kept/child/a.txt", "text/plain", "x")
	var stop *files.StopError
	if _, err := e.store.Remove(e.ctx, "/kept/child/a.txt", files.StepBegin); !errors.As(err, &stop) {
		t.Fatalf("Remove --fail-after begin = %v", err)
	}
	if _, err := e.store.RemoveDirectory(e.ctx, "/kept/child"); !errors.Is(err, blobfs.ErrNotEmpty) {
		t.Errorf("rmdir over a deleting file = %v, want ErrNotEmpty: the row keeps its slot", err)
	}
}

// TestRemoveTreeRacesAnInsert is the second gate item, per variant: a
// second connection inserts into a directory the walk has just found
// empty, at the observer's DirectoryEmptied event, so the interleaving is
// deterministic. Inserted once, the directory's removal is refused by the
// foreign key, the walk empties it again, and the delete finishes clean.
// Inserted at every pass, the walk gives up after its bounded passes with
// ErrNotEmpty, nothing dangles (the last inserted row sits in its
// directory, and no object is left without a row), and a rerun converges.
// A bookmarked file under the tree stops the walk with ErrBookmarked and
// a rerun after the bookmark goes converges too.
func TestRemoveTreeRacesAnInsert(t *testing.T) {
	perVariant(t, func(t *testing.T, e env, _ variant) {
		other := e.second(t)
		st := e.storage(t)
		e.mkdir(t, "/t", "")
		a := e.mkdir(t, "/t/a", "")
		e.mkdir(t, "/t/a/b", "")
		e.put(t, "/t/top.txt", "text/plain", "1")
		e.put(t, "/t/a/b/deep.txt", "text/plain", "22")
		e.put(t, "/t/a/mid.txt", "text/plain", "333")

		// One insert, at the moment /t/a is found empty.
		inserted := 0
		res, err := e.store.RemoveTree(e.ctx, "/t", func(ev files.RemovalEvent) {
			if ev.Kind == files.DirectoryEmptied && ev.Path == "/t/a" && inserted == 0 {
				inserted++
				insertFile(e.ctx, t, other, a.ID, "late.txt")
			}
		})
		if err != nil {
			t.Fatalf("RemoveTree with one racing insert: %v", err)
		}
		if res.Files != 4 || res.Directories != 3 || inserted != 1 {
			t.Errorf("RemoveTree = %+v after %d inserts, want 4 files and 3 directories", res, inserted)
		}
		if n := e.count(t, "SELECT COUNT(*) FROM blobfs_file") + e.count(t, "SELECT COUNT(*) FROM blobfs_directory WHERE parent_id IS NOT NULL"); n != 0 {
			t.Errorf("%d rows remain under the removed tree", n)
		}

		// An insert at every pass over /u: the walk stops after its bounded
		// passes, the tree is consistent, and a rerun finishes.
		u := e.mkdir(t, "/u", "")
		e.put(t, "/u/x.txt", "text/plain", "x")
		passes := 0
		res, err = e.store.RemoveTree(e.ctx, "/u", func(ev files.RemovalEvent) {
			if ev.Kind == files.DirectoryEmptied && ev.Path == "/u" {
				passes++
				if _, err := other.ExecContext(e.ctx, "INSERT INTO blobfs_directory (id, parent_id, name) VALUES ($1, $2, $3)", blobfs.NewID(), u.ID, "late-"+strings.Repeat("x", passes)); err != nil {
					t.Errorf("the racing mkdir: %v", err)
				}
			}
		})
		if !errors.Is(err, blobfs.ErrNotEmpty) || passes != 3 {
			t.Fatalf("RemoveTree under a sustained inserter = %v after %d passes, want ErrNotEmpty after 3", err, passes)
		}
		if res.Files != 1 || res.Directories != 2 {
			t.Errorf("RemoveTree = %+v, want the file and the two earlier late directories removed", res)
		}
		if n := e.count(t, "SELECT COUNT(*) FROM blobfs_directory WHERE parent_id = $1", u.ID); n != 1 {
			t.Errorf("%d children of /u remain, want the last inserted one", n)
		}
		if _, err := e.store.Stat(e.ctx, "/u/x.txt"); !errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("the file under /u remains: %v", err)
		}
		res, err = e.store.RemoveTree(e.ctx, "/u", nil)
		if err != nil || res.Directories != 2 || res.Files != 0 {
			t.Errorf("the rerun = %+v, %v; want the late child and /u removed", res, err)
		}

		// A bookmark under the tree stops the walk; the rest stays consistent.
		unit := blobfs.NewID()
		e.mkdir(t, "/v", "")
		e.mkdir(t, "/v/w", "")
		kept := e.put(t, "/v/w/kept.txt", "text/plain", "k").File
		e.put(t, "/v/w/gone.txt", "text/plain", "g")
		if _, err := e.store.AddBookmark(e.ctx, "/v/w/kept.txt", unit, false); err != nil {
			t.Fatalf("AddBookmark: %v", err)
		}
		if _, err := e.store.RemoveTree(e.ctx, "/v", nil); !errors.Is(err, files.ErrBookmarked) {
			t.Fatalf("RemoveTree over a bookmarked file = %v, want ErrBookmarked", err)
		}
		if row, err := e.store.Stat(e.ctx, "/v/w/kept.txt"); err != nil || row.Status != blobfs.StatusAvailable || !e.objectHeld(t, st, kept.Key) {
			t.Errorf("the bookmarked file after the refusal = %+v, %v; want it available with its object", row, err)
		}
		if _, err := e.store.Stat(e.ctx, "/v/w/gone.txt"); !errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("the unbookmarked sibling remains: %v", err)
		}
		if _, err := e.store.RemoveBookmark(e.ctx, "/v/w/kept.txt", unit); err != nil {
			t.Fatal(err)
		}
		if res, err := e.store.RemoveTree(e.ctx, "/v", nil); err != nil || res.Files != 1 || res.Directories != 2 {
			t.Errorf("the rerun after the bookmark went = %+v, %v", res, err)
		}
		if n := e.count(t, "SELECT COUNT(*) FROM blobfs_directory WHERE parent_id IS NOT NULL"); n != 0 {
			t.Errorf("%d directories remain", n)
		}
		if _, err := e.store.RemoveTree(e.ctx, "/", nil); !errors.Is(err, blobfs.ErrRootDirectory) {
			t.Errorf("rm -r / = %v, want ErrRootDirectory", err)
		}
	})
}

// TestRemoveTreeRacesAnInsertBetweenPasses proves the other interleaving:
// a row inserted while a pass is under way, at a file's removal, is
// listed by the next pass and removed, and the walk finishes clean.
func TestRemoveTreeRacesAnInsertBetweenPasses(t *testing.T) {
	perVariant(t, func(t *testing.T, e env, _ variant) {
		other := e.second(t)
		d := e.mkdir(t, "/d", "")
		e.put(t, "/d/one.txt", "text/plain", "1")
		inserted := false
		res, err := e.store.RemoveTree(e.ctx, "/d", func(ev files.RemovalEvent) {
			if ev.Kind == files.RemovedFile && !inserted {
				inserted = true
				insertFile(e.ctx, t, other, d.ID, "two.txt")
			}
		})
		if err != nil || res.Files != 2 || res.Directories != 1 {
			t.Errorf("RemoveTree = %+v, %v; want both files and the directory removed", res, err)
		}
		if n := e.count(t, "SELECT COUNT(*) FROM blobfs_file"); n != 0 {
			t.Errorf("%d file rows remain", n)
		}
	})
}
