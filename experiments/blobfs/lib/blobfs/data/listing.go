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
//
// A page is reached by its number (offset paging) or by a cursor (keyset
// paging, see cursor.go). Both walk the same order, so the two return the
// same rows over the same data. The composer fetches one row beyond the
// page whenever the sort can be continued by a cursor, so it knows
// whether a next page exists without a count, and fills Page.Next with
// the cursor of the row after the page's last row.

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
// terms over the statement's declared fields, the page size, and either
// the 1-based page number or a cursor. Filters and Sort reuse the query
// library's declarations, and an unknown field, an unknown operator, or a
// value of the wrong shape is refused with the library's own error types,
// each of which unwraps to query.ErrDirectives.
//
// The sort is the caller's terms followed by the key, name, as the
// tie-breaker when the caller did not name it; the tie-breaker takes the
// terms' direction when they share one, so a descending sort is the exact
// reverse of the ascending one. A term after the key orders nothing, since
// the key is unique within a listing.
//
// After, when not empty, is the Next of an earlier page of the same
// listing under the same sort, and the page is the rows after that page's
// last row. A cursor page ignores Page (it may be zero) and carries no
// total whatever Total says, because a total computed under the cursor's
// predicate would count the rows after the cursor and not the listing; a
// caller that wants the total reads an offset page under TotalExact. A
// cursor is refused, with a CursorError, when it is malformed or edited,
// was issued by the other listing or under another sort, or when the sort
// cannot be continued: a sort whose terms up to the key mix directions,
// or one naming a field that can be NULL (size, etag, parent_id). Such a
// sort still pages by number, and its pages carry no Next.
type Listing struct {
	Filters []query.Filter
	Sort    []query.Sort
	Page    int
	Size    int
	After   string
	Total   TotalMode
}

// Page is one page of a listing: its rows, never more than the requested
// size; its total, NoTotal when the page carries none; and Next, the
// cursor of the following page, empty on the last page and for a sort a
// cursor cannot continue. Next is filled for an offset page too, so a
// caller can read page one with its total and then walk by cursor.
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
	clauses  clauses
	plain    query.Statement
	counted  query.Statement
	key      string
	fields   map[string]query.Field
	order    []query.Field
	nullable map[string]bool
}

