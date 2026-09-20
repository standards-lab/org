//go:build integration

package data_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/migrate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/migrations"
)

// env is one test's throwaway database with blobfs's migration set applied,
// the consumer's catalog, and the store compiled against it.
type env struct {
	ctx     context.Context
	db      *sqlate.DB
	catalog *query.Catalog
	store   *data.Store
}

// open builds the environment and proves Verify against the migrated
// schema on the way, so every test starts from a store whose statements
// the schema satisfies.
func open(t *testing.T) env {
	t.Helper()
	ctx := context.Background()
	db := livetest.Open(t)
	set, err := migrations.Migrations(db.Dialect())
	if err != nil {
		t.Fatalf("Migrations: %v", err)
	}
	m, err := migrate.New(db, set, migrate.Options{Table: migrations.Table})
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	if err := m.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	c, err := query.NewCatalog(query.Patterns(), data.Patterns())
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	s, err := data.New(c, db.Dialect())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Verify(ctx, db); err != nil {
		t.Fatalf("Verify against the migrated schema: %v", err)
	}
	return env{ctx: ctx, db: db, catalog: c, store: s}
}

// pair is a volume and its root directory, as CreateVolume returns them.
type pair struct {
	volume blobfs.Volume
	root   blobfs.Directory
}

// createVolume creates a volume in its own transaction and fails the test
// on any error.
func (e env) createVolume(t *testing.T, name string) pair {
	t.Helper()
	p, err := e.db.Transact(e.ctx, func(tx *sqlate.Tx) (pair, error) {
		v, root, err := e.store.CreateVolume(e.ctx, tx, name)
		return pair{v, root}, err
	})
	if err != nil {
		t.Fatalf("CreateVolume(%q): %v", name, err)
	}
	return p
}

// mkdir creates a directory and fails the test on any error.
func (e env) mkdir(t *testing.T, parentID, name string) blobfs.Directory {
	t.Helper()
	d, err := e.store.Mkdir(e.ctx, e.db, parentID, name)
	if err != nil {
		t.Fatalf("Mkdir(%q): %v", name, err)
	}
	return d
}

