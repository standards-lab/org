//go:build compose

// Live proofs for the projection and the guard against the compose
// PostgreSQL: the engine parses request values through the contract's
// declared types, a value it cannot read is an InvalidValueError, and the
// guard's three outcomes hold against real rows. `mise run test-compose`.
package query_test

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/standards-lab/go-database"
	"github.com/standards-lab/go-database/postgres"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/pgdialect"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/query"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

// live opens a started session over the compose database, or skips.
func live(t testing.TB) *sqldb.DB {
	t.Helper()
	dsn := os.Getenv("SQLDSL_DSN")
	if dsn == "" {
		t.Skip("SQLDSL_DSN not set")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(u.Port())
	pw, _ := u.User.Password()
	cfg := database.Config{Host: u.Hostname(), Port: &port, User: u.User.Username(), Password: pw,
		Name: strings.TrimPrefix(u.Path, "/"), Options: map[string]string{}}
	for k, v := range u.Query() {
		cfg.Options[k] = v[0]
	}
	if err := cfg.Finalize(""); err != nil {
		t.Fatal(err)
	}
	base, err := postgres.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := base.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = base.Shutdown(context.Background()) })
	return sqldb.Wrap(base.Conn(), pgdialect.Wrap(base.Dialect()))
}

var liveFiles = fstest.MapFS{
	"sql/view.sql":    {Data: []byte("--| tier: standard\n--| key: id\n--| field: id uuid\n--| field: name text\n--| field: n integer\n--| field: at timestamp\nSELECT id, name, n, at FROM live_q")},
	"sql/edit.sql":    {Data: []byte("--| tier: standard\nUPDATE live_q SET name = {{name}}, version = version + 1 WHERE id = {{id}} AND version = {{version}}")},
	"sql/version.sql": {Data: []byte("--| tier: standard\nSELECT version FROM live_q WHERE id = {{id}}")},
}

type row struct {
	ID, Name string
	N        int64
	At       sql.NullTime
}

func scanRow(rows *sql.Rows) (row, error) {
	var r row
	err := rows.Scan(&r.ID, &r.Name, &r.N, &r.At)
	return r, err
}

func TestLive_ProjectionAndGuard(t *testing.T) {
	ctx := context.Background()
	db := live(t)
	_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS live_q")
	if _, err := db.ExecContext(ctx, "CREATE TABLE live_q (id uuid PRIMARY KEY DEFAULT uuidv7(), name text NOT NULL, n integer NOT NULL, at timestamp, version bigint NOT NULL DEFAULT 1)"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS live_q") })
	if _, err := db.ExecContext(ctx, "INSERT INTO live_q (name, n, at) VALUES ('a', 1, '2026-01-01'), ('b', 2, NULL), ('c', 3, '2026-03-01')"); err != nil {
		t.Fatal(err)
	}

	stmts := catalog().MustCompile(liveFiles, "sql", db.Dialect())
	view := stmts.Statement("view").Project(scanRow)
	if err := query.Verify(ctx, db, stmts, view); err != nil {
		t.Fatalf("Verify: %v", err)
	}

	// Request values arrive as text and the engine parses them by the
	// contract's types: "2" as integer, an RFC 3339 date as timestamp.
	items, total, err := view.List(ctx, db, query.Directives{
		Page:    query.Page{Number: 1, Size: 10},
		Sort:    []query.Sort{{Field: "n", Descending: true}},
		Filters: []query.Filter{{Field: "n", Op: query.OpGe, Value: "2"}, {Field: "at", Op: query.OpIsNotNull}},
	})
	if err != nil || total != 1 || len(items) != 1 || items[0].Name != "c" {
		t.Fatalf("List = %v, %d, %v", items, total, err)
	}
	items, total, err = view.List(ctx, db, query.Directives{
		Page:    query.Page{Number: 2, Size: 2},
		Filters: []query.Filter{{Field: "at", Op: query.OpLt, Value: "2026-02-01T00:00:00Z"}, {Field: "name", Op: query.OpIn, Value: []any{"a", "b", "c"}}},
	})
	if err != nil || total != 1 || len(items) != 0 {
		t.Fatalf("page past the end: List = %v, %d, %v", items, total, err)
	}

	// A value the engine cannot read as the field's type is the request's
	// fault, classified through the dialect's class-22 mapping.
	for _, f := range []query.Filter{
		{Field: "id", Op: query.OpEq, Value: "not-a-uuid"},
		{Field: "n", Op: query.OpGt, Value: "many"},
		{Field: "at", Op: query.OpGe, Value: "not-a-date"},
	} {
		_, _, err := view.List(ctx, db, query.Directives{Page: query.Page{Number: 1, Size: 1}, Filters: []query.Filter{f}})
		var invalid *query.InvalidValueError
		if !errors.As(err, &invalid) || !errors.Is(err, query.ErrDirectives) {
			t.Errorf("%s %v: err = %v, want InvalidValueError", f.Field, f.Value, err)
		} else {
			t.Logf("%s=%v → %v", f.Field, f.Value, err)
		}
	}

	one, err := view.One(ctx, db, "name", "b")
	if err != nil || one.N != 2 || one.At.Valid {
		t.Fatalf("One = %+v, %v", one, err)
	}
	if _, err := view.One(ctx, db, "name", "zzz"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("One miss = %v", err)
	}

	edit := stmts.Statement("edit").Guarded(stmts.Statement("version"), "version")
	v, err := edit.Run(ctx, db, 1, query.Args{"id": one.ID, "name": "B"})
	if err != nil || v != 2 {
		t.Fatalf("guard hit = %d, %v", v, err)
	}
	_, err = edit.Run(ctx, db, 1, query.Args{"id": one.ID, "name": "B2"})
	if !errors.Is(err, query.ErrVersionMismatch) || !strings.Contains(err.Error(), "expected 1, current 2") {
		t.Errorf("stale guard = %v", err)
	}
	if _, err := edit.Run(ctx, db, 1, query.Args{"id": "00000000-0000-0000-0000-000000000000", "name": "x"}); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("missing row = %v", err)
	}
	if got, _ := view.One(ctx, db, "id", one.ID); got.Name != "B" {
		t.Errorf("the hit did not persist: %+v", got)
	}
}
