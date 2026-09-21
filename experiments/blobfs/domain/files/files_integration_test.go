//go:build integration

package files_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/postgres"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/domain/files"
	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	blobfsmigrations "github.com/standards-lab/org/experiments/blobfs/lib/blobfs/migrations"
	"github.com/standards-lab/org/experiments/blobfs/lib/migrator"
	appmigrations "github.com/standards-lab/org/experiments/blobfs/migrations"
)

// env is one test's throwaway database with both migration sets applied
// and the store verified against it.
type env struct {
	ctx   context.Context
	db    *sqlate.DB
	dsn   string
	store *files.Store
}

// open applies blobfs's set and then the consumer's through the migrator,
// as schema up does, builds the store over an object store opener that
// targets a container of the test's own, and proves Verify against the
// migrated schema on the way. The opener is the composition root's:
// OpenStorage over the BLOBFS_STORAGE_* variables, with the container
// overridden for the test.
func open(t *testing.T) env {
	t.Helper()
	ctx := context.Background()
	db, dsn := livetest.OpenDSN(t)
	t.Setenv("BLOBFS_STORAGE_CONTAINER", livetest.Container(t))
	blobfsSet, err := blobfsmigrations.Migrations(db.Dialect())
	if err != nil {
		t.Fatalf("blobfs Migrations: %v", err)
	}
	consumerSet, err := appmigrations.Migrations()
	if err != nil {
		t.Fatalf("consumer Migrations: %v", err)
	}
	m, err := migrator.New(db, []migrator.Set{
		{Name: blobfsmigrations.Source, Table: blobfsmigrations.Table, Migrations: blobfsSet},
		{Name: "consumer", Migrations: consumerSet},
	}, migrator.Options{})
	if err != nil {
		t.Fatalf("migrator.New: %v", err)
	}
	if err := m.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	s, err := files.New(db, func(ctx context.Context) (*files.Storage, error) {
		return files.OpenStorage(ctx, "blobfs")
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Verify(ctx); err != nil {
		t.Fatalf("Verify against the migrated schema: %v", err)
	}
	return env{ctx: ctx, db: db, dsn: dsn, store: s}
}

// second opens a second pool to the same database, so a write through it
// runs on a connection of its own.
func (e env) second(t *testing.T) *sqlate.DB {
	t.Helper()
	pool, err := sql.Open("pgx", e.dsn)
	if err != nil {
		t.Fatalf("open a second pool: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	return sqlate.Wrap(pool, postgres.Dialect{})
}

// count runs an independent COUNT(*) query with its arguments.
func (e env) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	rows, err := e.db.QueryContext(e.ctx, sql, args...)
	if err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	defer func() { _ = rows.Close() }()
	var n int
	if !rows.Next() {
		t.Fatalf("%s: no row: %v", sql, rows.Err())
	}
	if err := rows.Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

// strings1 runs a one-column text query and returns the column.
func (e env) strings1(t *testing.T, sql string, args ...any) []string {
	t.Helper()
	rows, err := e.db.QueryContext(e.ctx, sql, args...)
	if err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return out
}

// mkdir creates a directory at a path, owned by unit when one is given,
// and fails the test on any error.
func (e env) mkdir(t *testing.T, path, unit string) blobfs.Directory {
	t.Helper()
	d, err := e.store.Mkdir(e.ctx, path, unit)
	if err != nil {
		t.Fatalf("Mkdir(%s): %v", path, err)
	}
	return d
}

// insertFile inserts an available file row directly through sess, since
// the write path is a later stage, and returns its id.
func insertFile(ctx context.Context, t *testing.T, sess sqlate.Session, dir, name string) string {
	t.Helper()
	id := blobfs.NewID()
	_, err := sess.ExecContext(ctx,
		"INSERT INTO blobfs_file (id, directory_id, name, status, key, content_type, size) VALUES ($1, $2, $3, 'available', $4, 'text/plain', $5)",
		id, dir, name, id+"/"+blobfs.SanitizeFilename(name), len(name))
	if err != nil {
		t.Fatalf("insert file %s: %v", name, err)
	}
	return id
}

// list runs List and fails the test on any error.
func (e env) list(t *testing.T, path string, l files.Listing) files.Contents {
	t.Helper()
	c, err := e.store.List(e.ctx, path, l)
	if err != nil {
		t.Fatalf("List(%s, %+v): %v", path, l, err)
	}
	return c
}

// names returns the names of a directory page.
func names(dirs []blobfs.Directory) []string {
	var out []string
	for _, d := range dirs {
		out = append(out, *d.Name)
	}
	return out
}

// TestMkdir proves directory creation through the consumer: nested paths
// under the root, the owner row written with the directory for a
// top-level path under a unit, and the refusals: --unit below depth one
// writes nothing, the root, a trailing slash, a missing parent, a taken
// name, and a unit id the engine cannot read, which rolls the directory
// back after blobfs's insert succeeded inside the transaction.
func TestMkdir(t *testing.T) {
	e := open(t)
	unit := blobfs.NewID()
	a := e.mkdir(t, "/a", "")
	b := e.mkdir(t, "/a/b", "")
	c := e.mkdir(t, "/a/b/c", "")
	if a.ParentID == nil || *a.ParentID != blobfs.RootID || *b.ParentID != a.ID || *c.ParentID != b.ID || *c.Name != "c" {
		t.Errorf("rows = %+v %+v %+v", a, b, c)
	}
	owned := e.mkdir(t, "/owned", unit)
	if units := e.strings1(t, "SELECT CAST(unit_id AS text) FROM directory_owner WHERE directory_id = $1", owned.ID); len(units) != 1 || units[0] != unit {
		t.Errorf("owner of /owned = %v, want %s", units, unit)
	}
	if n := e.count(t, "SELECT COUNT(*) FROM directory_owner"); n != 1 {
		t.Errorf("owner rows = %d, want the one for /owned", n)
	}

	for path, want := range map[string]error{
		"/a/b/d": files.ErrUnitDepth,
		"/":      blobfs.ErrRootDirectory,
		"/x/":    blobfs.ErrInvalidPath,
	} {
		if _, err := e.store.Mkdir(e.ctx, path, unit); !errors.Is(err, want) {
			t.Errorf("Mkdir(%s, unit) = %v, want %v", path, err, want)
		}
	}
	for path, want := range map[string]error{
		"/x/y":     blobfs.ErrNotFound,
		"/a/b":     blobfs.ErrNameTaken,
		"/a//c":    blobfs.ErrInvalidPath,
		"/a/..":    blobfs.ErrInvalidName,
		"relative": blobfs.ErrInvalidPath,
	} {
		if _, err := e.store.Mkdir(e.ctx, path, ""); !errors.Is(err, want) {
			t.Errorf("Mkdir(%s) = %v, want %v", path, err, want)
		}
	}
	_, err := e.store.Mkdir(e.ctx, "/orphan", "not-a-uuid")
	if err == nil || !errors.Is(err, sqlate.ErrInvalidValue) {
		t.Errorf("Mkdir with an unreadable unit = %v, want the owner insert's data exception", err)
	}
	if n := e.count(t, "SELECT COUNT(*) FROM blobfs_directory"); n != 5 {
		t.Errorf("directories = %d, want the root and four: the refused ones left nothing", n)
	}
	if n := e.count(t, "SELECT COUNT(*) FROM blobfs_directory WHERE name = 'orphan'"); n != 0 {
		t.Error("the directory of the refused owner survived the rollback")
	}
}

// TestList proves ls without a unit: under /a the directories come one
// page at a time sorted by name with their total, the files one page at a
// time under the caller's sort in both directions with the total equal to
// an independent count, a sort by a file-only field sorts the files and
// leaves the directories in name order, TotalNone reports NoTotal, an
// empty later page reports NoTotal under TotalExact while an empty first
// page reports 0, a missing path is ErrNotFound, and an unknown sort
// field is ErrDirectives.
func TestList(t *testing.T) {
	e := open(t)
	a := e.mkdir(t, "/a", "")
	e.mkdir(t, "/a/z", "")
	e.mkdir(t, "/a/m", "")
	e.mkdir(t, "/a/b", "")
	deep := e.mkdir(t, "/a/b/c", "")
	fileNames := []string{"r.txt", "b.txt", "a.txt", "m.txt", "z.txt", "k.txt", "q.txt"}
	for _, name := range fileNames {
		insertFile(e.ctx, t, e.db, a.ID, name)
	}
	insertFile(e.ctx, t, e.db, deep.ID, "deep.txt")

	var dirs []string
	for page := 1; page <= 2; page++ {
		c := e.list(t, "/a", files.Listing{Page: page, Size: 2})
		if c.Directories.Total != 3 || c.Files.Total != len(fileNames) || c.Path != "/a" {
			t.Errorf("page %d totals = %d directories, %d files; want 3, %d", page, c.Directories.Total, c.Files.Total, len(fileNames))
		}
		dirs = append(dirs, names(c.Directories.Rows)...)
	}
	if want := []string{"b", "m", "z"}; !slices.Equal(dirs, want) {
		t.Errorf("directories paged = %v, want %v", dirs, want)
	}

	for _, field := range []string{"name", "size"} {
		for _, desc := range []bool{false, true} {
			dir := "ASC"
			if desc {
				dir = "DESC"
			}
			want := e.strings1(t, fmt.Sprintf("SELECT name FROM blobfs_file WHERE directory_id = $1 ORDER BY %s %s, name %s", field, dir, dir), a.ID)
			var got []string
			for page := 1; page <= 3; page++ {
				c := e.list(t, "/a", files.Listing{Page: page, Size: 3, Sort: []files.Sort{{Field: field, Descending: desc}}})
				if c.Files.Total != len(fileNames) {
					t.Errorf("%s %s page %d: file total %d, want %d", field, dir, page, c.Files.Total, len(fileNames))
				}
				wantDirs := []string{"b", "m", "z"}
				if field == "name" && desc {
					wantDirs = []string{"z", "m", "b"}
				}
				if page == 1 && !slices.Equal(names(c.Directories.Rows), wantDirs) {
					t.Errorf("%s %s: directories = %v, want %v: a shared field sorts both halves, a file-only one leaves name order", field, dir, names(c.Directories.Rows), wantDirs)
				}
				for _, f := range c.Files.Rows {
					got = append(got, f.Name)
					if f.DirectoryID != a.ID || f.Size == nil || f.Status != blobfs.StatusAvailable {
						t.Errorf("file %+v: want directory %s, a size, available", f, a.ID)
					}
				}
			}
			if !slices.Equal(got, want) {
				t.Errorf("%s %s: pages concatenated = %v, want %v", field, dir, got, want)
			}
		}
	}

	c := e.list(t, "/a/b/c", files.Listing{Page: 1, Size: 10})
	if c.Files.Total != 1 || c.Directories.Total != 0 || c.Files.Rows[0].Name != "deep.txt" || len(c.Directories.Rows) != 0 {
		t.Errorf("List(/a/b/c) = %+v", c)
	}
	c = e.list(t, "/a", files.Listing{Page: 1, Size: 10, Total: files.TotalNone})
	if c.Files.Total != files.NoTotal || c.Directories.Total != files.NoTotal || len(c.Files.Rows) != 7 || len(c.Directories.Rows) != 3 {
		t.Errorf("List(/a) without a total = %+v", c)
	}
	c = e.list(t, "/a", files.Listing{Page: 4, Size: 10})
	if c.Files.Total != files.NoTotal || c.Directories.Total != files.NoTotal || len(c.Files.Rows) != 0 {
		t.Errorf("an empty later page = %+v, want NoTotal on both halves", c)
	}
	c = e.list(t, "/a/b/c", files.Listing{Page: 1, Size: 10})
	if c.Directories.Total != 0 {
		t.Errorf("an empty first page reports total %d, want 0", c.Directories.Total)
	}
	c = e.list(t, "/", files.Listing{Page: 1, Size: 10})
	if !slices.Equal(names(c.Directories.Rows), []string{"a"}) || c.Files.Total != 0 {
		t.Errorf("List(/) = %+v, want the one top-level directory and no files", c)
	}

	if _, err := e.store.List(e.ctx, "/a/missing", files.Listing{Page: 1, Size: 10}); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("List(/a/missing) = %v, want ErrNotFound", err)
	}
	if _, err := e.store.List(e.ctx, "/a", files.Listing{Page: 1, Size: 10, Sort: []files.Sort{{Field: "owner"}}}); !errors.Is(err, query.ErrDirectives) {
		t.Errorf("List sorted by an undeclared field = %v, want ErrDirectives", err)
	}
}

// TestListByCursor proves the consumer's cursor walk on the engine: each
// half is continued from its own cursor, the halves walked by cursor
// return the rows the halves paged by number return, a half read after a
// cursor carries no total while the other half keeps its total, and a
// cursor at / under a unit is refused before any I/O.
func TestListByCursor(t *testing.T) {
	e := open(t)
	unit := blobfs.NewID()
	a := e.mkdir(t, "/a", unit)
	for _, name := range []string{"z", "m", "b", "q", "c"} {
		e.mkdir(t, "/a/"+name, "")
	}
	for _, name := range []string{"r.txt", "b.txt", "a.txt", "m.txt", "z.txt", "k.txt", "q.txt"} {
		insertFile(e.ctx, t, e.db, a.ID, name)
	}
	l := files.Listing{Page: 1, Size: 2, Sort: []files.Sort{{Field: "name", Descending: true}}}
	var dirs, fileNames []string
	for {
		c := e.list(t, "/a", l)
		dirs = append(dirs, names(c.Directories.Rows)...)
		for _, f := range c.Files.Rows {
			fileNames = append(fileNames, f.Name)
		}
		if l.After.Directories != "" && c.Directories.Total != files.NoTotal {
			t.Errorf("a directory half after a cursor reports total %d", c.Directories.Total)
		}
		if l.After.Files == "" && c.Files.Total != 7 {
			t.Errorf("the file half by number reports total %d, want 7", c.Files.Total)
		}
		if c.Directories.Next == "" && c.Files.Next == "" {
			break
		}
		if c.Directories.Next != "" {
			l.After.Directories = c.Directories.Next
		} else {
			// The directory half is exhausted; keep reading its last page by
			// cursor so it lists nothing more, as a client would.
			l.After.Directories = ""
			l.Page = 100
		}
		l.After.Files = c.Files.Next
	}
	if !slices.Equal(dirs, []string{"z", "q", "m", "c", "b"}) {
		t.Errorf("directories by cursor = %v", dirs)
	}
	if !slices.Equal(fileNames, []string{"z.txt", "r.txt", "q.txt", "m.txt", "k.txt", "b.txt", "a.txt"}) {
		t.Errorf("files by cursor = %v", fileNames)
	}
	_, err := e.store.List(e.ctx, "/", files.Listing{Page: 1, Size: 2, Unit: unit, After: files.After{Directories: "x"}})
	if !errors.Is(err, files.ErrNoCursorAtRoot) {
		t.Errorf("ls / --unit with a cursor = %v, want ErrNoCursorAtRoot", err)
	}
}

// TestListScope is the stage gate's scoping proof: the owning unit lists
// its top-level directory and everything below it, another unit is
// refused with ErrNotOwned at any depth, a top-level directory without an
// owner row is refused for every unit, and at the root a unit lists only
// its own top-level directories with a total that agrees, through the
// owner read model, with no files.
func TestListScope(t *testing.T) {
	e := open(t)
	unit, other := blobfs.NewID(), blobfs.NewID()
	docs := e.mkdir(t, "/docs", unit)
	e.mkdir(t, "/docs/2026", "")
	e.mkdir(t, "/beta", other)
	e.mkdir(t, "/alpha", unit)
	e.mkdir(t, "/shared", "")
	insertFile(e.ctx, t, e.db, docs.ID, "plan.txt")
	insertFile(e.ctx, t, e.db, blobfs.RootID, "root.txt")

	c := e.list(t, "/docs", files.Listing{Page: 1, Size: 10, Unit: unit})
	if !slices.Equal(names(c.Directories.Rows), []string{"2026"}) || c.Files.Total != 1 || c.Files.Rows[0].Name != "plan.txt" {
		t.Errorf("List(/docs) as the owner = %+v", c)
	}
	c = e.list(t, "/docs/2026", files.Listing{Page: 1, Size: 10, Unit: unit})
	if c.Directories.Total != 0 || c.Files.Total != 0 || c.Path != "/docs/2026" {
		t.Errorf("List(/docs/2026) as the owner = %+v", c)
	}
	for _, path := range []string{"/docs", "/docs/2026", "/shared", "/shared/x"} {
		_, err := e.store.List(e.ctx, path, files.Listing{Page: 1, Size: 10, Unit: other})
		if !errors.Is(err, files.ErrNotOwned) {
			t.Errorf("List(%s) as another unit = %v, want ErrNotOwned", path, err)
		}
	}
	if _, err := e.store.List(e.ctx, "/docs/missing", files.Listing{Page: 1, Size: 10, Unit: unit}); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("List(/docs/missing) as the owner = %v, want ErrNotFound", err)
	}
	if _, err := e.store.List(e.ctx, "/missing", files.Listing{Page: 1, Size: 10, Unit: unit}); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("List(/missing) as a unit = %v, want ErrNotFound", err)
	}

	c = e.list(t, "/", files.Listing{Page: 1, Size: 10, Unit: unit})
	if !slices.Equal(names(c.Directories.Rows), []string{"alpha", "docs"}) || c.Directories.Total != 2 {
		t.Errorf("List(/) as the unit = %v, total %d; want alpha and docs", names(c.Directories.Rows), c.Directories.Total)
	}
	if c.Files.Total != 0 || len(c.Files.Rows) != 0 {
		t.Errorf("List(/) as the unit lists files %+v; the root's files belong to no unit", c.Files)
	}
	for _, d := range c.Directories.Rows {
		if d.ParentID == nil || *d.ParentID != blobfs.RootID || d.Version != 1 || d.CreatedAt.IsZero() {
			t.Errorf("owned directory %+v lacks the library columns", d)
		}
	}
	c = e.list(t, "/", files.Listing{Page: 2, Size: 1, Unit: unit, Sort: []files.Sort{{Field: "name", Descending: true}}})
	if !slices.Equal(names(c.Directories.Rows), []string{"alpha"}) || c.Directories.Total != 2 {
		t.Errorf("List(/) as the unit, page 2 of 1 by name desc = %v, total %d", names(c.Directories.Rows), c.Directories.Total)
	}
	c = e.list(t, "/", files.Listing{Page: 1, Size: 10, Unit: other})
	if !slices.Equal(names(c.Directories.Rows), []string{"beta"}) || c.Directories.Total != 1 {
		t.Errorf("List(/) as the other unit = %v, total %d; want beta", names(c.Directories.Rows), c.Directories.Total)
	}
	c = e.list(t, "/", files.Listing{Page: 1, Size: 10, Unit: blobfs.NewID()})
	if len(c.Directories.Rows) != 0 || c.Directories.Total != 0 {
		t.Errorf("List(/) as a unit that owns nothing = %+v", c.Directories)
	}
	c = e.list(t, "/", files.Listing{Page: 1, Size: 10})
	if !slices.Equal(names(c.Directories.Rows), []string{"alpha", "beta", "docs", "shared"}) || c.Files.Total != 1 {
		t.Errorf("List(/) without a unit = %v, %d files; want every top-level directory and the root's file", names(c.Directories.Rows), c.Files.Total)
	}
}

// TestListHalvesAgreeUnderConcurrentWrites is the stage gate's engine
// proof that one ls is one snapshot: a second connection commits, in one
// transaction each, a directory and a file into the listed directory as
// fast as it can, while the listing runs repeatedly. Under the read-only
// repeatable-read transaction both halves see the same commits, so the
// two totals are equal on every run; halves read on the pool could differ
// by the pairs committed between them.
func TestListHalvesAgreeUnderConcurrentWrites(t *testing.T) {
	e := open(t)
	target := e.mkdir(t, "/target", "")
	writer := e.second(t)
	ctx, cancel := context.WithCancel(e.ctx)
	done := make(chan error, 1)
	go func() {
		defer close(done)
		for i := 0; ctx.Err() == nil; i++ {
			_, err := writer.Transact(ctx, func(tx *sqlate.Tx) (struct{}, error) {
				name := fmt.Sprintf("pair-%04d", i)
				if _, err := tx.ExecContext(ctx, "INSERT INTO blobfs_directory (id, parent_id, name) VALUES ($1, $2, $3)", blobfs.NewID(), target.ID, name); err != nil {
					return struct{}{}, err
				}
				id := blobfs.NewID()
				_, err := tx.ExecContext(ctx, "INSERT INTO blobfs_file (id, directory_id, name, status, key, content_type) VALUES ($1, $2, $3, 'available', $4, 'text/plain')", id, target.ID, name, id+"/"+name)
				return struct{}{}, err
			})
			if err != nil && ctx.Err() == nil {
				done <- err
				return
			}
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	runs := 0
	for time.Now().Before(deadline) {
		c := e.list(t, "/target", files.Listing{Page: 1, Size: 1})
		runs++
		if c.Directories.Total != c.Files.Total {
			t.Fatalf("run %d: %d directories but %d files; the halves saw different snapshots", runs, c.Directories.Total, c.Files.Total)
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("writer: %v", err)
	}
	pairs := e.count(t, "SELECT COUNT(*) FROM blobfs_directory WHERE parent_id = $1", target.ID)
	if runs < 10 || pairs < 10 {
		t.Errorf("%d listings against %d committed pairs; too few to have interleaved", runs, pairs)
	}
	t.Logf("%d listings agreed while %d pairs were committed", runs, pairs)
}
