# Composing `blobfs` into a consumer

How a service builds around the `blobfs` library once it exists: one install per configuration, the
composition root, ownership at two grains, the write, delete, and move protocols as the consumer
sequences them, seeding, and the operating constraints `blobfs.experiment`'s evidence established.
It excludes the library's own protocol semantics and listing conventions, which are `blobfs.md`'s
subject, the multi-set migrator's mechanics, which are `migration-sets.md`'s, and authorization,
which waits for `go-auth` (`auth-strategy.md` §8). Its worked example is the organization: an image
at the file grain, a document hierarchy at the directory grain.

## One install per configuration

An install is one database and one container, with fixed object names the library owns. Nothing in
the library or a consumer's own code names a second database or container: no volume id, no key
prefix, no schema qualifier. A service that serves several isolated trees runs one configuration per
tree — its own database and its own container — rather than segregating trees inside one of either.
A container-level operation, such as removing one, then belongs to exactly one tree by
construction, even though the object keys themselves (`<file id>/<name>`) carry no mark of the
configuration and two configurations sharing one container would not literally collide.

## The composition root

A consumer builds one pattern catalog for its program, combining `sqlate`'s own patterns with the
library's published namespace, and compiles the library's statements against it along with its own.
The engine is chosen by importing the engine package and passing its constructor as an option to the
library's own constructor (`blobfs.md`, "Layers and the engine package"); nothing else in the
consumer names the engine package.

The object-store adapter is the one place a consumer names its object-store library. It is the
library's key-validation interface, built from a store the consumer has already started under its
own lifecycle and handed to the library's constructor, so the library never reads an environment
variable of its own. The adapter's own job, learned from building one: a missing object on delete is
success, the same way the library's own delete step treats it, but a missing *container* is not — it
means the configured target itself is gone, and the object may well exist somewhere else, so the
adapter refuses the step rather than treating it as done. A put should echo the content type the
caller declared, since a store's own report of the content type may differ once written. A store
that fails to start, credentials rejected included, should be reported as one unavailability
condition, not several.

## Ownership at two grains

A consumer composes ownership through its own tables; the library holds no owner and no unit
anywhere. Two grains cover what a document hierarchy needs.

**The directory grain.** An owner row binds one top-level directory to a unit. A scoped listing
resolves that top-level ancestor of the path it was asked for, reads the owner row once, and refuses
before the rest of the path resolves if the unit does not own it; the library's own listing
statements carry no ownership predicate and run unchanged beneath the check. At the root, the scope
is the unit's own top-level directories, read through the consumer's own projection over the owner
table; a file stored at the root belongs to no unit, since it has no top-level ancestor to be
scoped by. Checking a client-named scope by id (rather than by a path already known to be inside
it) reads the owner row of the named scope directory first and only then asks the library whether
the target lies within it, so a unit that names a directory it does not own learns nothing about
what is under it — the supplied id is an input to check, never a fact to trust. Removing an owned
directory removes its owner row in the same transaction, since the owner row is the consumer's own
record of the directory and has no life apart from it; refusing to remove an owned directory instead
would make it permanently undeletable.

A move stays under one top-level directory. The owner row binds a top-level directory and the scope
is checked only at that ancestor, so a move across two top-level directories would carry an entry
from one unit's scope into another's without either unit's say; a rename of a top-level directory is
allowed, since it changes no depth and no owner row moves. A document moved from one organization's
tree to another's is therefore a copy and a delete, not a move.

**The file grain.** A join row binds one file to a unit, and a partial unique index enforces at most
one active row per unit — the shape an organization's single active logo needs. The library's
constraints reach the consumer as its own sentinels through the same constraint-to-sentinel mapping
the library uses for its own violations, so a second active row, a file already bound, or a bound
file that was deleted meanwhile are each a distinct, classified error a handler turns into a
response. The listing of a unit's own bound files computes each row's path only when the caller
actually asks for it, since the recursion that computes a path costs real buffers per row and most
listings of this kind don't display it.

