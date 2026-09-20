package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"
)

// The listing composer. A listing is an authored statement anchored on one
// directory (its own WHERE clause binds the directory), and the composer
// appends the caller's predicates, the ORDER BY, and the paging clause to
// that text. There is no derived-table wrap: the clauses attach at the
// statement's own level, so the engine sees one flat query over one table
// and can page a name sort through the unique index. The clause text comes
// from the query library's own patterns, read through the catalog, so the
// spelling of a predicate, a sort term, and the paging clause is the
// library's and an engine overlay of the paging pattern still applies.
//
// The total travels in the page: the TotalExact statement carries
// COUNT(*) OVER () AS total in its select list, evaluated over the rows the
// WHERE clause keeps and before the paging clause cuts them, so every row
// of a page carries the total of the listing it came from and the two
// cannot disagree under any isolation level.

// TotalMode says whether a listing computes its total.
type TotalMode int

const (
	// TotalExact, the default, computes the total in the same statement as
	// the page.
	TotalExact TotalMode = iota

	// TotalNone omits the total. The page's Total is NoTotal.
	TotalNone
)

// NoTotal is the Total of a page that carries none: the listing asked for
// TotalNone, or the page is empty and is not the first, so no row carried
// the window count. An empty first page has the exact total 0.
const NoTotal = -1

// Listing is one page request against a listing: the filters and sort
// terms over the statement's declared fields, the 1-based page number and
// the page size, and the total mode. Filters and Sort reuse the query
// library's declarations, and an unknown field, an unknown operator, or a
// value of the wrong shape is refused with the library's own error types,
// each of which unwraps to query.ErrDirectives. After is the keyset cursor
// of a later stage; a non-empty After is refused until then.
type Listing struct {
	Filters []query.Filter
	Sort    []query.Sort
	Page    int
	Size    int
	After   string
	Total   TotalMode
}

// Page is one page of a listing: its rows, its total (NoTotal when the
// page carries none), and Next, the cursor of the following page, which
// the keyset cursor of a later stage fills and which is empty until then.
type Page[T any] struct {
	Rows  []T
	Total int
	Next  string
}

// totalColumn is the alias of the window count in a TotalExact statement.
const totalColumn = "total"

// slot matches one slot of a clause pattern, as the query library spells
// it.
var slot = regexp.MustCompile(`\{\{\s*([a-z_][a-z0-9_]*)\s*\}\}`)

// clauseNames are the query library's patterns the composer fills: one
// predicate per filter operator, the value cast, the sort terms, the ORDER
// BY, and the paging clause.
var clauseNames = []string{
	"filter_eq", "filter_ne", "filter_gt", "filter_ge", "filter_lt", "filter_le",
	"filter_like", "filter_null", "filter_notnull", "filter_in",
	"value", "order", "order_term", "order_term_desc", "paging",
}

// clauses is the composer's copy of the clause patterns, read from the
// catalog under the query library's namespace, and the dialect's
// placeholder. The catalog exposes its inventory and not its renderer, so
// the composer fills the slots itself.
type clauses struct {
	patterns    map[string]string
	placeholder func(int) string
}

// newClauses reads the clause patterns from catalog. A catalog whose
// library namespace lacks one is refused, naming it.
func newClauses(catalog *query.Catalog, dialect sqlate.Dialect) (clauses, error) {
	c := clauses{patterns: map[string]string{}, placeholder: dialect.Placeholder}
	for _, p := range catalog.Patterns() {
		if p.Namespace == query.Namespace {
			c.patterns[p.Name] = p.Text
		}
	}
	for _, name := range clauseNames {
		if _, ok := c.patterns[name]; !ok {
			return clauses{}, fmt.Errorf("data: the catalog has no pattern %s.%s; register query.Patterns() in it", query.Namespace, name)
		}
	}
	return c, nil
}

// fill renders one clause pattern with its slots filled. A slot the fill
// does not name is a defect in this package and panics.
func (c clauses) fill(name string, fill map[string]string) string {
	return slot.ReplaceAllStringFunc(c.patterns[name], func(m string) string {
		s := slot.FindStringSubmatch(m)[1]
		v, ok := fill[s]
		if !ok {
			panic(fmt.Sprintf("data: pattern %s.%s: slot %q not filled", query.Namespace, name, s))
		}
		return v
	})
}

// listing is one listing's pair of compiled statements, without and with
// the total, and the field contract they share.
type listing[T any] struct {
	clauses clauses
	plain   query.Statement
	counted query.Statement
	key     string
	fields  map[string]query.Field
	order   []query.Field
}

