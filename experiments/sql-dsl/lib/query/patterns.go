package query

import (
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqlheader"
)

// The pattern catalog: the library's own SQL, authored as files under
// patterns/ like any statement, each declaring its tier. A pattern's body holds
// slots in the {{ }} syntax; the library fills them with text it composed
// from other patterns, never with request input. Two uses:
//
//   - at request time, the collection read composes count, page, and one
//     from the patterns over a domain's base and the request's directives;
//   - at load time, a statement includes a pattern with {{> sql.name}}, and
//     the pattern's text is spliced in before parameters are rewritten, so
//     the guard's predicate and protocol columns are written once.
//
// The catalog is the library's; stage 11 sources patterns from any fs.FS,
// overriding by name — a port supplies its own paging pattern.

//go:embed patterns/*.sql
var patternFiles embed.FS

var (
	slot    = regexp.MustCompile(`\{\{\s*([a-z_][a-z0-9_]*)\s*\}\}`)
	include = regexp.MustCompile(`\{\{>\s*([a-z_][a-z0-9_]*)\.([a-z_][a-z0-9_]*)\s*\}\}`)
	bare    = regexp.MustCompile(`\{\{>[^}]*\}\}`)
)

// Namespace is the library's own catalog's namespace. Every include is
// qualified — {{> sql.guard_where}} — so a pattern's origin is visible in
// the statement and two sources cannot collide. Stage 11 registers further
// namespaces (an engine sub-module, a service, a domain) and lets a
// registrant alias one, the way an import is aliased, as the last resort
// against a collision.
const Namespace = "sql"

// pattern is one catalog entry: its body and the slots it declares.
type pattern struct {
	name  string
	text  string
	slots []string
}

// patterns is the catalog, loaded once.
var patterns = mustLoadPatterns(patternFiles, "patterns")

func mustLoadPatterns(fsys fs.FS, dir string) map[string]pattern {
	out, err := loadPatterns(fsys, dir)
	if err != nil {
		panic(err)
	}
	return out
}

// loadPatterns reads every pattern file: the header must declare a tier, and
// the body's {{ }} occurrences are its slots.
func loadPatterns(fsys fs.FS, dir string) (map[string]pattern, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("query: patterns: %w", err)
	}
	out := map[string]pattern{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		text, err := fs.ReadFile(fsys, dir+"/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("query: pattern %s: %w", e.Name(), err)
		}
		h, err := sqlheader.Parse(string(text))
		if err != nil {
			return nil, fmt.Errorf("query: pattern %s: %w", e.Name(), err)
		}
		if _, ok := h.Get("tier"); !ok {
			return nil, fmt.Errorf("query: pattern %s: no tier directive", e.Name())
		}
		f := pattern{name: strings.TrimSuffix(e.Name(), ".sql"), text: strings.TrimRight(string(text)[h.End():], "\n")}
		for _, m := range slot.FindAllStringSubmatch(f.text, -1) {
			f.slots = append(f.slots, m[1])
		}
		out[f.name] = f
	}
	return out, nil
}

// render fills a pattern's slots. Every slot must be filled and every fill
// must name a slot; a mismatch is a defect in the library, not a request
// error, and panics.
func render(name string, fill map[string]string) string {
	f, ok := patterns[name]
	if !ok {
		panic(fmt.Sprintf("query: no pattern %q", name))
	}
	for _, s := range f.slots {
		if _, ok := fill[s]; !ok {
			panic(fmt.Sprintf("query: pattern %s: slot %q not filled", name, s))
		}
	}
	return slot.ReplaceAllStringFunc(f.text, func(m string) string {
		return fill[slot.FindStringSubmatch(m)[1]]
	})
}

// expand splices {{> namespace.name}} includes into a statement body,
// recursively, so a statement carries a pattern's text as if authored
// there. An unqualified include, an unknown namespace or pattern, or an
// include cycle is a load error naming it.
func expand(body string) (string, error) {
	const limit = 8
	for depth := 0; bare.MatchString(body); depth++ {
		if depth == limit {
			return "", fmt.Errorf("includes nest past %d levels; a cycle?", limit)
		}
		var err error
		body = bare.ReplaceAllStringFunc(body, func(m string) string {
			parts := include.FindStringSubmatch(m)
			if parts == nil {
				err = fmt.Errorf("include %s must be qualified: {{> namespace.name}}", m)
				return m
			}
			ns, name := parts[1], parts[2]
			if ns != Namespace {
				err = fmt.Errorf("include of unknown namespace %q (only %q is registered)", ns, Namespace)
				return m
			}
			f, ok := patterns[name]
			if !ok {
				err = fmt.Errorf("include of unknown pattern %q", ns+"."+name)
				return m
			}
			return f.text
		})
		if err != nil {
			return "", err
		}
	}
	return body, nil
}
