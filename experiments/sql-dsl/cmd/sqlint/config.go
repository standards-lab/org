package main

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// The lint's configuration: sqlint.toml beside go.mod. Each role — the
// statement directories, the pattern directories, the migrations — owns a
// table of the directory globs it covers and the switches of its checks;
// a directory set that needs an exception overrides the role's switches
// under its own glob. Pattern sources and the engine are paths, resolved
// the way the runtime resolves them, and a producer's own sqlint.toml
// declares in [export] what a consumer reads from it. Absent the file, the
// roles are the conventions as they stand, every check on, and no source
// is registered.

// File is the configuration file's name, at the root and in each producer.
const File = "sqlint.toml"

// Config is the parsed configuration.
type Config struct {
	// Engine is the path of the engine whose export declares the native
	// forms; empty means the native-forms check has no list.
	Engine string
	// Sources maps a namespace to the source registered under it.
	Sources map[string]Source
	// Roles holds the three roles by name.
	Roles map[string]*Role
	// Export is what this module declares to a consumer.
	Export Export
}

// Source is one registered pattern namespace: a path, and for the library
// an optional engine overlay respelling its patterns.
type Source struct {
	Path    string `toml:"path"`
	Overlay string `toml:"overlay"`
}

// Export is a producer's declaration: the directory its namespace
// publishes, the overlay directory an engine supplies, and the native
// forms an engine names — each a regular expression under the name the
// finding reports, so the engine states in what position a spelling
// counts (word boundaries, case), not just which spelling.
type Export struct {
	Patterns    string            `toml:"patterns"`
	Overlay     string            `toml:"overlay"`
	NativeForms map[string]string `toml:"native_forms"`
}

// Role is one role's directory globs and switches, with any per-glob
// overrides.
type Role struct {
	Dirs      []string
	Checks    map[string]bool
	Overrides map[string]map[string]bool
}

// The checks each role knows, by switch name.
var roleChecks = map[string][]string{
	"statements": {"verb_named", "delimiter", "native_forms"},
	"patterns":   {"delimiter", "native_forms"},
	"migrations": {"single_statement"},
}

// defaults are the conventions as they stand: every directory of each
// role's name, every check on.
func defaults() *Config {
	c := &Config{Sources: map[string]Source{}, Roles: map[string]*Role{}}
	for name, checks := range roleChecks {
		r := &Role{Dirs: []string{"**/" + name}, Checks: map[string]bool{}, Overrides: map[string]map[string]bool{}}
		for _, ch := range checks {
			r.Checks[ch] = true
		}
		c.Roles[name] = r
	}
	return c
}

// raw is the file as TOML sees it; the role tables decode in two passes
// because they mix scalar switches with glob-keyed override tables.
type raw struct {
	Engine     string                    `toml:"engine"`
	Sources    map[string]toml.Primitive `toml:"sources"`
	Statements map[string]toml.Primitive `toml:"statements"`
	Patterns   map[string]toml.Primitive `toml:"patterns"`
	Migrations map[string]toml.Primitive `toml:"migrations"`
	Export     Export                    `toml:"export"`
}

// loadConfig reads File at the root of fsys; a missing file is the
// defaults. Every configuration error is reported, joined.
func loadConfig(fsys fs.FS) (*Config, error) {
	text, err := fs.ReadFile(fsys, File)
	if errors.Is(err, fs.ErrNotExist) {
		return defaults(), nil
	}
	if err != nil {
		return nil, err
	}
	return parseConfig(string(text))
}

func parseConfig(text string) (*Config, error) {
	var r raw
	md, err := toml.Decode(text, &r)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", File, err)
	}
	c := defaults()
	c.Engine = r.Engine
	c.Export = r.Export
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(File+": "+format, args...)) }
	for ns, prim := range r.Sources {
		var s Source
		var p string
		if err := md.PrimitiveDecode(prim, &p); err == nil {
			s.Path = p
		} else if err := md.PrimitiveDecode(prim, &s); err != nil {
			fail("sources.%s: a path, or a table with path and overlay", ns)
			continue
		}
		if s.Path == "" {
			fail("sources.%s: no path", ns)
			continue
		}
		c.Sources[ns] = s
	}
	for name, table := range map[string]map[string]toml.Primitive{"statements": r.Statements, "patterns": r.Patterns, "migrations": r.Migrations} {
		if table == nil {
			continue
		}
		if err := parseRole(md, name, table, c.Roles[name]); err != nil {
			errs = append(errs, err)
		}
	}
	// Every primitive is decoded by now, so what remains undecoded is a key
	// the configuration does not know.
	for _, k := range md.Undecoded() {
		fail("unknown key %s", k)
	}
	return c, errors.Join(errs...)
}

