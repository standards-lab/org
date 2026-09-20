//go:build integration

package volume_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/domain/volume"
	"github.com/standards-lab/org/experiments/blobfs/internal/livetest"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
	blobfsmigrations "github.com/standards-lab/org/experiments/blobfs/lib/blobfs/migrations"
	"github.com/standards-lab/org/experiments/blobfs/lib/migrator"
	appmigrations "github.com/standards-lab/org/experiments/blobfs/migrations"
)

// env is one test's throwaway database with both migration sets applied
// and the store verified against it.
type env struct {
	ctx   context.Context
	db    *sqlate.DB
	store *volume.Store
}

// open applies blobfs's set and then the consumer's through the migrator,
// as schema up does, builds the store, and proves Verify against the
// migrated schema on the way.
func open(t *testing.T) env {
	t.Helper()
	ctx := context.Background()
	db := livetest.Open(t)
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
	s, err := volume.New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Verify(ctx); err != nil {
		t.Fatalf("Verify against the migrated schema: %v", err)
	}
	return env{ctx: ctx, db: db, store: s}
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

// rowCounts returns the row counts of the three tables a volume create
// touches: volumes, directories, owners.
func (e env) rowCounts(t *testing.T) [3]int {
	t.Helper()
	return [3]int{
		e.count(t, "SELECT COUNT(*) FROM blobfs_volume"),
		e.count(t, "SELECT COUNT(*) FROM blobfs_directory"),
		e.count(t, "SELECT COUNT(*) FROM volume_owner"),
	}
}

// createVolume creates a volume and fails the test on any error.
func (e env) createVolume(t *testing.T, name, unit string) volume.VolumeEntry {
	t.Helper()
	v, err := e.store.CreateVolume(e.ctx, name, unit)
	if err != nil {
		t.Fatalf("CreateVolume(%q): %v", name, err)
	}
	return v
}

// mkdir creates a directory at an address and fails the test on any error.
func (e env) mkdir(t *testing.T, address string) blobfs.Directory {
	t.Helper()
	addr, err := volume.ParseAddress(address)
	if err != nil {
		t.Fatalf("ParseAddress(%q): %v", address, err)
	}
	d, err := e.store.Mkdir(e.ctx, addr)
	if err != nil {
		t.Fatalf("Mkdir(%s): %v", address, err)
	}
	return d
}

// insertFile inserts an available file row directly, since the write path
// is a later stage, and returns its id.
func (e env) insertFile(t *testing.T, dir, name string) string {
	t.Helper()
	id := blobfs.NewID()
	_, err := e.db.ExecContext(e.ctx,
		"INSERT INTO blobfs_file (id, directory_id, name, status, key, size, content_type) VALUES ($1, $2, $3, 'available', $4, $5, 'text/plain')",
		id, dir, name, id+"/"+blobfs.SanitizeFilename(name), len(name))
	if err != nil {
		t.Fatalf("insert file %s: %v", name, err)
	}
	return id
}

// address parses an address or fails the test.
func address(t *testing.T, s string) volume.Address {
	t.Helper()
	a, err := volume.ParseAddress(s)
	if err != nil {
		t.Fatalf("ParseAddress(%q): %v", s, err)
	}
	return a
}

// TestCreateVolumeIsAtomic proves the three-row unit through the consumer:
// a create writes the volume, its root, and the owner row, the entry
// carries the unit, and a refusal at either step leaves nothing behind: a
// name another volume holds fails at blobfs's insert, and a unit id the
// engine cannot read fails at the consumer's insert after blobfs's two
// inserts succeeded inside the transaction.
func TestCreateVolumeIsAtomic(t *testing.T) {
	e := open(t)
	unit := blobfs.NewID()
	v := e.createVolume(t, "docs", unit)
	if v.Name != "docs" || v.UnitID != unit || v.Version != 1 || v.CreatedAt.IsZero() {
		t.Errorf("entry = %+v", v)
	}
	if got := e.rowCounts(t); got != [3]int{1, 1, 1} {
		t.Errorf("rows after create = %v, want one volume, one root, one owner", got)
	}
	if units := e.strings1(t, "SELECT CAST(unit_id AS text) FROM volume_owner WHERE volume_id = $1", v.ID); len(units) != 1 || units[0] != unit {
		t.Errorf("owner of the volume = %v, want %s", units, unit)
	}

	_, err := e.store.CreateVolume(e.ctx, "docs", blobfs.NewID())
	if !errors.Is(err, blobfs.ErrNameTaken) {
		t.Errorf("CreateVolume(docs) again = %v, want ErrNameTaken", err)
	}
	_, err = e.store.CreateVolume(e.ctx, "orphan", "not-a-uuid")
	if err == nil || !strings.Contains(err.Error(), "create owner") {
		t.Errorf("CreateVolume with an unreadable unit = %v, want the owner insert's failure", err)
	}
	if got := e.rowCounts(t); got != [3]int{1, 1, 1} {
		t.Errorf("rows after the two refusals = %v, want the first volume's three rows only", got)
	}
	if _, err := e.store.VolumeByName(e.ctx, "orphan"); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("the refused volume is readable: %v", err)
	}
}

