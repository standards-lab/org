# Migration sets as layers

What a shipped migration set requires of the library that ships it and the consumers that adopt
it, now that `sqlate` hosts the multi-set model itself (`docs/features.md`, "migrate: schema
versioning," in the `sqlate` repository): what a shipper guarantees, what `go-database`'s admin
surface exposes over a shipped set, and how a consumer adopts one. It concerns three
repositories — `go-database`, the shippers `blobfs` and `go-auth`, and every consumer of a
shipped set. It excludes the contents of `blobfs`'s own set, which `blobfs`'s `docs/features.md` documents, and
every `sqlate` adjustment outside the migrate group, which is `sqlate-library-support.md`'s.

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

- `go-auth`'s own set fits the same model with no further change to it.
- No migration-text checksum column is needed until a golden test somewhere misses an amended
  release; the golden-hash test at the shipping library is the defense until then.