// insertFile inserts an available file row directly, since the write path
// is a later stage, and returns its id.
func (e env) insertFile(t *testing.T, dir, name string) string {
	t.Helper()
	id := blobfs.NewID()
	_, err := e.db.ExecContext(e.ctx,
		"INSERT INTO blobfs_file (id, directory_id, name, status, key, content_type) VALUES ($1, $2, $3, 'available', $4, 'text/plain')",
		id, dir, name, id+"/"+blobfs.SanitizeFilename(name))
	if err != nil {
		t.Fatalf("insert file %s: %v", name, err)
	}
	return id
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

const (
	composed   = "café"  // é as one code point
	decomposed = "café" // e followed by a combining acute accent
)

// TestCreateVolume proves the pair: the volume and its root come back as
// the database holds them, the root carries the volume's id and no parent
// and no name, RootDirectory finds it through the back-reference, and the
// two rows are one unit, so a transaction rolled back after the call
// leaves neither.
func TestCreateVolume(t *testing.T) {
	e := open(t)
	p := e.createVolume(t, "vol")
	if p.volume.Name != "vol" || p.volume.Version != 1 || p.volume.CreatedAt.IsZero() || p.volume.UpdatedAt.IsZero() {
		t.Errorf("volume = %+v, want name vol, version 1, timestamps set", p.volume)
	}
	if p.root.ParentID != nil || p.root.Name != nil || p.root.VolumeID == nil || *p.root.VolumeID != p.volume.ID || p.root.Version != 1 {
		t.Errorf("root = %+v, want no parent, no name, volume %s", p.root, p.volume.ID)
	}
	root, err := e.store.RootDirectory(e.ctx, e.db, p.volume.ID)
	if err != nil || root.ID != p.root.ID {
		t.Errorf("RootDirectory = %+v, %v, want %s", root, err, p.root.ID)
	}
	v, err := e.store.Volume(e.ctx, e.db, p.volume.ID)
	if err != nil || v != p.volume {
		t.Errorf("Volume = %+v, %v, want %+v", v, err, p.volume)
	}
	v, err = e.store.VolumeByName(e.ctx, e.db, "vol")
	if err != nil || v != p.volume {
		t.Errorf("VolumeByName = %+v, %v, want %+v", v, err, p.volume)
	}

	_, err = e.db.Transact(e.ctx, func(tx *sqlate.Tx) (pair, error) {
		v, root, err := e.store.CreateVolume(e.ctx, tx, "abandoned")
		if err != nil {
			return pair{}, err
		}
		return pair{v, root}, errors.New("abandon")
	})
	if err == nil || err.Error() != "abandon" {
		t.Fatalf("abandoned transaction = %v", err)
	}
	if n := e.count(t, "SELECT COUNT(*) FROM blobfs_volume") + e.count(t, "SELECT COUNT(*) FROM blobfs_directory"); n != 2 {
		t.Errorf("rows after the rollback = %d, want 2 (the one volume and its root)", n)
	}
}

// TestCreateVolumeRefusals proves the refusals: the pool session is
// refused with ErrTransactionRequired and writes nothing, a duplicate name
// is ErrNameTaken, and the composed and decomposed spellings of one name
// collide because names are normalized before the insert and compared as
// stored.
func TestCreateVolumeRefusals(t *testing.T) {
	e := open(t)
	if _, _, err := e.store.CreateVolume(e.ctx, e.db, "vol"); !errors.Is(err, query.ErrTransactionRequired) {
		t.Errorf("CreateVolume on the pool = %v, want ErrTransactionRequired", err)
	}
	if n := e.count(t, "SELECT COUNT(*) FROM blobfs_volume"); n != 0 {
		t.Errorf("volumes after the refusal = %d, want 0", n)
	}
	e.createVolume(t, "vol")
	e.createVolume(t, composed)
	for _, name := range []string{"vol", composed, decomposed} {
		_, err := e.db.Transact(e.ctx, func(tx *sqlate.Tx) (pair, error) {
			v, root, err := e.store.CreateVolume(e.ctx, tx, name)
			return pair{v, root}, err
		})
		if !errors.Is(err, blobfs.ErrNameTaken) {
			t.Errorf("CreateVolume(%q) again = %v, want ErrNameTaken", name, err)
		}
		var ce *sqlate.ConstraintError
		if !errors.As(err, &ce) || ce.Constraint != blobfs.ConstraintUniqueVolumeName {
			t.Errorf("CreateVolume(%q) again does not carry the constraint: %v", name, err)
		}
	}
	v, err := e.store.VolumeByName(e.ctx, e.db, decomposed)
	if err != nil || v.Name != composed {
		t.Errorf("VolumeByName(decomposed) = %+v, %v, want the composed row", v, err)
	}
	if n := e.count(t, "SELECT COUNT(*) FROM blobfs_volume"); n != 2 {
		t.Errorf("volumes = %d, want 2", n)
	}
}

// TestNotFound proves ErrNotFound on every single-row read of a row that
// does not exist.
func TestNotFound(t *testing.T) {
	e := open(t)
	id := blobfs.NewID()
	if _, err := e.store.Volume(e.ctx, e.db, id); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("Volume(missing) = %v, want ErrNotFound", err)
	}
	if _, err := e.store.VolumeByName(e.ctx, e.db, "missing"); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("VolumeByName(missing) = %v, want ErrNotFound", err)
	}
	if _, err := e.store.RootDirectory(e.ctx, e.db, id); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("RootDirectory(missing) = %v, want ErrNotFound", err)
	}
	if _, err := e.store.Directory(e.ctx, e.db, id); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("Directory(missing) = %v, want ErrNotFound", err)
	}
	if _, err := e.store.ResolveDirectory(e.ctx, e.db, id, "/"); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("ResolveDirectory(missing volume) = %v, want ErrNotFound", err)
	}
}

