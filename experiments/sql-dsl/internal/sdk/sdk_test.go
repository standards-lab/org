package sdk_test

import (
	"errors"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/standards-lab/go-web-sdk"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/sdk"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/query"
)

func ifMatch(t *testing.T, header string) (int64, error) {
	t.Helper()
	r := httptest.NewRequest("PATCH", "/", nil)
	if header != "" {
		r.Header.Set("If-Match", header)
	}
	return sdk.IfMatch(r)
}

func TestIfMatch(t *testing.T) {
	for header, want := range map[string]int64{`"3"`: 3, `"0"`: 0, ` "7" `: 7} {
		if got, err := ifMatch(t, header); err != nil || got != want {
			t.Errorf("IfMatch(%q) = %d, %v, want %d", header, got, err, want)
		}
	}
	var pre *sdk.PreconditionError
	if _, err := ifMatch(t, ""); !errors.As(err, &pre) || !pre.Missing {
		t.Errorf("missing = %v", err)
	}
	for _, header := range []string{`3`, `*`, `W/"3"`, `""`, `"abc"`, `"1", "2"`} {
		if _, err := ifMatch(t, header); !errors.As(err, &pre) || pre.Missing {
			t.Errorf("IfMatch(%q) = %v, want a malformed PreconditionError", header, err)
		}
	}
}

func TestDirectives_LowersTheParsedQuery(t *testing.T) {
	q, err := web.ParseQuery(url.Values{"page": {"2"}, "size": {"5"}, "sort": {"-name,code"}, "path": {"/acme"}, "code": {"x"}}, web.Limits{DefaultSize: 20, MaxSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	d := sdk.Directives(q)
	if d.Page != (query.Page{Number: 2, Size: 5}) || len(d.Sort) != 2 || !d.Sort[0].Descending || d.Sort[1].Field != "code" {
		t.Errorf("page/sort = %+v", d)
	}
	if len(d.Filters) != 2 || d.Filters[0].Field != "code" || d.Filters[1] != (query.Filter{Field: "path", Op: query.OpEq, Value: "/acme"}) {
		t.Errorf("filters = %+v, want name order, exact match, text values", d.Filters)
	}
}