// newListing binds the two statements of one listing and checks they
// agree: both declare the key and the same fields, and both bind the same
// parameters.
func newListing[T any](c clauses, plain, counted query.Statement) (listing[T], error) {
	l := listing[T]{clauses: c, plain: plain, counted: counted, key: plain.Key(), order: plain.Fields(), fields: map[string]query.Field{}}
	if l.key == "" || len(l.order) == 0 {
		return l, fmt.Errorf("data: %s declares no key or no fields", plain.Name())
	}
	if counted.Key() != l.key || !reflect.DeepEqual(counted.Fields(), l.order) || !reflect.DeepEqual(counted.Params(), plain.Params()) {
		return l, fmt.Errorf("data: %s and %s declare different contracts", plain.Name(), counted.Name())
	}
	for _, f := range l.order {
		l.fields[f.Name] = f
	}
	return l, nil
}

// run composes and runs one page: the statement chosen by the total mode,
// the listing's own arguments, and the caller's directives.
func (l listing[T]) run(ctx context.Context, sess sqlate.Session, args query.Args, d Listing) (Page[T], error) {
	st, counted := l.plain, false
	if d.Total == TotalExact {
		st, counted = l.counted, true
	}
	text, values, err := l.compose(st, args, d)
	if err != nil {
		return Page[T]{}, err
	}
	rows, err := sess.QueryContext(ctx, text, values...)
	if err != nil {
		return Page[T]{}, engine(err)
	}
	defer func() { _ = rows.Close() }()
	page := Page[T]{Total: NoTotal}
	scan, err := scanner[T](rows, counted)
	if err != nil {
		return Page[T]{}, mapErr(sess, err)
	}
	for rows.Next() {
		v, total, err := scan(rows)
		if err != nil {
			return Page[T]{}, mapErr(sess, err)
		}
		page.Rows = append(page.Rows, v)
		page.Total = total
	}
	if err := rows.Err(); err != nil {
		return Page[T]{}, mapErr(sess, err)
	}
	if counted && len(page.Rows) == 0 && d.Page == 1 {
		page.Total = 0
	}
	return page, nil
}

// compose renders st with d's clauses appended and returns the text and
// the values to bind: st's own arguments in its parameter order, then each
// filter value, then the offset and the fetch count. The directives are
// checked before any text is composed: a bad page, an unknown field or
// operator, a malformed value, and a cursor are refused here, and the
// engine never sees them.
func (l listing[T]) compose(st query.Statement, args query.Args, d Listing) (string, []any, error) {
	if d.Page < 1 {
		return "", nil, fmt.Errorf("%w: page number must be at least 1", query.ErrDirectives)
	}
	if d.Size < 1 {
		return "", nil, fmt.Errorf("%w: page size must be at least 1", query.ErrDirectives)
	}
	if d.After != "" {
		return "", nil, fmt.Errorf("%w: cursor paging (After) is not supported yet", query.ErrDirectives)
	}
	var values []any
	for _, name := range st.Params() {
		v, ok := args[name]
		if !ok {
			return "", nil, &query.ArgumentError{Statement: st.Name(), Name: name}
		}
		values = append(values, v)
	}
	value := func(f query.Field, v any) string {
		values = append(values, v)
		return l.clauses.fill("value", map[string]string{"placeholder": l.clauses.placeholder(len(values)), "type": f.Type})
	}

	var text strings.Builder
	text.WriteString(st.Text())
	for _, f := range d.Filters {
		field, ok := l.fields[f.Field]
		if !ok {
			return "", nil, &query.UnknownFieldError{Field: f.Field, Use: query.FieldUseFilter}
		}
		fill := map[string]string{"field": field.Name}
		switch f.Op {
		case query.OpEq, query.OpNe, query.OpGt, query.OpGe, query.OpLt, query.OpLe, query.OpLike:
			fill["value"] = value(field, f.Value)
		case query.OpIsNull, query.OpIsNotNull:
		case query.OpIn:
			vals, ok := f.Value.([]any)
			if !ok || len(vals) == 0 {
				return "", nil, &query.InvalidValueError{Field: f.Field, Err: errors.New("an in filter takes a non-empty []any")}
			}
			parts := make([]string, len(vals))
			for i, v := range vals {
				parts[i] = value(field, v)
			}
			fill["values"] = strings.Join(parts, ", ")
		default:
			return "", nil, &query.UnknownOperatorError{Op: f.Op}
		}
		text.WriteString(" AND ")
		text.WriteString(l.clauses.fill("filter_"+string(f.Op), fill))
	}

	terms := make([]string, 0, len(d.Sort)+1)
	keySorted := false
	for _, s := range d.Sort {
		field, ok := l.fields[s.Field]
		if !ok {
			return "", nil, &query.UnknownFieldError{Field: s.Field, Use: query.FieldUseSort}
		}
		name := "order_term"
		if s.Descending {
			name = "order_term_desc"
		}
		terms = append(terms, l.clauses.fill(name, map[string]string{"field": field.Name}))
		keySorted = keySorted || s.Field == l.key
	}
	if !keySorted {
		terms = append(terms, l.clauses.fill("order_term", map[string]string{"field": l.key}))
	}
	text.WriteString(l.clauses.fill("order", map[string]string{"terms": strings.Join(terms, ", ")}))

	text.WriteString(l.clauses.fill("paging", map[string]string{
		"offset": l.clauses.placeholder(len(values) + 1),
		"fetch":  l.clauses.placeholder(len(values) + 2),
	}))
	values = append(values, (d.Page-1)*d.Size, d.Size)
	return text.String(), values, nil
}

