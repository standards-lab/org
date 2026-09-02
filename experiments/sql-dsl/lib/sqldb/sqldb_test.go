package sqldb_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"slices"
	"testing"

	"github.com/standards-lab/go-database"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/drivertest"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

var errDriver = errors.New("driver says no")

func wrap(t *testing.T, responses ...drivertest.Response) (*sqldb.DB, *drivertest.Recorder) {
	t.Helper()
	base, rec := drivertest.DB(t, responses...)
	return sqldb.Wrap(base, base.Dialect()), rec
}

func mapped(t *testing.T, err error) {
	t.Helper()
	var m *drivertest.MappedError
	if !errors.As(err, &m) {
		t.Errorf("error did not cross the mapping boundary: %v", err)
	}
	if !errors.Is(err, errDriver) {
		t.Errorf("driver error not reachable: %v", err)
	}
}

func TestWrap_PanicsOnNil(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Wrap(nil, nil) did not panic")
		}
	}()
	sqldb.Wrap(nil, nil)
}

func TestDB_EveryMethodMaps(t *testing.T) {
	ctx := context.Background()
	db, rec := wrap(t,
		drivertest.Response{Err: errDriver}, // exec
		drivertest.Response{Err: errDriver}, // query
	)
	rec.FailPrepare = func(string) error { return errDriver }

	_, err := db.ExecContext(ctx, "UPDATE t SET a = 1")
	mapped(t, err)
	_, err = db.QueryContext(ctx, "SELECT 1")
	mapped(t, err)
	_, err = db.PrepareContext(ctx, "SELECT 1")
	mapped(t, err)
	if db.MapError(nil) != nil {
		t.Error("MapError(nil) != nil")
	}
	if db.Dialect().Name() != "test" || db.Base() == nil {
		t.Error("Dialect or Base not carried")
	}
}

func TestDB_SuccessPassesThrough(t *testing.T) {
	ctx := context.Background()
	db, _ := wrap(t,
		drivertest.Response{Affected: 2},
		drivertest.Response{Columns: []string{"a"}, Rows: [][]driver.Value{{"x"}}},
	)
	res, err := db.ExecContext(ctx, "UPDATE t SET a = 1")
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if n, _ := res.RowsAffected(); n != 2 {
		t.Errorf("affected = %d", n)
	}
	rows, err := db.QueryContext(ctx, "SELECT a FROM t")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var a string
	if !rows.Next() || rows.Scan(&a) != nil || a != "x" {
		t.Errorf("scan = %q", a)
	}
}

func TestTx_EveryMethodMapsIncludingCommit(t *testing.T) {
	ctx := context.Background()
	db, rec := wrap(t,
		drivertest.Response{Err: errDriver},
		drivertest.Response{Err: errDriver},
	)
	rec.FailPrepare = func(string) error { return errDriver }
	rec.FailCommit = errDriver

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	_, err = tx.ExecContext(ctx, "x")
	mapped(t, err)
	_, err = tx.QueryContext(ctx, "x")
	mapped(t, err)
	_, err = tx.PrepareContext(ctx, "x")
	mapped(t, err)
	mapped(t, tx.Commit())
}

func TestBegin_OptionsReachTheDriverAndFailureWrapsConnectionFailed(t *testing.T) {
	ctx := context.Background()
	db, rec := wrap(t)

	tx, err := db.Begin(ctx, sqldb.Isolation(sql.LevelSerializable), sqldb.ReadOnly())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	_ = tx.Rollback()
	begin := rec.Calls()[0]
	if begin.Op != drivertest.OpBegin ||
		begin.TxOptions.Isolation != driver.IsolationLevel(sql.LevelSerializable) ||
		!begin.TxOptions.ReadOnly {
		t.Errorf("begin recorded %+v", begin)
	}

	rec.FailBegin = errDriver
	_, err = db.Begin(ctx)
	if !errors.Is(err, database.ErrConnectionFailed) || !errors.Is(err, errDriver) {
		t.Errorf("begin failure = %v, want ErrConnectionFailed wrapping the driver error", err)
	}
}

