package sqldb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/standards-lab/go-database"
)

// ErrInvalidValue classifies a data exception: a bound value the engine
// could not read as the type it was cast to (SQLSTATE class 22). It is the
// engine-side half of request validation — a filter value that is not a
// uuid, a date that is not a date — and a dialect maps its engine's form to
// it. At promotion it joins the root package's constraint classes.
var ErrInvalidValue = errors.New("invalid value")

// Session is the stdlib method set a runner needs, implemented by *DB and
// *Tx, so the same statement handle runs against the pool or inside a
// transaction. Every method maps driver errors through the dialect; the
// dialect itself is not on the interface, because runners never need it at
// request time.
type Session interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
}

// ErrorMapper is the capability both sessions expose for errors that arise
// after a call returns — rows.Err and Scan — which the seam cannot see. A
// runner type-asserts it on its Session.
type ErrorMapper interface {
	MapError(err error) error
}

// Locker is the dialect capability a concurrent-starter protocol takes: a
// session-scoped exclusive lock on a dedicated connection. A provider
// without it cannot serialize migrations across processes.
type Locker interface {
	Lock(ctx context.Context, conn *sql.Conn, key int64) error
	Unlock(ctx context.Context, conn *sql.Conn, key int64) error
}

// DB is the pool session over a started go-database DB. Lifecycle stays with
// the base; DB adds mapping, prepare, options on Begin, and pinned
// connections.
type DB struct {
	base    *database.DB
	pool    *sql.DB
	dialect database.Dialect
}

var (
	_ Session     = (*DB)(nil)
	_ Session     = (*Tx)(nil)
	_ ErrorMapper = (*DB)(nil)
	_ ErrorMapper = (*Tx)(nil)
)

// Wrap builds the session over base with dialect, which is normally
// base.Dialect() or a capability-adding wrapper of it. Nil arguments are a
// wiring defect and panic.
func Wrap(base *database.DB, dialect database.Dialect) *DB {
	if base == nil || dialect == nil {
		panic("sqldb: Wrap requires a base DB and a dialect")
	}
	return &DB{base: base, pool: base.Conn(), dialect: dialect}
}

// Base returns the lifecycle wrapper the session was built over.
func (d *DB) Base() *database.DB { return d.base }

// Dialect returns the dialect statements are loaded against.
func (d *DB) Dialect() database.Dialect { return d.dialect }

// MapError routes err through the dialect; nil stays nil.
func (d *DB) MapError(err error) error {
	if err == nil {
		return nil
	}
	return d.dialect.MapError(err)
}

// Conn pins one connection for a protocol that needs session scope — a
// session-level lock, a run of non-transactional DDL. The caller closes it.
func (d *DB) Conn(ctx context.Context) (*sql.Conn, error) {
	conn, err := d.pool.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", database.ErrConnectionFailed, err)
	}
	return conn, nil
}

func (d *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	res, err := d.pool.ExecContext(ctx, query, args...)
	return res, d.MapError(err)
}

func (d *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	rows, err := d.pool.QueryContext(ctx, query, args...)
	return rows, d.MapError(err)
}

func (d *DB) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	st, err := d.pool.PrepareContext(ctx, query)
	return st, d.MapError(err)
}
