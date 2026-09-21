package postgres_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

// accepting is a KeyValidator that accepts every key.
type accepting struct{}

func (accepting) ValidateKey(string) error { return nil }

// directoryColumns is the published directory column list's order, as the
// scripted driver must return a RETURNING row.
var directoryColumns = []string{"id", "parent_id", "name", "version", "created_at", "updated_at"}

// fileResponse scripts one file row with a status and version, its
// columns in fileColumns order.
func fileResponse(id, name string, status blobfs.Status, version int64) sqltest.Response {
	now := time.Now()
	row := []driver.Value{id, blobfs.RootID, name, string(status), id + "/" + name, nil, "text/plain", nil, version, now, now}
	return sqltest.Response{Columns: fileColumns, Rows: [][]driver.Value{row}}
}

// openNative builds a store over the Postgres variant and the scripted
// session together.
func openNative(t *testing.T, responses ...sqltest.Response) (*data.Store, *sqlate.DB, *sqltest.Recorder) {
	t.Helper()
	s, err := data.New(catalog(t), sqltest.Dialect{}, data.WithVariant(newVariant(t)))
	if err != nil {
		t.Fatalf("data.New: %v", err)
	}
	pool, rec := sqltest.Open(t, responses...)
	return s, sqlate.Wrap(pool, sqltest.Dialect{}), rec
}

// TestBeginFileWriteIsOneStatement proves the write's begin step through
// the Postgres variant is one query, the insert with RETURNING over the
// published column list, bound as the baseline binds its insert, and
// that a violation still classifies through the store's write mapping.
func TestBeginFileWriteIsOneStatement(t *testing.T) {
	ctx := context.Background()
	s, db, rec := openNative(t, fileResponse("F", "a.txt", blobfs.StatusPending, 1))
	f, err := s.BeginFileWrite(ctx, db, accepting{}, blobfs.RootID, "a.txt", "text/plain")
	if err != nil || f.ID != "F" || f.Status != blobfs.StatusPending {
		t.Fatalf("BeginFileWrite = %+v, %v, want the returned pending row", f, err)
	}
	if ops := rec.Ops(); !slices.Equal(ops, []sqltest.Op{sqltest.OpQuery}) {
		t.Fatalf("ops = %v, want one query", ops)
	}
	c := rec.Calls()[0]
	if !strings.HasPrefix(c.SQL, "INSERT INTO blobfs_file AS f (id, directory_id, name, status, key, content_type)") ||
		!strings.HasSuffix(c.SQL, "RETURNING f.id, f.directory_id, f.name, f.status, f.key, f.size, f.content_type, f.etag, f.version, f.created_at, f.updated_at") {
		t.Errorf("the insert is not the RETURNING form:\n%s", c.SQL)
	}
	id, _ := c.Args[0].(string)
	if len(c.Args) != 5 || len(id) != 36 || c.Args[1] != blobfs.RootID || c.Args[2] != "a.txt" || c.Args[3] != id+"/a.txt" || c.Args[4] != "text/plain" {
		t.Errorf("the insert bound %v, want the id, directory, name, key, and content type", c.Args)
	}

	cause := errors.New("duplicate")
	s, db, _ = openNative(t, sqltest.Response{Err: &sqlate.ConstraintError{Constraint: blobfs.ConstraintUniqueFileDirectoryName, Class: sqlate.ErrUniqueViolation, Err: cause}})
	_, err = s.BeginFileWrite(ctx, db, accepting{}, blobfs.RootID, "a.txt", "text/plain")
	var ve *blobfs.ViolationError
	if !errors.Is(err, blobfs.ErrNameTaken) || !errors.As(err, &ve) || ve.Constraint != blobfs.ConstraintUniqueFileDirectoryName || strings.Contains(err.Error(), cause.Error()) {
		t.Errorf("BeginFileWrite under the unique constraint = %v, want ErrNameTaken classified by the store", err)
	}
}

// TestMkdirIsOneStatement proves Mkdir through the Postgres variant is
// one query, the insert with RETURNING over the published column list,
// and that a violation still classifies through the store's write
// mapping.
func TestMkdirIsOneStatement(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	id := blobfs.NewID()
	s, db, rec := openNative(t, sqltest.Response{Columns: directoryColumns, Rows: [][]driver.Value{{id, blobfs.RootID, "docs", int64(1), now, now}}})
	d, err := s.Mkdir(ctx, db, blobfs.RootID, "docs", data.WithID(id))
	if err != nil || d.ID != id || d.Name != "docs" || d.Version != 1 {
		t.Fatalf("Mkdir = %+v, %v, want the returned row", d, err)
	}
	if ops := rec.Ops(); !slices.Equal(ops, []sqltest.Op{sqltest.OpQuery}) {
		t.Fatalf("ops = %v, want one query", ops)
	}
	c := rec.Calls()[0]
	if !strings.HasPrefix(c.SQL, "INSERT INTO blobfs_directory AS d (id, parent_id, name)") ||
		!strings.HasSuffix(c.SQL, "RETURNING d.id, d.parent_id, d.name, d.version, d.created_at, d.updated_at") {
		t.Errorf("the insert is not the RETURNING form:\n%s", c.SQL)
	}
	if !slices.Equal(c.Args, []any{id, blobfs.RootID, "docs"}) {
		t.Errorf("the insert bound %v, want the id, parent, and name", c.Args)
	}

	s, db, _ = openNative(t, sqltest.Response{Err: &sqlate.ConstraintError{Constraint: blobfs.ConstraintForeignKeyDirectoryParent, Class: sqlate.ErrForeignKeyViolation, Err: errors.New("no parent")}})
	_, err = s.Mkdir(ctx, db, blobfs.NewID(), "docs")
	var ve *blobfs.ViolationError
	if !errors.Is(err, blobfs.ErrNotFound) || !errors.As(err, &ve) || ve.Constraint != blobfs.ConstraintForeignKeyDirectoryParent {
		t.Errorf("Mkdir under a missing parent = %v, want ErrNotFound classified by the store", err)
	}
}

