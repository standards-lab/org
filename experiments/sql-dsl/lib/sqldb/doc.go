// Package sqldb is the session wrapper: the one surface every statement in
// the experiment runs through. It wraps go-database's pool and transactions
// behind the stdlib method set — ExecContext, QueryContext, PrepareContext —
// maps every driver error through the dialect at that boundary, and owns the
// transaction runner. It exists because go-database v0.3.0's seam cannot be
// changed in place; at promotion its symbols merge into the root database
// package: Session becomes this method set, Begin takes options, Transact
// and ExecTx replace the v0.3.0 ExecTx, and Locker is the dialect capability
// migrate takes.
package sqldb
