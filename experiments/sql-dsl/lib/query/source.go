package query

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/standards-lab/go-database"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqlheader"
)

// Source is a domain's statements, loaded from one directory: the inventory
// the verification step and the management surface walk, keyed by file
// name. It is not a registry; a domain fetches each statement once at
// wiring.
type Source struct {
	statements map[string]Statement
}

// Load reads every .sql file under dir in fsys, parses its header, and
// resolves its parameters against d's placeholders. A file without a
// header, with an unknown directive, or with a header the grammar rejects
// is a load error naming the file; the domain treats it as a wiring defect.
func Load(fsys fs.FS, dir string, d database.Dialect) (*Source, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("query: read %s: %w", dir, err)
	}
	src := &Source{statements: map[string]Statement{}}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		text, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("query: read %s: %w", e.Name(), err)
		}
		st, err := parse(strings.TrimSuffix(e.Name(), ".sql"), string(text), d.Placeholder)
		if err != nil {
			return nil, fmt.Errorf("query: %s: %w", e.Name(), err)
		}
		src.statements[st.name] = st
	}
	return src, nil
}

// MustLoad is Load for wiring functions, where a load error is a defect.
func MustLoad(fsys fs.FS, dir string, d database.Dialect) *Source {
	src, err := Load(fsys, dir, d)
	if err != nil {
		panic(err)
	}
	return src
}

// Statement returns the statement named by its file's base name; a missing
// name is a wiring defect and panics.
func (s *Source) Statement(name string) Statement {
	st, ok := s.statements[name]
	if !ok {
		panic(fmt.Sprintf("query: no statement %q", name))
	}
	return st
}

// Statements returns the inventory in name order.
func (s *Source) Statements() []Statement {
	out := make([]Statement, 0, len(s.statements))
	for _, st := range s.statements {
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// Verify prepares every statement against db, so a reference the schema no
// longer satisfies fails here rather than at first request. Every failure is
// reported, joined, each naming its statement.
func (s *Source) Verify(ctx context.Context, db sqldb.Session) error {
	var errs []error
	for _, st := range s.Statements() {
		stmt, err := db.PrepareContext(ctx, st.text)
		if err != nil {
			errs = append(errs, fmt.Errorf("query: %s: %w", st.name, err))
			continue
		}
		_ = stmt.Close()
	}
	return errors.Join(errs...)
}

// Verifier is what Verify composes: a Source, a Projection, anything that
// can check itself against the live schema.
type Verifier interface {
	Verify(ctx context.Context, db sqldb.Session) error
}

// Verify runs every verifier and joins their failures; startup and the
// management surface call it with the same arguments.
func Verify(ctx context.Context, db sqldb.Session, vs ...Verifier) error {
	var errs []error
	for _, v := range vs {
		if err := v.Verify(ctx, db); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// parse reads the header and rewrites the body's parameters; the engine
// receives the body. The header grammar, from the sketch: tier required
// (standard | native); native required when the tier is native, the reach
// and the port as free text; transaction optional (required); key optional,
// naming a field; field repeated, "<name> <type>".
func parse(name, text string, placeholder func(int) string) (Statement, error) {
	st := Statement{name: name}
	h, err := sqlheader.Parse(text)
	if err != nil {
		return st, err
	}
	for _, dir := range h.Directives() {
		switch dir.Key {
		case "tier", "native", "transaction", "key", "field":
		default:
			return st, fmt.Errorf("line %d: unknown directive %q", dir.Line, dir.Key)
		}
	}
	tier, ok := h.Get("tier")
	if !ok {
		return st, errors.New("no tier directive")
	}
	switch Tier(tier) {
	case TierStandard, TierNative:
		st.tier = Tier(tier)
	default:
		return st, fmt.Errorf("tier %q is not standard or native", tier)
	}
	st.native, _ = h.Get("native")
	if st.tier == TierNative && st.native == "" {
		return st, errors.New("a native statement declares its reach and port in a native directive")
	}
	if st.tier == TierStandard && st.native != "" {
		return st, errors.New("a standard statement has no native directive")
	}
	if tx, ok := h.Get("transaction"); ok {
		if tx != "required" {
			return st, fmt.Errorf("transaction directive %q is not required", tx)
		}
		st.txRequired = true
	}
	for _, f := range h.All("field") {
		fname, typ, ok := strings.Cut(f, " ")
		typ = strings.TrimSpace(typ)
		if !ok || !sqlType.MatchString(typ) {
			return st, fmt.Errorf("field directive %q is not \"<name> <type>\"", f)
		}
		st.fields = append(st.fields, Field{Name: fname, Type: typ})
	}
	if key, ok := h.Get("key"); ok {
		found := false
		for _, f := range st.fields {
			found = found || f.Name == key
		}
		if !found {
			return st, fmt.Errorf("key %q is not a declared field", key)
		}
		st.key = key
	}
	st.text, st.params, err = rewrite(text[h.End():], placeholder)
	return st, err
}
