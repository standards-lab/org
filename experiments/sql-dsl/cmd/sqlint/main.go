package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/standards-lab/org/experiments/sql-dsl/lib/drivertest"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/query"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqlheader"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	findings := lint(os.DirFS(root))
	sort.Strings(findings)
	for _, f := range findings {
		fmt.Println(f)
	}
	if len(findings) > 0 {
		fmt.Fprintf(os.Stderr, "sqlint: %d finding(s)\n", len(findings))
		os.Exit(1)
	}
	fmt.Println("sqlint: ok")
}

var (
	// verbNamed catches a file named for its SQL verb rather than its
	// operation; delete is both a verb and a command and is allowed.
	verbNamed = regexp.MustCompile(`^(insert|select|update|upsert|merge)(_|\.)`)
	// nativeForms are engine-specific spellings a standard-tier file must
	// not use; each names the form the file should declare native for.
	nativeForms = []string{"RETURNING", "ON CONFLICT", "ILIKE", "CONCURRENTLY", "::", "pg_", "now()", "uuidv7()", "LIMIT ", "SERIAL", "JSONB", "timestamptz"}
)

// inLiteral reports a {{ inside a single-quoted literal on one line,
// tracking the quote state so a closed literal followed by a parameter is
// not one.
func inLiteral(line string) bool {
	open := false
	for i := 0; i < len(line); i++ {
		switch {
		case line[i] == '\'':
			open = !open
		case open && strings.HasPrefix(line[i:], "{{"):
			return true
		}
	}
	return false
}

// lint walks fsys for sql/ and migrations/ directories and returns every
// finding as "path:line: message".
func lint(fsys fs.FS) []string {
	var out []string
	report := func(path string, line int, msg string) {
		if line > 0 {
			out = append(out, fmt.Sprintf("%s:%d: %s", path, line, msg))
			return
		}
		out = append(out, fmt.Sprintf("%s: %s", path, msg))
	}
	_ = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") && path != "." {
			return fs.SkipDir
		}
		switch d.Name() {
		case "statements", "patterns":
			lintSource(fsys, path, report)
		case "migrations":
			lintMigrations(fsys, path, report)
		}
		return nil
	})
	return out
}

// lintSource loads a statement directory the way a domain does, then
// applies the rules the loader leaves to review.
func lintSource(fsys fs.FS, dir string, report func(string, int, string)) {
	if _, err := query.Load(fsys, dir, drivertest.Dialect{}); err != nil {
		report(dir, 0, err.Error())
	}
	entries, _ := fs.ReadDir(fsys, dir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if verbNamed.MatchString(e.Name()) {
			report(path, 0, "named for its SQL verb; name a statement for its operation")
		}
		text, _ := fs.ReadFile(fsys, path)
		h, err := sqlheader.Parse(string(text))
		if err != nil {
			continue // reported by Load
		}
		tier, _ := h.Get("tier")
		lintBody(path, string(text), h.End(), tier == "standard", report)
	}
}

// lintBody checks the body's lines: the reserved delimiter in a comment or
// a literal, and native forms in a standard-tier file.
func lintBody(path, text string, end int, standard bool, report func(string, int, string)) {
	line := 1 + strings.Count(text[:end], "\n")
	for _, l := range strings.Split(text[end:], "\n") {
		trimmed := strings.TrimSpace(l)
		if i := strings.Index(l, "--"); i >= 0 && strings.Contains(l[i:], "{{") {
			report(path, line, "{{ inside a comment: the delimiter is reserved for parameters")
		}
		if inLiteral(l) {
			report(path, line, "{{ inside a string literal: the delimiter is reserved for parameters")
		}
		if standard && !strings.HasPrefix(trimmed, "--") {
			for _, form := range nativeForms {
				if strings.Contains(l, form) {
					report(path, line, fmt.Sprintf("%q in a standard-tier file; declare the tier native and name the port", strings.TrimSpace(form)))
				}
			}
		}
		line++
	}
}

// lintMigrations checks each migration's header and, for a
// non-transactional file, that it holds one statement.
func lintMigrations(fsys fs.FS, dir string, report func(string, int, string)) {
	entries, _ := fs.ReadDir(fsys, dir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		text, _ := fs.ReadFile(fsys, path)
		h, err := sqlheader.Parse(string(text))
		if err != nil {
			report(path, 0, err.Error())
			continue
		}
		if v, ok := h.Get("transaction"); ok && v == "none" {
			body := strings.TrimRight(strings.TrimSpace(string(text)[h.End():]), ";")
			if strings.Contains(body, ";") {
				report(path, 0, "a non-transactional migration holds one statement")
			}
		}
	}
}
