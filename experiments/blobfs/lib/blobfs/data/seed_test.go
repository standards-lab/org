package data_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

// childResponse scripts one directory row under the root at version 1.
func childResponse(id, name string) sqltest.Response {
	return directoryResponse(id, blobfs.RootID, name, 1)
}

// noDirectory scripts the read of a name no directory holds.
func noDirectory() sqltest.Response { return sqltest.Response{Columns: directoryColumns} }

// noFile scripts the read of a name no file holds.
func noFile() sqltest.Response { return sqltest.Response{Columns: fileColumns} }

// nameTaken scripts an insert refused by the unique constraint on the
// name, as the engine reports a concurrent creator.
func nameTaken(constraint string) sqltest.Response {
	return sqltest.Response{Err: &sqlate.ConstraintError{Constraint: constraint, Class: sqlate.ErrUniqueViolation, Err: errors.New("duplicate key")}}
}

// ops joins the recorder's calls into one line of op names.
func ops(rec *sqltest.Recorder) string {
	var out []string
	for _, op := range rec.Ops() {
		out = append(out, string(op))
	}
	return strings.Join(out, " ")
}

// TestEnsureDirectory proves the insert-or-find: a name a directory holds
// is one lookup and the row found, not created; a name nothing holds is
// the lookup, the insert, and the read-back, created; and a creator that
// wins between the lookup and the insert on the pool is recovered by one
// more lookup, whose row is returned as found. Inside a transaction the
// same race returns ErrNameTaken and no further statement runs, since
// the failed insert has aborted the transaction. A violation that is not
// the name's, an id another row carries, is returned as it came from the
// write mapping.
func TestEnsureDirectory(t *testing.T) {
	ctx := context.Background()
	s, db, rec := openStore(t, childResponse("D", "docs"))
	d, created, err := s.EnsureDirectory(ctx, db, blobfs.RootID, "docs")
	if err != nil || created || d.ID != "D" {
		t.Fatalf("EnsureDirectory of a held name = %+v, %v, %v; want the row found", d, created, err)
	}
	if calls := rec.Calls(); len(calls) != 1 || calls[0].Op != sqltest.OpQuery || calls[0].Args[0] != blobfs.RootID || calls[0].Args[1] != "docs" {
		t.Errorf("calls = %v, want one lookup bound to the parent and the name", calls)
	}

	s, db, rec = openStore(t, noDirectory(), sqltest.Response{Affected: 1}, childResponse("N", "docs"))
	d, created, err = s.EnsureDirectory(ctx, db, blobfs.RootID, "docs")
	if err != nil || !created || d.ID != "N" {
		t.Fatalf("EnsureDirectory of a free name = %+v, %v, %v; want the row created", d, created, err)
	}
	if got := ops(rec); got != "query exec query" {
		t.Errorf("ops = %q, want the lookup, the insert, and the read-back", got)
	}
	calls := rec.Calls()
	id, ok := calls[1].Args[0].(string)
	if !ok || len(id) != 36 || calls[1].Args[1] != blobfs.RootID || calls[1].Args[2] != "docs" || calls[2].Args[0] != id {
		t.Errorf("the insert bound %v and the read-back %v; want a minted id, the parent, and the name, then the id", calls[1].Args, calls[2].Args)
	}

	s, db, rec = openStore(t, noDirectory(), nameTaken(blobfs.ConstraintUniqueDirectoryParentName), childResponse("C", "docs"))
	d, created, err = s.EnsureDirectory(ctx, db, blobfs.RootID, "docs")
	if err != nil || created || d.ID != "C" {
		t.Fatalf("EnsureDirectory under a concurrent creator = %+v, %v, %v; want the creator's row found", d, created, err)
	}
	if got := ops(rec); got != "query exec query" {
		t.Errorf("ops = %q, want the lookup, the refused insert, and the recovery lookup", got)
	}

	s, db, rec = openStore(t, noDirectory(), nameTaken(blobfs.ConstraintUniqueDirectoryParentName))
	_, err = db.Transact(ctx, func(tx *sqlate.Tx) (blobfs.Directory, error) {
		d, _, err := s.EnsureDirectory(ctx, tx, blobfs.RootID, "docs")
		return d, err
	})
	if !errors.Is(err, blobfs.ErrNameTaken) {
		t.Errorf("EnsureDirectory under a concurrent creator inside a transaction = %v, want ErrNameTaken", err)
	}
	if got := ops(rec); got != "begin query exec rollback" {
		t.Errorf("ops = %q, want no lookup after the refused insert inside the transaction", got)
	}

	s, db, rec = openStore(t, noDirectory(), nameTaken(blobfs.ConstraintPrimaryKeyDirectory))
	_, _, err = s.EnsureDirectory(ctx, db, blobfs.RootID, "docs", data.WithID(blobfs.NewID()))
	if !errors.Is(err, blobfs.ErrIDTaken) || errors.Is(err, blobfs.ErrNameTaken) {
		t.Errorf("EnsureDirectory under a taken id = %v, want ErrIDTaken", err)
	}
	if got := ops(rec); got != "query exec" {
		t.Errorf("ops = %q, want no recovery lookup after a violation that is not the name's", got)
	}
}

