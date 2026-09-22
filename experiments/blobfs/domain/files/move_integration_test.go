//go:build integration

package files_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/domain/files"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

// move runs Move and fails the test on any error.
func (e env) move(t *testing.T, src, dst string) files.MoveResult {
	t.Helper()
	res, err := e.store.Move(e.ctx, src, dst)
	if err != nil {
		t.Fatalf("Move(%s, %s): %v", src, dst, err)
	}
	return res
}

// stat runs Stat and fails the test on any error.
func (e env) stat(t *testing.T, path string) blobfs.File {
	t.Helper()
	f, err := e.store.Stat(e.ctx, path)
	if err != nil {
		t.Fatalf("Stat(%s): %v", path, err)
	}
	return f
}

// read returns the content of the file at path.
func (e env) read(t *testing.T, path string) string {
	t.Helper()
	body, _, err := e.store.Open(e.ctx, path)
	if err != nil {
		t.Fatalf("Open(%s): %v", path, err)
	}
	defer func() { _ = body.Close() }()
	b, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// TestMove is the scripted run of mv through the store, per variant: a
// file into an existing directory and renamed, with its key untouched and
// its content readable at the new path; a directory into an existing
// directory and renamed, with everything under it following; the cycle
// refusals; the root refusal; the missing source and parent; the taken
// name for each kind, and the separate name spaces of the two kinds; the
// scope rule, with the owner row of a renamed top-level directory kept;
// a bookmark whose listed path follows the file and then its directory;
// a deleting file refused; and a pending file moved and then completed
// by a put at its new path.
func TestMove(t *testing.T) {
	perVariant(t, func(t *testing.T, e env, _ variant) {
		unit := blobfs.NewID()
		for _, p := range []string{"/a", "/a/x", "/a/x/deep", "/a/y", "/b"} {
			e.mkdir(t, p, "")
		}
		e.put(t, "/a/x/f.txt", "text/plain", "content")
		e.put(t, "/a/top.txt", "text/plain", "top")

		// A file into an existing directory, then renamed.
		if res := e.move(t, "/a/top.txt", "/a/y"); res.Kind != files.EntryFile || res.To != "/a/y/top.txt" {
			t.Errorf("mv of a file into a directory = %+v", res)
		}
		if _, err := e.store.Stat(e.ctx, "/a/top.txt"); !errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("the file is still at its old path: %v", err)
		}
		before := e.stat(t, "/a/x/f.txt")
		if res := e.move(t, "/a/x/f.txt", "/a/x/g.txt"); res.To != "/a/x/g.txt" || res.ID != before.ID {
			t.Errorf("the rename = %+v", res)
		}
		after := e.stat(t, "/a/x/g.txt")
		if after.Key != before.Key || after.Version != before.Version+1 || after.Name != "g.txt" {
			t.Errorf("after the rename the row is %+v, want the key %q kept", after, before.Key)
		}
		if got := e.read(t, "/a/x/g.txt"); got != "content" {
			t.Errorf("cat after the rename = %q", got)
		}

		// A directory into an existing directory, then renamed; the
		// contents follow.
		if res := e.move(t, "/a/x", "/a/y"); res.Kind != files.EntryDirectory || res.To != "/a/y/x" {
			t.Errorf("mv of a directory into a directory = %+v", res)
		}
		if f := e.stat(t, "/a/y/x/g.txt"); f.ID != before.ID {
			t.Errorf("the file under the moved directory = %+v", f)
		}
		e.list(t, "/a/y/x/deep", files.Listing{Page: 1, Size: 1})
		if res := e.move(t, "/a/y/x", "/a/y/z"); res.To != "/a/y/z" {
			t.Errorf("the directory rename = %+v", res)
		}
		e.stat(t, "/a/y/z/g.txt")
		if _, err := e.store.List(e.ctx, "/a/x", files.Listing{Page: 1, Size: 1}); !errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("the old directory path still lists: %v", err)
		}

		// Cycles and the root.
		for _, dst := range []string{"/a/y/z", "/a/y/z/deep", "/a/y"} {
			if _, err := e.store.Move(e.ctx, "/a/y", dst); !errors.Is(err, blobfs.ErrCycle) {
				t.Errorf("mv /a/y %s = %v, want ErrCycle", dst, err)
			}
		}
		if _, err := e.store.Move(e.ctx, "/", "/elsewhere"); !errors.Is(err, blobfs.ErrRootDirectory) {
			t.Errorf("mv / = %v, want ErrRootDirectory", err)
		}

		// Missing and taken.
		if _, err := e.store.Move(e.ctx, "/a/missing", "/a/y"); !errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("mv of a missing source = %v, want ErrNotFound", err)
		}
		if _, err := e.store.Move(e.ctx, "/a/y/z", "/a/nope/z"); !errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("mv to a missing parent = %v, want ErrNotFound", err)
		}
		e.mkdir(t, "/a/held", "")
		e.mkdir(t, "/a/y/held", "")
		if _, err := e.store.Move(e.ctx, "/a/held", "/a/y"); !errors.Is(err, blobfs.ErrNameTaken) {
			t.Errorf("mv of a directory under a taken name = %v, want ErrNameTaken", err)
		}
		e.put(t, "/a/dup.txt", "text/plain", "dup")
		if _, err := e.store.Move(e.ctx, "/a/dup.txt", "/a/y/top.txt"); !errors.Is(err, blobfs.ErrNameTaken) {
			t.Errorf("mv of a file under a taken name = %v, want ErrNameTaken", err)
		}
		// The two kinds have separate name spaces: a directory may take a
		// file's name beside it, after which the directory wins a resolution
		// of that path as a source.
		if res := e.move(t, "/a/held", "/a/y/top.txt"); res.Kind != files.EntryDirectory || res.To != "/a/y/top.txt" {
			t.Errorf("mv of a directory under a file's name = %+v", res)
		}
		e.stat(t, "/a/y/top.txt")
		e.list(t, "/a/y/top.txt", files.Listing{Page: 1, Size: 1})
		if res := e.move(t, "/a/y/top.txt", "/a/held"); res.Kind != files.EntryDirectory {
			t.Errorf("mv of the shared name moved the %s, want the directory", res.Kind)
		}

		// The scope rule.
		owned := e.mkdir(t, "/owned", unit)
		if res := e.move(t, "/owned", "/renamed"); res.To != "/renamed" {
			t.Errorf("the top-level rename = %+v", res)
		}
		if units := e.strings1(t, "SELECT CAST(unit_id AS text) FROM directory_owner WHERE directory_id = $1", owned.ID); len(units) != 1 || units[0] != unit {
			t.Errorf("the owner row after the rename = %v, want it kept", units)
		}
		if c := e.list(t, "/", files.Listing{Page: 1, Size: 10, Unit: unit}); strings.Join(names(c.Directories.Rows), " ") != "renamed" {
			t.Errorf("ls / as the unit after the rename = %v", names(c.Directories.Rows))
		}
		e.put(t, "/root.txt", "text/plain", "r")
		for _, mv := range [][2]string{{"/renamed", "/b"}, {"/renamed", "/b/renamed"}, {"/a/y", "/b/y"}, {"/a/y/z", "/z"}, {"/root.txt", "/a/root.txt"}, {"/a/dup.txt", "/dup.txt"}} {
			if _, err := e.store.Move(e.ctx, mv[0], mv[1]); !errors.Is(err, files.ErrMoveAcrossScopes) {
				t.Errorf("mv %s %s = %v, want ErrMoveAcrossScopes", mv[0], mv[1], err)
			}
		}
		if res := e.move(t, "/root.txt", "/renamed-root.txt"); res.To != "/renamed-root.txt" {
			t.Errorf("a rename at the root = %+v", res)
		}
		if n := e.count(t, "SELECT COUNT(*) FROM directory_owner"); n != 1 {
			t.Errorf("%d owner rows, want the one", n)
		}

		// A bookmark's listed path follows the file, then its directory.
		e.bookmark(t, "/a/y/z/g.txt", unit, true)
		e.move(t, "/a/y/z/g.txt", "/a/y/z/h.txt")
		if p := paths(e.bookmarksOf(t, unit, files.Listing{Page: 1, Size: 10}).Rows); strings.Join(p, " ") != "/a/y/z/h.txt" {
			t.Errorf("bookmark ls after the file's rename = %v", p)
		}
		e.move(t, "/a/y/z", "/a/w")
		if p := paths(e.bookmarksOf(t, unit, files.Listing{Page: 1, Size: 10}).Rows); strings.Join(p, " ") != "/a/w/h.txt" {
			t.Errorf("bookmark ls after the directory's move = %v", p)
		}
		if got := e.read(t, "/a/w/h.txt"); got != "content" {
			t.Errorf("cat after both moves = %q", got)
		}

		// A deleting file is refused; a pending one moves and a put at the
		// new path completes it.
		e.put(t, "/a/del.txt", "text/plain", "d")
		var stop *files.StopError
		if _, err := e.store.Remove(e.ctx, "/a/del.txt", files.StepBegin); !errors.As(err, &stop) {
			t.Fatalf("Remove --fail-after begin = %v", err)
		}
		if _, err := e.store.Move(e.ctx, "/a/del.txt", "/a/del2.txt"); !errors.Is(err, blobfs.ErrDeleting) {
			t.Errorf("mv of a deleting file = %v, want ErrDeleting", err)
		}
		if _, err := e.store.Put(e.ctx, files.PutRequest{Path: "/a/pend.txt", Body: strings.NewReader("never"), StopAfter: files.StepInsert}); !errors.As(err, &stop) {
			t.Fatalf("Put --fail-after insert = %v", err)
		}
		if res := e.move(t, "/a/pend.txt", "/a/pend2.txt"); res.ID != stop.File.ID {
			t.Errorf("mv of the pending row = %+v", res)
		}
		if f := e.stat(t, "/a/pend2.txt"); f.Status != blobfs.StatusPending || f.Key != stop.File.Key {
			t.Errorf("the moved pending row = %+v", f)
		}
		if res := e.put(t, "/a/pend2.txt", "text/plain", "finished"); !res.Resumed || res.File.ID != stop.File.ID {
			t.Errorf("a put at the new path = %+v, want the pending row resumed", res)
		}
		if got := e.read(t, "/a/pend2.txt"); got != "finished" {
			t.Errorf("cat of the resumed row = %q", got)
		}
	})
}

