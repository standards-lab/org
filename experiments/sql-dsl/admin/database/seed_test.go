package database_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/standards-lab/org/experiments/sql-dsl/admin/database"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/data"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/drivertest"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/pgdialect"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/query"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

const seedOrganizations, seedPeople = 7, 6

// seedScript is one seed run's responses over the driver fake: for each
// organization the insert's RETURNING row on a fresh database, or no row
// and then the find on a seeded one; for each person the insert's count.
func seedScript(fresh bool) []drivertest.Response {
	var rs []drivertest.Response
	for i := range seedOrganizations {
		id := fmt.Sprintf("o%d", i+1)
		if fresh {
			rs = append(rs, drivertest.Response{Columns: []string{"id"}, Rows: [][]driver.Value{{id}}})
		} else {
			rs = append(rs, drivertest.Response{Columns: []string{"id"}},
				drivertest.Response{Columns: []string{"id"}, Rows: [][]driver.Value{{id}}})
		}
	}
	for range seedPeople {
		if fresh {
			rs = append(rs, drivertest.Response{Affected: 1})
		} else {
			rs = append(rs, drivertest.Response{})
		}
	}
	return rs
}

func newSeeding(t *testing.T, responses ...drivertest.Response) (*database.Service, *drivertest.Recorder) {
	t.Helper()
	pool, rec := drivertest.Open(t, responses...)
	db := data.New(sqldb.Wrap(pool, pgdialect.Wrap(drivertest.Dialect{})), query.MustCatalog(query.Patterns(), data.Patterns()))
	s, err := database.New(started(t, pool), db, slog.New(slog.DiscardHandler), database.Options{Seed: true})
	if err != nil {
		t.Fatal(err)
	}
	return s, rec
}

// Seed loads both files in one transaction, parents before children with
// the parent's id bound, people bound to their unit's id; a second run over
// a seeded database finds every row and inserts nothing.
func TestSeed_InsertsInOrderThenIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s, rec := newSeeding(t, seedScript(true)...)
	n, err := s.Seed(ctx)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if n != (data.Seeded{Organizations: seedOrganizations, People: seedPeople}) {
		t.Errorf("seeded = %+v", n)
	}
	ops := rec.Ops()
	if ops[0] != drivertest.OpBegin || ops[len(ops)-1] != drivertest.OpCommit || len(ops) != 2+seedOrganizations+seedPeople {
		t.Errorf("ops = %v", ops)
	}
	calls := rec.Calls()
	if calls[1].Args[0] != nil || calls[1].Args[1] != "acme" {
		t.Errorf("root insert = %+v, want a nil parent", calls[1])
	}
	if calls[2].Args[0] != "o1" || calls[2].Args[1] != "engineering" {
		t.Errorf("child insert = %+v, want the parent's id bound", calls[2])
	}
	if person := calls[1+seedOrganizations]; person.Args[0] != "o3" || person.Args[3] != "ada@acme.example" {
		t.Errorf("person insert = %+v, want the platform unit's id bound", person)
	}
	if rec.Pending() != 0 || rec.RowsLeaked() != 0 {
		t.Errorf("pending = %d, leaked = %d", rec.Pending(), rec.RowsLeaked())
	}

	rec.Queue(seedScript(false)...)
	n, err = s.Seed(ctx)
	if err != nil || n != (data.Seeded{}) {
		t.Errorf("second Seed = %+v, %v, want zeros", n, err)
	}
	if !slices.ContainsFunc(rec.SQL(drivertest.OpQuery), func(q string) bool {
		return strings.Contains(q, "WHERE parent_id IS NOT DISTINCT FROM CAST($1 AS uuid) AND code = $2")
	}) {
		t.Error("an existing organization was not looked up through the authored statement")
	}
}

// A seed file that names an unseeded parent rolls the run back.
func TestSeed_FailureRollsBack(t *testing.T) {
	s, rec := newSeeding(t, drivertest.Response{Err: errors.New("engine says no")})
	if _, err := s.Seed(context.Background()); err == nil || !strings.Contains(err.Error(), "seed organization acme") {
		t.Fatalf("Seed = %v, want the failing row named", err)
	}
	if ops := rec.Ops(); !slices.Equal(ops, []drivertest.Op{drivertest.OpBegin, drivertest.OpQuery, drivertest.OpRollback}) {
		t.Errorf("ops = %v", ops)
	}
}

// Without the switch, Seed refuses before any I/O and the endpoint answers
// 403 with the reason.
func TestSeed_DisabledRefusesAnd403(t *testing.T) {
	s, rec := newService(t)
	if _, err := s.Seed(context.Background()); !errors.Is(err, database.ErrSeedDisabled) {
		t.Fatalf("Seed = %v, want ErrSeedDisabled", err)
	}
	rr := httptest.NewRecorder()
	router(s).ServeHTTP(rr, httptest.NewRequest("POST", "/admin/database/seed", nil))
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "seeding is disabled") {
		t.Errorf("POST seed = %d: %s", rr.Code, rr.Body)
	}
	if len(rec.Calls()) != 0 {
		t.Errorf("a disabled seed reached the database: %v", rec.Ops())
	}
}

// With the switch, Start seeds once the schema is current, and the endpoint
// reports what a run inserted.
func TestStart_SeedsWhenEnabled(t *testing.T) {
	responses := append([]drivertest.Response{exists(true), applied(), exists(true), head(3, false)}, seedScript(true)...)
	s, rec := newSeeding(t, responses...)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !s.Ready() || rec.Pending() != 0 {
		t.Errorf("ready = %v, pending = %d", s.Ready(), rec.Pending())
	}

	rec.Queue(seedScript(false)...)
	rr := httptest.NewRecorder()
	router(s).ServeHTTP(rr, httptest.NewRequest("POST", "/admin/database/seed", nil))
	if rr.Code != http.StatusOK || rr.Body.String() != "{\"organizations\":0,\"people\":0}\n" {
		t.Errorf("POST seed = %d: %q", rr.Code, rr.Body)
	}
}
