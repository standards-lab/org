package data_test

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

// deletingRow is one scripted file row in status deleting, its columns in
// fileColumns order.
func deletingRow(id, name string) []driver.Value {
	now := time.Now()
	return []driver.Value{id, blobfs.RootID, name, "deleting", id + "/" + name, int64(3), "text/plain", nil, int64(2), now, now}
}

// TestStandardTreeLock proves the baseline's tree lock: it reports that it
// does not serialize, refuses the pool with ErrTransactionRequired before
// any SQL, and inside a transaction runs no statement at all.
func TestStandardTreeLock(t *testing.T) {
	s := newStore(t)
	pool, rec := sqltest.Open(t)
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	ctx := context.Background()
	if s.Serializes() {
		t.Error("Serializes = true, want false for the baseline")
	}
	if err := s.LockTree(ctx, db); !errors.Is(err, query.ErrTransactionRequired) {
		t.Errorf("LockTree on the pool = %v, want ErrTransactionRequired", err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := s.LockTree(ctx, tx); err != nil {
		t.Errorf("LockTree in a transaction = %v, want nil", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if ops := rec.Ops(); !slices.Equal(ops, []sqltest.Op{sqltest.OpBegin, sqltest.OpCommit}) {
		t.Errorf("the baseline's LockTree ran %v, want only the begin and the commit", ops)
	}
}

// TestStandardBeginFileDelete proves the baseline's file-delete begin: the
// pool is refused before any SQL; in a transaction it runs the update that
// skips a row already deleting and then the read by id, in that order and
// nothing else, and returns the row the read produced; and a read that
// finds no row is ErrNotFound.
func TestStandardBeginFileDelete(t *testing.T) {
	s := newStore(t)
	pool, rec := sqltest.Open(t)
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	ctx := context.Background()
	id := blobfs.NewID()

	if _, err := s.BeginFileDelete(ctx, db, id); !errors.Is(err, query.ErrTransactionRequired) {
		t.Errorf("BeginFileDelete on the pool = %v, want ErrTransactionRequired", err)
	}
	if calls := rec.Calls(); len(calls) != 0 {
		t.Errorf("the refusal reached the driver: %v", calls)
	}

	rec.Queue(
		sqltest.Response{Affected: 1},
		sqltest.Response{Columns: fileColumns, Rows: [][]driver.Value{deletingRow(id, "a.txt")}},
	)
	f, err := db.Transact(ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
		return s.BeginFileDelete(ctx, tx, id)
	})
	if err != nil {
		t.Fatalf("BeginFileDelete: %v", err)
	}
	if f.ID != id || f.Status != blobfs.StatusDeleting || f.Version != 2 || f.Name != "a.txt" {
		t.Errorf("BeginFileDelete = %+v, want the scripted deleting row", f)
	}
	if ops := rec.Ops(); !slices.Equal(ops, []sqltest.Op{sqltest.OpBegin, sqltest.OpExec, sqltest.OpQuery, sqltest.OpCommit}) {
		t.Errorf("ops = %v, want begin, exec, query, commit", ops)
	}
	update := rec.SQL(sqltest.OpExec)[0]
	if !strings.HasPrefix(update, "UPDATE blobfs_file") || !strings.Contains(update, "status = 'deleting'") ||
		!strings.Contains(update, "version = version + 1") || !strings.Contains(update, "AND status <> 'deleting'") ||
		strings.Contains(update, "RETURNING") {
		t.Errorf("the update is not the baseline's:\n%s", update)
	}
	read := rec.SQL(sqltest.OpQuery)[0]
	if !strings.HasPrefix(read, "SELECT f.id, f.directory_id") || !strings.HasSuffix(read, "WHERE f.id = CAST($1 AS uuid)") {
		t.Errorf("the read-back is not file_by_id:\n%s", read)
	}
	for _, c := range rec.Calls() {
		if (c.Op == sqltest.OpExec || c.Op == sqltest.OpQuery) && !slices.Equal(c.Args, []any{id}) {
			t.Errorf("%s bound %v, want the id alone", c.Op, c.Args)
		}
	}

	rec.Queue(sqltest.Response{Affected: 0}, sqltest.Response{Columns: fileColumns})
	_, err = db.Transact(ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
		return s.BeginFileDelete(ctx, tx, id)
	})
	if !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("BeginFileDelete of a missing row = %v, want ErrNotFound", err)
	}
}

// TestFile proves File reads by id through file_by_id and maps no row to
// ErrNotFound.
func TestFile(t *testing.T) {
	s := newStore(t)
	pool, rec := sqltest.Open(t)
	db := sqlate.Wrap(pool, sqltest.Dialect{})
	ctx := context.Background()
	id := blobfs.NewID()
	rec.Queue(sqltest.Response{Columns: fileColumns, Rows: [][]driver.Value{fileRow(id, "a.txt")}})
	f, err := s.File(ctx, db, id)
	if err != nil || f.ID != id || f.Name != "a.txt" || f.Status != blobfs.StatusAvailable {
		t.Errorf("File = %+v, %v, want the scripted row", f, err)
	}
	rec.Queue(sqltest.Response{Columns: fileColumns})
	if _, err := s.File(ctx, db, id); !errors.Is(err, blobfs.ErrNotFound) {
		t.Errorf("File(missing) = %v, want ErrNotFound", err)
	}
}

// beginOverride is a consumer's variant: the baseline with the file-delete
// begin overridden. It embeds the interface, so the methods it does not
// declare are the embedded variant's.
type beginOverride struct {
	data.Variant
	calls []string
}

func (v *beginOverride) BeginFileDelete(_ context.Context, _ sqlate.Session, id string) (blobfs.File, error) {
	v.calls = append(v.calls, id)
	return blobfs.File{ID: id, Status: blobfs.StatusDeleting, Version: 7}, nil
}

// lockOverride is the other direction: a variant with the tree lock
// overridden and the file-delete begin left to the embedded baseline.
type lockOverride struct {
	data.Variant
	locked int
}

func (v *lockOverride) LockTree(context.Context, sqlate.Session) error {
	v.locked++
	return nil
}

func (*lockOverride) Serializes() bool { return true }

// TestConsumerVariantSwapsOneMethod is the stage gate's proof that a
// consumer-supplied variant needs no fork: a wrapper that embeds the
// baseline built with NewStandard and overrides one method is handed to
// New through WithVariant, the store runs the overridden method, and the
// other method still runs as the baseline does. Both directions are
// shown: the begin overridden with the lock from the baseline, and the
// lock overridden with the begin from the baseline.
func TestConsumerVariantSwapsOneMethod(t *testing.T) {
	ctx := context.Background()
	base, err := data.NewStandard(catalog(t), sqltest.Dialect{})
	if err != nil {
		t.Fatalf("NewStandard: %v", err)
	}

	t.Run("BeginOverridden", func(t *testing.T) {
		v := &beginOverride{Variant: base}
		s, err := data.New(catalog(t), sqltest.Dialect{}, data.WithVariant(v))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if s.Variant() != v {
			t.Errorf("Variant() = %T, want the wrapper", s.Variant())
		}
		pool, rec := sqltest.Open(t)
		db := sqlate.Wrap(pool, sqltest.Dialect{})
		id := blobfs.NewID()
		f, err := s.BeginFileDelete(ctx, db, id)
		if err != nil || f.Version != 7 || f.Status != blobfs.StatusDeleting {
			t.Errorf("BeginFileDelete = %+v, %v, want the wrapper's row", f, err)
		}
		if !slices.Equal(v.calls, []string{id}) {
			t.Errorf("the wrapper saw %v, want %v", v.calls, []string{id})
		}
		if s.Serializes() {
			t.Error("Serializes = true, want the baseline's false through the wrapper")
		}
		if err := s.LockTree(ctx, db); !errors.Is(err, query.ErrTransactionRequired) {
			t.Errorf("LockTree on the pool = %v, want the baseline's refusal through the wrapper", err)
		}
		if calls := rec.Calls(); len(calls) != 0 {
			t.Errorf("the driver saw %v, want nothing: the override ran no SQL and the baseline's lock ran none", calls)
		}
		if n := len(s.Statements()); n != 20 {
			t.Errorf("Statements() lists %d, want the persistence package's 20: the wrapper compiled nothing of its own", n)
		}
	})

	t.Run("LockOverridden", func(t *testing.T) {
		v := &lockOverride{Variant: base}
		s, err := data.New(catalog(t), sqltest.Dialect{}, data.WithVariant(v))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		pool, rec := sqltest.Open(t)
		db := sqlate.Wrap(pool, sqltest.Dialect{})
		if !s.Serializes() {
			t.Error("Serializes = false, want the wrapper's true")
		}
		if err := s.LockTree(ctx, db); err != nil || v.locked != 1 {
			t.Errorf("LockTree = %v with %d wrapper calls, want nil and one call", err, v.locked)
		}
		id := blobfs.NewID()
		rec.Queue(
			sqltest.Response{Affected: 1},
			sqltest.Response{Columns: fileColumns, Rows: [][]driver.Value{deletingRow(id, "a.txt")}},
		)
		f, err := db.Transact(ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
			return s.BeginFileDelete(ctx, tx, id)
		})
		if err != nil || f.ID != id || f.Status != blobfs.StatusDeleting {
			t.Errorf("BeginFileDelete = %+v, %v, want the baseline's row through the wrapper", f, err)
		}
		if ops := rec.Ops(); !slices.Equal(ops, []sqltest.Op{sqltest.OpBegin, sqltest.OpExec, sqltest.OpQuery, sqltest.OpCommit}) {
			t.Errorf("ops = %v, want the baseline's begin, exec, query, commit", ops)
		}
	})
}