// TestVolumes proves volume ls: pages sorted by name descending concatenate
// to the independent order with the total on every page, --unit narrows
// both the rows and the total to the unit's volumes, and an unknown sort
// field is ErrDirectives.
func TestVolumes(t *testing.T) {
	e := open(t)
	units := []string{blobfs.NewID(), blobfs.NewID()}
	for i, name := range []string{"delta", "alpha", "charlie", "bravo", "echo"} {
		e.createVolume(t, name, units[i%2])
	}
	want := e.strings1(t, "SELECT name FROM blobfs_volume ORDER BY name DESC, id")
	var got []string
	for page := 1; ; page++ {
		rows, total, err := e.store.Volumes(e.ctx, volume.Listing{Page: page, Size: 2, Sort: []volume.Sort{{Field: "name", Descending: true}}})
		if err != nil {
			t.Fatalf("Volumes page %d: %v", page, err)
		}
		if total != 5 {
			t.Errorf("page %d total = %d, want 5", page, total)
		}
		for _, v := range rows {
			got = append(got, v.Name)
			if v.UnitID == "" {
				t.Errorf("volume %s lists no unit", v.Name)
			}
		}
		if len(rows) < 2 {
			break
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("pages concatenated = %v, want %v", got, want)
	}

	for i, unit := range units {
		rows, total, err := e.store.Volumes(e.ctx, volume.Listing{Page: 1, Size: 10, Unit: unit, Sort: []volume.Sort{{Field: "name"}}})
		if err != nil {
			t.Fatalf("Volumes for unit %d: %v", i, err)
		}
		n := e.count(t, "SELECT COUNT(*) FROM volume_owner WHERE unit_id = $1", unit)
		if total != n || len(rows) != n || n != 3-i {
			t.Errorf("unit %d: %d rows, total %d, want %d", i, len(rows), total, n)
		}
		for _, v := range rows {
			if v.UnitID != unit {
				t.Errorf("unit %d listing shows %s owned by %s", i, v.Name, v.UnitID)
			}
		}
	}
	_, total, err := e.store.Volumes(e.ctx, volume.Listing{Page: 1, Size: 10, Unit: blobfs.NewID()})
	if err != nil || total != 0 {
		t.Errorf("Volumes for a unit that owns nothing = total %d, %v", total, err)
	}
	if _, _, err := e.store.Volumes(e.ctx, volume.Listing{Page: 1, Size: 10, Sort: []volume.Sort{{Field: "owner"}}}); !errors.Is(err, query.ErrDirectives) {
		t.Errorf("Volumes sorted by an undeclared field = %v, want ErrDirectives", err)
	}
}

// TestRenameVolume proves the rename through the lookup: the version
// advances and the new name resolves, the old one does not, a missing
// volume is ErrNotFound, and a name another volume holds ErrNameTaken.
func TestRenameVolume(t *testing.T) {
	e := open(t)
	unit := blobfs.NewID()
	e.createVolume(t, "docs", unit)
	e.createVolume(t, "other", unit)
	v, err := e.store.RenameVolume(e.ctx, "docs", "manuals")
	if err != nil || v.Name != "manuals" || v.Version != 2 {
		t.Fatalf("RenameVolume = %+v, %v, want manuals at version 2", v, err)
	}
	if _, err := e.store.VolumeByName(e.ctx, "docs"); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("the old name still resolves: %v", err)
	}
	v, err = e.store.RenameVolume(e.ctx, "manuals", "guides")
	if err != nil || v.Version != 3 {
		t.Errorf("second rename = %+v, %v, want version 3", v, err)
	}
	if _, err := e.store.RenameVolume(e.ctx, "missing", "x"); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("rename of a missing volume = %v, want ErrNotFound", err)
	}
	if _, err := e.store.RenameVolume(e.ctx, "guides", "other"); !errors.Is(err, blobfs.ErrNameTaken) {
		t.Errorf("rename to a taken name = %v, want ErrNameTaken", err)
	}
	if n := e.count(t, "SELECT COUNT(*) FROM volume_owner"); n != 2 {
		t.Errorf("owners after renames = %d, want 2 (a rename never touches the owner row)", n)
	}
}

