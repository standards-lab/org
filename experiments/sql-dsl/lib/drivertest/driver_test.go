package drivertest_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/standards-lab/org/experiments/sql-dsl/lib/drivertest"
)

func TestOpen_RecordsCallsAndServesScriptedRows(t *testing.T) {
	ctx := context.Background()
	pool, rec := drivertest.Open(t,
		drivertest.Response{Affected: 3},
		drivertest.Response{Columns: []string{"n"}, Rows: [][]driver.Value{{int64(7)}}},
	)

	res, err := pool.ExecContext(ctx, "UPDATE t SET a = $1", "x")
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if n, _ := res.RowsAffected(); n != 3 {
		t.Errorf("affected = %d, want 3", n)
	}

	var n int64
	if err := pool.QueryRowContext(ctx, "SELECT n", true).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 7 {
		t.Errorf("n = %d, want 7", n)
	}

	calls := rec.Calls()
	if len(calls) != 2 || calls[0].Op != drivertest.OpExec || calls[1].Op != drivertest.OpQuery {
		t.Fatalf("calls = %+v", calls)
	}
	if calls[0].Args[0] != "x" || calls[1].Args[0] != true {
		t.Errorf("args recorded unconverted: %+v", calls)
	}
	if rec.Pending() != 0 || rec.RowsLeaked() != 0 {
		t.Errorf("pending = %d, leaked = %d", rec.Pending(), rec.RowsLeaked())
	}
}

func TestOpen_PrepareRecordsTextAndRunsThroughTheStatement(t *testing.T) {
	ctx := context.Background()
	pool, rec := drivertest.Open(t, drivertest.Response{Affected: 1})

	st, err := pool.PrepareContext(ctx, "DELETE FROM t WHERE id = $1")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	defer func() { _ = st.Close() }()
	if _, err := st.ExecContext(ctx, 42); err != nil {
		t.Fatalf("stmt exec: %v", err)
	}

	if got := rec.SQL(drivertest.OpPrepare); len(got) != 1 || got[0] != "DELETE FROM t WHERE id = $1" {
		t.Errorf("prepared = %v", got)
	}
	if got := rec.SQL(drivertest.OpExec); len(got) != 1 || got[0] != "DELETE FROM t WHERE id = $1" {
		t.Errorf("exec via stmt = %v", got)
	}
}

func TestOpen_PrepareFailureIsScriptable(t *testing.T) {
	pool, rec := drivertest.Open(t)
	boom := errors.New("column does not exist")
	rec.FailPrepare = func(q string) error {
		if q == "bad" {
			return boom
		}
		return nil
	}
	if _, err := pool.PrepareContext(context.Background(), "good"); err != nil {
		t.Errorf("good prepare failed: %v", err)
	}
	if _, err := pool.PrepareContext(context.Background(), "bad"); !errors.Is(err, boom) {
		t.Errorf("bad prepare err = %v, want %v", err, boom)
	}
}

func TestOpen_BeginRecordsOptions(t *testing.T) {
	pool, rec := drivertest.Open(t)
	tx, err := pool.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	calls := rec.Calls()
	if calls[0].Op != drivertest.OpBegin || calls[1].Op != drivertest.OpCommit {
		t.Fatalf("ops = %v", rec.Ops())
	}
	if calls[0].TxOptions.Isolation != driver.IsolationLevel(sql.LevelSerializable) || !calls[0].TxOptions.ReadOnly {
		t.Errorf("tx options = %+v", calls[0].TxOptions)
	}
}

func TestDB_IsStartedAndMapsThroughTheStubDialect(t *testing.T) {
	db, _ := drivertest.DB(t)
	if !db.Ready() {
		t.Fatal("DB not ready after Start")
	}
	err := db.Dialect().MapError(errors.New("x"))
	var m *drivertest.MappedError
	if !errors.As(err, &m) {
		t.Errorf("MapError did not wrap: %v", err)
	}
	if again := db.Dialect().MapError(err); again != err {
		t.Error("MapError re-wrapped an already mapped error")
	}
}
