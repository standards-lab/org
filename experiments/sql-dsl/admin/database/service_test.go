package database_test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/standards-lab/go-web-sdk"
	"github.com/standards-lab/org/experiments/sql-dsl/admin/database"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/drivertest"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/migrate"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/pgdialect"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

var (
	historyCols = []string{"version", "name", "dirty"}
	headCols    = []string{"version", "dirty"}
	locked      = drivertest.Response{}
	created     = drivertest.Response{}
	unlocked    = drivertest.Response{Columns: []string{"pg_advisory_unlock"}, Rows: [][]driver.Value{{true}}}
)

func exists(yes bool) drivertest.Response {
	n := int64(0)
	if yes {
		n = 1
	}
	return drivertest.Response{Columns: []string{"count"}, Rows: [][]driver.Value{{n}}}
}

func history(rows ...[]driver.Value) drivertest.Response {
	return drivertest.Response{Columns: historyCols, Rows: rows}
}

func head(version int64, dirty bool) drivertest.Response {
	return drivertest.Response{Columns: headCols, Rows: [][]driver.Value{{version, dirty}}}
}

// applied is the history of the whole embedded set, clean.
func applied() drivertest.Response {
	var rows [][]driver.Value
	for _, m := range database.Migrations() {
		rows = append(rows, []driver.Value{int64(m.Version), m.Name, false})
	}
	return history(rows...)
}

func newService(t *testing.T, responses ...drivertest.Response) (*database.Service, *drivertest.Recorder) {
	t.Helper()
	base, rec := drivertest.DB(t, responses...)
	db := sqldb.Wrap(base, pgdialect.Wrap(base.Dialect()))
	s, err := database.New(db, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, rec
}

// The embedded set parses, orders, and declares its transaction modes as
// authored: the index build runs outside a transaction.
func TestMigrations_EmbeddedSetParses(t *testing.T) {
	ms := database.Migrations()
	if len(ms) != 3 {
		t.Fatalf("len = %d, want 3", len(ms))
	}
	want := []struct {
		version       int
		name          string
		transactional bool
	}{{1, "organization", true}, {2, "person", true}, {3, "person_unit_index", false}}
	for i, w := range want {
		if ms[i].Version != w.version || ms[i].Name != w.name || ms[i].Transactional != w.transactional {
			t.Errorf("[%d] = %d %s tx=%v, want %+v", i, ms[i].Version, ms[i].Name, ms[i].Transactional, w)
		}
		if ms[i].Down == "" {
			t.Errorf("[%d] has no down", i)
		}
	}
}

// Start on a complete, clean history verifies and reports ready without
// taking the lock or writing.
func TestStart_CleanHistoryIsReady(t *testing.T) {
	s, rec := newService(t, exists(true), applied(), exists(true), head(3, false))
	if s.Ready() {
		t.Fatal("ready before Start")
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !s.Ready() {
		t.Error("not ready after a clean Start")
	}
	if len(rec.SQL(drivertest.OpExec)) != 0 {
		t.Errorf("Start wrote: %v", rec.SQL(drivertest.OpExec))
	}
}

// Start on an empty database applies the whole set under the lock, then
// verifies.
func TestStart_PendingHistoryIsApplied(t *testing.T) {
	ms := database.Migrations()
	responses := []drivertest.Response{exists(false), locked, created, history()}
	for _, m := range ms {
		if m.Transactional {
			responses = append(responses, drivertest.Response{}, drivertest.Response{}) // up, insert
		} else {
			responses = append(responses, drivertest.Response{}, drivertest.Response{}, drivertest.Response{}) // dirty, up, clean
		}
	}
	responses = append(responses, unlocked, exists(true), applied(), exists(true), head(3, false))
	s, rec := newService(t, responses...)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !s.Ready() || rec.Pending() != 0 {
		t.Errorf("ready = %v, pending responses = %d", s.Ready(), rec.Pending())
	}
	ups := rec.SQL(drivertest.OpExec)
	if !strings.HasPrefix(ups[0], "SELECT pg_advisory_lock") {
		t.Errorf("first exec = %q, want the lock", ups[0])
	}
}

// A dirty history is a state startup cannot correct: Start fails, the
// service stays not-ready, and nothing runs against the schema.
func TestStart_DirtyHistoryFailsStartup(t *testing.T) {
	s, rec := newService(t, exists(true),
		history([]driver.Value{int64(1), "organization", false}, []driver.Value{int64(2), "person", true}))
	err := s.Start(context.Background())
	if !errors.Is(err, migrate.ErrDirty) {
		t.Fatalf("Start = %v, want ErrDirty", err)
	}
	if s.Ready() || len(rec.SQL(drivertest.OpExec)) != 0 {
		t.Error("a dirty schema reported ready or was written to")
	}
}

func router(s *database.Service) http.Handler {
	r := web.NewRouter()
	adminMount := web.NewGroup("/admin")
	adminMount.Mount(database.Routes(s))
	r.Mount(web.NewModule(adminMount))
	return r
}

func TestRoutes_SchemaStatusAndVerifyConflict(t *testing.T) {
	s, _ := newService(t,
		exists(true), head(1, false), exists(true), history([]driver.Value{int64(1), "organization", false}), // status
		exists(true), history([]driver.Value{int64(1), "organization", false}), // verify
	)
	h := router(s)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/admin/database/schema", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET schema = %d: %s", rr.Code, rr.Body)
	}
	var st database.Status
	if err := json.Unmarshal(rr.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.Version != 1 || st.Ready || len(st.Pending) != 2 || len(st.Migrations) != 3 || !st.Migrations[0].Applied || st.Migrations[1].Applied {
		t.Errorf("status = %+v", st)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("POST", "/admin/database/schema/verify", nil))
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Header().Get("Content-Type"), "problem+json") {
		t.Errorf("POST verify on a pending schema = %d %s: %s", rr.Code, rr.Header().Get("Content-Type"), rr.Body)
	}
	if !strings.Contains(rr.Body.String(), "pending: [2 3]") {
		t.Errorf("conflict problem carries no detail for the operator: %s", rr.Body)
	}
}

func TestRoutes_BodiesAreValidatedBeforeAnyIO(t *testing.T) {
	s, rec := newService(t)
	h := router(s)
	cases := []struct{ path, body string }{
		{"/admin/database/schema/steps", `{"steps": 0}`},
		{"/admin/database/schema/steps", `{"steps": "two"}`},
		{"/admin/database/schema/steps", `{"stepz": 1}`},
		{"/admin/database/schema/down", `{"steps": -1}`},
		{"/admin/database/schema/force", `{"version": -1}`},
		{"/admin/database/schema/force", `nope`},
	}
	for _, c := range cases {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("POST", c.path, strings.NewReader(c.body)))
		if rr.Code != http.StatusBadRequest {
			t.Errorf("POST %s %s = %d, want 400", c.path, c.body, rr.Code)
		}
	}
	if len(rec.Calls()) != 0 {
		t.Errorf("rejected requests reached the database: %v", rec.Ops())
	}
}

func TestRoutes_ForceOutsideTheSetIs400(t *testing.T) {
	s, _ := newService(t)
	rr := httptest.NewRecorder()
	router(s).ServeHTTP(rr, httptest.NewRequest("POST", "/admin/database/schema/force", strings.NewReader(`{"version": 9}`)))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("force 9 = %d, want 400", rr.Code)
	}
}
