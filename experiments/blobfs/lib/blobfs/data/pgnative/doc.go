// Package pgnative is the Postgres variant of the persistence package's
// two variation points, an implementation of data.Variant over native-tier
// statements. Its tree lock is a transaction-scoped advisory lock
// (pg_advisory_xact_lock) over one fixed key, so two transactions that
// move directories run one after the other and their cycle checks cannot
// both pass. Its file-delete begin is one UPDATE ... RETURNING, so the
// step is one round trip instead of the baseline's update and read-back.
//
// Every statement file declares its tier as native and carries a port
// note: the engine feature it uses and what a port to another engine must
// provide. The package imports the persistence package and sqlate only; it
// runs plain SQL through the session and names no driver.
//
// A consumer composes it at its composition root:
//
//	v, err := pgnative.New(catalog, dialect)
//	store, err := data.New(catalog, dialect, data.WithVariant(v))
//
// and the store then forwards LockTree, Serializes, and BeginFileDelete
// to it. The datatest package's suite proves the variant against the same
// contract the baseline satisfies.
package pgnative
