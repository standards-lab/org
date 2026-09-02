package sqlheader_test

import (
	"slices"
	"testing"

	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqlheader"
)

func TestParse_ReadsDirectivesUntilTheFirstSQLToken(t *testing.T) {
	h := sqlheader.Parse(`
-- tier: native
--   native: postgres — pg_advisory_xact_lock. Ports: sp_getapplock, GET_LOCK.
-- a line of prose, not a directive
-- field: id uuid

-- field: name text
SELECT pg_advisory_xact_lock(:key)
-- transaction: none  (after the body: not a directive)
`)
	if v, ok := h.Get("tier"); !ok || v != "native" {
		t.Errorf("tier = %q, %v", v, ok)
	}
	if v, _ := h.Get("native"); v != "postgres — pg_advisory_xact_lock. Ports: sp_getapplock, GET_LOCK." {
		t.Errorf("native = %q", v)
	}
	if got := h.All("field"); !slices.Equal(got, []string{"id uuid", "name text"}) {
		t.Errorf("fields = %v", got)
	}
	if _, ok := h.Get("transaction"); ok {
		t.Error("a directive after the body was read")
	}
	if got := h.Keys(); !slices.Equal(got, []string{"tier", "native", "field"}) {
		t.Errorf("keys = %v", got)
	}
	if ds := h.Directives(); ds[0].Line != 2 || ds[3].Line != 7 {
		t.Errorf("line numbers = %d, %d", ds[0].Line, ds[3].Line)
	}
}

func TestParse_EmptyAndHeaderlessTexts(t *testing.T) {
	for _, text := range []string{"", "SELECT 1", "-- just a comment\nSELECT 1", "-- Tier: standard\nSELECT 1"} {
		if h := sqlheader.Parse(text); len(h.Directives()) != 0 {
			t.Errorf("%q: directives = %v", text, h.Directives())
		}
	}
	if v, ok := sqlheader.Parse("-- key:\nSELECT 1").Get("key"); !ok || v != "" {
		t.Errorf("empty value: %q, %v", v, ok)
	}
}
