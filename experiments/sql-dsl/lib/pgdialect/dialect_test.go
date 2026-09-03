package pgdialect_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/standards-lab/org/experiments/sql-dsl/lib/drivertest"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/pgdialect"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

func TestWrap_KeepsTheInnerDialect(t *testing.T) {
	d := pgdialect.Wrap(drivertest.Dialect{})
	if d.Name() != "test" || d.Placeholder(3) != "$3" {
		t.Errorf("inner dialect not delegated: %s %s", d.Name(), d.Placeholder(3))
	}
	var m *drivertest.MappedError
	if !errors.As(d.MapError(errors.New("x")), &m) {
		t.Error("MapError not delegated")
	}
}

func TestLockUnlock_IssueTheAdvisoryCallsOnTheConnection(t *testing.T) {
	ctx := context.Background()
	pool, rec := drivertest.Open(t,
		drivertest.Response{Affected: 0},
		drivertest.Response{Columns: []string{"pg_advisory_unlock"}, Rows: [][]driver.Value{{true}}},
	)
	conn, err := pool.Conn(ctx)
	if err != nil {
		t.Fatalf("conn: %v", err)
	}
	defer func() { _ = conn.Close() }()

	d := pgdialect.Wrap(drivertest.Dialect{})
	if err := d.Lock(ctx, conn, 42); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if err := d.Unlock(ctx, conn, 42); err != nil {
		t.Fatalf("Unlock: %v", err)
	}

	calls := rec.Calls()
	if calls[0].SQL != "SELECT pg_advisory_lock($1)" || calls[0].Args[0] != int64(42) {
		t.Errorf("lock call = %+v", calls[0])
	}
	if calls[1].SQL != "SELECT pg_advisory_unlock($1)" || calls[1].Args[0] != int64(42) {
		t.Errorf("unlock call = %+v", calls[1])
	}
}

func TestUnlock_FalseIsErrLockNotHeld(t *testing.T) {
	ctx := context.Background()
	pool, _ := drivertest.Open(t,
		drivertest.Response{Columns: []string{"pg_advisory_unlock"}, Rows: [][]driver.Value{{false}}},
	)
	conn, err := pool.Conn(ctx)
	if err != nil {
		t.Fatalf("conn: %v", err)
	}
	defer func() { _ = conn.Close() }()

	err = pgdialect.Wrap(drivertest.Dialect{}).Unlock(ctx, conn, 7)
	if !errors.Is(err, pgdialect.ErrLockNotHeld) {
		t.Errorf("Unlock = %v, want ErrLockNotHeld", err)
	}
}

type pgError struct{ code string }

func (e pgError) Error() string    { return "engine: " + e.code }
func (e pgError) SQLState() string { return e.code }

func TestMapError_DataExceptionIsErrInvalidValue(t *testing.T) {
	d := pgdialect.Wrap(drivertest.Dialect{})
	err := d.MapError(pgError{"22P02"})
	if !errors.Is(err, sqldb.ErrInvalidValue) || !strings.Contains(err.Error(), "22P02") {
		t.Errorf("class 22 = %v, want ErrInvalidValue carrying the engine error", err)
	}
	var m *drivertest.MappedError
	if err := d.MapError(pgError{"23505"}); !errors.As(err, &m) {
		t.Errorf("other classes must reach the inner dialect: %v", err)
	}
	if d.MapError(nil) != nil {
		t.Error("nil maps to nil")
	}
}
