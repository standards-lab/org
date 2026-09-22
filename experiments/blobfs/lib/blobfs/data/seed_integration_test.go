//go:build integration

package data_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
	blobfspostgres "github.com/standards-lab/org/experiments/blobfs/lib/blobfs/postgres"
)

// openPostgres builds the environment with the store over the Postgres
// variant, for a test whose subject goes through a variation point.
func openPostgres(t *testing.T) env {
	t.Helper()
	e := open(t)
	c, err := query.NewCatalog(query.Patterns(), data.Patterns())
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	v, err := blobfspostgres.New(c, e.db.Dialect())
	if err != nil {
		t.Fatalf("postgres.New: %v", err)
	}
	s, err := data.New(c, e.db.Dialect(), data.WithVariant(v))
	if err != nil {
		t.Fatalf("New over the Postgres variant: %v", err)
	}
	e.store = s
	return e
}

// sameDirectory reports whether two directory rows are the same row at
// the same version, by value, since ParentID is a pointer.
func sameDirectory(a, b blobfs.Directory) bool {
	return a.ID == b.ID && a.Name == b.Name && a.Version == b.Version && a.CreatedAt.Equal(b.CreatedAt)
}

// sameFile reports whether two file rows are the same row in the same
// status at the same version, by value, since Size and ETag are pointers.
func sameFile(a, b blobfs.File) bool {
	return a.ID == b.ID && a.Name == b.Name && a.Status == b.Status && a.Key == b.Key && a.Version == b.Version
}

// TestEnsureDirectoryOnTheEngine proves the insert-or-find against the
// schema: the first call creates the row and the second, in either
// spelling of the name and inside a transaction as well, finds the same
// row and creates nothing; a supplied id is the row's, and a later call
// under another supplied id still finds the row under its own; and the
// directory holds one row at the end.
func TestEnsureDirectoryOnTheEngine(t *testing.T) {
	e := open(t)
	d, created, err := e.store.EnsureDirectory(e.ctx, e.db, blobfs.RootID, decomposed)
	if err != nil || !created || d.Name != composed || d.ParentID == nil || *d.ParentID != blobfs.RootID {
		t.Fatalf("the first EnsureDirectory = %+v, %v, %v; want the row created under the normalized name", d, created, err)
	}
	for _, name := range []string{composed, decomposed} {
		again, created, err := e.store.EnsureDirectory(e.ctx, e.second(t), blobfs.RootID, name)
		if err != nil || created || !sameDirectory(again, d) {
			t.Errorf("EnsureDirectory(%q) again = %+v, %v, %v; want the same row found", name, again, created, err)
		}
	}
	inTx, err := e.db.Transact(e.ctx, func(tx *sqlate.Tx) (blobfs.Directory, error) {
		found, created, err := e.store.EnsureDirectory(e.ctx, tx, blobfs.RootID, composed, data.WithID(blobfs.NewID()))
		if err == nil && created {
			return found, errors.New("created a second row")
		}
		return found, err
	})
	if err != nil || !sameDirectory(inTx, d) {
		t.Errorf("EnsureDirectory inside a transaction = %+v, %v; want the row found and the transaction committed", inTx, err)
	}

	id := blobfs.NewID()
	seeded, created, err := e.store.EnsureDirectory(e.ctx, e.db, blobfs.RootID, "seeded", data.WithID(id))
	if err != nil || !created || seeded.ID != id {
		t.Fatalf("EnsureDirectory with an id = %+v, %v, %v; want the row created under the id", seeded, created, err)
	}
	if got, err := e.store.Directory(e.ctx, e.db, id); err != nil || !sameDirectory(got, seeded) {
		t.Errorf("Directory(%s) = %+v, %v; want the seeded row", id, got, err)
	}
	if n := count(t, e.db, "SELECT COUNT(*) FROM blobfs_directory WHERE parent_id = $1", blobfs.RootID); n != 2 {
		t.Errorf("the root holds %d directories, want 2", n)
	}
}

// TestEnsureDirectoryConcurrent proves the race on the pool: two callers
// on separate connections ensure the same name at once, exactly one
// creates it, both return the same row, and the directory holds one row.
// The engine blocks the second insert on the unique index until the
// first commits and then refuses it, and the second caller recovers by
// looking the row up.
func TestEnsureDirectoryConcurrent(t *testing.T) {
	e := open(t)
	pools := []*sqlate.DB{e.db, e.second(t)}
	var (
		start   sync.WaitGroup
		done    sync.WaitGroup
		mu      sync.Mutex
		results []blobfs.Directory
		creates int
	)
	start.Add(1)
	for _, pool := range pools {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			d, created, err := e.store.EnsureDirectory(e.ctx, pool, blobfs.RootID, "shared")
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				t.Errorf("EnsureDirectory: %v", err)
				return
			}
			results = append(results, d)
			if created {
				creates++
			}
		}()
	}
	start.Done()
	done.Wait()
	if len(results) != 2 || !sameDirectory(results[0], results[1]) || creates != 1 {
		t.Errorf("the two callers got %+v with %d creates; want the same row and one create", results, creates)
	}
	if n := count(t, e.db, "SELECT COUNT(*) FROM blobfs_directory WHERE name = 'shared'"); n != 1 {
		t.Errorf("%d rows are named shared, want one", n)
	}
}