// TestVolumes proves the volume listing: pages sorted by name descending
// concatenate to the independent ORDER BY, every page carries the total,
// and a LIKE filter narrows both the rows and the total to the independent
// count.
func TestVolumes(t *testing.T) {
	e := open(t)
	for _, name := range []string{"delta", "alpha", "charlie", "bravo", "echo"} {
		e.createVolume(t, name)
	}
	want := e.strings1(t, "SELECT name FROM blobfs_volume ORDER BY name DESC, id")
	var got []string
	for page := 1; ; page++ {
		rows, total, err := e.store.Volumes(e.ctx, e.db, query.Directives{
			Page: query.Page{Number: page, Size: 2},
			Sort: []query.Sort{{Field: "name", Descending: true}},
		})
		if err != nil {
			t.Fatalf("Volumes page %d: %v", page, err)
		}
		if total != 5 {
			t.Errorf("page %d total = %d, want 5", page, total)
		}
		for _, v := range rows {
			got = append(got, v.Name)
		}
		if len(rows) < 2 {
			break
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("pages concatenated = %v, want %v", got, want)
	}

	rows, total, err := e.store.Volumes(e.ctx, e.db, query.Directives{
		Page:    query.Page{Number: 1, Size: 10},
		Filters: []query.Filter{{Field: "name", Op: query.OpLike, Value: "%l%"}},
	})
	if err != nil {
		t.Fatalf("Volumes filtered: %v", err)
	}
	if n := e.count(t, "SELECT COUNT(*) FROM blobfs_volume WHERE name LIKE '%l%'"); total != n || len(rows) != n || n != 3 {
		t.Errorf("filtered listing = %d rows, total %d, want %d (alpha, charlie, delta)", len(rows), total, n)
	}
	if _, _, err := e.store.Volumes(e.ctx, e.db, query.Directives{
		Page: query.Page{Number: 1, Size: 10},
		Sort: []query.Sort{{Field: "owner"}},
	}); !errors.Is(err, query.ErrDirectives) {
		t.Errorf("Volumes sorted by an undeclared field = %v, want ErrDirectives", err)
	}
}

// TestRenameVolume proves the guard: a rename at the current version bumps
// it and the row shows the new name, a stale version is ErrVersionMismatch,
// a missing volume ErrNotFound, and a name another volume holds, in either
// spelling, ErrNameTaken.
func TestRenameVolume(t *testing.T) {
	e := open(t)
	p := e.createVolume(t, "vol")
	e.createVolume(t, composed)
	next, err := e.store.RenameVolume(e.ctx, e.db, p.volume.ID, 1, "renamed")
	if err != nil || next != 2 {
		t.Fatalf("RenameVolume = %d, %v, want version 2", next, err)
	}
	v, err := e.store.Volume(e.ctx, e.db, p.volume.ID)
	if err != nil || v.Name != "renamed" || v.Version != 2 || v.UpdatedAt.Before(v.CreatedAt) {
		t.Errorf("after rename = %+v, %v", v, err)
	}
	if _, err := e.store.RenameVolume(e.ctx, e.db, p.volume.ID, 1, "stale"); !errors.Is(err, query.ErrVersionMismatch) {
		t.Errorf("stale rename = %v, want ErrVersionMismatch", err)
	}
	if _, err := e.store.RenameVolume(e.ctx, e.db, blobfs.NewID(), 1, "missing"); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("rename of a missing volume = %v, want ErrNotFound", err)
	}
	for _, name := range []string{composed, decomposed} {
		if _, err := e.store.RenameVolume(e.ctx, e.db, p.volume.ID, 2, name); !errors.Is(err, blobfs.ErrNameTaken) {
			t.Errorf("rename to %q = %v, want ErrNameTaken", name, err)
		}
	}
	if _, err := e.store.RenameVolume(e.ctx, e.db, p.volume.ID, 2, "a/b"); !errors.Is(err, blobfs.ErrInvalidName) {
		t.Errorf("rename to a/b = %v, want ErrInvalidName", err)
	}
}

