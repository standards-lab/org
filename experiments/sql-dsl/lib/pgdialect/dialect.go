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

// Lock takes the session-level advisory lock for name on conn, blocking
// until it is granted or ctx ends; the name enters the engine's 32-bit key
// space through hashtext, the same mapping a domain's transaction-scoped
// lock file uses. The lock belongs to the connection's session and outlives
// any transaction on it; Unlock on the same conn releases it.
func (Dialect) Lock(ctx context.Context, conn *sql.Conn, name string) error {
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock(hashtext($1))", name); err != nil {
		return fmt.Errorf("advisory lock %s: %w", name, err)
	}
	return nil
}

// Unlock releases the session-level advisory lock for name on conn.
func (Dialect) Unlock(ctx context.Context, conn *sql.Conn, name string) error {
	var released bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_advisory_unlock(hashtext($1))", name).Scan(&released); err != nil {
		return fmt.Errorf("advisory unlock %s: %w", name, err)
	}
	if !released {
		return fmt.Errorf("%w: %s", ErrLockNotHeld, name)
	}
	return nil
}