// Verify prepares one canonical rendering of each of the listing's two
// statements: a predicate on every declared field, a sort on every field,
// and the paging clause, so a field the table no longer has fails at
// startup and not at the first request that names it.
func (l listing[T]) Verify(ctx context.Context, db sqlate.Session) error {
	d := Listing{Page: 1, Size: 1}
	for _, f := range l.order {
		d.Filters = append(d.Filters, query.Filter{Field: f.Name, Op: query.OpIsNotNull})
		d.Sort = append(d.Sort, query.Sort{Field: f.Name})
	}
	var errs []error
	for _, st := range []query.Statement{l.plain, l.counted} {
		args := query.Args{}
		for _, name := range st.Params() {
			args[name] = nil
		}
		text, _, err := l.compose(st, args, d)
		if err != nil {
			errs = append(errs, fmt.Errorf("data: %s: %w", st.Name(), err))
			continue
		}
		stmt, err := db.PrepareContext(ctx, text)
		if err != nil {
			errs = append(errs, fmt.Errorf("data: %s: listing contract: %w", st.Name(), err))
			continue
		}
		_ = stmt.Close()
	}
	return errors.Join(errs...)
}

// scanner returns the scan of one row into a T and the total: T's exported
// fields matched to the row's columns by tag, and the total column, when
// counted, into the total. The query library's Scanner refuses a column T
// has no field for, so the page statement's total needs this scan of its
// own. A column that is neither is an error, as it is there.
func scanner[T any](rows *sql.Rows, counted bool) (func(*sql.Rows) (T, int, error), error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	fields := fieldsOf(reflect.TypeFor[T]())
	plan := make([]int, len(cols))
	totalAt := -1
	for i, c := range cols {
		if counted && c == totalColumn {
			totalAt = i
			plan[i] = -1
			continue
		}
		idx, ok := fields[c]
		if !ok {
			return nil, fmt.Errorf("data: column %q has no field in %s", c, reflect.TypeFor[T]())
		}
		plan[i] = idx
	}
	if counted && totalAt < 0 {
		return nil, fmt.Errorf("data: the counted statement returns no %s column", totalColumn)
	}
	return func(rows *sql.Rows) (T, int, error) {
		var v T
		var total int64 = NoTotal
		rv := reflect.ValueOf(&v).Elem()
		dests := make([]any, len(cols))
		for i, idx := range plan {
			if i == totalAt {
				dests[i] = &total
				continue
			}
			dests[i] = rv.Field(idx).Addr().Interface()
		}
		if err := rows.Scan(dests...); err != nil {
			return v, 0, err
		}
		return v, int(total), nil
	}, nil
}

var fieldCache sync.Map // reflect.Type to map[string]int

// fieldsOf indexes a struct type's exported fields by column name, once
// per type, by the query library's rule: the db tag, else the json tag's
// name, else the field name lowercased; "-" excludes the field.
func fieldsOf(t reflect.Type) map[string]int {
	if m, ok := fieldCache.Load(t); ok {
		return m.(map[string]int)
	}
	m := map[string]int{}
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name := f.Tag.Get("db")
		if name == "" {
			name, _, _ = strings.Cut(f.Tag.Get("json"), ",")
		}
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		if name == "-" {
			continue
		}
		m[name] = i
	}
	fieldCache.Store(t, m)
	return m
}

// engine classifies a query failure the way the query library's projection
// does: a data exception (a value the engine could not read as the field's
// type) is the request's fault and becomes a query.InvalidValueError;
// anything else is returned as it came.
func engine(err error) error {
	if errors.Is(err, sqlate.ErrInvalidValue) {
		return &query.InvalidValueError{Err: err}
	}
	return err
}

// mapErr routes an error that arose after a call returned (rows.Err, Scan)
// through the session's mapper, since the session's methods cannot see it.
func mapErr(sess sqlate.Session, err error) error {
	if err == nil {
		return nil
	}
	if m, ok := sess.(sqlate.ErrorMapper); ok {
		return m.MapError(err)
	}
	return err
}