// parseRole fills role from its table: dirs, the known switches, and one
// override table per dirs entry, holding switches only.
func parseRole(md toml.MetaData, name string, table map[string]toml.Primitive, role *Role) error {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(File+": "+name+": "+format, args...)) }
	known := roleChecks[name]
	if prim, ok := table["dirs"]; ok {
		if err := md.PrimitiveDecode(prim, &role.Dirs); err != nil {
			fail("dirs: %v", err)
		}
	}
	keys := make([]string, 0, len(table))
	for k := range table {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if k == "dirs" {
			continue
		}
		if slices.Contains(known, k) {
			var on bool
			if err := md.PrimitiveDecode(table[k], &on); err != nil {
				fail("%s: %v", k, err)
				continue
			}
			role.Checks[k] = on
			continue
		}
		if md.Type(name, k) != "Hash" {
			fail("%q is neither a switch of the role (%s) nor an override table", k, strings.Join(known, ", "))
			continue
		}
		var prims map[string]toml.Primitive
		if err := md.PrimitiveDecode(table[k], &prims); err != nil {
			fail("override %q: %v", k, err)
			continue
		}
		if !slices.Contains(role.Dirs, k) {
			fail("override %q names no entry of dirs", k)
			continue
		}
		over := map[string]bool{}
		for ch, prim := range prims {
			if !slices.Contains(known, ch) {
				fail("override %q: %q is not a switch of the role (%s)", k, ch, strings.Join(known, ", "))
				continue
			}
			var on bool
			if err := md.PrimitiveDecode(prim, &on); err != nil {
				fail("override %q: %s: %v", k, ch, err)
				continue
			}
			over[ch] = on
		}
		role.Overrides[k] = over
	}
	return errors.Join(errs...)
}

// switches resolves the checks in force for dir: the first dirs entry
// that matches names the set, and its override refines the role's
// switches. A directory no entry matches is not the role's.
func (r *Role) switches(dir string) (map[string]bool, bool) {
	for _, glob := range r.Dirs {
		if !match(glob, dir) {
			continue
		}
		out := make(map[string]bool, len(r.Checks))
		maps.Copy(out, r.Checks)
		maps.Copy(out, r.Overrides[glob])
		return out, true
	}
	return nil, false
}

// match reports whether the slash-separated glob matches the directory
// path: each segment is a path.Match pattern, and "**" matches any run of
// segments, including none.
func match(glob, dir string) bool {
	return matchSegments(strings.Split(glob, "/"), strings.Split(path.Clean(dir), "/"))
}

func matchSegments(gs, ps []string) bool {
	if len(gs) == 0 {
		return len(ps) == 0
	}
	if gs[0] == "**" {
		for i := 0; i <= len(ps); i++ {
			if matchSegments(gs[1:], ps[i:]) {
				return true
			}
		}
		return false
	}
	if len(ps) == 0 {
		return false
	}
	ok, err := path.Match(gs[0], ps[0])
	return err == nil && ok && matchSegments(gs[1:], ps[1:])
}

// isPackagePath reports whether a source or engine value is a Go package
// path — its first segment holds a dot, as a module path's host does —
// rather than a directory relative to the configuration.
func isPackagePath(p string) bool {
	first, _, _ := strings.Cut(p, "/")
	return strings.Contains(first, ".")
}

// isProducer reports whether base in fsys declares itself: a package path
// always does, and a directory does when it holds File. A directory
// without one is the pattern files themselves, the service's own,
// declared entirely in the root configuration.
func isProducer(fsys fs.FS, base string) bool {
	if base == "." {
		return true
	}
	_, err := fs.Stat(fsys, path.Join(base, File))
	return err == nil
}

// readExport reads a producer's File under base in fsys and returns its
// export; a producer without one declares nothing.
func readExport(fsys fs.FS, base string) (Export, error) {
	text, err := fs.ReadFile(fsys, path.Join(base, File))
	if errors.Is(err, fs.ErrNotExist) {
		return Export{}, fmt.Errorf("%s declares no %s", base, File)
	}
	if err != nil {
		return Export{}, err
	}
	var r struct {
		Export Export `toml:"export"`
	}
	if _, err := toml.Decode(string(text), &r); err != nil {
		return Export{}, fmt.Errorf("%s: %w", path.Join(base, File), err)
	}
	return r.Export, nil
}