// TestMkdirAndResolve proves directory creation under nested addresses and
// resolution through the address: each level resolves to the row Mkdir
// returned, a missing parent is ErrNotFound, a taken name ErrNameTaken,
// the root ErrInvalidAddress, a missing volume ErrNotFound, and a path the
// library refuses ErrInvalidPath.
func TestMkdirAndResolve(t *testing.T) {
	e := open(t)
	e.createVolume(t, "docs", blobfs.NewID())
	a := e.mkdir(t, "docs:/a")
	b := e.mkdir(t, "docs:/a/b")
	c := e.mkdir(t, "docs:/a/b/c")
	if a.ParentID == nil || b.ParentID == nil || *b.ParentID != a.ID || *c.ParentID != b.ID || *c.Name != "c" {
		t.Errorf("rows = %+v %+v %+v", a, b, c)
	}
	for addr, want := range map[string]string{"docs:/a": a.ID, "docs:/a/b": b.ID, "docs:/a/b/c": c.ID} {
		v, d, err := e.store.Resolve(e.ctx, address(t, addr))
		if err != nil || d.ID != want || v.Name != "docs" {
			t.Errorf("Resolve(%s) = %s, %v, want %s", addr, d.ID, err, want)
		}
	}
	_, root, err := e.store.Resolve(e.ctx, address(t, "docs:/"))
	if err != nil || root.ParentID != nil || root.VolumeID == nil {
		t.Errorf("Resolve(docs:/) = %+v, %v, want the root", root, err)
	}
	for addr, want := range map[string]error{
		"docs:/x/y":   blobfs.ErrNotFound,
		"docs:/a/b":   blobfs.ErrNameTaken,
		"docs:/":      volume.ErrInvalidAddress,
		"nope:/a":     blobfs.ErrNotFound,
		"docs:/a//c":  blobfs.ErrInvalidPath,
		"docs:/a/../": volume.ErrInvalidAddress,
		"docs:/a/..":  blobfs.ErrInvalidName,
	} {
		_, err := e.store.Mkdir(e.ctx, address(t, addr))
		if !errors.Is(err, want) {
			t.Errorf("Mkdir(%s) = %v, want %v", addr, err, want)
		}
	}
	if _, _, err := e.store.Resolve(e.ctx, address(t, "docs:/a/missing")); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("Resolve(docs:/a/missing) = %v, want ErrNotFound", err)
	}
	if _, _, err := e.store.Resolve(e.ctx, address(t, "docs:/a/")); !errors.Is(err, blobfs.ErrInvalidPath) {
		t.Errorf("Resolve(docs:/a/) = %v, want ErrInvalidPath", err)
	}
	if n := e.count(t, "SELECT COUNT(*) FROM blobfs_directory"); n != 4 {
		t.Errorf("directories = %d, want the root and three", n)
	}
}

