package volume_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/domain/volume"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// newStore builds the store over the scripted driver under the stub
// dialect, with responses queued for the calls a test expects.
func newStore(t *testing.T, responses ...sqltest.Response) (*volume.Store, *sqltest.Recorder) {
	t.Helper()
	pool, rec := sqltest.Open(t, responses...)
	s, err := volume.New(sqlate.Wrap(pool, sqltest.Dialect{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, rec
}

// TestNew proves the catalog builds from the two pattern sources and every
// statement compiles and every projection constructs, blobfs's and the
// consumer's, with no I/O: the recorder sees no call.
func TestNew(t *testing.T) {
	_, rec := newStore(t)
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("New reached the driver with %d calls", len(calls))
	}
}

// TestVerify proves Verify prepares the whole inventory, the consumer's
// four statements and two projections and blobfs's twelve statements and
// two projections, twenty prepares, and wraps a failure in ErrVerify with
// the failing statement named.
func TestVerify(t *testing.T) {
	s, rec := newStore(t)
	if err := s.Verify(context.Background()); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if n := len(rec.SQL(sqltest.OpPrepare)); n != 20 {
		t.Errorf("Verify prepared %d statements, want 20", n)
	}

	s, rec = newStore(t)
	rec.FailPrepare = func(q string) error {
		if strings.Contains(q, "volume_owner") {
			return errors.New(`relation "volume_owner" does not exist`)
		}
		return nil
	}
	err := s.Verify(context.Background())
	if !errors.Is(err, volume.ErrVerify) {
		t.Fatalf("Verify against a missing table = %v, want ErrVerify", err)
	}
	for _, want := range []string{"blobfs schema up", "file_view", "volume_view", "create_owner", "owner_by_volume"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Verify's error lacks %q:\n%v", want, err)
		}
	}
}

// TestVolumesLowersTheListing proves database.go's lowering of a Listing:
// the page becomes the offset and fetch arguments, each sort term an ORDER
// BY term with the key appended, and the unit an equality filter on
// unit_id cast to uuid, in both the count and the page query.
func TestVolumesLowersTheListing(t *testing.T) {
	s, rec := newStore(t,
		sqltest.Response{Columns: []string{"count"}, Rows: [][]driver.Value{{int64(0)}}},
		sqltest.Response{Columns: []string{"id", "name", "version", "created_at", "updated_at", "unit_id"}},
	)
	unit := blobfs.NewID()
	_, total, err := s.Volumes(context.Background(), volume.Listing{
		Page: 3, Size: 10, Unit: unit,
		Sort: []volume.Sort{{Field: "name", Descending: true}, {Field: "created_at"}},
	})
	if err != nil || total != 0 {
		t.Fatalf("Volumes = total %d, %v", total, err)
	}
	queries := rec.Calls()
	if len(queries) != 2 {
		t.Fatalf("Volumes ran %d calls, want the count and the page", len(queries))
	}
	count, page := queries[0], queries[1]
	for _, q := range []sqltest.Call{count, page} {
		if !strings.Contains(q.SQL, "JOIN volume_owner o ON o.volume_id = v.id") || !strings.Contains(q.SQL, "WHERE q.unit_id = CAST($1 AS uuid)") {
			t.Errorf("query lacks the owner join or the unit filter:\n%s", q.SQL)
		}
		if q.Args[0] != unit {
			t.Errorf("query binds %v first, want the unit", q.Args[0])
		}
	}
	if !strings.HasSuffix(page.SQL, "ORDER BY q.name DESC, q.created_at, q.id OFFSET $2 ROWS FETCH NEXT $3 ROWS ONLY") {
		t.Errorf("page query ends %q", page.SQL[strings.LastIndex(page.SQL, "ORDER BY"):])
	}
	if len(page.Args) != 3 || fmt.Sprint(page.Args[1:]) != "[20 10]" {
		t.Errorf("page arguments = %v, want the unit, offset 20, fetch 10", page.Args)
	}
	if strings.Contains(count.SQL, "ORDER BY") || len(count.Args) != 1 {
		t.Errorf("count query carries an order or paging: %s %v", count.SQL, count.Args)
	}
}

// TestCreateVolumeRollsBackOnTheOwner proves the three-row unit: when the
// owner insert fails after blobfs's two inserts succeeded, the transaction
// is rolled back and never committed, and the owner's error reaches the
// caller.
func TestCreateVolumeRollsBackOnTheOwner(t *testing.T) {
	now := time.Now()
	owner := errors.New("owner refused")
	s, rec := newStore(t,
		sqltest.Response{Affected: 1}, // create_volume
		sqltest.Response{Affected: 1}, // create_root_directory
		sqltest.Response{Columns: []string{"id", "name", "version", "created_at", "updated_at"}, Rows: [][]driver.Value{{"v", "docs", int64(1), now, now}}},
		sqltest.Response{Columns: []string{"id", "parent_id", "volume_id", "name", "version", "created_at", "updated_at"}, Rows: [][]driver.Value{{"r", nil, "v", nil, int64(1), now, now}}},
		sqltest.Response{Err: owner}, // create_owner
	)
	_, err := s.CreateVolume(context.Background(), "docs", blobfs.NewID())
	if !errors.Is(err, owner) {
		t.Fatalf("CreateVolume = %v, want the owner's error", err)
	}
	ops := rec.Ops()
	want := []sqltest.Op{sqltest.OpBegin, sqltest.OpExec, sqltest.OpExec, sqltest.OpQuery, sqltest.OpQuery, sqltest.OpExec, sqltest.OpRollback}
	if strings.Join(opNames(ops), " ") != strings.Join(opNames(want), " ") {
		t.Errorf("ops = %v, want %v", ops, want)
	}
	if rec.Pending() != 0 {
		t.Errorf("%d scripted responses unconsumed", rec.Pending())
	}

	// A name that fails validation runs nothing at all: the transaction is
	// begun and rolled back around the refusal.
	s, rec = newStore(t)
	if _, err := s.CreateVolume(context.Background(), "a/b", blobfs.NewID()); !errors.Is(err, blobfs.ErrInvalidName) {
		t.Fatalf("CreateVolume(a/b) = %v, want ErrInvalidName", err)
	}
	if got := strings.Join(opNames(rec.Ops()), " "); got != "begin rollback" {
		t.Errorf("ops for an invalid name = %q, want begin rollback", got)
	}
}

func opNames(ops []sqltest.Op) []string {
	out := make([]string, len(ops))
	for i, op := range ops {
		out[i] = string(op)
	}
	return out
}

// TestFileEntryScans proves the file view's scan contract: a row with every
// column of file_view scans into FileEntry, nullable columns included, and
// the embedded alternative does not scan, which is why the entry types are
// flat: the struct-tag mapper indexes a struct's own fields and maps an
// embedded struct under its own lowercased type name.
func TestFileEntryScans(t *testing.T) {
	now := time.Now()
	cols := []string{"id", "directory_id", "name", "status", "key", "size", "content_type", "etag", "version", "created_at", "updated_at", "path", "volume_id", "unit_id"}
	row := []driver.Value{"f", "d", "a.txt", "available", "f/a.txt", int64(12), "text/plain", "etag", int64(1), now, now, "/x/a.txt", "v", "u"}
	pool, _ := sqltest.Open(t, sqltest.Response{Columns: cols, Rows: [][]driver.Value{row}})
	rows, err := pool.Query("SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatal("no row")
	}
	entry, err := query.Scanner[volume.FileEntry]()(rows)
	if err != nil {
		t.Fatalf("scan FileEntry: %v", err)
	}
	if entry.ID != "f" || entry.Status != blobfs.StatusAvailable || entry.Size == nil || *entry.Size != 12 || entry.Path != "/x/a.txt" || entry.VolumeID != "v" || entry.UnitID != "u" || entry.ETag == nil {
		t.Errorf("FileEntry = %+v", entry)
	}

	type embedded struct {
		blobfs.File
		Path     string `json:"path"`
		VolumeID string `json:"volume_id"`
		UnitID   string `json:"unit_id"`
	}
	pool, _ = sqltest.Open(t, sqltest.Response{Columns: cols, Rows: [][]driver.Value{row}})
	rows2, err := pool.Query("SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows2.Close() }()
	rows2.Next()
	_, err = query.Scanner[embedded]()(rows2)
	if err == nil || !strings.Contains(err.Error(), `column "id" has no field`) {
		t.Errorf("scanning into an embedded blobfs.File = %v, want the mapper to miss the embedded columns", err)
	}
	_ = sql.ErrNoRows
}
