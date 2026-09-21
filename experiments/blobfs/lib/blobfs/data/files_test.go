package data_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

// accepting is a KeyValidator that accepts every key.
type accepting struct{}

func (accepting) ValidateKey(string) error { return nil }

// refusing is a KeyValidator that refuses every key with a fixed error.
type refusing struct{ err error }

func (r refusing) ValidateKey(string) error { return r.err }

// fileResponse scripts one file row with a status and version, its
// columns in fileColumns order.
func fileResponse(id, name string, status blobfs.Status, version int64) sqltest.Response {
	now := time.Now()
	row := []driver.Value{id, blobfs.RootID, name, string(status), id + "/" + name, nil, "text/plain", nil, version, now, now}
	return sqltest.Response{Columns: fileColumns, Rows: [][]driver.Value{row}}
}

// openStore builds the store and the scripted session together.
func openStore(t *testing.T, responses ...sqltest.Response) (*data.Store, *sqlate.DB, *sqltest.Recorder) {
	t.Helper()
	pool, rec := sqltest.Open(t, responses...)
	return newStore(t), sqlate.Wrap(pool, sqltest.Dialect{}), rec
}

// TestBeginFileWrite proves the first step: the name is normalized, the id
// is minted in Go, the key is built from the id and the sanitized name and
// bound with the declared content type, the row is inserted as pending in
// one exec, and the row is read back by id. A refused name and a refused
// key fail before any SQL, the key's refusal as a KeyError that carries
// the store's reason, and a violation of blobfs's own constraints
// classifies through the write mapping.
func TestBeginFileWrite(t *testing.T) {
	ctx := context.Background()
	s, db, rec := openStore(t, sqltest.Response{Affected: 1}, fileResponse("F", "résumé.pdf", blobfs.StatusPending, 1))
	f, err := s.BeginFileWrite(ctx, db, accepting{}, blobfs.RootID, "résumé.pdf", "application/pdf")
	if err != nil {
		t.Fatalf("BeginFileWrite: %v", err)
	}
	if f.ID != "F" || f.Status != blobfs.StatusPending {
		t.Errorf("BeginFileWrite returned %+v, want the pending row read back", f)
	}
	calls := rec.Calls()
	if len(calls) != 2 || calls[0].Op != sqltest.OpExec || calls[1].Op != sqltest.OpQuery {
		t.Fatalf("calls = %v, want the insert then the read-back", calls)
	}
	if !strings.HasPrefix(calls[0].SQL, "INSERT INTO blobfs_file") || !strings.Contains(calls[0].SQL, "'pending'") {
		t.Errorf("insert SQL = %q", calls[0].SQL)
	}
	args := calls[0].Args
	id, ok := args[0].(string)
	if !ok || len(id) != 36 {
		t.Fatalf("the insert bound %v as the id, want a minted UUID", args[0])
	}
	name := "résumé.pdf"
	if args[1] != blobfs.RootID || args[2] != name || args[3] != id+"/"+name || args[4] != "application/pdf" {
		t.Errorf("the insert bound %v, want the directory, the normalized name, the key, and the content type", args)
	}
	if calls[1].Args[0] != id {
		t.Errorf("the read-back bound %v, want the minted id", calls[1].Args)
	}

	s, db, rec = openStore(t)
	if _, err := s.BeginFileWrite(ctx, db, accepting{}, blobfs.RootID, "a/b", "text/plain"); !errors.Is(err, blobfs.ErrInvalidName) {
		t.Errorf("BeginFileWrite(a/b) = %v, want ErrInvalidName", err)
	}
	cause := errors.New("the store says no")
	_, err = s.BeginFileWrite(ctx, db, refusing{cause}, blobfs.RootID, "ok.txt", "text/plain")
	if !errors.Is(err, blobfs.ErrInvalidKey) || !errors.Is(err, cause) {
		t.Errorf("BeginFileWrite with a refusing store = %v, want ErrInvalidKey wrapping the cause", err)
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("the refusals reached the driver with %v", calls)
	}

	for constraint, want := range map[string]error{
		blobfs.ConstraintUniqueFileDirectoryName: blobfs.ErrNameTaken,
		blobfs.ConstraintForeignKeyFileDirectory: blobfs.ErrNotFound,
	} {
		class := sqlate.ErrUniqueViolation
		if want == blobfs.ErrNotFound {
			class = sqlate.ErrForeignKeyViolation
		}
		s, db, _ := openStore(t, sqltest.Response{Err: &sqlate.ConstraintError{Constraint: constraint, Class: class, Err: cause}})
		if _, err := s.BeginFileWrite(ctx, db, accepting{}, blobfs.RootID, "ok.txt", "text/plain"); !errors.Is(err, want) {
			t.Errorf("BeginFileWrite under %s = %v, want %v", constraint, err, want)
		}
	}
}

