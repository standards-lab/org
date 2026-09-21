// Package migrator runs several migration sets against one database, each
// set under its own history table, in the order the consumer declares them,
// under one lock for the whole run. It is the multi-set migrator the blobfs
// concept describes: a library ships its schema as a migration source (a
// named set with its own version line and history table), and a service
// adds that source to its one migrator ahead of its own set.
//
// # What a consumer passes and receives
//
// A consumer passes New the session over its database, the sets in
// canonical order, and the options. Each Set is a source's name, the
// history table its migrations are recorded in (empty for sqlate's
// default, schema_version), and the source's migrations in version order,
// as migrate.Files reads them. A consumer declares blobfs's set first and
// its own set last, so blobfs's schema is at its head before the
// consumer's migrations reference it. The consumer receives a Migrator
// with five operations:
//
//   - Up applies every pending migration of every set, sets in declared
//     order.
//   - Down reverts every applied migration of every set, sets in reverse
//     declared order, so a consumer's foreign key into a blobfs table never
//     blocks blobfs's down. The history tables stay, empty: they record
//     that the sets were reverted.
//   - Reset is Down followed by dropping every history table, so the
//     database returns to its state before the first Up.
//   - Status reads each set's head, latest, pending migrations, and dirty
//     mark, without the lock.
//   - Force sets one set's history to a version as an operator override,
//     which is how a dirty set is repaired after its schema is fixed by
//     hand.
//
// # The lock and the dirty check
//
// Up, Down, Reset, and Force pin one connection from the pool, take the
// dialect's session-scoped lock on it (sqlate.Locker, under
// Options.LockName), and hold it for the whole run. Every set's inner
// migrate.Migrator is built with migrate.Options.Unlocked, so it takes no
// lock of its own, and the sets run one after another under the outer
// lock. Two processes that start at the same time therefore serialize on
// the outer lock, and the second finds every set at its head. A dialect
// without the lock capability is migrate.ErrNoLocker unless
// Options.Unlocked is set, in which case concurrent starters are unsafe.
//
// Under the lock, before any set runs, every set's history is checked
// against its migrations. A dirty set, one whose last non-transactional
// migration failed midway, or a history that does not match the set,
// refuses the run with a SetError that names the set and wraps the inner
// migrator's error (migrate.ErrDirty or migrate.ErrUnknownVersion), and
// no set runs. The check covers every set, so a dirty second set stops
// the first from applying.
//
// # The shim and what it needs from sqlate
//
// This is a shim over published sqlate v0.1.1: one migrate.Migrator per
// set, each with its own table, unlocked, under the shim's outer lock. It
// imports only sqlate and the standard library. The shim repeats sqlate's
// default table name, because migrate does not export it, and runs the
// history tables' DROP TABLE itself, because migrate offers no operation
// that removes its table. NOTES.md records the hooks that let the shim
// move into sqlate.
package migrator