// TestCompleteFileWriteRoundTrips proves the complete step through the
// Postgres variant: a completed write is one query, the update with
// RETURNING bound with the object, the id, and the expected version; a
// refused one is two, the update that returns no row and the read by id,
// classified as the baseline classifies: no row is ErrNotFound, another
// version is ErrVersionMismatch naming both versions, and the expected
// version in another status is the TransitionError from that status.
func TestCompleteFileWriteRoundTrips(t *testing.T) {
	ctx := context.Background()
	obj := blobfs.Object{Size: 42, ContentType: "text/plain", ETag: `"abc"`}
	s, db, rec := openNative(t, fileResponse("F", "a.txt", blobfs.StatusAvailable, 2))
	f, err := s.CompleteFileWrite(ctx, db, "F", 1, obj)
	if err != nil || f.Status != blobfs.StatusAvailable || f.Version != 2 {
		t.Fatalf("CompleteFileWrite = %+v, %v, want the returned available row", f, err)
	}
	if ops := rec.Ops(); !slices.Equal(ops, []sqltest.Op{sqltest.OpQuery}) {
		t.Fatalf("ops = %v, want one query", ops)
	}
	c := rec.Calls()[0]
	if !strings.HasPrefix(c.SQL, "UPDATE blobfs_file AS f") || !strings.Contains(c.SQL, "status = 'available'") ||
		!strings.Contains(c.SQL, "updated_at = CURRENT_TIMESTAMP, version = f.version + 1") ||
		!strings.Contains(c.SQL, "WHERE f.id = CAST($4 AS uuid) AND f.version = CAST($5 AS bigint) AND f.status = 'pending'") ||
		!strings.HasSuffix(c.SQL, "RETURNING f.id, f.directory_id, f.name, f.status, f.key, f.size, f.content_type, f.etag, f.version, f.created_at, f.updated_at") {
		t.Errorf("the update is not the RETURNING form:\n%s", c.SQL)
	}
	if !slices.Equal(c.Args, []any{int64(42), "text/plain", `"abc"`, "F", int64(1)}) {
		t.Errorf("the update bound %v, want the size, content type, etag, id, and expected version", c.Args)
	}

	for _, r := range []struct {
		name string
		read sqltest.Response
		want error
		not  error
		text string
	}{
		{"NotFound", sqltest.Response{Columns: fileColumns}, blobfs.ErrNotFound, query.ErrVersionMismatch, ""},
		{"VersionMismatch", fileResponse("F", "a.txt", blobfs.StatusPending, 3), query.ErrVersionMismatch, blobfs.ErrInvalidTransition, "version mismatch: expected 1, current 3"},
		{"AlreadyAvailable", fileResponse("F", "a.txt", blobfs.StatusAvailable, 1), blobfs.ErrInvalidTransition, blobfs.ErrDeleting, ""},
		{"Deleting", fileResponse("F", "a.txt", blobfs.StatusDeleting, 1), blobfs.ErrDeleting, query.ErrVersionMismatch, ""},
	} {
		t.Run(r.name, func(t *testing.T) {
			s, db, rec := openNative(t, sqltest.Response{Columns: fileColumns}, r.read)
			_, err := s.CompleteFileWrite(ctx, db, "F", 1, obj)
			if !errors.Is(err, r.want) || errors.Is(err, r.not) || !strings.Contains(err.Error(), r.text) {
				t.Errorf("CompleteFileWrite = %v, want %v and not %v", err, r.want, r.not)
			}
			if ops := rec.Ops(); !slices.Equal(ops, []sqltest.Op{sqltest.OpQuery, sqltest.OpQuery}) {
				t.Errorf("ops = %v, want the update and one read", ops)
			}
			if read := rec.Calls()[1]; !strings.HasSuffix(read.SQL, "FROM blobfs_file f\nWHERE f.id = CAST($1 AS uuid)") || !slices.Equal(read.Args, []any{"F"}) {
				t.Errorf("the read is not file_by_id bound to the id: %q %v", read.SQL, read.Args)
			}
		})
	}
}
