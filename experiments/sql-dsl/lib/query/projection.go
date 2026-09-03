package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

// Page is the 1-based page directive of a read: which page, and how many
// rows per page. Both must be at least 1.
type Page struct {
	Number int
	Size   int
}

// Sort is one sort directive, naming a contract field.
type Sort struct {
	Field      string
	Descending bool
}

// Op is a filter directive's operator. The values are short strings so a
// read contract can carry them verbatim.
type Op string

const (
	OpEq        Op = "eq"
	OpNe        Op = "ne"
	OpGt        Op = "gt"
	OpGe        Op = "ge"
	OpLt        Op = "lt"
	OpLe        Op = "le"
	OpLike      Op = "like"
	OpIsNull    Op = "null"
	OpIsNotNull Op = "notnull"
	OpIn        Op = "in"
)

// Filter is one filter directive: a contract field, an operator, and the
// value. OpIsNull and OpIsNotNull ignore Value; OpIn requires a []any. A
// value binds as given — a request's text included — cast to the field's
// declared type, so the engine parses it and a value it cannot read is an
// InvalidValueError.
type Filter struct {
	Field string
	Op    Op
	Value any
}

// Directives is one read request against a projection. Filters and Sort
// reference contract field names; an unknown name is an UnknownFieldError,
// never SQL.
type Directives struct {
	Page    Page
	Sort    []Sort
	Filters []Filter
}

// Pager is the dialect capability rendering the paging fragment for an
// engine without OFFSET … FETCH (MySQL, SQLite); offset and fetch are the
// bound placeholders. The standard form otherwise.
type Pager interface {
	Paging(offset, fetch string) string
}

// Projection is a base statement bound to a scan function and its declared
// field contract: the typed handle for a collection read. The collection
// frame wraps the base as a derived table, so the base may be any query —
// a recursive CTE, a join tree — and the only names a directive can
// reference are the base's output columns the header declared.
//
// The frame's text lives here for now, as the baseline the frame catalog
// is judged against (stage 10): the end state is every frame and fragment
// an authored .sql file the library ships, slots in the {{ }} syntax, with
// Go holding only what cannot be text — the whitelist check, list arity,
// parameter positions.
type Projection[T any] struct {
	base   Statement
	scan   ScanFunc[T]
	fields map[string]Field
}

// Project binds base to scan. A base without a key or field contract, or
// one that binds parameters of its own, is a wiring defect and panics.
func Project[T any](base Statement, scan ScanFunc[T]) Projection[T] {
	if base.key == "" || len(base.fields) == 0 {
		panic(fmt.Sprintf("query: %s: a projection base declares a key and its fields", base.name))
	}
	if len(base.params) != 0 {
		panic(fmt.Sprintf("query: %s: a projection base binds no parameters of its own", base.name))
	}
	fields := make(map[string]Field, len(base.fields))
	for _, f := range base.fields {
		fields[f.Name] = f
	}
	return Projection[T]{base: base, scan: scan, fields: fields}
}

// Statement returns the base.
func (p Projection[T]) Statement() Statement { return p.base }