// TestMkdir proves directory creation: the row as the database holds it,
// a taken name in either spelling, an invalid name, a missing parent as
// ErrNotFound through the foreign key, the same name allowed under another
// parent, and Children listing one parent's directories with the total.
func TestMkdir(t *testing.T) {
	e := open(t)
	a := e.createVolume(t, "a")
	b := e.createVolume(t, "b")
	docs := e.mkdir(t, a.root.ID, "docs")
	if docs.ParentID == nil || *docs.ParentID != a.root.ID || docs.VolumeID != nil || docs.Name == nil || *docs.Name != "docs" || docs.Version != 1 || docs.CreatedAt.IsZero() {
		t.Errorf("docs = %+v", docs)
	}
	e.mkdir(t, a.root.ID, composed)
	for _, name := range []string{"docs", composed, decomposed} {
		_, err := e.store.Mkdir(e.ctx, e.db, a.root.ID, name)
		if !errors.Is(err, blobfs.ErrNameTaken) {
			t.Errorf("Mkdir(%q) again = %v, want ErrNameTaken", name, err)
		}
	}
	if _, err := e.store.Mkdir(e.ctx, e.db, a.root.ID, "no/slash"); !errors.Is(err, blobfs.ErrInvalidName) {
		t.Errorf("Mkdir(no/slash) = %v, want ErrInvalidName", err)
	}
	_, err := e.store.Mkdir(e.ctx, e.db, blobfs.NewID(), "orphan")
	if !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("Mkdir under a missing parent = %v, want ErrNotFound", err)
	}
	var ce *sqlate.ConstraintError
	if !errors.As(err, &ce) || ce.Constraint != blobfs.ConstraintForeignKeyDirectoryParent {
		t.Errorf("Mkdir under a missing parent does not carry the foreign key: %v", err)
	}
	e.mkdir(t, b.root.ID, "docs")
	e.mkdir(t, docs.ID, "docs")
	d, err := e.store.Directory(e.ctx, e.db, docs.ID)
	if err != nil || d.ID != docs.ID || *d.Name != "docs" {
		t.Errorf("Directory = %+v, %v", d, err)
	}

	rows, total, err := e.store.Children(e.ctx, e.db, a.root.ID, query.Directives{
		Page: query.Page{Number: 1, Size: 10},
		Sort: []query.Sort{{Field: "name"}},
	})
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	var names []string
	for _, d := range rows {
		names = append(names, *d.Name)
	}
	if want := e.strings1(t, "SELECT name FROM blobfs_directory WHERE parent_id = $1 ORDER BY name, id", a.root.ID); !slices.Equal(names, want) || total != 2 {
		t.Errorf("Children of a's root = %v, total %d, want %v, total 2", names, total, want)
	}
	rows, total, err = e.store.Children(e.ctx, e.db, blobfs.NewID(), query.Directives{Page: query.Page{Number: 1, Size: 10}})
	if err != nil || len(rows) != 0 || total != 0 {
		t.Errorf("Children of a missing parent = %d rows, total %d, %v; want none", len(rows), total, err)
	}
}

// TestResolveDirectory proves iterative resolution: / is the root, /a and
// /a/b walk one child per segment, a decomposed segment resolves to the
// composed row, and a missing segment at any depth is ErrNotFound naming
// the prefix that failed.
func TestResolveDirectory(t *testing.T) {
	e := open(t)
	p := e.createVolume(t, "vol")
	a := e.mkdir(t, p.root.ID, "a")
	b := e.mkdir(t, a.ID, "b")
	cafe := e.mkdir(t, b.ID, composed)
	for path, want := range map[string]string{
		"/":                  p.root.ID,
		"/a":                 a.ID,
		"/a/b":               b.ID,
		"/a/b/" + composed:   cafe.ID,
		"/a/b/" + decomposed: cafe.ID,
	} {
		d, err := e.store.ResolveDirectory(e.ctx, e.db, p.volume.ID, path)
		if err != nil || d.ID != want {
			t.Errorf("ResolveDirectory(%q) = %s, %v, want %s", path, d.ID, err, want)
		}
	}
	for _, path := range []string{"/missing", "/a/missing", "/a/b/c/d"} {
		_, err := e.store.ResolveDirectory(e.ctx, e.db, p.volume.ID, path)
		if !errors.Is(err, blobfs.ErrNotFound) {
			t.Errorf("ResolveDirectory(%q) = %v, want ErrNotFound", path, err)
		}
	}
	_, err := e.store.ResolveDirectory(e.ctx, e.db, p.volume.ID, "/a/missing/deeper")
	if err == nil || !strings.Contains(err.Error(), `at /a/missing`) {
		t.Errorf("ResolveDirectory(/a/missing/deeper) = %v, want the failing prefix named", err)
	}
}