// gated wraps a variant so that every LockTree, once the wrapped lock has
// returned, sends a release channel of its own on arrived and waits on it
// before it returns; the test decides when each mover proceeds past its
// lock, in arrival order.
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

// TestMoveOpposingConcurrentMoves is the gate through mv, per variant:
// mover A runs mv /s/x /s/y while mover B runs mv /s/y /s/x, interleaved
// through the gated variant. On Postgres B blocks inside the lock while A
// holds it, A commits, and B's check then sees x under y and is refused
// with ErrCycle. On the baseline B passes the no-op lock at once; the
// test releases B first, which commits y under x, and then A, whose check
// sees it and is refused: the consumer's Move commits as soon as the
// library's update returns, so this interleaving cannot reach the window
// between one mover's check and its commit, which is where the baseline
// forms a cycle. The library's conformance suite holds the commits and
// proves that cycle; here the consumer's composition is shown to take the
// lock before the check on both variants.
func TestMoveOpposingConcurrentMoves(t *testing.T) {
	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			g := &gated{arrived: make(chan chan struct{})}
			e := openWith(t, files.WithVariant(func(c *query.Catalog, d sqlate.Dialect) (data.Variant, error) {
				base, err := v.build(c, d)
				g.Variant = base
				return g, err
			}))
			e.mkdir(t, "/s", "")
			e.mkdir(t, "/s/x", "")
			e.mkdir(t, "/s/y", "")
			move := func(src, dst string) <-chan error {
				done := make(chan error, 1)
				go func() {
					_, err := e.store.Move(e.ctx, src, dst)
					done <- err
				}()
				return done
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

			a := move("/s/x", "/s/y")
			releaseA := arrived("A")
			b := move("/s/y", "/s/x")
			if v.name == "postgres" {
				select {
				case <-g.arrived:
					t.Fatal("B passed the lock while A held it")
				case err := <-b:
					t.Fatalf("B finished (%v) while A held the lock", err)
				case <-time.After(500 * time.Millisecond):
				}
				close(releaseA)
				if err := <-a; err != nil {
					t.Fatalf("A's mv: %v", err)
				}
				close(arrived("B"))
				if err := <-b; !errors.Is(err, blobfs.ErrCycle) {
					t.Fatalf("B's mv = %v, want ErrCycle", err)
				}
				e.list(t, "/s/y/x", files.Listing{Page: 1, Size: 1})
			} else {
				releaseB := arrived("B")
				close(releaseB)
				if err := <-b; err != nil {
					t.Fatalf("B's mv on the baseline: %v", err)
				}
				close(releaseA)
				if err := <-a; !errors.Is(err, blobfs.ErrCycle) {
					t.Fatalf("A's mv on the baseline = %v, want ErrCycle: B committed before A's check", err)
				}
				e.list(t, "/s/x/y", files.Listing{Page: 1, Size: 1})
			}
			if n := e.count(t, "WITH RECURSIVE tree (id) AS (SELECT d.id FROM blobfs_directory d WHERE d.parent_id IS NULL UNION ALL SELECT d.id FROM blobfs_directory d JOIN tree t ON d.parent_id = t.id) SELECT COUNT(*) FROM tree"); n != 4 {
				t.Errorf("%d directories are reachable from the root, want all 4", n)
			}
		})
	}
}