// newListing binds the two statements of one listing and checks they
// agree: both declare the key and the same fields, and both bind the same
// parameters.
func newListing[T any](c clauses, plain, counted query.Statement) (listing[T], error) {
	l := listing[T]{clauses: c, plain: plain, counted: counted, key: plain.Key(), order: plain.Fields(), fields: map[string]query.Field{}}
	l.nullable = nullableOf[T](l.key)
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

// run composes and runs one page: the statement chosen by the total mode
// (the plain one under a cursor, which carries no total), the listing's
// own arguments, and the caller's directives. When the sort can be
// continued by a cursor, one row beyond the page is fetched and dropped,
// and its presence fills Next with the cursor of the page's last row.
func (l listing[T]) run(ctx context.Context, sess sqlate.Session, args query.Args, d Listing) (Page[T], error) {
	var after *cursor
	if d.After != "" {
		c, err := decode(d.After)
		if err != nil {
			return Page[T]{}, err
		}
		after = &c
	}
	st, counted := l.plain, false
	if d.Total == TotalExact && after == nil {
		st, counted = l.counted, true
	}
	p, err := l.compose(st, args, d, after)
	if err != nil {
		return Page[T]{}, err
	}
	rows, err := sess.QueryContext(ctx, p.text, p.values...)
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
	if p.terms != nil && len(page.Rows) > d.Size {
		page.Rows = page.Rows[:d.Size]
		values, err := valuesOf(page.Rows[d.Size-1], p.terms)
		if err != nil {
			return Page[T]{}, err
		}
		if page.Next, err = encode(cursor{Listing: l.plain.Name(), Sort: p.terms, Values: values}); err != nil {
			return Page[T]{}, err
		}
	}
	if counted && len(page.Rows) == 0 && d.Page == 1 {
		page.Total = 0
	}
	return page, nil
}

// plan is one composed page statement: its text, the values to bind, and
// the cursor terms when the sort can be continued by a cursor (nil when it
// cannot), in which case the text fetches one row beyond the page.
type plan struct {
	text   string
	values []any
	terms  []term
}

// compose renders st with d's clauses appended and returns the text and
// the values to bind: st's own arguments in its parameter order, then each
// filter value, then the cursor's values, then the offset and the fetch
// count. The directives are checked before any text is composed: a bad
// page, an unknown field or operator, a malformed value, and a cursor
// that does not continue this sort are refused here, and the engine never
// sees them. after, when not nil, is the decoded cursor the page continues
// from; the page number is then ignored.
func (l listing[T]) compose(st query.Statement, args query.Args, d Listing, after *cursor) (plan, error) {
	if after == nil && d.Page < 1 {
		return plan{}, fmt.Errorf("%w: page number must be at least 1", query.ErrDirectives)
	}
	if d.Size < 1 {
		return plan{}, fmt.Errorf("%w: page size must be at least 1", query.ErrDirectives)
	}
	var values []any
	for _, name := range st.Params() {
		v, ok := args[name]
		if !ok {
			return plan{}, &query.ArgumentError{Statement: st.Name(), Name: name}
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
			return plan{}, &query.UnknownFieldError{Field: f.Field, Use: query.FieldUseFilter}
		}
		fill := map[string]string{"field": field.Name}
		switch f.Op {
		case query.OpEq, query.OpNe, query.OpGt, query.OpGe, query.OpLt, query.OpLe, query.OpLike:
			fill["value"] = value(field, f.Value)
		case query.OpIsNull, query.OpIsNotNull:
		case query.OpIn:
			vals, ok := f.Value.([]any)
			if !ok || len(vals) == 0 {
				return plan{}, &query.InvalidValueError{Field: f.Field, Err: errors.New("an in filter takes a non-empty []any")}
			}
			parts := make([]string, len(vals))
			for i, v := range vals {
				parts[i] = value(field, v)
			}
			fill["values"] = strings.Join(parts, ", ")
		default:
			return plan{}, &query.UnknownOperatorError{Op: f.Op}
		}
		text.WriteString(" AND ")
		text.WriteString(l.clauses.fill("filter_"+string(f.Op), fill))
	}

	order, keyed, err := l.orderTerms(d.Sort)
	if err != nil {
		return plan{}, err
	}
	terms, reason := l.cursorable(keyed)
	if after != nil {
		if terms == nil {
			return plan{}, &CursorError{Reason: reason}
		}
		if err := l.check(*after, terms); err != nil {
			return plan{}, err
		}
		text.WriteString(" AND ")
		text.WriteString(l.keyset(terms, after.Values, value))
	}

	rendered := make([]string, len(order))
	for i, t := range order {
		name := "order_term"
		if t.Descending {
			name = "order_term_desc"
		}
		rendered[i] = l.clauses.fill(name, map[string]string{"field": t.Field})
	}
	text.WriteString(l.clauses.fill("order", map[string]string{"terms": strings.Join(rendered, ", ")}))

	offset, fetch := 0, d.Size
	if after == nil {
		offset = (d.Page - 1) * d.Size
	}
	if terms != nil {
		fetch++
	}
	text.WriteString(l.clauses.fill("paging", map[string]string{
		"offset": l.clauses.placeholder(len(values) + 1),
		"fetch":  l.clauses.placeholder(len(values) + 2),
	}))
	values = append(values, offset, fetch)
	return plan{text: text.String(), values: values, terms: terms}, nil
}

// orderTerms resolves the caller's sort terms against the declared fields
// and appends the key as the tie-breaker when the caller did not name it,
// in the terms' shared direction (ascending when they share none or there
// are none). It returns the full ORDER BY list and the prefix up to and
// including the key, which is the order a cursor continues: a term after
// the key orders nothing.
func (l listing[T]) orderTerms(sort []query.Sort) (order, keyed []term, err error) {
	order = make([]term, 0, len(sort)+1)
	keyAt := -1
	for i, s := range sort {
		if _, ok := l.fields[s.Field]; !ok {
			return nil, nil, &query.UnknownFieldError{Field: s.Field, Use: query.FieldUseSort}
		}
		order = append(order, term{Field: s.Field, Descending: s.Descending})
		if s.Field == l.key && keyAt < 0 {
			keyAt = i
		}
	}
	if keyAt < 0 {
		descending := len(order) > 0
		for _, t := range order {
			descending = descending && t.Descending
		}
		order = append(order, term{Field: l.key, Descending: descending})
		keyAt = len(order) - 1
	}
	return order, order[:keyAt+1], nil
}

// cursorable reports whether a cursor can continue the sort keyed: the
// terms are returned when it can, and nil with the reason when the terms
// mix directions or name a field that can be NULL.
func (l listing[T]) cursorable(keyed []term) ([]term, string) {
	for _, t := range keyed {
		if l.nullable[t.Field] {
			return nil, fmt.Sprintf("%s can be NULL, and a cursor cannot continue a sort by it; page by number instead", t.Field)
		}
		if t.Descending != keyed[0].Descending {
			return nil, fmt.Sprintf("the sort %s mixes directions, and a cursor continues one direction only", spell(keyed))
		}
	}
	return keyed, ""
}

// keyset renders the cursor predicate over terms with values: for one term
// q.f > v (or < under a descending sort), and for several the expanded
// form (a > x) OR (a = x AND b > y) OR ..., each comparison and each cast
// spelled by the query library's own patterns. Every occurrence of a
// value binds its own placeholder, so the text holds for a dialect whose
// placeholders cannot be repeated.
func (l listing[T]) keyset(terms []term, values []string, value func(query.Field, any) string) string {
	after := "filter_gt"
	if terms[0].Descending {
		after = "filter_lt"
	}
	compare := func(name string, i int) string {
		return l.clauses.fill(name, map[string]string{"field": terms[i].Field, "value": value(l.fields[terms[i].Field], values[i])})
	}
	if len(terms) == 1 {
		return compare(after, 0)
	}
	disjuncts := make([]string, len(terms))
	for i := range terms {
		conjuncts := make([]string, 0, i+1)
		for j := range i {
			conjuncts = append(conjuncts, compare("filter_eq", j))
		}
		conjuncts = append(conjuncts, compare(after, i))
		disjuncts[i] = strings.Join(conjuncts, " AND ")
		if i > 0 {
			disjuncts[i] = "(" + disjuncts[i] + ")"
		}
	}
	return "(" + strings.Join(disjuncts, " OR ") + ")"
}

// rendering is one canonical rendering Verify prepares: a statement, the
// directives to compose onto it, and the cursor to continue from, if any.
type rendering struct {
	st    query.Statement
	d     Listing
	after *cursor
}

// Verify prepares canonical renderings of the listing's statements: for
// each of the two, a predicate on every declared field, a sort on every
// field, and the paging clause; and for the plain one, a cursor page under
// a sort by every field a cursor can continue, so the keyset predicate
// prepares over each of their types. A field the table no longer has thus
// fails at startup and not at the first request that names it.
func (l listing[T]) Verify(ctx context.Context, db sqlate.Session) error {
	d := Listing{Page: 1, Size: 1}
	continued := Listing{Page: 1, Size: 1}
	for _, f := range l.order {
		d.Filters = append(d.Filters, query.Filter{Field: f.Name, Op: query.OpIsNotNull})
		d.Sort = append(d.Sort, query.Sort{Field: f.Name})
		if !l.nullable[f.Name] {
			continued.Sort = append(continued.Sort, query.Sort{Field: f.Name})
		}
	}
	renderings := []rendering{{l.plain, d, nil}, {l.counted, d, nil}}
	if _, keyed, err := l.orderTerms(continued.Sort); err == nil {
		c := &cursor{Listing: l.plain.Name(), Sort: keyed, Values: make([]string, len(keyed))}
		for i, t := range keyed {
			c.Values[i] = sampleValue(l.fields[t.Field])
		}
		renderings = append(renderings, rendering{l.plain, continued, c})
	}
	var errs []error
	for _, r := range renderings {
		args := query.Args{}
		for _, name := range r.st.Params() {
			args[name] = nil
		}
		p, err := l.compose(r.st, args, r.d, r.after)
		if err != nil {
			errs = append(errs, fmt.Errorf("data: %s: %w", r.st.Name(), err))
			continue
		}
		stmt, err := db.PrepareContext(ctx, p.text)
		if err != nil {
			errs = append(errs, fmt.Errorf("data: %s: listing contract: %w", r.st.Name(), err))
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
