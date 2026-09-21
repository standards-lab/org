//go:build integration

package data_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// anyKey is a KeyValidator that accepts every key, for tests whose subject
// is the row and not the key.
type anyKey struct{}

func (anyKey) ValidateKey(string) error { return nil }

// fileStatus reads a file's status, size, and etag directly through sess,
// so a test can see the row as another connection does.
func fileStatus(t *testing.T, sess sqlate.Session, id string) (status string, size *int64, etag *string) {
	t.Helper()
	rows, err := sess.QueryContext(t.Context(), "SELECT status, size, etag FROM blobfs_file WHERE id = $1", id)
	if err != nil {
		t.Fatalf("read file %s: %v", id, err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatalf("file %s: no row: %v", id, rows.Err())
	}
	if err := rows.Scan(&status, &size, &etag); err != nil {
		t.Fatalf("file %s: %v", id, err)
	}
	return status, size, etag
}

// TestFileWrite is the two-phase write on the engine: the begin step on
// the pool leaves a pending row that a second connection reads, with the
// key and the declared content type and no size or etag; the complete
// step moves it to available with the object's facts and advances the
// version once; a second complete is refused as an invalid transition
// (available to available), a complete at a stale version as a version
// mismatch, and a complete after a delete began as ErrDeleting. A name a
// pending row holds is taken, and a directory that does not exist is not
// found.
func TestFileWrite(t *testing.T) {
	e := open(t)
	other := e.second(t)
	dir := e.mkdir(t, blobfs.RootID, "docs")

	f, err := e.store.BeginFileWrite(e.ctx, e.db, anyKey{}, dir.ID, decomposed+".txt", "text/plain")
	if err != nil {
		t.Fatalf("BeginFileWrite: %v", err)
	}
	if f.Name != composed+".txt" || f.Status != blobfs.StatusPending || f.Key != f.ID+"/"+composed+".txt" ||
		f.ContentType != "text/plain" || f.Size != nil || f.ETag != nil || f.Version != 1 {
		t.Errorf("the pending row is %+v", f)
	}
	if status, size, etag := fileStatus(t, other, f.ID); status != "pending" || size != nil || etag != nil {
		t.Errorf("a second connection reads status %s, size %v, etag %v; want pending with nothing else", status, size, etag)
	}
	found, err := e.store.FileByName(e.ctx, other, dir.ID, decomposed+".txt")
	if err != nil || found.ID != f.ID {
		t.Errorf("FileByName from a second connection = %+v, %v; want the pending row", found, err)
	}

	if _, err := e.store.BeginFileWrite(e.ctx, e.db, anyKey{}, dir.ID, composed+".txt", "text/plain"); !errors.Is(err, blobfs.ErrNameTaken) {
		t.Errorf("a second begin under the same name = %v, want ErrNameTaken", err)
	}
	if _, err := e.store.BeginFileWrite(e.ctx, e.db, anyKey{}, blobfs.NewID(), "orphan.txt", "text/plain"); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("a begin in a missing directory = %v, want ErrNotFound", err)
	}

	obj := blobfs.Object{Size: 7, ContentType: "text/plain", ETag: `"0x1"`}
	done, err := e.store.CompleteFileWrite(e.ctx, e.db, f.ID, f.Version, obj)
	if err != nil {
		t.Fatalf("CompleteFileWrite: %v", err)
	}
	if done.Status != blobfs.StatusAvailable || done.Size == nil || *done.Size != 7 || done.ETag == nil || *done.ETag != `"0x1"` ||
		done.Version != 2 || !done.UpdatedAt.After(f.UpdatedAt) {
		t.Errorf("the completed row is %+v", done)
	}
	if status, size, _ := fileStatus(t, other, f.ID); status != "available" || size == nil || *size != 7 {
		t.Errorf("a second connection reads status %s, size %v; want available and 7", status, size)
	}

	_, err = e.store.CompleteFileWrite(e.ctx, e.db, f.ID, done.Version, obj)
	if !errors.Is(err, blobfs.ErrInvalidTransition) || errors.Is(err, blobfs.ErrDeleting) || errors.Is(err, query.ErrVersionMismatch) {
		t.Errorf("a second complete = %v, want an invalid transition from available", err)
	}
	if _, err := e.store.CompleteFileWrite(e.ctx, e.db, f.ID, f.Version, obj); !errors.Is(err, query.ErrVersionMismatch) {
		t.Errorf("a complete at the stale version = %v, want ErrVersionMismatch", err)
	}
	if _, err := e.store.CompleteFileWrite(e.ctx, e.db, blobfs.NewID(), 1, obj); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("a complete of a missing row = %v, want ErrNotFound", err)
	}

	// A write whose delete began before it completed.
	g, err := e.store.BeginFileWrite(e.ctx, e.db, anyKey{}, dir.ID, "gone.txt", "text/plain")
	if err != nil {
		t.Fatalf("BeginFileWrite(gone.txt): %v", err)
	}
	deleting, err := e.db.Transact(e.ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
		return e.store.BeginFileDelete(e.ctx, tx, g.ID)
	})
	if err != nil || deleting.Status != blobfs.StatusDeleting {
		t.Fatalf("BeginFileDelete = %+v, %v", deleting, err)
	}
	_, err = e.store.CompleteFileWrite(e.ctx, e.db, g.ID, deleting.Version, obj)
	if !errors.Is(err, blobfs.ErrDeleting) {
		t.Errorf("a complete of a deleting row = %v, want ErrDeleting", err)
	}
}

