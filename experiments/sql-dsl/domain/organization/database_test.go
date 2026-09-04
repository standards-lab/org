package organization_test

import (
	"context"
	"database/sql/driver"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/go-web-sdk"
	"github.com/standards-lab/org/experiments/sql-dsl/domain/organization"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/data"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/drivertest"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/pgdialect"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/query"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

const (
	validID  = "00000000-0000-7000-8000-000000000000"
	parentID = "00000000-0000-7000-8000-000000000001"
)

// service builds the domain over the strict driver fake, the way the
// composition root builds it over the pool: statements loaded and handles
// bound at construction, no I/O.
func service(t *testing.T, responses ...drivertest.Response) (*organization.Service, *drivertest.Recorder) {
	t.Helper()
	pool, rec := drivertest.Open(t, responses...)
	return organization.New(data.New(sqldb.Wrap(pool, pgdialect.Wrap(drivertest.Dialect{})), query.MustCatalog(query.Patterns(), data.Patterns()))), rec
}

func identity(id string, version int64) drivertest.Response {
	return drivertest.Response{Columns: []string{"id", "version"}, Rows: [][]driver.Value{{id, version}}}
}

func row() drivertest.Response {
	now := time.Now()
	return drivertest.Response{
		Columns: []string{"id", "parent_id", "code", "name", "version", "created_at", "updated_at", "path"},
		Rows:    [][]driver.Value{{validID, nil, "acme", "Acme", int64(1), now, now, "/acme"}},
	}
}

// The wiring test: every handle binds once with sample arguments, so a key
// that does not match its file's parameters, an argument count the
// statement does not bind, or a scan out of step with the SELECT list fails
// here, in CI, rather than on a request. The strict driver checks the
// placeholder count on every call.
func TestStore_EveryHandleBindsItsFilesParameters(t *testing.T) {
	ctx := context.Background()
	s, rec := service(t,
		drivertest.Response{Columns: []string{"count"}, Rows: [][]driver.Value{{int64(1)}}}, row(), // list
		row(),                            // find by id
		identity(validID, 1),             // create
		drivertest.Response{Affected: 1}, // edit
		drivertest.Response{Affected: 0}, // transfer: lock
		drivertest.Response{Columns: []string{"count"}, Rows: [][]driver.Value{{int64(0)}}}, // in_subtree
		drivertest.Response{Affected: 1}, // transfer: update
		drivertest.Response{Affected: 1}, // delete
	)
	q, _ := web.ParseQuery(url.Values{"code": {"acme"}, "sort": {"-path"}}, web.Limits{DefaultSize: 20, MaxSize: 100})
	if items, total, err := s.List(ctx, q); err != nil || total != 1 || items[0].Path != "/acme" {
		t.Fatalf("List = %v, %d, %v", items, total, err)
	}
	if o, err := s.Find(ctx, validID); err != nil || o.Code != "acme" || o.ParentID != nil {
		t.Fatalf("Find = %+v, %v", o, err)
	}
	if id, err := s.Create(ctx, organization.CreateOrganization{Code: "eng", Name: "Eng"}); err != nil || id.Version != 1 {
		t.Fatalf("Create = %+v, %v", id, err)
	}
	if id, err := s.Edit(ctx, validID, 1, organization.EditOrganization{Code: "eng", Name: "Eng"}); err != nil || id.Version != 2 {
		t.Fatalf("Edit = %+v, %v", id, err)
	}
	p := parentID
	if id, err := s.Transfer(ctx, validID, 2, organization.TransferOrganization{ParentID: &p}); err != nil || id.Version != 3 {
		t.Fatalf("Transfer = %+v, %v", id, err)
	}
	if err := s.Delete(ctx, validID, 3); err != nil {
		t.Fatalf("Delete = %v", err)
	}
	if rec.Pending() != 0 || rec.RowsLeaked() != 0 {
		t.Errorf("pending = %d, leaked = %d", rec.Pending(), rec.RowsLeaked())
	}
	sqls := rec.SQL(drivertest.OpQuery)
	if !strings.HasPrefix(sqls[0], "SELECT COUNT(*) FROM (WITH RECURSIVE lineage") || !strings.Contains(sqls[1], "WHERE q.code = CAST($1 AS text) ORDER BY q.path DESC, q.id OFFSET $2") {
		t.Errorf("list = %q\n%q", sqls[0], sqls[1])
	}
	if lock := rec.Calls()[6]; lock.SQL != "SELECT pg_advisory_xact_lock(hashtext($1))" || lock.Args[0] != "organization.tree" {
		t.Errorf("transfer did not take the named tree lock first: %+v", lock)
	}
	ops := rec.Ops()
	if ops[len(ops)-1] != drivertest.OpExec || ops[len(ops)-2] != drivertest.OpCommit {
		t.Errorf("ops = %v, want the transfer committed and the delete last", ops)
	}
}

// Verify prepares the eight statements and the read contract's probe.
func TestStore_VerifyPreparesEveryStatement(t *testing.T) {
	s, rec := service(t)
	if err := s.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	prepared := rec.SQL(drivertest.OpPrepare)
	if len(prepared) != 9 {
		t.Errorf("prepared %d statements, want 8 + the contract probe", len(prepared))
	}
	if probe := prepared[len(prepared)-1]; !strings.HasPrefix(probe, "SELECT q.id, q.parent_id, q.code") {
		t.Errorf("probe = %q", probe)
	}
}