// fileRow is the consumer-side row of the read model under test: a file's
// columns, its volume, and its path inside the volume.
type fileRow struct {
	ID          string        `json:"id"`
	DirectoryID string        `json:"directory_id"`
	Name        string        `json:"name"`
	Status      blobfs.Status `json:"status"`
	Key         string        `json:"key"`
	Size        *int64        `json:"size"`
	ContentType string        `json:"content_type"`
	ETag        *string       `json:"etag"`
	Version     int64         `json:"version"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	VolumeID    string        `json:"volume_id"`
	Path        string        `json:"path"`
}

// filesBase is the consumer's base statement over the published patterns:
// the tree first, then the file columns, the volume carried down the tree,
// and the path composed from it.
const filesBase = `--| tier: standard
--| key: id
--| field: id uuid
--| field: volume_id uuid
--| field: directory_id uuid
--| field: name text
--| field: status text
--| field: path text
{{> blobfs.tree}}
SELECT {{> blobfs.file_columns}}, t.volume_id AS volume_id, {{> blobfs.file_path}} AS path
FROM blobfs_file f
JOIN tree t ON t.id = f.directory_id
`

// seeded is the read-model fixture: two volumes with directories and files,
// and the path each file is expected at.
type seeded struct {
	a, b      pair
	dirsA     []string          // every directory id of volume a, the root included
	paths     map[string]string // file id to expected path, both volumes
	volumeOf  map[string]string // file id to volume id
	e1        string            // the directory /d1/e1 of volume a
	filesInE1 []string
	countA    int
	countB    int
}

// seed builds the fixture. Volume a holds files at four depths with
// repeated names across directories, so sorting by name needs the key
// tie-breaker; volume b repeats a's names so a leak between volumes would
// show.
func seed(t *testing.T, e env) seeded {
	t.Helper()
	s := seeded{paths: map[string]string{}, volumeOf: map[string]string{}}
	s.a = e.createVolume(t, "alpha")
	s.b = e.createVolume(t, "beta")

	d1 := e.mkdir(t, s.a.root.ID, "d1")
	d2 := e.mkdir(t, s.a.root.ID, "d2")
	e1 := e.mkdir(t, d1.ID, "e1")
	e2 := e.mkdir(t, d1.ID, "e2")
	g := e.mkdir(t, e1.ID, "g")
	s.e1 = e1.ID
	s.dirsA = []string{s.a.root.ID, d1.ID, d2.ID, e1.ID, e2.ID, g.ID}
	add := func(volume, dir, dirPath string, names ...string) {
		for _, name := range names {
			id := e.insertFile(t, dir, name)
			s.paths[id] = dirPath + "/" + name
			s.volumeOf[id] = volume
			if dir == e1.ID {
				s.filesInE1 = append(s.filesInE1, id)
			}
		}
	}
	add(s.a.volume.ID, s.a.root.ID, "", "r1", "r2")
	add(s.a.volume.ID, d1.ID, "/d1", "a1", "a2", "a3")
	add(s.a.volume.ID, e1.ID, "/d1/e1", "a1", "x1")
	add(s.a.volume.ID, e2.ID, "/d1/e2", "y1")
	add(s.a.volume.ID, g.ID, "/d1/e1/g", "z1", "a1")
	add(s.a.volume.ID, d2.ID, "/d2", "w1")
	s.countA = 11

	bd1 := e.mkdir(t, s.b.root.ID, "d1")
	be1 := e.mkdir(t, bd1.ID, "e1")
	add(s.b.volume.ID, s.b.root.ID, "", "a1")
	add(s.b.volume.ID, bd1.ID, "/d1", "a1")
	add(s.b.volume.ID, be1.ID, "/d1/e1", "a1")
	s.countB = 3
	return s
}

// filesProjection compiles filesBase against the consumer's catalog, the
// same one the store compiled against.
func filesProjection(t *testing.T, e env) query.Projection[fileRow] {
	t.Helper()
	fsys := fstest.MapFS{"statements/files.sql": {Data: []byte(filesBase)}}
	stmts, err := e.catalog.Compile(fsys, "statements", e.db.Dialect())
	if err != nil {
		t.Fatalf("Compile the consumer's base: %v", err)
	}
	p := stmts.Statement("files").Project(query.Scanner[fileRow]())
	if err := p.Verify(e.ctx, e.db); err != nil {
		t.Fatalf("Verify the consumer's base: %v", err)
	}
	return p
}

// expectedOrder returns the given file ids in the order the database's
// own collation sorts their keys, ties broken by id as the projection
// breaks them: an independent oracle that never touches the tree.
func expectedOrder(t *testing.T, e env, keys map[string]string, descending bool) []string {
	t.Helper()
	ids := make([]string, 0, len(keys))
	ks := make([]string, 0, len(keys))
	for id, k := range keys {
		ids = append(ids, id)
		ks = append(ks, k)
	}
	dir := "ASC"
	if descending {
		dir = "DESC"
	}
	return e.strings1(t, fmt.Sprintf("SELECT id FROM unnest($1::text[], $2::text[]) AS v(id, k) ORDER BY k %s, CAST(id AS uuid)", dir), ids, ks)
}

// pages runs the listing page by page under the scope and sorts until a
// short page, checking that every page reports the same total, and returns
// the rows concatenated and the total.
func pages(t *testing.T, e env, p query.Projection[fileRow], field, value string, size int, sort []query.Sort) ([]fileRow, int) {
	t.Helper()
	var all []fileRow
	total := -1
	for page := 1; page <= 100; page++ {
		rows, n, err := data.ListIn(e.ctx, e.db, p, field, value, query.Directives{Page: query.Page{Number: page, Size: size}, Sort: sort})
		if err != nil {
			t.Fatalf("page %d of size %d: %v", page, size, err)
		}
		if total >= 0 && n != total {
			t.Errorf("page %d of size %d reports total %d, earlier pages %d", page, size, n, total)
		}
		total = n
		all = append(all, rows...)
		if len(rows) < size {
			return all, total
		}
	}
	t.Fatalf("listing of size %d never ended", size)
	return nil, 0
}

// TestReadModel is the read-model correctness proof: a consumer's base
// over blobfs.tree, blobfs.file_columns, and blobfs.file_path, listed
// through ListIn. Scoped to one volume and sorted by name or by path in
// both directions at several page sizes, the pages concatenated equal the
// independent order, the total equals an independent count, every path
// matches the one the fixture expects, starts at /, and never names the
// volume, and the other volume's rows never appear. Scoped to one
// directory, the listing is that directory's files alone.
func TestReadModel(t *testing.T) {
	e := open(t)
	s := seed(t, e)
	p := filesProjection(t, e)

	countA := e.count(t, "SELECT COUNT(*) FROM blobfs_file WHERE CAST(directory_id AS text) = ANY($1)", s.dirsA)
	if countA != s.countA {
		t.Fatalf("independent count of volume a = %d, fixture says %d", countA, s.countA)
	}
	keysA := map[string]map[string]string{"name": {}, "path": {}}
	for id, path := range s.paths {
		if s.volumeOf[id] == s.a.volume.ID {
			keysA["name"][id] = path[strings.LastIndex(path, "/")+1:]
			keysA["path"][id] = path
		}
	}

	for _, field := range []string{"name", "path"} {
		for _, descending := range []bool{false, true} {
			want := expectedOrder(t, e, keysA[field], descending)
			for _, size := range []int{1, 4, 5, 11, 20} {
				rows, total := pages(t, e, p, "volume_id", s.a.volume.ID, size, []query.Sort{{Field: field, Descending: descending}})
				if total != countA {
					t.Errorf("%s desc=%v size %d: total %d, want %d", field, descending, size, total, countA)
				}
				var got []string
				for _, r := range rows {
					got = append(got, r.ID)
					if r.Path != s.paths[r.ID] {
						t.Errorf("file %s path %q, want %q", r.Name, r.Path, s.paths[r.ID])
					}
					if !strings.HasPrefix(r.Path, "/") || strings.Contains(r.Path, "alpha") {
						t.Errorf("path %q does not start at / inside the volume", r.Path)
					}
					if r.VolumeID != s.a.volume.ID || s.volumeOf[r.ID] != s.a.volume.ID {
						t.Errorf("file %s at %q belongs to another volume", r.ID, r.Path)
					}
					if r.Status != blobfs.StatusAvailable || r.Key == "" || r.Size != nil {
						t.Errorf("file columns scanned wrong: %+v", r)
					}
				}
				if !slices.Equal(got, want) {
					t.Errorf("%s desc=%v size %d: pages concatenated differ from the independent order", field, descending, size)
				}
			}
		}
	}

	rows, total := pages(t, e, p, "directory_id", s.e1, 10, []query.Sort{{Field: "name"}})
	if total != 2 || len(rows) != 2 || rows[0].Path != "/d1/e1/a1" || rows[1].Path != "/d1/e1/x1" {
		t.Errorf("listing of /d1/e1 = %d rows, total %d: %+v", len(rows), total, rows)
	}
	rows, total = pages(t, e, p, "volume_id", s.b.volume.ID, 10, []query.Sort{{Field: "path"}})
	if total != s.countB || len(rows) != s.countB {
		t.Errorf("listing of volume b = %d rows, total %d, want %d", len(rows), total, s.countB)
	}
	for _, r := range rows {
		if r.Path != s.paths[r.ID] || strings.Contains(r.Path, "beta") {
			t.Errorf("volume b file path %q, want %q", r.Path, s.paths[r.ID])
		}
	}
	if n := e.count(t, "SELECT COUNT(*) FROM blobfs_file"); n != s.countA+s.countB {
		t.Errorf("all files = %d, want %d", n, s.countA+s.countB)
	}

	rows, total = pages(t, e, p, "volume_id", s.a.volume.ID, 10, []query.Sort{{Field: "name"}})
	names := map[string]int{}
	for _, r := range rows {
		names[r.Name]++
	}
	if names["a1"] != 3 || total != countA {
		t.Errorf("a1 appears %d times in volume a, want 3 (three directories)", names["a1"])
	}
}
