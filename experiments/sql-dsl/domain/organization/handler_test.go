package organization_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/standards-lab/go-web-sdk"
	"github.com/standards-lab/org/experiments/sql-dsl/domain/organization"
)

// module compiles the layer's route group the way the composition root
// does, over the driver fake with nothing scripted: these are the
// handler-local rejection paths, which answer before any operation runs.
func module(t *testing.T) http.Handler {
	t.Helper()
	svc, _ := service(t)
	r := web.NewRouter()
	r.Mount(web.NewModule(organization.Routes(svc, web.Limits{DefaultSize: 20, MaxSize: 100})))
	return r
}

func send(t *testing.T, method, path, ifMatch, body string) *httptest.ResponseRecorder {
	t.Helper()
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
	module(t).ServeHTTP(rec, r)
	return rec
}

func problem(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) map[string]any {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, wantStatus, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != web.ProblemMediaType {
		t.Errorf("Content-Type = %q, want %q", ct, web.ProblemMediaType)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	return body
}

func TestRoutes_RejectBeforeAnyOperation(t *testing.T) {
	cases := map[string]struct {
		method, path, ifMatch, body string
		status                      int
		detail                      string
	}{
		"malformed page":       {"GET", "/organizations?page=x", "", "", 400, ""},
		"oversized page":       {"GET", "/organizations?size=1000", "", "", 400, ""},
		"unknown filter field": {"GET", "/organizations?email=x", "", "", 400, "unknown filter field"},
		"unknown sort field":   {"GET", "/organizations?sort=email", "", "", 400, "unknown sort field"},
		"malformed id":         {"GET", "/organizations/not-a-uuid", "", "", 400, "id must be a UUID"},
		"malformed body":       {"POST", "/organizations", "", "{", 400, ""},
		"unknown body field":   {"POST", "/organizations", "", `{"codex":"a"}`, 400, ""},
		"bad code":             {"POST", "/organizations", "", `{"code":"Bad_Code","name":"X"}`, 400, "code"},
		"empty name":           {"POST", "/organizations", "", `{"code":"ok","name":""}`, 400, "name"},
		"bad parent":           {"POST", "/organizations", "", `{"parent_id":"nope","code":"ok","name":"X"}`, 400, "parent_id"},
		"edit without version": {"PATCH", "/organizations/" + validID, "", `{"code":"ok","name":"X"}`, 428, ""},
		"transfer no version":  {"POST", "/organizations/" + validID + "/transfer", "", `{}`, 428, ""},
		"delete no version":    {"DELETE", "/organizations/" + validID, "", "", 428, ""},
		"weak if-match":        {"PATCH", "/organizations/" + validID, `W/"3"`, `{"code":"ok","name":"X"}`, 400, "If-Match"},
		"transfer bad parent":  {"POST", "/organizations/" + validID + "/transfer", `"1"`, `{"parent_id":"nope"}`, 400, "parent_id"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			body := problem(t, send(t, c.method, c.path, c.ifMatch, c.body), c.status)
			detail, _ := body["detail"].(string)
			if c.detail != "" && !strings.Contains(detail, c.detail) {
				t.Errorf("detail = %q, want it to mention %q", detail, c.detail)
			}
			if c.status == 428 && detail != "" {
				t.Errorf("428 carries detail %q; only a 400 carries detail", detail)
			}
		})
	}
}
