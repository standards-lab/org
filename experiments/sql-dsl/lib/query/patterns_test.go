package query_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/standards-lab/org/experiments/sql-dsl/lib/drivertest"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/query"
)

// A domain file includes the guard patterns; the spliced text binds the
// pattern's parameters like the file's own.
func TestLoad_ExpandsPatternIncludes(t *testing.T) {
	st := load(t, "--| tier: standard\nUPDATE t SET a = {{a}}, {{> sql.guard_set}}\nWHERE {{> sql.guard_where}}")
	if st.Text() != "UPDATE t SET a = $1, updated_at = CURRENT_TIMESTAMP, version = version + 1\nWHERE id = $2 AND version = $3" {
		t.Errorf("text = %q", st.Text())
	}
	if got := st.Params(); !slices.Equal(got, []string{"a", "id", "version"}) {
		t.Errorf("params = %v", got)
	}
}

func TestLoad_RejectsBadIncludes(t *testing.T) {
	cases := map[string]string{
		"unqualified":       "{{> guard_where}}",
		"unknown namespace": "{{> org.lineage}}",
		"unknown pattern":   "{{> sql.nope}}",
	}
	for name, inc := range cases {
		_, err := query.Load(fstest.MapFS{"sql/s.sql": {Data: []byte("--| tier: standard\nSELECT 1 WHERE " + inc)}}, "sql", drivertest.Dialect{})
		if err == nil || !strings.Contains(err.Error(), "s.sql") {
			t.Errorf("%s: err = %v, want a load error naming the file", name, err)
		}
	}
}

// The guard runs over the included patterns exactly as over hand-written
// text.
func TestGuard_OverIncludedPatterns(t *testing.T) {
	src := query.MustLoad(fstest.MapFS{
		"sql/edit.sql":    {Data: []byte("--| tier: standard\nUPDATE t SET a = {{a}}, {{> sql.guard_set}} WHERE {{> sql.guard_where}}")},
		"sql/version.sql": {Data: []byte("--| tier: standard\nSELECT version FROM t WHERE id = {{id}}")},
	}, "sql", drivertest.Dialect{})
	db, rec := session(t, drivertest.Response{Affected: 1})
	v, err := query.Guarded(src.Statement("edit"), src.Statement("version"), "version").Run(context.Background(), db, 4, query.Args{"id": "x", "a": 1})
	if err != nil || v != 5 {
		t.Fatalf("Run = %d, %v", v, err)
	}
	if c := rec.Calls()[0]; c.Args[1] != "x" || c.Args[2] != int64(4) {
		t.Errorf("bound %v", c.Args)
	}
}
