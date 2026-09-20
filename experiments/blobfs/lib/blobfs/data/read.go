package data

import (
	"context"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"
)

// ListIn runs the projection p scoped to the rows whose field equals value:
// it appends an equality filter on field to the caller's directives and
// runs p.List, returning the page and the total under the caller's filters
// and the scope together. It is how a listing is bound to one directory or
// one volume when the base binds no parameters of its own: the scope is a
// filter over a field the base declared. The caller's directives are not
// modified.
//
// A base that did not declare field makes the scope silently vanish, so
// the projection rejects it before composing any SQL with a
// query.UnknownFieldError naming the field. That error unwraps to
// query.ErrDirectives, the sentinel a generic handler maps to a client
// error; a consumer that wants to tell a forgotten filter from a bad
// request matches the type with errors.As.
func ListIn[T any](ctx context.Context, sess sqlate.Session, p query.Projection[T], field string, value any, d query.Directives) ([]T, int, error) {
	filters := make([]query.Filter, 0, len(d.Filters)+1)
	filters = append(filters, d.Filters...)
	d.Filters = append(filters, query.Filter{Field: field, Op: query.OpEq, Value: value})
	return p.List(ctx, sess, d)
}
