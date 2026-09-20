//go:build compose

// Package livetest is the helper the compose-backed tests share: it gives
// each test its own throwaway database on the server BLOBFS_DSN names, so no
// test depends on the state of the compose stack's database or on another
// test, and drops the database when the test ends.
package livetest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/postgres"
)

// Open creates a uniquely named database on the server BLOBFS_DSN names and
// returns the session over a pool connected to it. The pool is closed and
// the database dropped, with FORCE so open sessions do not block the drop,
// when the test ends. A missing BLOBFS_DSN fails the test: the compose tag
// states that the stack is expected.
func Open(t testing.TB) *sqlate.DB {
	t.Helper()
	db, _ := OpenDSN(t)
	return db
}

// OpenDSN is Open, also returning the throwaway database's DSN, for a test
// that hands the DSN to code that opens its own pool.
func OpenDSN(t testing.TB) (*sqlate.DB, string) {
	t.Helper()
	dsn := os.Getenv("BLOBFS_DSN")
	if dsn == "" {
		t.Fatal("BLOBFS_DSN is not set; run under mise with the compose stack up")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open %s: %v", dsn, err)
	}
	name := "blobfs_test_" + suffix(t)
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		_ = admin.Close()
		t.Fatalf("create database %s: %v", name, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("drop database %s: %v", name, err)
		}
		_ = admin.Close()
	})

	testDSN := databaseDSN(t, dsn, name)
	pool, err := sql.Open("pgx", testDSN)
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	if err := pool.PingContext(ctx); err != nil {
		t.Fatalf("ping %s: %v", name, err)
	}
	return sqlate.Wrap(pool, postgres.Dialect{}), testDSN
}

// suffix returns eight random hex characters, so parallel packages never
// pick the same database name.
func suffix(t testing.TB) string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("random suffix: %v", err)
	}
	return hex.EncodeToString(b[:])
}

// databaseDSN returns dsn with its database replaced by name.
func databaseDSN(t testing.TB, dsn, name string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse %s: %v", dsn, err)
	}
	u.Path = "/" + name
	return u.String()
}

// Constraint returns the constraint name a classified violation carries,
// or fails the test when err is not a sqlate.ConstraintError of class.
func Constraint(t testing.TB, err error, class error) string {
	t.Helper()
	var ce *sqlate.ConstraintError
	if !errors.As(err, &ce) {
		t.Fatalf("error %v is not a sqlate.ConstraintError", err)
	}
	if !errors.Is(ce.Class, class) {
		t.Fatalf("error %v has class %v, want %v", err, ce.Class, class)
	}
	return ce.Constraint
}

// Exists reports whether the relation named (a table or an index) exists in
// the connected database.
func Exists(ctx context.Context, t testing.TB, db *sqlate.DB, relation string) bool {
	t.Helper()
	rows, err := db.QueryContext(ctx, "SELECT to_regclass($1) IS NOT NULL", relation)
	if err != nil {
		t.Fatalf("to_regclass(%s): %v", relation, err)
	}
	defer func() { _ = rows.Close() }()
	var exists bool
	if !rows.Next() {
		t.Fatalf("to_regclass(%s): no row: %v", relation, rows.Err())
	}
	if err := rows.Scan(&exists); err != nil {
		t.Fatalf("to_regclass(%s): %v", relation, err)
	}
	return exists
}

// RowExists reports whether query, with args bound, returns at least one
// row.
func RowExists(ctx context.Context, t testing.TB, db *sqlate.DB, query string, args ...any) bool {
	t.Helper()
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer func() { _ = rows.Close() }()
	found := rows.Next()
	if err := rows.Err(); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return found
}

// Head returns the highest applied version in the history table named, or
// zero when the table has no rows.
func Head(ctx context.Context, t testing.TB, db *sqlate.DB, table string) int {
	t.Helper()
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT COALESCE(MAX(version), 0) FROM %s", table))
	if err != nil {
		t.Fatalf("head of %s: %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	var head int
	if !rows.Next() {
		t.Fatalf("head of %s: no row: %v", table, rows.Err())
	}
	if err := rows.Scan(&head); err != nil {
		t.Fatalf("head of %s: %v", table, err)
	}
	return head
}