// TestList proves ls: under docs:/a the directories come one page at a
// time sorted by name with their total, the files one page at a time under
// the caller's sort by name or by path in both directions with the total
// equal to an independent count, every path starts at the listed
// directory, a second volume's same-named rows never appear, and --unit
// scopes both halves: the owning unit sees everything and another unit
// sees nothing with totals of zero.
func TestList(t *testing.T) {
	e := open(t)
	unit, other := blobfs.NewID(), blobfs.NewID()
	e.createVolume(t, "docs", unit)
	e.createVolume(t, "beta", other)
	a := e.mkdir(t, "docs:/a")
	e.mkdir(t, "docs:/a/z")
	e.mkdir(t, "docs:/a/m")
	e.mkdir(t, "docs:/a/b")
	deep := e.mkdir(t, "docs:/a/b/c")
	ba := e.mkdir(t, "beta:/a")
	names := []string{"r.txt", "b.txt", "a.txt", "m.txt", "z.txt", "k.txt", "q.txt"}
	for _, name := range names {
		e.insertFile(t, a.ID, name)
		e.insertFile(t, ba.ID, name) // the other volume repeats the names
	}
	e.insertFile(t, deep.ID, "deep.txt")

	// The directory half.
	var dirs []string
	for page := 1; page <= 3; page++ {
		c, err := e.store.List(e.ctx, address(t, "docs:/a"), volume.Listing{Page: page, Size: 2})
		if err != nil {
			t.Fatalf("List page %d: %v", page, err)
		}
		if c.DirectoryTotal != 3 || c.FileTotal != len(names) {
			t.Errorf("page %d totals = %d directories, %d files; want 3, %d", page, c.DirectoryTotal, c.FileTotal, len(names))
		}
		for _, d := range c.Directories {
			dirs = append(dirs, *d.Name)
		}
	}
	if want := []string{"b", "m", "z"}; !slices.Equal(dirs, want) {
		t.Errorf("directories paged = %v, want %v", dirs, want)
	}

	// The file half, in every sort, at page size 3.
	for _, field := range []string{"name", "path"} {
		for _, desc := range []bool{false, true} {
			dir := "ASC"
			if desc {
				dir = "DESC"
			}
			want := e.strings1(t, fmt.Sprintf("SELECT name FROM blobfs_file WHERE directory_id = $1 ORDER BY name %s, id", dir), a.ID)
			var got []string
			for page := 1; page <= 3; page++ {
				c, err := e.store.List(e.ctx, address(t, "docs:/a"), volume.Listing{Page: page, Size: 3, Sort: []volume.Sort{{Field: field, Descending: desc}}})
				if err != nil {
					t.Fatalf("List %s %s page %d: %v", field, dir, page, err)
				}
				if c.FileTotal != len(names) {
					t.Errorf("%s %s page %d: file total %d, want %d", field, dir, page, c.FileTotal, len(names))
				}
				for _, f := range c.Files {
					got = append(got, f.Name)
					if f.Path != "/a/"+f.Name || f.UnitID != unit || f.Size == nil || f.Status != blobfs.StatusAvailable {
						t.Errorf("file %+v: want path /a/%s, unit %s, size, available", f, f.Name, unit)
					}
				}
			}
			if !slices.Equal(got, want) {
				t.Errorf("%s %s: pages concatenated = %v, want %v", field, dir, got, want)
			}
		}
	}

	// The deep directory, and the other volume, each see their own rows.
	c, err := e.store.List(e.ctx, address(t, "docs:/a/b/c"), volume.Listing{Page: 1, Size: 10})
	if err != nil || c.FileTotal != 1 || c.DirectoryTotal != 0 || c.Files[0].Path != "/a/b/c/deep.txt" {
		t.Errorf("List(docs:/a/b/c) = %+v, %v", c, err)
	}
	c, err = e.store.List(e.ctx, address(t, "beta:/a"), volume.Listing{Page: 1, Size: 10, Sort: []volume.Sort{{Field: "name"}}})
	if err != nil || c.FileTotal != len(names) || c.DirectoryTotal != 0 {
		t.Fatalf("List(beta:/a) = %+v, %v", c, err)
	}
	for _, f := range c.Files {
		if f.UnitID != other || f.DirectoryID != ba.ID {
			t.Errorf("beta's file %+v carries docs's unit or directory", f)
		}
	}

	// --unit: the owner sees everything, another unit nothing.
	c, err = e.store.List(e.ctx, address(t, "docs:/a"), volume.Listing{Page: 1, Size: 10, Unit: unit})
	if err != nil || c.DirectoryTotal != 3 || c.FileTotal != len(names) {
		t.Errorf("List(docs:/a) as the owner = %d dirs, %d files, %v", c.DirectoryTotal, c.FileTotal, err)
	}
	c, err = e.store.List(e.ctx, address(t, "docs:/a"), volume.Listing{Page: 1, Size: 10, Unit: other})
	if err != nil || c.DirectoryTotal != 0 || c.FileTotal != 0 || len(c.Directories) != 0 || len(c.Files) != 0 {
		t.Errorf("List(docs:/a) as another unit = %d dirs, %d files, %v; want nothing", c.DirectoryTotal, c.FileTotal, err)
	}
	if _, err := e.store.List(e.ctx, address(t, "docs:/a"), volume.Listing{Page: 1, Size: 10, Sort: []volume.Sort{{Field: "owner"}}}); !errors.Is(err, query.ErrDirectives) {
		t.Errorf("List sorted by an undeclared field = %v, want ErrDirectives", err)
	}
}