// TestEnsureDirectoryRefusals proves the checks that run before any SQL:
// an invalid name, and a supplied id that is the nil UUID or no UUID.
func TestEnsureDirectoryRefusals(t *testing.T) {
	ctx := context.Background()
	s, db, rec := openStore(t)
	if _, _, err := s.EnsureDirectory(ctx, db, blobfs.RootID, "a/b"); !errors.Is(err, blobfs.ErrInvalidName) {
		t.Errorf("EnsureDirectory(a/b) = %v, want ErrInvalidName", err)
	}
	for _, id := range []string{blobfs.RootID, "", "not-a-uuid"} {
		if _, _, err := s.EnsureDirectory(ctx, db, blobfs.RootID, "docs", data.WithID(id)); !errors.Is(err, blobfs.ErrInvalidID) {
			t.Errorf("EnsureDirectory with the id %q = %v, want ErrInvalidID", id, err)
		}
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("the refusals reached the driver with %v", calls)
	}
}

// TestBeginOrResumeFileWrite proves the outcomes: a free name is the
// lookup, the insert, and the read-back, created; a pending row is one
// lookup, resumed; an available or a deleting row is one lookup, exists,
// with the row's status telling which; and a writer that wins between the
// lookup and the insert on the pool is recovered by one more lookup,
// whose row is reported by its status. Inside a transaction the same race
// returns ErrNameTaken and no further statement runs.
func TestBeginOrResumeFileWrite(t *testing.T) {
	ctx := context.Background()
	s, db, rec := openStore(t, noFile(), sqltest.Response{Affected: 1}, fileResponse("F", "a.txt", blobfs.StatusPending, 1))
	f, outcome, err := s.BeginOrResumeFileWrite(ctx, db, accepting{}, blobfs.RootID, "a.txt", "text/plain")
	if err != nil || outcome != data.WriteCreated || f.ID != "F" || f.Status != blobfs.StatusPending {
		t.Fatalf("BeginOrResumeFileWrite of a free name = %+v, %v, %v; want the pending row created", f, outcome, err)
	}
	if got := ops(rec); got != "query exec query" {
		t.Errorf("ops = %q, want the lookup, the insert, and the read-back", got)
	}
	calls := rec.Calls()
	if calls[0].Args[0] != blobfs.RootID || calls[0].Args[1] != "a.txt" {
		t.Errorf("the lookup bound %v, want the directory and the name", calls[0].Args)
	}
	id, ok := calls[1].Args[0].(string)
	if !ok || len(id) != 36 || calls[1].Args[1] != blobfs.RootID || calls[1].Args[2] != "a.txt" || calls[1].Args[3] != id+"/a.txt" || calls[1].Args[4] != "text/plain" || calls[2].Args[0] != id {
		t.Errorf("the insert bound %v and the read-back %v; want the minted id, the directory, the name, the key, and the content type, then the id", calls[1].Args, calls[2].Args)
	}

	s, db, rec = openStore(t, fileResponse("P", "a.txt", blobfs.StatusPending, 1))
	f, outcome, err = s.BeginOrResumeFileWrite(ctx, db, accepting{}, blobfs.RootID, "a.txt", "text/plain")
	if err != nil || outcome != data.WriteResumed || f.ID != "P" {
		t.Fatalf("BeginOrResumeFileWrite over a pending row = %+v, %v, %v; want the row resumed", f, outcome, err)
	}
	if got := ops(rec); got != "query" {
		t.Errorf("ops = %q, want the lookup alone", got)
	}

	for _, status := range []blobfs.Status{blobfs.StatusAvailable, blobfs.StatusDeleting} {
		s, db, rec = openStore(t, fileResponse("A", "a.txt", status, 2))
		f, outcome, err = s.BeginOrResumeFileWrite(ctx, db, accepting{}, blobfs.RootID, "a.txt", "text/plain")
		if err != nil || outcome != data.WriteExists || f.ID != "A" || f.Status != status {
			t.Errorf("BeginOrResumeFileWrite over a %s row = %+v, %v, %v; want the row as it is and exists", status, f, outcome, err)
		}
		if got := ops(rec); got != "query" {
			t.Errorf("ops = %q, want the lookup alone", got)
		}
	}

	for status, want := range map[blobfs.Status]data.WriteOutcome{blobfs.StatusPending: data.WriteResumed, blobfs.StatusAvailable: data.WriteExists} {
		s, db, rec = openStore(t, noFile(), nameTaken(blobfs.ConstraintUniqueFileDirectoryName), fileResponse("C", "a.txt", status, 1))
		f, outcome, err = s.BeginOrResumeFileWrite(ctx, db, accepting{}, blobfs.RootID, "a.txt", "text/plain")
		if err != nil || outcome != want || f.ID != "C" {
			t.Errorf("BeginOrResumeFileWrite under a concurrent %s writer = %+v, %v, %v; want the writer's row and %s", status, f, outcome, err, want)
		}
		if got := ops(rec); got != "query exec query" {
			t.Errorf("ops = %q, want the lookup, the refused insert, and the recovery lookup", got)
		}
	}

	s, db, rec = openStore(t, noFile(), nameTaken(blobfs.ConstraintUniqueFileDirectoryName))
	_, err = db.Transact(ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
		f, _, err := s.BeginOrResumeFileWrite(ctx, tx, accepting{}, blobfs.RootID, "a.txt", "text/plain")
		return f, err
	})
	if !errors.Is(err, blobfs.ErrNameTaken) {
		t.Errorf("BeginOrResumeFileWrite under a concurrent writer inside a transaction = %v, want ErrNameTaken", err)
	}
	if got := ops(rec); got != "begin query exec rollback" {
		t.Errorf("ops = %q, want no lookup after the refused insert inside the transaction", got)
	}
}

