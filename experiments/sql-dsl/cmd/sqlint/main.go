package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
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
	findings := lint(os.DirFS(root), goList(root))
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

// goList resolves a Go package path to its directory in the build list of
// the module at root — the version go.mod pins — so an external source's
// files are the ones the runtime embeds.
func goList(root string) func(string) (fs.FS, error) {
	return func(pkg string) (fs.FS, error) {
		cmd := exec.Command("go", "list", "-f", "{{.Dir}}", pkg)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("go list %s: %w", pkg, err)
		}
		return os.DirFS(strings.TrimSpace(string(out))), nil
	}
}

// verbNamed catches a file named for its SQL verb rather than its
// operation; delete is both a verb and a command and is allowed.
var verbNamed = regexp.MustCompile(`^(insert|select|update|upsert|merge)(_|\.)`)

// linter is one run over one tree.
type linter struct {
	fsys     fs.FS
	cfg      *Config
	lookup   func(string) (fs.FS, error)
	catalog  *query.Catalog
	forms    []form
	findings []string
}

// form is one native form the engine declared: the name the finding
// reports and the expression that recognizes it.
type form struct {
	name string
	re   *regexp.Regexp
}

// lint walks fsys under its configuration and returns every finding as
// "path:line: message". lookup resolves a package path to the package's
// directory; nil refuses every package path.
func lint(fsys fs.FS, lookup func(string) (fs.FS, error)) []string {
	if lookup == nil {
		lookup = func(pkg string) (fs.FS, error) { return nil, fmt.Errorf("no package resolution for %s", pkg) }
	}
	l := &linter{fsys: fsys, lookup: lookup}
	cfg, err := loadConfig(fsys)
	if err != nil {
		// Configuration errors carry the file's name already, one per line.
		l.findings = append(l.findings, strings.Split(err.Error(), "\n")...)
	}
	if cfg == nil {
		return l.findings
	}
	l.cfg = cfg
	l.resolveSources()
	l.resolveEngine()
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") && p != "." {
			return fs.SkipDir
		}
		if on, ok := cfg.Roles["statements"].switches(p); ok {
			l.lintStatements(p, on)
		}
		if on, ok := cfg.Roles["patterns"].switches(p); ok {
			l.lintPatterns(p, on)
		}
		if on, ok := cfg.Roles["migrations"].switches(p); ok {
			l.lintMigrations(p, on)
		}
		return nil
	})
	return l.findings
}

func (l *linter) report(p string, line int, msg string) {
	if line > 0 {
		l.findings = append(l.findings, fmt.Sprintf("%s:%d: %s", p, line, msg))
		return
	}
	l.findings = append(l.findings, fmt.Sprintf("%s: %s", p, msg))
}

// resolve turns a source or engine value into a filesystem and a base
// path: a package path through lookup, anything else a directory of the
// tree.
func (l *linter) resolve(value string) (fs.FS, string, error) {
	if isPackagePath(value) {
		fsys, err := l.lookup(value)
		return fsys, ".", err
	}
	if _, err := fs.Stat(l.fsys, value); err != nil {
		return nil, "", err
	}
	return l.fsys, value, nil
}

// resolveSources builds the catalog the statement directories compile
// against: each namespace's source, and for the library its engine
// overlay. A producer — a package path, or a directory holding its own
// configuration — names its pattern directory in its export; a bare
// directory is the pattern files themselves. A source that does not
// resolve is a finding against the configuration; the catalog is built
// from the rest.
func (l *linter) resolveSources() {
	var sources []query.Source
	names := make([]string, 0, len(l.cfg.Sources))
	for ns := range l.cfg.Sources {
		names = append(names, ns)
	}
	sort.Strings(names)
	for _, ns := range names {
		s := l.cfg.Sources[ns]
		fsys, base, err := l.resolve(s.Path)
		if err != nil {
			l.report(File, 0, fmt.Sprintf("sources.%s: %v", ns, err))
			continue
		}
		dir := base
		if isProducer(fsys, base) {
			export, err := readExport(fsys, base)
			if err != nil {
				l.report(File, 0, fmt.Sprintf("sources.%s: %v", ns, err))
				continue
			}
			if export.Patterns == "" {
				l.report(File, 0, fmt.Sprintf("sources.%s: %s exports no patterns", ns, s.Path))
				continue
			}
			dir = path.Join(base, export.Patterns)
		}
		src := query.Publish(ns, fsys, dir)
		if s.Overlay != "" {
			ofs, obase, err := l.resolve(s.Overlay)
			if err != nil {
				l.report(File, 0, fmt.Sprintf("sources.%s.overlay: %v", ns, err))
				continue
			}
			odir := obase
			if isProducer(ofs, obase) {
				oexport, err := readExport(ofs, obase)
				if err != nil {
					l.report(File, 0, fmt.Sprintf("sources.%s.overlay: %v", ns, err))
					continue
				}
				if oexport.Overlay == "" {
					l.report(File, 0, fmt.Sprintf("sources.%s.overlay: %s exports no overlay", ns, s.Overlay))
					continue
				}
				odir = path.Join(obase, oexport.Overlay)
			}
			src = src.Overlay(ofs, odir)
		}
		sources = append(sources, src)
	}
	catalog, err := query.NewCatalog(sources...)
	if err != nil {
		l.report(File, 0, err.Error())
		return
	}
	l.catalog = catalog
}