// TestBeginFileWriteKeyBoundary proves the key is validated by the store's
// rules as given: a name whose key sits exactly at a rune limit is
// accepted, and one rune over is refused before any SQL, with the store's
// reason reachable.
func TestBeginFileWriteKeyBoundary(t *testing.T) {
	ctx := context.Background()
	const limit = 60
	// The id is 36 runes and the slash one, so 23 two-byte runes fill the
	// limit exactly.
	fits := strings.Repeat("é", limit-37)
	s, db, rec := openStore(t, sqltest.Response{Affected: 1}, fileResponse("F", fits, blobfs.StatusPending, 1))
	if _, err := s.BeginFileWrite(ctx, db, runeLimit(limit), blobfs.RootID, fits, "text/plain"); err != nil {
		t.Fatalf("BeginFileWrite at the boundary: %v", err)
	}
	key, _ := rec.Calls()[0].Args[3].(string)
	if n := utf8.RuneCountInString(key); n != limit {
		t.Errorf("the key bound is %d runes, want %d", n, limit)
	}
	if len(key) <= limit {
		t.Errorf("the key bound is %d bytes; the case needs more bytes than runes", len(key))
	}

	s, db, rec = openStore(t)
	_, err := s.BeginFileWrite(ctx, db, runeLimit(limit), blobfs.RootID, fits+"é", "text/plain")
	if !errors.Is(err, blobfs.ErrInvalidKey) {
		t.Fatalf("BeginFileWrite one rune over = %v, want ErrInvalidKey", err)
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("the refusal reached the driver with %v", calls)
	}
}

// runeLimit is a KeyValidator that refuses a key longer than its value in
// runes, as the providers blobfs targets count.
type runeLimit int

func (l runeLimit) ValidateKey(key string) error {
	if n := utf8.RuneCountInString(key); n > int(l) {
		return errors.New("over the limit")
	}
	return nil
}

