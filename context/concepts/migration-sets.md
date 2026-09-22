# Migration sets as layers

The model `blobfs.sources` promotes into `sqlate`: what a library that ships its own schema
guarantees, the multi-set migrator `sqlate` gains, the hooks `sqlate` needs for it, where
`blobfs.experiment`'s own shim goes, what `go-database`'s admin surface exposes over it, and how a
consumer adopts a shipped set. It concerns four repositories — `sqlate`, `go-database`, the
shippers `blobfs` and `go-auth`, and every consumer of a shipped set — and it is written at full
detail because `blobfs.sources` is the roadmap's next task. It excludes the contents of `blobfs`'s
own set, which is `blobfs.md`'s subject, and every `sqlate` adjustment outside the migrate group,
which is `sqlate-library-support.md`'s.

## A set is a layer

A migration set is a self-contained layer: a `Set` value a library ships whole, carrying its name,
its history table, and its migrations in version order, with the file naming (`NNNN_name.up.sql`
and `.down.sql`) unchanged and the version sequence private to the set. Sets are declared
bottom-first by the consumer that adopts them. `Up` runs every set's pending migrations in that
declared order; `Down` and `Reset` run in the reverse order, so a consumer's own foreign key into a
lower set's table never blocks that set's revert. The verbs that take a set name — `Down`, `Force`,
and reading a set's own status — default to the top layer when none is named. `Down` of one set is
refused up front, before anything runs, while any set above it still has applied migrations, so a
run never fails partway through with a dependent-objects error; no explicit `Requires` declaration
between sets is needed for this, since declared order already states it.

## What a shipper guarantees

A library that ships a set owns every object the set creates, every object's name starting with the
set's own name and an underscore. It never references a consumer's objects — a shipper's migrations
reference nothing outside what the shipper itself created. A released migration's text and name
never change once shipped, and each migration ships its own down, so a consumer's revert always has
a path back; a change to already-seeded data is a new migration, never an amendment to a released
one. A golden-hash test in the shipping library's own repository pins every released migration's
text, so an accidental in-place edit is caught at the source, not by a consumer's migrator, which
carries no checksum column of its own. A breaking schema change is a major release of the shipping
library. No Postgres schema per set: `sqlate`'s migration catalog targets engines where a schema is
a whole separate database, so a schema-qualified statement would not be portable and a foreign key
across schemas would fail. `blobfs` is the first shipper of a set under this model; `go-auth` is the
second.

## The migrator in `sqlate`

