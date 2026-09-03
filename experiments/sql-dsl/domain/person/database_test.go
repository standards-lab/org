package person_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/go-web-sdk"
	"github.com/standards-lab/org/experiments/sql-dsl/domain/person"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/data"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/drivertest"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/pgdialect"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/query"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

const (
	validID = "00000000-0000-7000-8000-000000000000"
	unitID  = "00000000-0000-7000-8000-000000000001"
)

func service(t *testing.T, responses ...drivertest.Response) (*person.Service, *drivertest.Recorder) {
	t.Helper()
	pool, rec := drivertest.Open(t, responses...)
	return person.New(data.New(sqldb.Wrap(pool, pgdialect.Wrap(drivertest.Dialect{})), query.MustCatalog(query.Patterns(), data.Patterns()))), rec
}

func identity(version int64) drivertest.Response {
	return drivertest.Response{Columns: []string{"id", "version"}, Rows: [][]driver.Value{{validID, version}}}
}

func state(status string, version int64) drivertest.Response {
	return drivertest.Response{Columns: []string{"status", "version"}, Rows: [][]driver.Value{{status, version}}}
}

func row() drivertest.Response {
	now := time.Now()
	return drivertest.Response{
		Columns: []string{"id", "unit_id", "given_name", "family_name", "email", "status", "version", "created_at", "updated_at"},
		Rows:    [][]driver.Value{{validID, unitID, "Ada", "Lovelace", "ada@acme.example", "pending", int64(1), now, now}},
	}
}

var affected = drivertest.Response{Affected: 1}

// The wiring test: every handle binds once over the strict driver, the
// entities' tags scanning and binding; each action reads state inside its
// transaction before the guard.
func TestStore_EveryHandleBindsItsFilesParameters(t *testing.T) {
	ctx := context.Background()
	s, rec := service(t,
		drivertest.Response{Columns: []string{"count"}, Rows: [][]driver.Value{{int64(1)}}}, row(), // list
		row(),                         // find
		identity(1),                   // create
		affected,                      // edit
		state("pending", 2), affected, // activate
		state("active", 3), affected, // deactivate
		state("inactive", 4), affected, // transfer-unit
		affected, // delete
	)
	q, _ := web.ParseQuery(url.Values{"status": {"pending"}, "sort": {"family_name"}}, web.Limits{DefaultSize: 20, MaxSize: 100})
	if items, total, err := s.List(ctx, q); err != nil || total != 1 || items[0].Status != person.StatusPending {
		t.Fatalf("List = %v, %d, %v", items, total, err)
	}
	if p, err := s.Find(ctx, validID); err != nil || p.Email != "ada@acme.example" {
		t.Fatalf("Find = %+v, %v", p, err)
	}
	if id, err := s.Create(ctx, person.CreatePerson{UnitID: unitID, GivenName: "A", FamilyName: "B", Email: "a@b"}); err != nil || id.Version != 1 {
		t.Fatalf("Create = %+v, %v", id, err)
	}
	if id, err := s.Edit(ctx, validID, 1, person.EditPerson{GivenName: "A", FamilyName: "B", Email: "a@b"}); err != nil || id.Version != 2 {
		t.Fatalf("Edit = %+v, %v", id, err)
	}
	if id, err := s.Activate(ctx, validID, 2); err != nil || id.Version != 3 {
		t.Fatalf("Activate = %+v, %v", id, err)
	}
	if id, err := s.Deactivate(ctx, validID, 3); err != nil || id.Version != 4 {
		t.Fatalf("Deactivate = %+v, %v", id, err)
	}
	if id, err := s.TransferUnit(ctx, validID, 4, person.TransferUnit{UnitID: unitID}); err != nil || id.Version != 5 {
		t.Fatalf("TransferUnit = %+v, %v", id, err)
	}
	if err := s.Delete(ctx, validID, 5); err != nil {
		t.Fatalf("Delete = %v", err)
	}
	if rec.Pending() != 0 || rec.RowsLeaked() != 0 {
		t.Errorf("pending = %d, leaked = %d", rec.Pending(), rec.RowsLeaked())
	}
	sqls := rec.SQL(drivertest.OpQuery)
	if sqls[0] != "SELECT COUNT(*) FROM (SELECT id, unit_id, given_name, family_name, email, status, version, created_at, updated_at\nFROM person) q WHERE q.status = CAST($1 AS text)" {
		t.Errorf("count = %q", sqls[0])
	}
	if !strings.Contains(sqls[1], "ORDER BY q.family_name, q.id OFFSET $2") {
		t.Errorf("page = %q", sqls[1])
	}
	calls := rec.Calls()
	if create := calls[3]; !strings.HasPrefix(create.SQL, "INSERT INTO person") || create.Args[0] != unitID || create.Args[3] != "a@b" {
		t.Errorf("create = %+v, want the command's fields bound by tag", create)
	}
	// activate: begin, state read, guarded update, commit
	if ops := rec.Ops()[5:9]; ops[0] != drivertest.OpBegin || ops[1] != drivertest.OpQuery || ops[2] != drivertest.OpExec || ops[3] != drivertest.OpCommit {
		t.Errorf("activate ops = %v, want begin, state, update, commit", ops)
	}
}

func TestStore_ActionsApplyTheTransitionRuleBeforeTheGuard(t *testing.T) {
	ctx := context.Background()
	s, rec := service(t,
		state("active", 1),  // activate an active person: refused
		state("pending", 1), // deactivate a pending person: refused
		state("active", 2),  // stale activate: the version read answers before the rule would
		drivertest.Response{Columns: []string{"status", "version"}}, // missing record
	)
	if _, err := s.Activate(ctx, validID, 1); !errors.Is(err, person.ErrTransition) {
		t.Errorf("activate active = %v, want ErrTransition", err)
	}
	if _, err := s.Deactivate(ctx, validID, 1); !errors.Is(err, person.ErrTransition) {
		t.Errorf("deactivate pending = %v, want ErrTransition", err)
	}
	if _, err := s.Activate(ctx, validID, 1); !errors.Is(err, query.ErrVersionMismatch) {
		t.Errorf("stale activate = %v, want ErrVersionMismatch before the rule", err)
	}
	if _, err := s.Activate(ctx, validID, 1); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("missing = %v, want sql.ErrNoRows", err)
	}
	if execs := rec.SQL(drivertest.OpExec); len(execs) != 0 {
		t.Errorf("a refused action reached the update: %v", execs)
	}
	if got := rec.Ops(); got[0] != drivertest.OpBegin || got[2] != drivertest.OpRollback {
		t.Errorf("refusal did not roll back: %v", got[:3])
	}
}

func TestStore_VerifyPreparesEveryStatement(t *testing.T) {
	s, rec := service(t)
	if err := s.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(rec.SQL(drivertest.OpPrepare)); n != 10 {
		t.Errorf("prepared %d, want 9 statements + the contract probe", n)
	}
}