// TestBeginOrResumeFileWriteRefusals proves the checks that run before
// any SQL, the same as BeginFileWrite's: an invalid name, a key the store
// refuses, and a supplied id that is the nil UUID or no UUID.
func TestBeginOrResumeFileWriteRefusals(t *testing.T) {
	ctx := context.Background()
	s, db, rec := openStore(t)
	if _, _, err := s.BeginOrResumeFileWrite(ctx, db, accepting{}, blobfs.RootID, "a/b", "text/plain"); !errors.Is(err, blobfs.ErrInvalidName) {
		t.Errorf("BeginOrResumeFileWrite(a/b) = %v, want ErrInvalidName", err)
	}
	cause := errors.New("the store says no")
	if _, _, err := s.BeginOrResumeFileWrite(ctx, db, refusing{cause}, blobfs.RootID, "ok.txt", "text/plain"); !errors.Is(err, blobfs.ErrInvalidKey) || !errors.Is(err, cause) {
		t.Errorf("BeginOrResumeFileWrite with a refusing store = %v, want ErrInvalidKey wrapping the cause", err)
	}
	for _, id := range []string{blobfs.RootID, "", "not-a-uuid"} {
		if _, _, err := s.BeginOrResumeFileWrite(ctx, db, accepting{}, blobfs.RootID, "ok.txt", "text/plain", data.WithID(id)); !errors.Is(err, blobfs.ErrInvalidID) {
			t.Errorf("BeginOrResumeFileWrite with the id %q = %v, want ErrInvalidID", id, err)
		}
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("the refusals reached the driver with %v", calls)
	}
}

