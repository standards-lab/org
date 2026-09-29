# Composing `blobfs` into a consumer

How a service builds around the `blobfs` library: one install per configuration, the
composition root, ownership at two grains, the write, delete, and move protocols as the consumer
sequences them, seeding, and the operating constraints `blobfs.experiment`'s evidence established.
It excludes the library's own protocol semantics and listing conventions, which `blobfs`'s own
guide documents (`docs/concepts.md` and `docs/features.md` in that repository), the multi-set
migrator's mechanics, which are `migration-sets.md`'s, and authorization, which waits for
`go-auth` (`auth-strategy.md` §8). Its worked example is go-web-service's organization: a logo at
the file grain, a document hierarchy at the directory grain, both built and validated in
`v1.storage.service`.

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
The engine is chosen by importing the engine sub-module and installing its engine on the library's
own constructor, `data.New(catalog, dialect, data.WithEngine(postgres.Engine))`; nothing else in
the consumer names the engine.

The object-store adapter is where the domains' file operations reach the object-store library, so
no domain imports it. It satisfies the library's two one-method interfaces, the key validator each
write's first step takes and the object deleter the sweep takes, built from a store the consumer
has already started under its own lifecycle, so the library never reads an environment variable of
its own. The adapter's own job, learned from building one: a missing object on delete is
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
what is under it — the supplied id is an input to check, never a fact to trust. The owner row's
foreign key into the library's directory table cascades: the owner row is the consumer's own record
of the directory and has no life apart from it, so whatever removes the directory, a plain delete or
the sweep, removes the row with it. The library forbids a cascade only on its own keys, where one
would strand objects; an owner row holds none. A removal hook instead would leak a session type into
the domain, and the library keeps only one hook, so a second directory-grain domain would silently
replace the first.

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

**The upload.** In one transaction: check the scope and begin the file's write, and nothing else —
no consumer row may reference the pending file, since a reference refuses the library's stale
reclaim on every pass and strands the row (the logo's first write did exactly this, found by a
layering review). Outside any transaction: upload under the row's key. On the pool: complete the
write. A put or completion that fails abandons the row through the delete steps, on a context that
outlives the request's cancellation; a completion refused because a sweep reached the row first
deletes the object just stored (the writer rule). A consumer's reference to the file, such as the
logo's join row, is written after completion, in the transaction that holds the file. The service
stages these steps once, as shared protocols in its `data` package (write, ensure, retire, purge,
serve), bound for the library.

**The delete.** In one transaction: hold the file (the reference-then-delete rule) if any of the
consumer's own rows are about to reference it, or check the consumer's own referencing rows and
refuse while any exist, then begin the library's delete. Outside any transaction: delete the object.
On the pool: purge the row. The consumer's own foreign key into the library's file table is
the backstop for a reference that slips in after the check, and the consumer maps that constraint's
name to its own sentinel at the purge step. A recursive removal is the library's mark and sweep: the
request marks the branch deleting at the version it read and answers 202, and the sweep, run as a
background worker woken by the request, on an interval, and once at startup, removes the branch's
objects and rows and reclaims stale pending and deleting rows past an age. The sweep's refusals are
logged and retried, never fatal.

**The move.** Resolve both paths and run the library's move in one transaction; a directory move
takes the engine's lock through the variant. On an engine with no native lock, the consumer
serializes directory moves itself — one mover per process, or every moving transaction at
serializable isolation with a retry on the driver's serialization-failure class.

## Seeding

A service seeds named states through the library's insert-or-find operations, never by hand-rolled
lookups, in two kinds of contribution each domain declares: rows applied in the seeder's one
transaction, and stored files written once it commits, since a file's object is put outside any
transaction. Every seeded row carries a fixed id, so a reseed finds it, a reset writes it again
under the same key (the all-or-nothing put replacing the leftover object), and a row found under
another id — a client's upload of the same name — is not the seed's and is left alone. The object
store must already be started before the seed step runs. A reset otherwise leaves orphaned objects
in the store, accepted in development.

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
  thousands of files should read its total once, on the first page, and walk the rest by cursor with
  `query.TotalNone`.
- **The baseline engine has no tree lock.** A consumer on an engine without a native one must
  serialize directory moves itself, as above.
- **Schema changes and the sweep deadlock.** Reverting a consumer migration that references the
  library's directory table locks the two tables in the opposite order from a sweep's cascading
  removal, and Postgres aborts one side. The service quiesces the sweep around its schema-changing
  admin verbs with a per-process gate; a multi-replica reset would need a database lock the sweep
  also takes.

## Assumptions

- The `org_image`-style composition — a partial unique index enforcing one active row — generalizes
  to a second domain once one exists.
- A service's error handler maps the library's violation type and its referenced-row sentinel to
  conflict responses.
- The composition root can hand an already-started object store to the adapter and let the adapter
  own no lifecycle of its own.