// resolveEngine reads the native forms the configured engine declares and
// compiles each; an engine is always a producer, since a service never
// defines one. A malformed expression is a finding against the
// configuration, before any file is read.
func (l *linter) resolveEngine() {
	if l.cfg.Engine == "" {
		return
	}
	fsys, base, err := l.resolve(l.cfg.Engine)
	if err != nil {
		l.report(File, 0, fmt.Sprintf("engine: %v", err))
		return
	}
	export, err := readExport(fsys, base)
	if err != nil {
		l.report(File, 0, fmt.Sprintf("engine: %v", err))
		return
	}
	names := make([]string, 0, len(export.NativeForms))
	for name := range export.NativeForms {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		re, err := regexp.Compile(export.NativeForms[name])
		if err != nil {
			l.report(File, 0, fmt.Sprintf("engine: native_forms.%s: %v", name, err))
			continue
		}
		l.forms = append(l.forms, form{name: name, re: re})
	}
}

// lintStatements compiles a statement directory the way a domain does,
// against the resolved catalog, then applies the rules the compiler
// leaves to review.
func (l *linter) lintStatements(dir string, on map[string]bool) {
	if l.catalog != nil {
		if _, err := l.catalog.Compile(l.fsys, dir, drivertest.Dialect{}); err != nil {
			l.report(dir, 0, err.Error())
		}
	}
	l.lintFiles(dir, on)
}

// lintPatterns validates a pattern directory as a catalog source — tier
// declared, a port named for a native pattern, slots only — then applies
// the body rules.
func (l *linter) lintPatterns(dir string, on map[string]bool) {
	if _, err := query.NewCatalog(query.Publish("lint", l.fsys, dir)); err != nil {
		l.report(dir, 0, err.Error())
	}
	l.lintFiles(dir, on)
}

// lintFiles applies the per-file rules of a statement or pattern
// directory under its switches.
func (l *linter) lintFiles(dir string, on map[string]bool) {
	entries, _ := fs.ReadDir(l.fsys, dir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		p := path.Join(dir, e.Name())
		if on["verb_named"] && verbNamed.MatchString(e.Name()) {
			l.report(p, 0, "named for its SQL verb; name a statement for its operation")
		}
		text, _ := fs.ReadFile(l.fsys, p)
		h, err := sqlheader.Parse(string(text))
		if err != nil {
			continue // reported by the compile or the catalog
		}
		tier, _ := h.Get("tier")
		l.lintBody(p, string(text), h.End(), on["delimiter"], on["native_forms"] && tier == "standard")
	}
}

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

// lintBody checks the body's lines: the reserved delimiter in a comment or
// a literal, and the engine's native forms in a standard-tier file. A
// form is matched against the line's code only — string literals and the
// comment tail stripped first — because whether text is data or syntax is
// a property of SQL, not of the engine, and a per-line expression cannot
// track quote state.
func (l *linter) lintBody(p, text string, end int, delimiter, forms bool) {
	line := 1 + strings.Count(text[:end], "\n")
	for _, ln := range strings.Split(text[end:], "\n") {
		if delimiter {
			if i := strings.Index(ln, "--"); i >= 0 && strings.Contains(ln[i:], "{{") {
				l.report(p, line, "{{ inside a comment: the delimiter is reserved for parameters")
			}
			if inLiteral(ln) {
				l.report(p, line, "{{ inside a string literal: the delimiter is reserved for parameters")
			}
		}
		if forms {
			code := codeOnly(ln)
			for _, f := range l.forms {
				if m := f.re.FindString(code); m != "" {
					l.report(p, line, fmt.Sprintf("%q (%s) in a standard-tier file; declare the tier native and name the port", m, f.name))
				}
			}
		}
		line++
	}
}

// codeOnly returns the line with every single-quoted literal emptied and
// the comment tail removed, so a form matches syntax and never data or
// prose. A quote doubled inside a literal ('it”s') closes and reopens,
// which empties it all the same.
func codeOnly(line string) string {
	var b strings.Builder
	open := false
	for i := 0; i < len(line); i++ {
		switch {
		case line[i] == '\'':
			open = !open
			b.WriteByte('\'')
		case open:
		case strings.HasPrefix(line[i:], "--"):
			return b.String()
		default:
			b.WriteByte(line[i])
		}
	}
	return b.String()
}

// lintMigrations checks each migration's header and, for a
// non-transactional file, that it holds one statement.
func (l *linter) lintMigrations(dir string, on map[string]bool) {
	entries, _ := fs.ReadDir(l.fsys, dir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		p := path.Join(dir, e.Name())
		text, _ := fs.ReadFile(l.fsys, p)
		h, err := sqlheader.Parse(string(text))
		if err != nil {
			l.report(p, 0, err.Error())
			continue
		}
		if v, ok := h.Get("transaction"); ok && v == "none" && on["single_statement"] {
			body := strings.TrimRight(strings.TrimSpace(string(text)[h.End():]), ";")
			if strings.Contains(body, ";") {
				l.report(p, 0, "a non-transactional migration holds one statement")
			}
		}
	}
}
