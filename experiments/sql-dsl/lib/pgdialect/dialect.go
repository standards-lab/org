// Package pgdialect adds to the postgres dialect go-database/postgres v0.2.0
// ships, by wrapping it: the lock capability (session-level advisory locks
// over plain SQL on a pinned connection) and the data-exception class in
// MapError. It imports no driver. At promotion the methods move into the
// postgres sub-module's dialect.
package pgdialect

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/standards-lab/go-database"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

// ErrLockNotHeld reports an Unlock of a key this session did not hold; the
// engine answers false rather than failing, so the wrapper makes it an error.
var ErrLockNotHeld = errors.New("advisory lock not held by this session")

// Dialect is the wrapped postgres dialect: everything the inner dialect does,
// plus sqldb.Locker and the class-22 mapping.
type Dialect struct {
	database.Dialect
}

var _ sqldb.Locker = Dialect{}

// sqlStater is what the driver's error exposes: pgx's *pgconn.PgError
// satisfies it at runtime. lib/ never names the driver (split-check), so
// the structural interface is how the mapping reads the SQLSTATE.
type sqlStater interface{ SQLState() string }

// MapError classifies a data exception (SQLSTATE class 22 — invalid text
// for a type, a value out of range, a bad datetime) as sqldb.ErrInvalidValue
// with the engine's message reachable, and delegates everything else to the
// inner dialect's constraint mapping.
func (d Dialect) MapError(err error) error {
	if err == nil {
		return nil
	}
	var st sqlStater
	if errors.As(err, &st) && strings.HasPrefix(st.SQLState(), "22") {
		return fmt.Errorf("%w: %w", sqldb.ErrInvalidValue, err)
	}
	return d.Dialect.MapError(err)
}

// Wrap adds the lock capability to inner, normally the postgres dialect a
// started DB reports.
func Wrap(inner database.Dialect) Dialect {
	if inner == nil {
		panic("pgdialect: Wrap requires a dialect")
	}
	return Dialect{Dialect: inner}
}

// Lock takes the session-level advisory lock for key on conn, blocking until
// it is granted or ctx ends. The lock belongs to the connection's session
// and outlives any transaction on it; Unlock on the same conn releases it.
func (Dialect) Lock(ctx context.Context, conn *sql.Conn, key int64) error {
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", key); err != nil {
		return fmt.Errorf("advisory lock %d: %w", key, err)
	}
	return nil
}

// Unlock releases the session-level advisory lock for key on conn.
func (Dialect) Unlock(ctx context.Context, conn *sql.Conn, key int64) error {
	var released bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_advisory_unlock($1)", key).Scan(&released); err != nil {
		return fmt.Errorf("advisory unlock %d: %w", key, err)
	}
	if !released {
		return fmt.Errorf("%w: key %d", ErrLockNotHeld, key)
	}
	return nil
}