// TestBeginOrResumeFileWriteOnTheEngine proves the outcomes against the
// schema, over the baseline and over the Postgres variant since the
// deleting case goes through the variant's delete begin: a free name is
// created pending; the same name is then resumed as the same row, at its
// version, and inserted nothing; once the write completes the name
// exists as available; once a delete begins it exists as deleting; and
// the directory holds one row throughout. A supplied id names the
// created row and is ignored when the name is found.
func TestBeginOrResumeFileWriteOnTheEngine(t *testing.T) {
	for name, open := range map[string]func(*testing.T) env{"standard": open, "postgres": openPostgres} {
		t.Run(name, func(t *testing.T) {
			e := open(t)
			dir := e.mkdir(t, blobfs.RootID, "docs")
			id := blobfs.NewID()
			f, outcome, err := e.store.BeginOrResumeFileWrite(e.ctx, e.db, anyKey{}, dir.ID, decomposed+".txt", "text/plain", data.WithID(id))
			if err != nil || outcome != data.WriteCreated || f.ID != id || f.Status != blobfs.StatusPending || f.Key != id+"/"+composed+".txt" || f.Version != 1 {
				t.Fatalf("the first call = %+v, %v, %v; want the pending row created under the id", f, outcome, err)
			}
			resumed, outcome, err := e.store.BeginOrResumeFileWrite(e.ctx, e.second(t), anyKey{}, dir.ID, composed+".txt", "text/plain", data.WithID(blobfs.NewID()))
			if err != nil || outcome != data.WriteResumed || !sameFile(resumed, f) {
				t.Errorf("the second call = %+v, %v, %v; want the same pending row resumed", resumed, outcome, err)
			}
			done, err := e.store.CompleteFileWrite(e.ctx, e.db, resumed.ID, resumed.Version, blobfs.Object{Size: 4, ContentType: "text/plain", ETag: `"x"`})
			if err != nil {
				t.Fatalf("CompleteFileWrite: %v", err)
			}
			exists, outcome, err := e.store.BeginOrResumeFileWrite(e.ctx, e.db, anyKey{}, dir.ID, composed+".txt", "text/plain")
			if err != nil || outcome != data.WriteExists || !sameFile(exists, done) || exists.Status != blobfs.StatusAvailable {
				t.Errorf("the call over the available row = %+v, %v, %v; want the row as it is and exists", exists, outcome, err)
			}
			deleting, err := e.db.Transact(e.ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
				return e.store.BeginFileDelete(e.ctx, tx, f.ID)
			})
			if err != nil {
				t.Fatalf("BeginFileDelete: %v", err)
			}
			exists, outcome, err = e.store.BeginOrResumeFileWrite(e.ctx, e.db, anyKey{}, dir.ID, composed+".txt", "text/plain")
			if err != nil || outcome != data.WriteExists || !sameFile(exists, deleting) || exists.Status != blobfs.StatusDeleting {
				t.Errorf("the call over the deleting row = %+v, %v, %v; want the row as it is and exists", exists, outcome, err)
			}
			if n := count(t, e.db, "SELECT COUNT(*) FROM blobfs_file WHERE directory_id = $1", dir.ID); n != 1 {
				t.Errorf("the directory holds %d files, want one", n)
			}
			if _, _, err := e.store.BeginOrResumeFileWrite(e.ctx, e.db, anyKey{}, blobfs.NewID(), "orphan.txt", "text/plain"); !errors.Is(err, blobfs.ErrNotFound) {
				t.Errorf("a call in a missing directory = %v, want ErrNotFound", err)
			}
		})
	}
}