// TestWithID proves a supplied id is the id the insert binds and the
// read-back reads, in canonical form whatever form it came in, on Mkdir
// and on BeginFileWrite, where it is also the key's first segment; that
// the nil UUID, an empty id, and text that is no UUID are refused before
// any SQL; and that an id another row carries, refused by the primary
// key, is ErrIDTaken naming the key, with the sqlate.ConstraintError
// reachable and the driver's text hidden.
func TestWithID(t *testing.T) {
	ctx := context.Background()
	id := blobfs.NewID()
	s, db, rec := openStore(t, sqltest.Response{Affected: 1}, childResponse(id, "docs"))
	d, err := s.Mkdir(ctx, db, blobfs.RootID, "docs", data.WithID(id))
	if err != nil || d.ID != id {
		t.Fatalf("Mkdir with an id = %+v, %v", d, err)
	}
	calls := rec.Calls()
	if calls[0].Args[0] != id || calls[1].Args[0] != id {
		t.Errorf("the insert bound %v and the read-back %v; want the supplied id under both", calls[0].Args, calls[1].Args)
	}

	s, db, rec = openStore(t, sqltest.Response{Affected: 1}, childResponse(id, "docs"))
	if _, err := s.Mkdir(ctx, db, blobfs.RootID, "docs", data.WithID("{"+strings.ToUpper(id)+"}")); err != nil {
		t.Fatalf("Mkdir with a braced upper-case id: %v", err)
	}
	if got := rec.Calls()[0].Args[0]; got != id {
		t.Errorf("the insert bound %v, want the canonical form %q", got, id)
	}

	s, db, rec = openStore(t, sqltest.Response{Affected: 1}, fileResponse(id, "a.txt", blobfs.StatusPending, 1))
	f, err := s.BeginFileWrite(ctx, db, accepting{}, blobfs.RootID, "a.txt", "text/plain", data.WithID(id))
	if err != nil || f.ID != id {
		t.Fatalf("BeginFileWrite with an id = %+v, %v", f, err)
	}
	if args := rec.Calls()[0].Args; args[0] != id || args[3] != id+"/a.txt" {
		t.Errorf("the insert bound %v, want the supplied id and the key built from it", args)
	}

	s, db, rec = openStore(t)
	for _, bad := range []string{blobfs.RootID, "", "not-a-uuid"} {
		if _, err := s.Mkdir(ctx, db, blobfs.RootID, "docs", data.WithID(bad)); !errors.Is(err, blobfs.ErrInvalidID) {
			t.Errorf("Mkdir with the id %q = %v, want ErrInvalidID", bad, err)
		}
		if _, err := s.BeginFileWrite(ctx, db, accepting{}, blobfs.RootID, "a.txt", "text/plain", data.WithID(bad)); !errors.Is(err, blobfs.ErrInvalidID) {
			t.Errorf("BeginFileWrite with the id %q = %v, want ErrInvalidID", bad, err)
		}
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("the refusals reached the driver with %v", calls)
	}

	for constraint, begin := range map[string]func(*data.Store, *sqlate.DB) error{
		blobfs.ConstraintPrimaryKeyDirectory: func(s *data.Store, db *sqlate.DB) error {
			_, err := s.Mkdir(ctx, db, blobfs.RootID, "docs", data.WithID(id))
			return err
		},
		blobfs.ConstraintPrimaryKeyFile: func(s *data.Store, db *sqlate.DB) error {
			_, err := s.BeginFileWrite(ctx, db, accepting{}, blobfs.RootID, "a.txt", "text/plain", data.WithID(id))
			return err
		},
	} {
		s, db, _ := openStore(t, nameTaken(constraint))
		err := begin(s, db)
		var ve *blobfs.ViolationError
		if !errors.Is(err, blobfs.ErrIDTaken) || !errors.As(err, &ve) || ve.Constraint != constraint {
			t.Errorf("an insert under %s = %v, want ErrIDTaken naming the constraint", constraint, err)
		}
		if !strings.HasSuffix(err.Error(), blobfs.ErrIDTaken.Error()+" (constraint "+constraint+")") || strings.Contains(err.Error(), "duplicate key") {
			t.Errorf("an insert under %s = %q, want the sentinel and the constraint named and the driver's text hidden", constraint, err)
		}
	}
}