// TestListInForgottenScope is the forgotten-scope proof against the
// engine: a consumer base over the same patterns that does not declare
// directory_id makes ListIn fail with query.UnknownFieldError before any
// SQL runs, and that error unwraps to query.ErrDirectives, the sentinel a
// generic handler maps to a client error, so a consumer that wants to tell
// the forgotten field from a bad request must match the type.
func TestListInForgottenScope(t *testing.T) {
	e := open(t)
	e.createVolume(t, "docs", blobfs.NewID())
	a := e.mkdir(t, "docs:/a")
	e.insertFile(t, a.ID, "x.txt")
	catalog, err := query.NewCatalog(query.Patterns(), data.Patterns())
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	base := "--| tier: standard\n--| key: id\n--| field: id uuid\n--| field: name text\n--| field: path text\n" +
		"{{> blobfs.tree}}\nSELECT f.id, f.name, {{> blobfs.file_path}} AS path FROM blobfs_file f JOIN tree t ON t.id = f.directory_id"
	stmts, err := catalog.Compile(fstest.MapFS{"statements/files.sql": {Data: []byte(base)}}, "statements", e.db.Dialect())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	type row struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Path string `json:"path"`
	}
	p := stmts.Statement("files").Project(query.Scanner[row]())
	if err := p.Verify(e.ctx, e.db); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	_, _, err = data.ListIn(e.ctx, e.db, p, "directory_id", a.ID, query.Directives{Page: query.Page{Number: 1, Size: 10}})
	var unknown *query.UnknownFieldError
	if !errors.As(err, &unknown) || unknown.Field != "directory_id" {
		t.Fatalf("ListIn over a base without directory_id = %v, want UnknownFieldError for directory_id", err)
	}
	if !errors.Is(err, query.ErrDirectives) {
		t.Errorf("the error does not unwrap to ErrDirectives: %v", err)
	}
	// The same base lists every file of the forest when nothing scopes it:
	// the scope is a filter, and a base that lacks the field has no scope.
	rows, total, err := p.List(e.ctx, e.db, query.Directives{Page: query.Page{Number: 1, Size: 10}})
	if err != nil || total != 1 || len(rows) != 1 || rows[0].Path != "/a/x.txt" {
		t.Errorf("unscoped List = %+v, total %d, %v", rows, total, err)
	}
}