// TestCompleteFileWrite proves the last step: the guarded update binds the
// object's facts, the id, and the expected version and, when it affects a
// row, is followed only by the read-back; when it affects none, the
// guard's check runs, and a missing row is ErrNotFound, a row at another
// version query.ErrVersionMismatch, and a row at the expected version that
// is not pending a TransitionError from its status, which matches
// ErrDeleting for a deleting row.
func TestCompleteFileWrite(t *testing.T) {
	ctx := context.Background()
	obj := blobfs.Object{Size: 42, ContentType: "text/plain", ETag: `"abc"`}
	s, db, rec := openStore(t, sqltest.Response{Affected: 1}, fileResponse("F", "a.txt", blobfs.StatusAvailable, 2))
	f, err := s.CompleteFileWrite(ctx, db, "F", 1, obj)
	if err != nil {
		t.Fatalf("CompleteFileWrite: %v", err)
	}
	if f.Status != blobfs.StatusAvailable || f.Version != 2 {
		t.Errorf("CompleteFileWrite returned %+v, want the available row read back", f)
	}
	calls := rec.Calls()
	if len(calls) != 2 || calls[0].Op != sqltest.OpExec || calls[1].Op != sqltest.OpQuery {
		t.Fatalf("calls = %v, want the guarded update then the read-back", calls)
	}
	text := calls[0].SQL
	if !strings.HasPrefix(text, "UPDATE blobfs_file") || !strings.Contains(text, "status = 'available'") ||
		!strings.Contains(text, "version = version + 1") || !strings.Contains(text, "AND status = 'pending'") {
		t.Errorf("update SQL = %q", text)
	}
	if args := calls[0].Args; len(args) != 5 || args[0] != int64(42) || args[1] != "text/plain" || args[2] != `"abc"` || args[3] != "F" || args[4] != int64(1) {
		t.Errorf("the update bound %v, want the size, content type, etag, id, and expected version", args)
	}

	// No row affected and no row on the check: not found.
	s, db, _ = openStore(t, sqltest.Response{Affected: 0}, sqltest.Response{Columns: []string{"version"}})
	if _, err := s.CompleteFileWrite(ctx, db, "F", 1, obj); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("CompleteFileWrite of a missing row = %v, want ErrNotFound", err)
	}

	// No row affected, the check reports another version: the guard's own
	// conflict, confirmed by the read of the row.
	s, db, _ = openStore(t, sqltest.Response{Affected: 0}, version(3), fileResponse("F", "a.txt", blobfs.StatusPending, 3))
	if _, err := s.CompleteFileWrite(ctx, db, "F", 1, obj); !errors.Is(err, query.ErrVersionMismatch) {
		t.Errorf("CompleteFileWrite at a stale version = %v, want ErrVersionMismatch", err)
	}

	// No row affected, the check reports the expected version: the status
	// predicate refused the row, and the read tells which status.
	for _, status := range []blobfs.Status{blobfs.StatusDeleting, blobfs.StatusAvailable} {
		s, db, _ = openStore(t, sqltest.Response{Affected: 0}, version(1), fileResponse("F", "a.txt", status, 1))
		_, err := s.CompleteFileWrite(ctx, db, "F", 1, obj)
		if !errors.Is(err, blobfs.ErrInvalidTransition) || errors.Is(err, query.ErrVersionMismatch) {
			t.Errorf("CompleteFileWrite of a %s row = %v, want ErrInvalidTransition and no version mismatch", status, err)
		}
		if errors.Is(err, blobfs.ErrDeleting) != (status == blobfs.StatusDeleting) {
			t.Errorf("CompleteFileWrite of a %s row = %v; ErrDeleting should match for deleting only", status, err)
		}
	}
}

// version scripts the guard's check answering one version.
func version(v int64) sqltest.Response {
	return sqltest.Response{Columns: []string{"version"}, Rows: [][]driver.Value{{v}}}
}

// TestFileByName proves the read binds the directory and the normalized
// name, reports no row as ErrNotFound, and refuses an invalid name before
// any SQL.
func TestFileByName(t *testing.T) {
	ctx := context.Background()
	s, db, rec := openStore(t, fileResponse("F", "résumé.pdf", blobfs.StatusPending, 1))
	f, err := s.FileByName(ctx, db, blobfs.RootID, "résumé.pdf")
	if err != nil || f.ID != "F" {
		t.Fatalf("FileByName = %+v, %v", f, err)
	}
	if args := rec.Calls()[0].Args; args[0] != blobfs.RootID || args[1] != "résumé.pdf" {
		t.Errorf("FileByName bound %v, want the directory and the normalized name", args)
	}
	s, db, rec = openStore(t, sqltest.Response{Columns: fileColumns})
	if _, err := s.FileByName(ctx, db, blobfs.RootID, "missing"); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("FileByName of a missing file = %v, want ErrNotFound", err)
	}
	if _, err := s.FileByName(ctx, db, blobfs.RootID, ""); !errors.Is(err, blobfs.ErrInvalidName) {
		t.Errorf("FileByName of an empty name = %v, want ErrInvalidName", err)
	}
	if calls := rec.Calls(); len(calls) != 1 {
		t.Errorf("the refusal reached the driver: %v", calls)
	}
}
