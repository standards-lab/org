package main

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLint_FindsEachConvention(t *testing.T) {
	fsys := fstest.MapFS{
		"domain/a/sql/insert_thing.sql":  {Data: []byte("--| tier: standard\nINSERT INTO t VALUES ({{x}})")},
		"domain/a/sql/list.sql":          {Data: []byte("--| tier: standard\n-- header prose may mention {{x}}\nSELECT 1 -- but body comments may not: {{docs}}\n, '{{x}}', id::text FROM t RETURNING id")},
		"domain/a/sql/native.sql":        {Data: []byte("--| tier: native\n--| native: postgres — RETURNING\nINSERT INTO t VALUES (1) RETURNING id")},
		"domain/b/sql/broken.sql":        {Data: []byte("--| teir: standard\nSELECT 1")},
		"admin/migrations/0001_a.up.sql": {Data: []byte("--| transaction: none\nCREATE INDEX CONCURRENTLY i ON t (x); ANALYZE t")},
		"admin/migrations/0002_b.up.sql": {Data: []byte("--| transaction none\nSELECT 1")},
		"docs/sql/notes.md":              {Data: []byte("ignored")},
	}
	findings := lint(fsys)
	want := []string{
		"insert_thing.sql: named for its SQL verb",
		"list.sql:4: {{ inside a string literal",
		`list.sql:4: "::" in a standard-tier file`,
		`list.sql:4: "RETURNING" in a standard-tier file`,
		"list.sql:3: {{ inside a comment",
		"domain/b/sql: query: broken.sql: line 1: unknown directive",
		"0001_a.up.sql: a non-transactional migration holds one statement",
		"0002_b.up.sql: sqlheader: line 1",
	}
	for _, w := range want {
		found := false
		for _, f := range findings {
			found = found || strings.Contains(f, w)
		}
		if !found {
			t.Errorf("missing finding %q in:\n%s", w, strings.Join(findings, "\n"))
		}
	}
	for _, f := range findings {
		if strings.Contains(f, "native.sql") {
			t.Errorf("a correct native file was reported: %s", f)
		}
	}
}
