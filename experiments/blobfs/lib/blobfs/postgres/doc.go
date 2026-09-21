// Package postgres is the Postgres engine of blobfs: the Postgres variant
// of the persistence package's variation points, the native-tier
// statements the variant runs, and the DDL of the two tables, exported as
// a migration set. A consumer selects the engine by importing this
// package, as it imports sqlate/postgres for the dialect. There is no
// registry, no init, and no flag; the standard baseline is what data.New
// builds when no engine package is used. The package imports the
// persistence package, the migrator's Set type, and sqlate only; it runs
// plain SQL through the session and names no driver.
//
// # The variant
//
// Variant implements data.Variant over native-tier statements. Its tree
// lock is a transaction-scoped advisory lock (pg_advisory_xact_lock) over
// one fixed key, so two transactions that move directories run one after
// the other and their cycle checks cannot both pass. Its file-delete begin
// is one UPDATE ... RETURNING, so the step is one round trip instead of
// the baseline's update and read-back.
//
// Every statement file declares its tier as native and carries a port
// note: the engine feature it uses and what a port to another engine must
// provide. A consumer composes the variant at its composition root:
//
//	v, err := postgres.New(catalog, dialect)
//	store, err := data.New(catalog, dialect, data.WithVariant(v))
//
// and the store then forwards LockTree, Serializes, and BeginFileDelete
// to it. The datatest package's suite proves the variant against the same
// contract the baseline satisfies.
//
// # The migration set
//
// Migrations returns blobfs's migration set: the name Source, the history
// table Table, and two migrations, directory and file, embedded from the
// migrations directory. A consumer adds the set to its migrator ahead of
// its own set, and blobfs's schema is at its head before the consumer's
// migrations reference it.
//
// The directory migration seeds the one root directory, the row with no
// parent, named /, and the id blobfs.RootID, and a partial unique index
// allows no second row without a parent. A check constraint states that a
// directory is named / exactly when it has no parent, so a root under
// another name or a non-root named / is refused.
//
// The set ships no index on blobfs_file (directory_id, created_at). The
// unique constraint on (directory_id, name) orders a sort by name, and a
// sort by created_at without that index sorts the directory: measured at
// 100,000 files with 10,009 in one directory, page 1 sorted by created_at
// without a total reads about 2,400 buffers, and 34 with the index. The
// index cost 3,992 kB at that volume. A consumer that lists by creation
// time adds the index in its own migration set, for example CREATE INDEX
// ix_blobfs_file_directory_created ON blobfs_file (directory_id,
// created_at); the measurement is in evidence/sort-index.txt.
//
// The set owns every object it creates, and every object's name starts
// with the set's name and an underscore: the tables blobfs_directory and
// blobfs_file, their constraints and indexes, and the history table
// blobfs_schema_version. Constraint and index names are public API,
// because a violation reaches a consumer as
// sqlate.ConstraintError.Constraint, and the persistence layer maps
// blobfs's own constraints to its sentinel errors. The scheme is
// blobfs_<kind>_<table>_<detail>, where kind is pk, fk, uq, cc, or ix (a
// plain index), table is the table name without its blobfs_ prefix, and
// detail names the referenced relation, the indexed columns, or the
// checked rule: blobfs_fk_directory_parent,
// blobfs_uq_directory_parent_name, blobfs_cc_directory_root_name.
//
// A released migration file never changes in text or name; the
// golden-hash test in this package pins each one. A second engine is a
// package of its own with the same file names in its own migrations
// directory.
package postgres