func TestTransact_CommitsAndReturnsTheResult(t *testing.T) {
	db, rec := wrap(t, drivertest.Response{Affected: 1})
	n, err := sqldb.Transact(context.Background(), db, func(tx *sqldb.Tx) (int64, error) {
		res, err := tx.ExecContext(context.Background(), "UPDATE t SET a = 1")
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	})
	if err != nil || n != 1 {
		t.Fatalf("Transact = %d, %v", n, err)
	}
	want := []drivertest.Op{drivertest.OpBegin, drivertest.OpExec, drivertest.OpCommit}
	if got := rec.Ops(); !slices.Equal(got, want) {
		t.Errorf("ops = %v, want %v", got, want)
	}
}

func TestTransact_RollsBackOnErrorAndJoinsRollbackFailure(t *testing.T) {
	db, rec := wrap(t)
	unit := errors.New("unit failed")

	_, err := sqldb.Transact(context.Background(), db, func(*sqldb.Tx) (int, error) { return 0, unit })
	if !errors.Is(err, unit) {
		t.Fatalf("err = %v, want the unit's error", err)
	}
	if got := rec.Ops(); !slices.Equal(got, []drivertest.Op{drivertest.OpBegin, drivertest.OpRollback}) {
		t.Errorf("ops = %v", got)
	}

	rec.FailRollback = errDriver
	_, err = sqldb.Transact(context.Background(), db, func(*sqldb.Tx) (int, error) { return 0, unit })
	if !errors.Is(err, unit) || !errors.Is(err, errDriver) {
		t.Errorf("err = %v, want the unit error joined with the rollback error", err)
	}
}

func TestTransact_CommitFailureIsMapped(t *testing.T) {
	db, rec := wrap(t)
	rec.FailCommit = errDriver
	_, err := sqldb.Transact(context.Background(), db, func(*sqldb.Tx) (int, error) { return 1, nil })
	mapped(t, err)
}

func TestTransact_PanicRollsBackAndRepanics(t *testing.T) {
	db, rec := wrap(t)
	defer func() {
		if p := recover(); p != "boom" {
			t.Errorf("recovered %v, want the original panic", p)
		}
		if got := rec.Ops(); !slices.Equal(got, []drivertest.Op{drivertest.OpBegin, drivertest.OpRollback}) {
			t.Errorf("ops = %v, want begin then rollback", got)
		}
	}()
	_, _ = sqldb.Transact(context.Background(), db, func(*sqldb.Tx) (int, error) { panic("boom") })
}

func TestExecTx_VoidUnit(t *testing.T) {
	db, rec := wrap(t)
	err := sqldb.ExecTx(context.Background(), db, func(tx *sqldb.Tx) error {
		_, err := tx.ExecContext(context.Background(), "DELETE FROM t")
		return err
	}, sqldb.ReadOnly())
	if err != nil {
		t.Fatalf("ExecTx: %v", err)
	}
	if got := rec.Ops(); !slices.Equal(got, []drivertest.Op{drivertest.OpBegin, drivertest.OpExec, drivertest.OpCommit}) {
		t.Errorf("ops = %v", got)
	}
	if !rec.Calls()[0].TxOptions.ReadOnly {
		t.Error("ReadOnly option not applied")
	}
}

func TestConn_PinsAConnection(t *testing.T) {
	db, rec := wrap(t, drivertest.Response{Affected: 0})
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("Conn: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(context.Background(), "SELECT pg_advisory_lock(1)"); err != nil {
		t.Fatalf("exec on conn: %v", err)
	}
	if got := rec.SQL(drivertest.OpExec); len(got) != 1 {
		t.Errorf("exec recorded %v", got)
	}
}