The promoted shape is one `Migrator` over several `Set` values,
`migrate.New(db, []migrate.Set{...}, opts)`, offering `Up`, `Down`, `Reset`, `Status`, and `Force`.
The default set keeps the existing history table name, so a database installed under the
single-set `migrate` never needs a history migration of its own to adopt the multi-set form. The
whole run takes one lock, on one connection the run pins for its duration (see "The hooks `sqlate`
needs" for what closes today's two-connection requirement). Before any set runs, every set's history
is checked against its own migrations; a dirty set, or a history that does not match what the
running binary expects, refuses the whole run and names the set at fault, so an older binary
started against a newer database's history fails cleanly at start rather than partway through.
`Reset` reverts every set and then drops every set's history table; `Down` reverts without dropping
the tables, which is the record that the sets were reverted. `Force` on one set is the operator's
repair after a failed migration is fixed by hand — it states the version explicitly rather than
guessing at it, which is why the admin surface offers `force` on a set as a distinct verb rather
than a flag on `Reset`: a flag that cleared the dirty mark and tried the revert automatically would
have to guess whether the failed migration's own objects still exist, and stating the version does
not. `Status` reads every set's head, latest, pending migrations, and dirty mark without taking the
lock, in one read per set rather than the several separate reads a naive implementation costs.

## The hooks `sqlate` needs

The published `migrate` package (v0.1.1) needs a handful of additions to host this model cleanly,
each a small, well-bounded change: export the default history-table name or add an accessor for it,
so a shim built on top does not repeat a private constant; let a migrator run its set on a
connection the caller already holds, rather than requiring a connection of its own, which is what
closes the two-connection requirement; add an operation that drops a set's own history table,
beside the existing operator-repair operation; document that a caller holding its own lock, on a
dialect without the lock capability, is the intended shape of the unlocked option that already
exists; state as a guarantee, in documentation, that reverting more migrations than are applied is
safe and is how a whole set is reverted; and map the two Postgres error classes a multi-set
migrator's own tests already need — one for a dependent-objects failure during a revert in the
wrong order, and one for a serialization failure, since serializable isolation is the standard-tier
alternative to an engine-native lock elsewhere in this workspace's own use of `sqlate`. One further
correction worth recording here: multi-statement transactional migrations already work against
Postgres on the published driver's simple protocol, so v0.1.1 can already host one `Migrator` per
set, each with its own table — a shim built that way needs nothing new from `sqlate` to exist; the
hooks above are what let such a shim disappear into `sqlate` itself, not what let it run at all.

## Where the experiment's shim goes

`blobfs.experiment` built a working shim over the published API, one file over `migrate`,
repeating only one constant and one statement `migrate` does not itself expose. It is well-shaped
for direct absorption: it needed nothing outside the public API, and its own engine-level tests —
a fresh replay, an upgrade rehearsal over an installed database, revert order across a foreign key
between two sets, two starters serializing under the lock, and refusal on a dirty set — are the
shim's specification, portable into `sqlate`'s own test suite largely unchanged. The shim is not
promoted with `blobfs`: a migrator that runs several sets is a property of the migration library
itself, not of whichever library happens to ship the first set, `go-auth` is already named as the
second shipper, and `go-database`'s admin service needs to take the multi-set migrator by its own
type, which it can only do if that type lives in `sqlate`.

Sequencing consequence: `blobfs.sources` ships before `blobfs.build` starts, or `blobfs.build`
takes the fallback named in `blobfs.md` and a consumer assembles one `Migrator` per set by hand
until `sources` lands. The second path costs the first real consumer more than it saves and is
taken only if `sources` is genuinely blocked.

## What the admin surface exposes

`go-database`'s admin release, `blobfs.admin`, exposes `status` (one row per set: its table, applied
version, latest version, pending migrations, and dirty mark), `up`, `down`, `reset` behind an
explicit confirmation, and `force <set> <version>` for dirty recovery. Every verb that names a
target takes a set name. `reset` and `status` cover every set in one call each; `up` at start
reports the pending migrations by set, so an operator sees exactly what a start will apply before
it runs. A revert refused partway through, in the wrong declared order, leaves some sets reverted
and others not; `status` afterward is what tells the operator exactly where each set stands, since
the refusal itself only names the set it stopped at.

## How a consumer adopts a set

A consumer with its own numbered migrations adopts a shipped set in four steps: register the
shipper's published patterns in its own catalog, build one migrator over the shipper's set followed
by its own, add the shipper's statements to its own verify stage, and write its own migrations
referencing the shipper's tables, which is safe once the shipper's set is declared ahead of the
consumer's own. The shipper's set is always at its head before the consumer's own migrations run.
Starting several replicas of a consumer at once serializes them on the migrator's one lock, and
every one ends at the same head. An upgrade is a `go.mod` bump: the next start finds the shipper's
new pending migration and applies only it, leaving the consumer's own rows untouched. A consumer on
a different migration tool entirely takes only the shipper's root and persistence layers and
authors its own DDL from the documented schema.

## Assumptions

- The shim's shape moves into `sqlate` largely unchanged apart from the hooks above.
- One connection, held for the run's duration, suffices once a set's inner migrator can run on it.
- `go-auth`'s own set fits the same model with no further change to it.
- No migration-text checksum column is needed until a golden test somewhere misses an amended
  release; the golden-hash test at the shipping library is the defense until then.