// TestBeginOrResumeFileWriteConcurrent proves the race on the pool: two
// callers on separate connections write the same free name at once,
// exactly one creates the pending row, the other resumes it, and both
// return the same row.
func TestBeginOrResumeFileWriteConcurrent(t *testing.T) {
	e := open(t)
	dir := e.mkdir(t, blobfs.RootID, "docs")
	pools := []*sqlate.DB{e.db, e.second(t)}
	var (
		start    sync.WaitGroup
		done     sync.WaitGroup
		mu       sync.Mutex
		results  []blobfs.File
		outcomes []data.WriteOutcome
	)
	start.Add(1)
	for _, pool := range pools {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			f, outcome, err := e.store.BeginOrResumeFileWrite(e.ctx, pool, anyKey{}, dir.ID, "shared.txt", "text/plain")
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				t.Errorf("BeginOrResumeFileWrite: %v", err)
				return
			}
			results = append(results, f)
			outcomes = append(outcomes, outcome)
		}()
	}
	start.Done()
	done.Wait()
	if len(results) != 2 || !sameFile(results[0], results[1]) {
		t.Errorf("the two callers got %+v; want the same row", results)
	}
	created := 0
	for _, o := range outcomes {
		switch o {
		case data.WriteCreated:
			created++
		case data.WriteResumed:
		default:
			t.Errorf("an outcome is %s; want created or resumed", o)
		}
	}
	if created != 1 {
		t.Errorf("%d callers created the row, want one", created)
	}
}

// TestSuppliedIDsOnTheEngine proves the caller-supplied id through the
// schema: a directory and a file are created under given ids, the file's
// key starts with its id, a delete through the two steps frees the id
// and a write under the same id and name reuses the same key, and an id
// a row still carries is ErrIDTaken under the table's primary key, with
// the driver's unique violation reachable and its text hidden. The nil
// UUID and text that is no UUID are refused before any row is touched.
func TestSuppliedIDsOnTheEngine(t *testing.T) {
	e := open(t)
	dirID, fileID := blobfs.NewID(), blobfs.NewID()
	dir, err := e.store.Mkdir(e.ctx, e.db, blobfs.RootID, "seed", data.WithID(dirID))
	if err != nil || dir.ID != dirID {
		t.Fatalf("Mkdir with an id = %+v, %v", dir, err)
	}
	f, err := e.store.BeginFileWrite(e.ctx, e.db, anyKey{}, dir.ID, "fixture.txt", "text/plain", data.WithID(fileID))
	if err != nil || f.ID != fileID || f.Key != fileID+"/fixture.txt" {
		t.Fatalf("BeginFileWrite with an id = %+v, %v; want the row and the key under the id", f, err)
	}

	for constraint, insert := range map[string]func() error{
		blobfs.ConstraintPrimaryKeyDirectory: func() error {
			_, err := e.store.Mkdir(e.ctx, e.db, blobfs.RootID, "other", data.WithID(dirID))
			return err
		},
		blobfs.ConstraintPrimaryKeyFile: func() error {
			_, err := e.store.BeginFileWrite(e.ctx, e.db, anyKey{}, dir.ID, "other.txt", "text/plain", data.WithID(fileID))
			return err
		},
	} {
		err := insert()
		if !errors.Is(err, blobfs.ErrIDTaken) {
			t.Errorf("an insert under a taken id = %v, want ErrIDTaken", err)
		}
		if name := livetest.Constraint(t, err, sqlate.ErrUniqueViolation); name != constraint {
			t.Errorf("the refusal names %q, want %s", name, constraint)
		}
		if want := blobfs.ErrIDTaken.Error() + " (constraint " + constraint + ")"; !strings.HasSuffix(err.Error(), want) || strings.Contains(err.Error(), "SQLSTATE") {
			t.Errorf("the refusal reads %q, want a message ending with %q and no driver text", err, want)
		}
	}
	if n := count(t, e.db, "SELECT COUNT(*) FROM blobfs_directory WHERE name = 'other'") + count(t, e.db, "SELECT COUNT(*) FROM blobfs_file WHERE name = 'other.txt'"); n != 0 {
		t.Errorf("%d rows exist under the refused inserts, want none", n)
	}

	if _, err := e.db.Transact(e.ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
		return e.store.BeginFileDelete(e.ctx, tx, fileID)
	}); err != nil {
		t.Fatalf("BeginFileDelete: %v", err)
	}
	if err := e.store.CompleteFileDelete(e.ctx, e.db, fileID); err != nil {
		t.Fatalf("CompleteFileDelete: %v", err)
	}
	again, err := e.store.BeginFileWrite(e.ctx, e.db, anyKey{}, dir.ID, "fixture.txt", "text/plain", data.WithID(fileID))
	if err != nil || again.ID != fileID || again.Key != f.Key || again.Version != 1 {
		t.Errorf("BeginFileWrite under the freed id = %+v, %v; want a fresh row under the same id and key", again, err)
	}

	for _, bad := range []string{blobfs.RootID, "not-a-uuid"} {
		if _, err := e.store.Mkdir(e.ctx, e.db, blobfs.RootID, "bad", data.WithID(bad)); !errors.Is(err, blobfs.ErrInvalidID) {
			t.Errorf("Mkdir with the id %q = %v, want ErrInvalidID", bad, err)
		}
	}
	if n := count(t, e.db, "SELECT COUNT(*) FROM blobfs_directory WHERE name = 'bad'"); n != 0 {
		t.Errorf("%d rows exist under the refused ids, want none", n)
	}
}