// List runs the collection read: the page under the directives, and the
// total under the same filters. Sorts gain the key as the tie-breaker
// whenever the caller did not sort by it, so offset paging is stable.
func (p Projection[T]) List(ctx context.Context, s sqldb.Session, d Directives) ([]T, int, error) {
	if d.Page.Number < 1 {
		return nil, 0, fmt.Errorf("%w: page number must be at least 1", ErrDirectives)
	}
	if d.Page.Size < 1 {
		return nil, 0, fmt.Errorf("%w: page size must be at least 1", ErrDirectives)
	}
	where, args, err := p.where(d.Filters)
	if err != nil {
		return nil, 0, err
	}
	order, err := p.order(d.Sort)
	if err != nil {
		return nil, 0, err
	}

	var total int
	rows, err := s.QueryContext(ctx, "SELECT COUNT(*) FROM ("+p.base.text+") q"+where, args...)
	if err != nil {
		return nil, 0, p.engine(err)
	}
	if !rows.Next() {
		_ = rows.Close()
		return nil, 0, errors.New("query: count returned no row")
	}
	if err := rows.Scan(&total); err != nil {
		_ = rows.Close()
		return nil, 0, mapErr(s, err)
	}
	_ = rows.Close()

	offset, fetch := p.base.dialect.Placeholder(len(args)+1), p.base.dialect.Placeholder(len(args)+2)
	paging := " OFFSET " + offset + " ROWS FETCH NEXT " + fetch + " ROWS ONLY"
	if pg, ok := p.base.dialect.(Pager); ok {
		paging = " " + pg.Paging(offset, fetch)
	}
	args = append(args, (d.Page.Number-1)*d.Page.Size, d.Page.Size)
	rows, err = s.QueryContext(ctx, "SELECT * FROM ("+p.base.text+") q"+where+order+paging, args...)
	if err != nil {
		return nil, 0, p.engine(err)
	}
	defer func() { _ = rows.Close() }()
	var out []T
	for rows.Next() {
		v, err := p.scan(rows)
		if err != nil {
			return nil, 0, mapErr(s, err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, mapErr(s, err)
	}
	return out, total, nil
}

// One runs the single-row read: the base under one equality filter on a
// contract field. No row is sql.ErrNoRows; when the field is not unique the
// first row wins.
func (p Projection[T]) One(ctx context.Context, s sqldb.Session, field string, value any) (T, error) {
	var zero T
	where, args, err := p.where([]Filter{{Field: field, Op: OpEq, Value: value}})
	if err != nil {
		return zero, err
	}
	rows, err := s.QueryContext(ctx, "SELECT * FROM ("+p.base.text+") q"+where, args...)
	if err != nil {
		return zero, p.engine(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return zero, mapErr(s, err)
		}
		return zero, sql.ErrNoRows
	}
	v, err := p.scan(rows)
	if err != nil {
		return zero, mapErr(s, err)
	}
	return v, mapErr(s, rows.Err())
}

// Verify prepares a probe naming every contract field and the key over the
// base, so a field the base no longer outputs fails at startup.
func (p Projection[T]) Verify(ctx context.Context, db sqldb.Session) error {
	cols := make([]string, 0, len(p.base.fields))
	for _, f := range p.base.fields {
		cols = append(cols, "q."+f.Name)
	}
	stmt, err := db.PrepareContext(ctx, "SELECT "+strings.Join(cols, ", ")+" FROM ("+p.base.text+") q")
	if err != nil {
		return fmt.Errorf("query: %s: field contract: %w", p.base.name, err)
	}
	return stmt.Close()
}

// where lowers the filters to the shared WHERE clause and its arguments,
// each value cast to its field's declared type.
func (p Projection[T]) where(filters []Filter) (string, []any, error) {
	if len(filters) == 0 {
		return "", nil, nil
	}
	var b strings.Builder
	var args []any
	b.WriteString(" WHERE ")
	for i, f := range filters {
		field, ok := p.fields[f.Field]
		if !ok {
			return "", nil, &UnknownFieldError{Field: f.Field, Use: FieldUseFilter}
		}
		if i > 0 {
			b.WriteString(" AND ")
		}
		b.WriteString("q.")
		b.WriteString(field.Name)
		bind := func(v any) {
			args = append(args, v)
			b.WriteString("CAST(")
			b.WriteString(p.base.dialect.Placeholder(len(args)))
			b.WriteString(" AS ")
			b.WriteString(field.Type)
			b.WriteString(")")
		}
		switch f.Op {
		case OpEq, OpNe, OpGt, OpGe, OpLt, OpLe, OpLike:
			b.WriteString(" ")
			b.WriteString(operators[f.Op])
			b.WriteString(" ")
			bind(f.Value)
		case OpIsNull:
			b.WriteString(" IS NULL")
		case OpIsNotNull:
			b.WriteString(" IS NOT NULL")
		case OpIn:
			vals, ok := f.Value.([]any)
			if !ok || len(vals) == 0 {
				return "", nil, &InvalidValueError{Field: f.Field, Err: errors.New("an in filter takes a non-empty []any")}
			}
			b.WriteString(" IN (")
			for j, v := range vals {
				if j > 0 {
					b.WriteString(", ")
				}
				bind(v)
			}
			b.WriteString(")")
		default:
			return "", nil, &UnknownOperatorError{Op: f.Op}
		}
	}
	return b.String(), args, nil
}

var operators = map[Op]string{
	OpEq: "=", OpNe: "<>", OpGt: ">", OpGe: ">=", OpLt: "<", OpLe: "<=", OpLike: "LIKE",
}

// order lowers the sorts to the ORDER BY clause, the key appended as the
// tie-breaker unless it was sorted by.
func (p Projection[T]) order(sorts []Sort) (string, error) {
	terms := make([]string, 0, len(sorts)+1)
	keySorted := false
	for _, s := range sorts {
		field, ok := p.fields[s.Field]
		if !ok {
			return "", &UnknownFieldError{Field: s.Field, Use: FieldUseSort}
		}
		term := "q." + field.Name
		if s.Descending {
			term += " DESC"
		}
		terms = append(terms, term)
		keySorted = keySorted || s.Field == p.base.key
	}
	if !keySorted {
		terms = append(terms, "q."+p.base.key)
	}
	return " ORDER BY " + strings.Join(terms, ", "), nil
}

// engine classifies a query failure: a data exception is the request's
// fault — a value the engine could not read as the field's type — and
// becomes an InvalidValueError; anything else passes through mapped.
func (p Projection[T]) engine(err error) error {
	if errors.Is(err, sqldb.ErrInvalidValue) {
		return &InvalidValueError{Err: err}
	}
	return err
}
