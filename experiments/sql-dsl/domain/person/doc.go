// Package person is the person domain layer: the stable anchor other
// domains reference, with its record status, its unit membership, and the
// actions that move it, served under /api/people as paginated, filtered,
// sorted reads, three commands, and three actions.
//
// The layout is the organization layer's: sql/ holds one authored file per
// statement, database.go is the SQL client and the sole importer of query,
// entities.go owns the shapes, their validation, and — through their tags —
// the binding and scan contract; service.go maps endpoints to operations;
// handler.go binds the route group. The read model is a plain-table base,
// the common case of the collection frame.
//
// The commands: create (status pending), edit (the descriptive fields and
// email), delete. The actions — activate, deactivate, transfer-unit — each
// read the record's state inside the transaction, check the transition
// rule, and run the guarded update; a disallowed transition is
// ErrTransition (409), a stale version 412. The cross-domain rule that
// blocks deactivation while custody is open belongs to the inventory
// domain and is declared here as an interface only when that domain exists.
package person
