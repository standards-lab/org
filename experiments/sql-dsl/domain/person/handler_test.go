package person_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/standards-lab/go-web-sdk"
	"github.com/standards-lab/org/experiments/sql-dsl/domain/person"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/drivertest"
)

func router(t *testing.T, responses ...drivertest.Response) http.Handler {
	t.Helper()
	svc, _ := service(t, responses...)
	r := web.NewRouter()
	r.Mount(web.NewModule(person.Routes(svc, web.Limits{DefaultSize: 20, MaxSize: 100})))
	return r
}

func send(h http.Handler, method, path, ifMatch, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	if ifMatch != "" {
		r.Header.Set("If-Match", ifMatch)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestRoutes_RejectBeforeAnyOperation(t *testing.T) {
	cases := map[string]struct {
		method, path, ifMatch, body string
		status                      int
		detail                      string
	}{
		"unknown filter":      {"GET", "/people?nickname=x", "", "", 400, "unknown filter field"},
		"malformed id":        {"GET", "/people/nope", "", "", 400, "id must be a UUID"},
		"bad email":           {"POST", "/people", "", `{"unit_id":"` + unitID + `","given_name":"A","family_name":"B","email":"nope"}`, 400, "email"},
		"bad unit":            {"POST", "/people", "", `{"unit_id":"x","given_name":"A","family_name":"B","email":"a@b"}`, 400, "unit_id"},
		"empty name":          {"POST", "/people", "", `{"unit_id":"` + unitID + `","given_name":"","family_name":"B","email":"a@b"}`, 400, "given_name"},
		"activate no version": {"POST", "/people/" + validID + "/activate", "", "", 428, ""},
		"transfer bad unit":   {"POST", "/people/" + validID + "/transfer-unit", `"1"`, `{"unit_id":"x"}`, 400, "unit_id"},
		"delete no version":   {"DELETE", "/people/" + validID, "", "", 428, ""},
	}
	h := router(t)
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			rec := send(h, c.method, c.path, c.ifMatch, c.body)
			if rec.Code != c.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, c.status, rec.Body)
			}
			var body map[string]any
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			detail, _ := body["detail"].(string)
			if c.detail != "" && !strings.Contains(detail, c.detail) {
				t.Errorf("detail = %q, want %q", detail, c.detail)
			}
		})
	}
}

func TestRoutes_TransitionRefusalIs409(t *testing.T) {
	h := router(t, state("active", 1))
	rec := send(h, "POST", "/people/"+validID+"/activate", `"1"`, "")
	if rec.Code != http.StatusConflict {
		t.Errorf("activate active = %d: %s", rec.Code, rec.Body)
	}
}
