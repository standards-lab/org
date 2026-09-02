package query_test

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/standards-lab/org/experiments/sql-dsl/lib/drivertest"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/query"
)

// load parses one file through Load, the only public path to the scanner.
func load(t *testing.T, text string) query.Statement {
	t.Helper()
	src, err := query.Load(fstest.MapFS{"sql/s.sql": {Data: []byte(text)}}, "sql", drivertest.Dialect{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return src.Statement("s")
}

func TestParams_Syntax(t *testing.T) {
	cases := []struct {
		name, in, out string
		params        []string
	}{
		{"simple", "SELECT {{a}}, {{b}}", "SELECT $1, $2", []string{"a", "b"}},
		{"repeat rebinds", "WHERE id = {{id}} OR parent = {{id}} AND v = {{v}}", "WHERE id = $1 OR parent = $1 AND v = $2", []string{"id", "v"}},
		{"type renders a cast", "WHERE parent_id IS NOT DISTINCT FROM {{parent:uuid}}", "WHERE parent_id IS NOT DISTINCT FROM CAST($1 AS uuid)", []string{"parent"}},
		{"type with precision", "SET amount = {{amount:numeric(12,2)}}, n = {{n:varchar(200)}}", "SET amount = CAST($1 AS numeric(12,2)), n = CAST($2 AS varchar(200))", []string{"amount", "n"}},
		{"multi-word type", "SELECT {{at:timestamp with time zone}}", "SELECT CAST($1 AS timestamp with time zone)", []string{"at"}},
		{"whitespace inside the braces", "SELECT {{ id : integer }}, {{ name }}", "SELECT CAST($1 AS integer), $2", []string{"id", "name"}},
		{"same name, cast once", "WHERE a = {{k:text}} OR b = {{k}}", "WHERE a = CAST($1 AS text) OR b = $1", []string{"k"}},
		{"colon forms are plain text", "SELECT x::uuid, :name, a := 1", "SELECT x::uuid, :name, a := 1", nil},
		{"the delimiter is reserved even in a literal", "SELECT '{{x}}'", "SELECT '$1'", []string{"x"}},
		{"no parameters", "SELECT 1", "SELECT 1", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := load(t, "--| tier: standard\n"+tc.in)
			if got := st.Text(); got != tc.out {
				t.Errorf("text = %q, want %q", got, tc.out)
			}
			if got := st.Params(); !slices.Equal(got, tc.params) {
				t.Errorf("params = %v, want %v", got, tc.params)
			}
		})
	}
}

// The header is not scanned and not sent: a {{ in a directive's prose is
// neither a parameter nor an error, and the engine receives the body only.
func TestParams_HeaderIsNotScannedOrSent(t *testing.T) {
	st := load(t, "--| tier: native\n--| native: postgres — binds {{key}} to pg_advisory_xact_lock\n-- prose\n\nSELECT pg_advisory_xact_lock({{key}})")
	if st.Text() != "SELECT pg_advisory_xact_lock($1)" {
		t.Errorf("text = %q", st.Text())
	}
	if got := st.Params(); !slices.Equal(got, []string{"key"}) {
		t.Errorf("params = %v", got)
	}
}

func TestParams_MalformedIsALoadError(t *testing.T) {
	cases := map[string]string{
		"digit first":       "SELECT {{1}}",
		"empty":             "SELECT {{ }}",
		"type with junk":    "SELECT {{a:uuid; DROP}}",
		"unclosed":          "SELECT {{a",
		"stray after valid": "SELECT {{a}}, {{",
		"space in name":     "SELECT {{a b}}",
	}
	for name, body := range cases {
		_, err := query.Load(fstest.MapFS{"sql/s.sql": {Data: []byte("--| tier: standard\n" + body)}}, "sql", drivertest.Dialect{})
		if err == nil || !strings.Contains(err.Error(), "s.sql") {
			t.Errorf("%s: err = %v, want a load error naming the file", name, err)
		}
	}
}