## The protocols as the service sequences them

**The upload.** In one transaction: resolve the parent, begin the file's write, and write the
consumer's own row (the owner or the join row) beside it, so the pending row and the consumer's
record of it commit together before any byte reaches the store. Outside any transaction: upload
under the row's key. On the pool: complete the write with what the store reported. A `pending` row
is the consumer's own state to sweep — there is no failed status, a retry of the same upload resumes
the row, and an abandoned one is removed through the delete steps. A sweeper, when one is built,
lists pending rows older than a threshold through the library's own listing filters and deletes
each.

**The delete.** In one transaction: hold the file (the reference-then-delete rule) if any of the
consumer's own rows are about to reference it, or check the consumer's own referencing rows and
refuse while any exist, then begin the library's delete. Outside any transaction: delete the object.
On the pool: complete the delete. The consumer's own foreign key into the library's file table is
the backstop for a reference that slips in after the check, and the consumer maps that constraint's
name to its own sentinel at the complete step. A directory removal removes the consumer's own rows
about that directory in the same transaction as the library's removal, and a recursive removal is
the consumer's own walk, children before their parent.

**The move.** Resolve both paths and run the library's move in one transaction; a directory move
takes the engine's lock through the variant. On an engine with no native lock, the consumer
serializes directory moves itself — one mover per process, or every moving transaction at
serializable isolation with a retry on the driver's serialization-failure class.

## Seeding

A service seeds named states through the library's insert-or-find operations, never by hand-rolled
lookups. The root directory is structural, created by the schema itself, and every reset of the
schema recreates it; named states beyond the root are the consumer's own, seeded through the
directory insert-or-find (with the consumer's own owner row in the same transaction) and the file
insert-or-resume operation, over a bytes source such as an embedded fixture. A seeder is idempotent
by name: an available file is skipped, a pending one resumes, and the object store must already be
started before the seed step runs. Caller-supplied ids on a seeded row keep its key stable across
resets, so a reseed overwrites the same object rather than orphaning the old one; a reset otherwise
leaves orphaned objects in the store, which is accepted in development.

## Registering the migration set

A consumer declares the library's shipped set in its own multi-set migrator, ahead of its own
migrations; `migration-sets.md` covers the migrator's mechanics and a consumer's adoption steps in
full and is not restated here.

## Operating constraints

- **A correlated path recursion crosses a JIT threshold at volume.** A consumer whose read model
  computes a path per row through a correlated recursion should expect the planner's JIT compiler
  to trigger once a unit's row count crosses roughly a hundred and forty, which costs more than the
  walk itself; sorting by an indexed key instead of relying on the recursion's own order, or
  lowering the JIT threshold for the session, avoids it. The evidence section of
  [spike-blobfs](https://github.com/JaimeStill/spike-blobfs)'s `REVIEW.md` has the measured
  numbers.
- **An exact total reads the whole directory.** The window count that produces an exact total costs
  in proportion to the directory's own size, never the whole tree, but a directory with tens of
  thousands of files should read its total once, on the first page, and walk the rest by cursor.
- **The baseline engine has no tree lock.** A consumer on an engine without a native one must
  serialize directory moves itself, as above.
- **The migrator needs two pool connections** until `blobfs.sources` lands; `migration-sets.md`
  covers this.
- **A recursive removal is not atomic** and can loop under sustained concurrent writes; the
  library's own walk bounds its retries and reports a busy error rather than looping forever.

## Assumptions

- The `org_image`-style composition — a partial unique index enforcing one active row — generalizes
  to a second domain once one exists.
- A service's error handler maps the library's violation type and its referenced-row sentinel to
  conflict responses.
- The composition root can hand an already-started object store to the adapter and let the adapter
  own no lifecycle of its own.
- A sweeper, when built, needs nothing beyond the library's own listing filters.
