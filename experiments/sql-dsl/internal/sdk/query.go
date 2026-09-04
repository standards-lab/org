package sdk

import (
	"maps"
	"slices"

	"github.com/standards-lab/go-web-sdk"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/query"
)

// Directives lowers a parsed read request onto the query vocabulary: page
// and sorts as given, and every remaining parameter an exact-match filter
// on a contract field, its value the request's text for the engine to type
// by the field's declared type. Filters iterate in name order so the
// composed WHERE is deterministic.
func Directives(q web.Query) query.Directives {
	d := query.Directives{Page: query.Page{Number: q.Page, Size: q.Size}}
	for _, s := range q.Sort {
		d.Sort = append(d.Sort, query.Sort{Field: s.Field, Descending: s.Descending})
	}
	for _, field := range slices.Sorted(maps.Keys(q.Filters)) {
		d.Filters = append(d.Filters, query.Filter{Field: field, Op: query.OpEq, Value: q.Filters.Get(field)})
	}
	return d
}
