package data_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

// catalog builds the catalog a consumer builds: the library's patterns and
// blobfs's, and nothing else.
func catalog(t *testing.T) *query.Catalog {
	t.Helper()
	c, err := query.NewCatalog(query.Patterns(), data.Patterns())
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	return c
}

// newStore compiles the store against the consumer's catalog under the
// stub dialect.
func newStore(t *testing.T) *data.Store {
	t.Helper()
	s, err := data.New(catalog(t), sqltest.Dialect{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// TestNew proves the catalog builds with the two sources, every statement
// compiles and every projection constructs, and reports the inventory the
// tier proof counts: twelve statements, all standard tier, of which the
// two inserts behind CreateVolume require a transaction.
func TestNew(t *testing.T) {
	s := newStore(t)
	stmts := s.Statements()
	if len(stmts) != 12 {
		t.Errorf("Statements returned %d statements, want 12", len(stmts))
	}
	txRequired := map[string]bool{"create_volume": true, "create_root_directory": true}
	for _, st := range stmts {
		if st.Tier() != query.TierStandard {
			t.Errorf("%s is %s tier, want standard", st.Name(), st.Tier())
		}
		if st.TransactionRequired() != txRequired[st.Name()] {
			t.Errorf("%s TransactionRequired = %v, want %v", st.Name(), st.TransactionRequired(), txRequired[st.Name()])
		}
	}
	for _, p := range catalog(t).Patterns() {
		if p.Namespace == data.Namespace && p.Tier != query.TierStandard {
			t.Errorf("pattern %s.%s is %s tier, want standard", p.Namespace, p.Name, p.Tier)
		}
		if p.Namespace == data.Namespace && len(p.Slots) != 0 {
			t.Errorf("pattern %s.%s declares slots %v; published patterns are parameter-free", p.Namespace, p.Name, p.Slots)
		}
	}
}

// TestPatterns proves the published namespace and its inventory: the tree,
// the three column lists, and the two path expressions.
func TestPatterns(t *testing.T) {
	var names []string
	for _, p := range catalog(t).Patterns() {
		if p.Namespace == data.Namespace {
			names = append(names, p.Name)
		}
	}
	want := "directory_columns directory_path file_columns file_path tree volume_columns"
	if got := strings.Join(names, " "); got != want {
		t.Errorf("blobfs patterns = %q, want %q", got, want)
	}
}

// TestNewWithoutPatterns proves a catalog that lacks the blobfs namespace
// is refused with an error naming it, before any statement compiles.
func TestNewWithoutPatterns(t *testing.T) {
	c, err := query.NewCatalog(query.Patterns())
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	_, err = data.New(c, sqltest.Dialect{})
	if err == nil || !strings.Contains(err.Error(), `"blobfs"`) {
		t.Fatalf("New without the blobfs namespace = %v, want an error naming the namespace", err)
	}
}

// TestVerify proves Verify prepares every statement and both field-contract
// probes: fourteen prepares against the scripted driver, none of which
// consumes a response.
func TestVerify(t *testing.T) {
	s := newStore(t)
	pool, rec := sqltest.Open(t)
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	if err := s.Verify(context.Background(), db); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if n := len(rec.SQL(sqltest.OpPrepare)); n != 14 {
		t.Errorf("Verify prepared %d statements, want 14 (12 statements and 2 projections)", n)
	}
}

// listing is the row shape of the test-local projection bases below.
type listing struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// projection compiles one test-local base statement against the consumer's
// catalog and binds it to a projection over listing.
func projection(t *testing.T, base string) query.Projection[listing] {
	t.Helper()
	fsys := fstest.MapFS{"statements/files.sql": {Data: []byte(base)}}
	stmts, err := catalog(t).Compile(fsys, "statements", sqltest.Dialect{})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return stmts.Statement("files").Project(query.Scanner[listing]())
}

// TestListInUnknownField proves the forgotten-filter case: a base that did
// not declare the scoping field makes ListIn fail before any SQL reaches
// the driver, with a query.UnknownFieldError naming the field as a filter,
// and that error unwraps to query.ErrDirectives, the sentinel a generic
// handler maps to a client error.
func TestListInUnknownField(t *testing.T) {
	p := projection(t, "--| tier: standard\n--| key: id\n--| field: id uuid\n--| field: name text\n"+
		"{{> blobfs.tree}}\nSELECT f.id, f.name FROM blobfs_file f JOIN tree t ON t.id = f.directory_id")
	pool, rec := sqltest.Open(t)
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	d := query.Directives{Page: query.Page{Number: 1, Size: 10}}
	_, _, err := data.ListIn(context.Background(), db, p, "directory_id", "x", d)
	var unknown *query.UnknownFieldError
	if !errors.As(err, &unknown) {
		t.Fatalf("ListIn on an undeclared field = %v, want UnknownFieldError", err)
	}
	if unknown.Field != "directory_id" || unknown.Use != query.FieldUseFilter {
		t.Errorf("UnknownFieldError = %+v, want field directory_id as a filter", unknown)
	}
	if !errors.Is(err, query.ErrDirectives) {
		t.Errorf("UnknownFieldError does not unwrap to ErrDirectives: %v", err)
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("ListIn reached the driver with %d calls before rejecting the field", len(calls))
	}
}

// TestListInAppendsScope proves ListIn appends the scope after the caller's
// filters, binds its value through the field's declared type, and leaves
// the caller's directives as they were.
func TestListInAppendsScope(t *testing.T) {
	p := projection(t, "--| tier: standard\n--| key: id\n--| field: id uuid\n--| field: name text\n--| field: directory_id uuid\n"+
		"SELECT f.id, f.name, f.directory_id FROM blobfs_file f")
	pool, rec := sqltest.Open(t,
		sqltest.Response{Columns: []string{"count"}, Rows: [][]driver.Value{{int64(0)}}},
		sqltest.Response{Columns: []string{"id", "name"}},
	)
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	d := query.Directives{
		Page:    query.Page{Number: 1, Size: 10},
		Filters: []query.Filter{{Field: "name", Op: query.OpLike, Value: "a%"}},
	}
	if _, total, err := data.ListIn(context.Background(), db, p, "directory_id", "dir", d); err != nil || total != 0 {
		t.Fatalf("ListIn = total %d, %v", total, err)
	}
	if len(d.Filters) != 1 {
		t.Errorf("ListIn modified the caller's directives: %d filters", len(d.Filters))
	}
	queries := rec.SQL(sqltest.OpQuery)
	if len(queries) != 2 {
		t.Fatalf("ListIn ran %d queries, want the count and the page", len(queries))
	}
	want := "WHERE q.name LIKE CAST($1 AS text) AND q.directory_id = CAST($2 AS uuid)"
	for _, q := range queries {
		if !strings.Contains(q, want) {
			t.Errorf("query %q lacks %q", q, want)
		}
	}
}

// TestResolveDirectoryPaths proves the path checks that run before any
// SQL: a relative path and an empty segment are ErrInvalidPath, a segment
// ValidateName refuses matches ErrInvalidName as well, and nothing reaches
// the driver.
func TestResolveDirectoryPaths(t *testing.T) {
	s := newStore(t)
	pool, rec := sqltest.Open(t)
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	for _, path := range []string{"", "a/b", "/a//b", "/a/", "/a/../b"} {
		_, err := s.ResolveDirectory(context.Background(), db, blobfs.NewID(), path)
		if !errors.Is(err, blobfs.ErrInvalidPath) {
			t.Errorf("ResolveDirectory(%q) = %v, want ErrInvalidPath", path, err)
		}
	}
	if _, err := s.ResolveDirectory(context.Background(), db, blobfs.NewID(), "/a/../b"); !errors.Is(err, blobfs.ErrInvalidName) {
		t.Errorf("ResolveDirectory(/a/../b) = %v, want ErrInvalidName as well", err)
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("path checks reached the driver with %d calls", len(calls))
	}
}

// TestInvalidNames proves the commands normalize and validate before any
// SQL: an empty name, a slash, and a control character are ErrInvalidName
// on CreateVolume, Mkdir, and RenameVolume alike, and nothing reaches the
// driver.
func TestInvalidNames(t *testing.T) {
	s := newStore(t)
	pool, rec := sqltest.Open(t)
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	ctx := context.Background()
	for _, name := range []string{"", "a/b", "tab\there", ".."} {
		if _, _, err := s.CreateVolume(ctx, db, name); !errors.Is(err, blobfs.ErrInvalidName) {
			t.Errorf("CreateVolume(%q) = %v, want ErrInvalidName", name, err)
		}
		if _, err := s.Mkdir(ctx, db, blobfs.NewID(), name); !errors.Is(err, blobfs.ErrInvalidName) {
			t.Errorf("Mkdir(%q) = %v, want ErrInvalidName", name, err)
		}
		if _, err := s.RenameVolume(ctx, db, blobfs.NewID(), 1, name); !errors.Is(err, blobfs.ErrInvalidName) {
			t.Errorf("RenameVolume(%q) = %v, want ErrInvalidName", name, err)
		}
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("name checks reached the driver with %d calls", len(calls))
	}
}

// TestCreateVolumeNeedsTransaction proves the header does its work without
// an engine: CreateVolume on the pool session is refused with
// query.ErrTransactionRequired before any statement runs.
func TestCreateVolumeNeedsTransaction(t *testing.T) {
	s := newStore(t)
	pool, rec := sqltest.Open(t)
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	if _, _, err := s.CreateVolume(context.Background(), db, "vol"); !errors.Is(err, query.ErrTransactionRequired) {
		t.Fatalf("CreateVolume on the pool = %v, want ErrTransactionRequired", err)
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("the refusal reached the driver with %d calls", len(calls))
	}
}