// TestFileWriteComposesIntoTheCallersTransaction proves the begin step
// runs inside a consumer's transaction beside the consumer's own row: a
// table the test creates references blobfs_file, the consumer inserts its
// row after the pending row in one Transact, and a rollback leaves
// neither row while a commit leaves both. The foreign key holds against
// the pending row inside the transaction, so a consumer can reference a
// file before its object exists.
func TestFileWriteComposesIntoTheCallersTransaction(t *testing.T) {
	e := open(t)
	if _, err := e.db.ExecContext(e.ctx, "CREATE TABLE consumer_note (file_id uuid PRIMARY KEY REFERENCES blobfs_file (id), note text NOT NULL)"); err != nil {
		t.Fatalf("create the consumer's table: %v", err)
	}
	dir := e.mkdir(t, blobfs.RootID, "notes")
	abort := errors.New("the consumer changed its mind")

	write := func(fail bool) (blobfs.File, error) {
		return e.db.Transact(e.ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
			f, err := e.store.BeginFileWrite(e.ctx, tx, anyKey{}, dir.ID, "plan.txt", "text/plain")
			if err != nil {
				return blobfs.File{}, err
			}
			if _, err := tx.ExecContext(e.ctx, "INSERT INTO consumer_note (file_id, note) VALUES ($1, $2)", f.ID, "draft"); err != nil {
				return blobfs.File{}, err
			}
			if fail {
				return blobfs.File{}, abort
			}
			return f, nil
		})
	}
	if _, err := write(true); !errors.Is(err, abort) {
		t.Fatalf("the aborted transaction = %v, want the consumer's error", err)
	}
	if n := count(t, e.db, "SELECT COUNT(*) FROM blobfs_file"); n != 0 {
		t.Errorf("after the rollback %d file rows exist, want none", n)
	}
	if n := count(t, e.db, "SELECT COUNT(*) FROM consumer_note"); n != 0 {
		t.Errorf("after the rollback %d notes exist, want none", n)
	}

	f, err := write(false)
	if err != nil {
		t.Fatalf("the committed transaction: %v", err)
	}
	if status, _, _ := fileStatus(t, e.second(t), f.ID); status != "pending" {
		t.Errorf("after the commit the file is %s, want pending", status)
	}
	if n := count(t, e.db, "SELECT COUNT(*) FROM consumer_note WHERE file_id = $1", f.ID); n != 1 {
		t.Errorf("after the commit %d notes reference the file, want one", n)
	}
}

// TestFileWriteRetryCompletesAPendingRow is the stop between the steps: a
// begin whose caller stops before the object is stored leaves a pending
// row; a later caller, on its own connection, finds the row by name in
// the directory and completes it at the version it read, and the row is
// then available once, with the retry's facts.
func TestFileWriteRetryCompletesAPendingRow(t *testing.T) {
	e := open(t)
	dir := e.mkdir(t, blobfs.RootID, "retry")
	first, err := e.store.BeginFileWrite(e.ctx, e.db, anyKey{}, dir.ID, "report.pdf", "application/pdf")
	if err != nil {
		t.Fatalf("BeginFileWrite: %v", err)
	}
	// The first caller stops here.

	later := e.second(t)
	pending, err := e.store.FileByName(e.ctx, later, dir.ID, "report.pdf")
	if err != nil {
		t.Fatalf("FileByName from the retry: %v", err)
	}
	if pending.ID != first.ID || pending.Status != blobfs.StatusPending {
		t.Fatalf("the retry found %+v, want the first caller's pending row", pending)
	}
	done, err := e.store.CompleteFileWrite(e.ctx, later, pending.ID, pending.Version, blobfs.Object{Size: 3, ContentType: "application/pdf", ETag: `"r"`})
	if err != nil {
		t.Fatalf("CompleteFileWrite from the retry: %v", err)
	}
	if done.Status != blobfs.StatusAvailable || done.Version != 2 || done.ID != first.ID {
		t.Errorf("the retry completed %+v", done)
	}
	if names := e.strings1(t, "SELECT name FROM blobfs_file WHERE directory_id = $1", dir.ID); strings.Join(names, ",") != "report.pdf" {
		t.Errorf("the directory holds %v, want the one row", names)
	}
}

// count runs a COUNT(*) query with its arguments.
func count(t *testing.T, sess sqlate.Session, sql string, args ...any) int {
	t.Helper()
	rows, err := sess.QueryContext(t.Context(), sql, args...)
	if err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	defer func() { _ = rows.Close() }()
	var n int
	if !rows.Next() || rows.Scan(&n) != nil {
		t.Fatalf("%s: no row: %v", sql, rows.Err())
	}
	return n
}