// raced runs op on a goroutine while a raw transaction on a second
// connection holds prepare uncommitted, checks that op blocks on it, ends
// the raw transaction through end, and returns op's result.
func raced(t *testing.T, e env, prepare string, args []any, op func() error, end func(*sqlate.Tx) error) error {
	t.Helper()
	tx, err := e.second(t).Begin(e.ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := tx.ExecContext(e.ctx, prepare, args...); err != nil {
		_ = tx.Rollback()
		t.Fatalf("%s: %v", prepare, err)
	}
	done := make(chan error, 1)
	go func() { done <- op() }()
	select {
	case err := <-done:
		_ = tx.Rollback()
		t.Fatalf("the operation returned (%v) while the raw transaction was open; it should block on the row", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := end(tx); err != nil {
		t.Fatalf("end the raw transaction: %v", err)
	}
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("the operation did not return after the raw transaction ended")
		return nil
	}
}

// TestMoveRacesADelete confirms, per variant, what stage 12 recorded: the
// delete takes no tree lock, and a move racing a delete is refused by one
// foreign key or the other. A move into a directory whose uncommitted
// removal holds the row waits on it, and is ErrNotFound once the removal
// commits or succeeds once it rolls back. A removal of a directory whose
// child is being moved out waits on the child's row, and succeeds once
// the move commits or is ErrNotEmpty once it rolls back. A removal of a
// directory a child is being moved into waits on the directory's row, and
// is ErrNotEmpty once the move commits or succeeds once it rolls back.
func TestMoveRacesADelete(t *testing.T) {
	perVariant(t, func(t *testing.T, e env, _ variant) {
		e.mkdir(t, "/p", "")
		commit, rollback := (*sqlate.Tx).Commit, (*sqlate.Tx).Rollback

		// A move into a directory being removed.
		target := e.mkdir(t, "/p/target", "")
		e.mkdir(t, "/p/x", "")
		moveIn := func() error { _, err := e.store.Move(e.ctx, "/p/x", "/p/target"); return err }
		if err := raced(t, e, "DELETE FROM blobfs_directory WHERE id = $1", []any{target.ID}, moveIn, commit); !errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("mv into a directory whose removal committed = %v, want ErrNotFound", err)
		}
		target = e.mkdir(t, "/p/target", "")
		if err := raced(t, e, "DELETE FROM blobfs_directory WHERE id = $1", []any{target.ID}, moveIn, rollback); err != nil {
			t.Errorf("mv into a directory whose removal rolled back = %v, want success", err)
		}
		e.list(t, "/p/target/x", files.Listing{Page: 1, Size: 1})

		// A removal of a directory whose child is moved out.
		elsewhereID := e.mkdir(t, "/p/elsewhere", "").ID
		e.mkdir(t, "/p/d", "")
		child := e.mkdir(t, "/p/d/child", "")
		removeD := func() error { _, err := e.store.RemoveDirectory(e.ctx, "/p/d"); return err }
		if err := raced(t, e, "UPDATE blobfs_directory SET parent_id = $1 WHERE id = $2", []any{elsewhereID, child.ID}, removeD, commit); err != nil {
			t.Errorf("rmdir of a directory whose child's move out committed = %v, want success", err)
		}
		e.list(t, "/p/elsewhere/child", files.Listing{Page: 1, Size: 1})
		e.mkdir(t, "/p/d", "")
		child = e.mkdir(t, "/p/d/other", "")
		if err := raced(t, e, "UPDATE blobfs_directory SET parent_id = $1 WHERE id = $2", []any{elsewhereID, child.ID}, removeD, rollback); !errors.Is(err, blobfs.ErrNotEmpty) {
			t.Errorf("rmdir of a directory whose child's move out rolled back = %v, want ErrNotEmpty", err)
		}

		// A removal of a directory a child is moved into.
		empty := e.mkdir(t, "/p/empty", "")
		mover := e.mkdir(t, "/p/mover", "")
		removeEmpty := func() error { _, err := e.store.RemoveDirectory(e.ctx, "/p/empty"); return err }
		if err := raced(t, e, "UPDATE blobfs_directory SET parent_id = $1 WHERE id = $2", []any{empty.ID, mover.ID}, removeEmpty, commit); !errors.Is(err, blobfs.ErrNotEmpty) {
			t.Errorf("rmdir of a directory a child was moved into = %v, want ErrNotEmpty", err)
		}
		e.list(t, "/p/empty/mover", files.Listing{Page: 1, Size: 1})
		e.move(t, "/p/empty/mover", "/p/mover")
		if err := raced(t, e, "UPDATE blobfs_directory SET parent_id = $1 WHERE id = $2", []any{empty.ID, mover.ID}, removeEmpty, rollback); err != nil {
			t.Errorf("rmdir of a directory whose incoming move rolled back = %v, want success", err)
		}
		if _, err := e.store.List(e.ctx, "/p/empty", files.Listing{Page: 1, Size: 1}); !errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("the directory remains after the rmdir: %v", err)
		}
	})
}
