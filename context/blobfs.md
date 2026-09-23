# blobfs

A standalone library that owns a SQL-backed virtual-directory and file-metadata layer over any
object store. It sits adjacent to the standard the way `sqlate` does. `go-storage`'s `Client` can
be the store, but `blobfs` never imports `go-storage`. It exists because the `v1.storage`
demonstration layer needs more than one attachment: a browsable document hierarchy under an
organization. Proving that layer correctly means proving directory listing, hierarchy, and a
guarded single-active-record pattern against real storage, and not only a put and a get.

This is a concept, provisional until `blobfs.build`. The spike that exercised it,
`blobfs.experiment`, is closed; its record is `REVIEW.md` in the archived [spike-blobfs](https://github.com/JaimeStill/spike-blobfs)
repository. Four companion documents carry what the spike settled: `blobfs-api.md` proposes the
library's operation set for `blobfs.build`'s own planning, `blobfs-composition.md` states how a
consumer builds around the library, `migration-sets.md` covers the multi-set migrator
`blobfs.sources` promotes into `sqlate`, and `sqlate-library-support.md` is the coordinator's
ledger of what else `sqlate` needs to host a library like this one.

## Position and audience

`blobfs` is for any Go project on an engine it ships DDL for, which is Postgres at v1. That is the
reason it is adjacent to the standard and not a member repository of it. Its repository takes no
`go-` prefix and its `references.toml` entry carries no `standard` key, as `sqlate`'s does. It
cannot be an infrastructure library: that tier is one repository per external technology, named
`go-<technology>` for the technology it presents as a service
(`architecture/principles/repository-topology.md`,
`architecture/standards/go-elemental/principles/topology-and-naming.md`), and `blobfs` presents no
external technology. It composes two technologies it does not own, a SQL database and an object
store.

The name `blobfs` stays. The final module path is checked against `repository-topology.md` when
`blobfs.build` promotes it to its own repository.

### Why a separate library

`dsl-driven-services.md` §2.1 classifies object storage as protocol-driven and SQL as the
organization's one DSL-driven capability. If the virtual-directory metadata lived inside
`go-storage`, that library would own SQL text and schema, duplicating the reason `sqlate` exists.
Keeping `go-storage` protocol-driven (go-storage's `docs/design.md`) and putting the SQL-backed
hierarchy in its own library, consuming `sqlate` the way `go-database` does, keeps the DSL-driven
and protocol-driven boundary intact.

Staging the capability as application code in `go-web-service`'s `sdk` package, the existing
promotion-candidate convention (`domain-architecture.md`), was set aside. The architecture's rule
is that a pattern is proven in application code first and then graduates, and `blobfs` departs from
it on the architect's judgment that its shape is general enough to build as its own repository. The
departure has a cost the `sqlate` precedent does not remove: the `sqlate` experiment
([spike-sql-dsl](https://github.com/JaimeStill/spike-sql-dsl)) redesigned a capability `go-database` had already built,
released, and reviewed against a real consumer, while `blobfs` has no predecessor and no consumer.
`blobfs.experiment` carried the proving burden instead, through a consumer-shaped command-line file
system.

## The library, not the tool

`blobfs.experiment` built two things, and only one is the library: a command-line file system that
proves it, and never the other way around. Nothing the tool needed to prove itself informs the
library's shape. The library's job is narrow: map flat blob objects, which the consumer stores
under opaque keys, onto a virtual tree of directories and file-metadata rows in SQL, one directory
at a time. Directories and files are separate resources, with separate name spaces and separate
operations; the library never calls the object store. It has no recursive or subtree operation — a
subtree search is deferred, with its trigger below — and no command vocabulary: `mkdir`, `rmdir`,
`cp`, `mv`, `rm`, `ls`, filtering, sorting, and cursor flags, and any `id:<uuid>`-style path
parsing are a consumer's own invention. It has no ownership or authorization vocabulary either: no
owner, no unit, no scope check. A consumer decides how to retrieve and layer the library's rows,
in whatever way its own goal calls for, and the library is permissive everywhere a consumer's own
policy might be restrictive — a directory move, for instance, carries no rule about which
directories a caller may move between; a consumer that wants such a rule builds it on top.

## Layers and the engine package

`blobfs` is one module in three layers, ordered by what a consumer accepts. Each layer imports the
one below it, and a consumer adopts as many as it wants.

1. **The root package** is Go only: the entity types, the status vocabulary, key construction, name
   normalization and validation, the table of allowed state transitions, the key-validation
   interface for the object store, and the error types, including a violation type that names a
   sentinel and the constraint that produced it. It imports neither `sqlate` nor `go-storage`. A
   consumer that keeps its own persistence takes only this layer, because the constraint constants
   and the sentinels must sit below the persistence package.
2. **The persistence package** holds the standard-tier SQL statements, the published pattern
   namespace, the listing composer, and the methods that take a `sqlate.Session`. It introduces the
   `sqlate` module dependency, and it is complete alone: any engine `sqlate` has a dialect for runs
   every operation correctly through it, with no engine package at all.
3. **An engine package** is how a consumer opts into an engine's native forms. It is selected by
   importing it — no registry, no init, no flag — and it owns two things: the engine's variant,
   overriding whichever persistence-package operations measurably win from a native form on that
   engine, and the engine's DDL, exported as a whole migration set. The persistence package's
   conformance suite ships beside it, so another package's variant can be checked against the same
   contract, the way `net/nettest` sits beside `net`.

A package splits from its parent by growth or by dependency weight, never by topic alone. This
split is by dependency at the second layer, where the `sqlate` module enters the import graph, and
by an engine's own commitment at the third. A sub-module is rejected for the engine package,
because it imports no driver and adds no dependency weight to isolate. A second engine adds a
package of its own, with the same shape: a variant and its DDL, and the native files' port notes
are its work list.

## The opaque key convention

`go-storage`'s own keys stay opaque (go-storage's `docs/design.md`). Authorization never
parses a key, because `auth-strategy.md` §8 routes it through the owning SQL row. The reason is
that object stores have no atomic rename: a key that encodes hierarchical position turns a
reorganization into a non-atomic copy-and-delete across everything beneath the moved node.
`auth-strategy.md` §10 rejected a materialized path for organization lineage for the same reason.

A file's key is `[blobfs_file.id]/[filename]`. The `id` is a UUID `blobfs` mints in Go with the
standard library's `uuid.NewV7()` when it inserts the `pending` row, unless the caller supplies one
of its own for a seeded row. The filename segment is frozen at upload, validated against the
provider through the key-validation method on `blobfs`'s object-store interface, and never kept in
sync with a later rename of the row's display name. A rename is a SQL `UPDATE` and touches no
object. The segment exists so an operator browsing a raw container can read it, and `Capabilities`
in `go-storage` exists partly for this case. Nothing parses the segment back out, and the interface
carries no maximum key length: the provider's own rule is the whole defense.

Directory hierarchy lives entirely in SQL. `blobfs_directory` references itself through
`parent_id`, the same shape as `organization`, and a file's virtual path is a read-time recursive
query at standard tier, walking upward from the directory to the root. A folder move or rename is
one SQL update, because no object's key encodes where it lives.

## The schema

The schema is two tables, `blobfs_directory` and `blobfs_file`.

- **`blobfs_directory`** carries `id`, `parent_id`, `name`, a version guard, and timestamps. Exactly
  one row has no parent: the seeded root, with the well-known id `blobfs.RootID` (the nil UUID) and
  the name `/`. A check constraint states that a directory is named `/` exactly when it has no
  parent, so a root under another name or a non-root named `/` is refused, and a partial unique
  index over the expression `(parent_id IS NULL)` allows only the one root row. No library
  operation can create a second root or a name-`/` non-root: `Mkdir` always binds a parent and a
  validated name.
- **`blobfs_file`** carries `id`, `directory_id` (`NOT NULL`), `name`, `status` (`pending`,
  `available`, or `deleting`, the vocabulary of the two-phase write in go-storage's `docs/design.md`), `key`, and the
  facts `go-storage`'s `Object` carries (size, content type, entity tag), plus a version guard and
  timestamps.
- **Uniqueness** is per table and exact match: `(parent_id, name)` on directories and
  `(directory_id, name)` on files, each in its own name space, so a file and a directory may share
  a name under one parent. `blobfs` normalizes names to NFC in Go with
  `golang.org/x/text/unicode/norm`, a direct dependency of the base module, because the Go standard
  library has no normalizer.

### Designs rejected

Three alternative schemas were measured against the one built, on Postgres 18 at 10,003 directories
and 100,000 files. A single `blobfs_entry` table with a `kind` column served an interleaved listing
in fewer buffers, but cost about 6% more buffers on an exact-total page and about 21% more storage,
and needed a generated `kind` column and NULL-safe check constraints per kind. A `blobfs_node`
table with directory and file subtype tables needed roughly twice the round trips for a file's
whole lifecycle, and a name-sorted page cost about eight times the buffers; a node with no subtype
row was representable, which let a row hide from every listing while still holding its name. A
two-table design with an added interleaved-listing statement cost about the same as the design
built, with no schema advantage. The design built won because the storage medium is a web-based
virtual file system, and consumers address directories and files as separate resources; an
interleaved listing was not needed. The full method and every number are in
`evidence/schema-alternatives/README.md` in [spike-blobfs](https://github.com/JaimeStill/spike-blobfs).

### The `created_at` index

The library ships no index on `blobfs_file (directory_id, created_at)`. It would turn a page sorted
by creation time without a total into an index read at a small fraction of the buffers a
directory-wide scan costs, but it buys nothing for a page that computes an exact total, which reads
every row of the directory for the window count regardless, and it costs real storage per file row
at scale. A library that ships an index imposes its write cost on every consumer whether or not
that consumer sorts by creation time, so the library documents the index and its cost and leaves it
to a consumer's own migration set to add.

### The schema is public API

The tables, columns, constraint names, referential actions, and migration set are public API under
semantic versioning, and the documentation says so. The refusal to delete a non-empty directory is
a foreign key: the documented schema requires the foreign keys from `parent_id` and `directory_id`
to have no cascading action. A consumer who writes `ON DELETE CASCADE` removes file rows whose
objects still exist, and the objects are then unreachable, because keys carry no directory prefix.

A violation of a documented constraint reaches the caller as a typed error that names the sentinel
it means and the constraint that produced it, with the underlying driver error still reachable for
a caller that wants the class. On a delete, a foreign key `blobfs` does not own — a consumer's own
join table referencing a row being removed — is classified by class, never by name, so `blobfs`
never names a consumer's constraint; the consumer matches the name itself. On a write, a
consumer's own constraint violation returns as it came.

A released migration's text and name never change, and each ships a down so a consumer's reset can
revert it; a change to already-seeded data, such as the root row's contents, is a new migration,
never an amendment.

## The standard tier and native variants

Every statement the persistence package ships is standard tier, complete on any engine `sqlate` has
a dialect for. An engine package may override a subset of operations through a variant interface,
each override justified by a measurement, not by convenience: a transaction-scoped lock for
directory moves on engines that have one, a faster form of the file-delete begin, the two write
steps and directory creation returning the changed row in one round trip instead of two, resolving
a path in one statement instead of one round trip per segment, and a keyset paging predicate that
stays cheap at any position in an indexed listing instead of growing with the cursor's depth. A
native statement declares its tier and a port note — the feature it depends on and what a port to
another engine must provide — and the toolchain holds a native file to the native forms and a
standard file to the standard ones. The conformance suite runs over every variant and the baseline
alike, so a variant must produce identical rows and identical errors to the baseline for every
input; a consumer's own variant embeds a base variant and overrides only the methods it needs.

## Writing, deleting, and moving

### The write

`blobfs` never calls the object store. It exposes the steps of go-storage's two-phase write,
two-phase write, and the consumer sequences them: `blobfs` inserts the `pending` row in the
caller's session, in the same transaction as the consumer's own rows, or resumes a `pending` row an
earlier attempt left; the consumer puts the object; `blobfs` marks the row `available` with what
the consumer's store reported. There is no failed status and no fail step: a stop between the steps
leaves the row `pending`, a retry of the same write resumes it, and an abandoned write is removed
through the delete steps. Every method takes a `sqlate.Session` and passes it through unwrapped
except the few whose correctness requires a transaction, which say so in their own signature. No
sweeper ships in v1; `v1.messaging` is the trigger.

`blobfs` declares its own object-store interface, one method, key validation, with no maximum
length declared on it. A small adapter at the consumer's composition root wires the object store's
own key validation to that interface. Key construction from an id and a filename, filename
sanitizing, and path resolution are `blobfs`'s own API and not part of the interface.

### Deleting

`blobfs` has no cascade and no recursive delete. It deletes files and directories individually, and
a consumer that wants a recursive delete layers the operations itself.

A directory is deleted by one statement the foreign keys refuse while the directory has children. A
file is deleted in two steps around the object delete: begin marks the row `deleting` and returns
its key, idempotent on a row already `deleting`; the consumer deletes the object, and the object
store treats a missing key as success, so the step is safe to repeat; complete removes the row,
succeeds when the row is already gone, and refuses a row that is not `deleting`. Every other
mutation refuses a `deleting` row, so a concurrent operation cannot act on a row whose object is
gone or about to be. A `deleting` row keeps its name-space slot until it is removed, so a write of
the same name in the window is refused as taken.

A row that another of the consumer's own rows references needs the reference-then-delete rule: the
library exposes an operation that holds a file's row for the rest of the caller's transaction
without changing its version, and a consumer calls it before inserting any row that references the
file. That closes the race between inserting a reference and beginning a delete, whichever runs
first: the two serialize on the file's row, and a delete's begin waits for a reference that is
being written and refuses one that already exists.

### Moving

A directory move takes the engine's lock, where an engine provides one, and runs a cycle check
before it reparents the directory, all in one transaction; the engine's variant supplies the lock,
and the baseline reports that it does not serialize, since standard SQL has no statement that holds
a lock to commit. On an engine with no native lock, a caller either runs every moving transaction
at serializable isolation and retries on a serialization failure, or serializes moves outside the
database itself; the cycle check assumes an acyclic tree, so the lock, or the equivalent isolation
guarantee, is a caller requirement and not a courtesy. A file move needs no lock, since a file
cannot be its own ancestor. A rename is a move to the same parent, and it pays the same lock and
check as any move.

## Listing and paging

A listing is one authored statement anchored on a single directory, never a projection, with the
caller's filters, sort, and page composed onto it and the exact total computed in the same select
list when a total is asked for, so a total can never disagree with its page. Offset paging and a
keyset cursor walk the same order; the cursor's tie-breaker follows the sort's own direction, so a
descending page is the exact reverse of the ascending one and either direction can take a cursor. A
cursor is opaque, bound to the listing and the sort that issued it, and refused if edited, issued by
another listing or sort, or issued under a sort that cannot be continued — one whose terms up to the
key mix directions, or that names a field that can be NULL.

Every page reports, independent of its total, whether rows remain after it. A page therefore reads
as one of three states: no rows remain; more rows remain and the sort can be continued, so the next
page comes by cursor; or more rows remain but the sort cannot be continued, so the next page comes
by number. An empty page after the first carries no total, since computing one there would count
what a cursor sees after it, a different quantity from the page's own total; the total is exact only
on a page that actually ran the count. There is no capped total: a listing with no total bounds its
own cost to the page it returns, and a caller with a very large directory reads the total once and
walks the rest by cursor.

## Ownership stays with the consumer

`blobfs` owns no notion of what a file or a directory belongs to: no owner column, no unit column,
and no domain vocabulary anywhere in its schema or its API. A consuming domain composes ownership
through its own join table and its own constraints, at whatever grain its goal needs — a table that
binds a directory, a table that binds a file, or both. `blobfs` publishes a pattern namespace
holding the two entities' column lists, for a consumer's own read models to include; the listing
statements described above are entirely `blobfs`'s own and carry no ownership predicate, so a
consumer's ownership check runs around the library's listing, never inside it.

## The migration set the library ships

`blobfs` ships its DDL as one migration set: a name, a history table, and its migrations, with the
Postgres form owned by the Postgres engine package. A set is self-contained: it owns every object it
creates under its own name prefix, it never references a consumer's objects, and its released
migrations never change in text or name once shipped. A consumer adopts a shipped set by declaring
it in its own multi-set migrator, ahead of its own migrations, which may then reference the
library's tables freely; the multi-set migrator itself, and how a consumer adopts a set in detail,
are `migration-sets.md`'s subject, not restated here.

## Sequencing

`go-storage` and `azureblob` are built and released, and neither depends on `blobfs`. `blobfs.build`
cannot start ahead of `blobfs.sources`: the engine package exports its migration set as the type
`blobfs.sources` promotes into `sqlate`, so the promoted repository cannot compile without it. If
`sources` must slip, the fallback is for the engine package to return its migrations as a plain
list instead of the promoted type, which the published `migrate` package already supports one set
at a time; that path is worse for the first consumer and is taken only if `sources` is blocked.
`blobfs.admin` follows `blobfs.build`, and `v1.storage.service` follows both.

## Deferred, with triggers

- **Subtree search.** The core API navigates one directory at a time; a search that descends a
  subtree is a separate, deferred capability with its own cost profile. No trigger is named yet.
- **Directory copy.** A file copy is a consumer composition over the library's primitives; a
  directory copy is the same kind of walk and is deferred with subtree search.
- **A lookup by storage key.** Today's file lookup is by directory and name. A lookup by the
  storage key itself would serve a reconciler that lists the object store's own keys and asks which
  rows they belong to; its trigger is `v1.messaging`, and its schema cost is a new unique index on
  the key column, since none exists today.
- **A directory rename that skips the tree lock.** A rename cannot form a cycle, so it does not need
  the lock a general move does; not measured, and worth adding only if a consumer renames
  directories at a volume where the lock's cost matters.
- **A serializing standard-tier variant.** A root-row update taken to commit would serialize moves
  on any engine without a native advisory lock, at the cost of writing to the root on every move;
  not built until a consumer on such an engine exists.
- **An optional consumer-supplied name validator.** The library validates names against its own
  rules; a consumer wanting a narrower policy of its own would supply an additional validator. Not
  built until a consumer asks.
- **Soft delete and a recycle bin.** The trigger is the first domain that needs the convention
  `data-layer.md` defers.
- **Content replacement and versioning.** The trigger is a consumer that overwrites a file's
  content.
- **File checksums.** `go-storage` exposes none, so `blobfs` would compute one while streaming.
- **A checksum for migration text.** The trigger is a released migration changed in place that the
  golden test missed.
- **The sweeper.** The trigger is `v1.messaging`, as go-storage's `docs/design.md` records. It is
  a candidate command beside the library.
- **A `blobfstest` toolkit.** The trigger is what `blobfs.build`'s own consumers show a test needs
  beyond the conformance suite.
- **An interleaved directory-and-file listing.** The trigger is a consumer that builds a folder
  browser.
- **MySQL and MariaDB DDL.** The trigger is the second SQL engine (`backlog.second-providers`).
- **Two architecture-layer promotions.** With `sqlate` and `blobfs`, two repositories sit outside
  the five tiers, which makes `repository-topology.md`'s "every module repository of a standard
  belongs to exactly one" false unless adjacency is named as a position; the trigger is
  `blobfs.build`. A library shipping its own object namespace as a migration source amends
  `baseline-standards.md`, which says nothing above the standard tier may harden into a library's
  contract; the trigger is `go-auth` as the second shipper.

## Assumptions

- The baseline is portable to any engine `sqlate` has a dialect for, but only Postgres was
  exercised; the DDL and the native variant are Postgres only.
- The provider's key-validation rule is proved by unit tests, not against a real Azure account; the
  emulator accepts keys the real provider rejects.
- The engine's variant interface, eight methods as `blobfs.build` inherits it, is expected to shrink
  once `sqlate` hosts pattern overlays and a way to declare a statement that returns the changed
  row; four of the eight exist only because those mechanisms don't yet.
- The listing's filters are what a sweeper, when one is built, would need to find abandoned rows.
- The `org_image`-style composition (a partial unique index enforcing one active row) generalizes to
  a second domain once one exists; nothing here commits a second consumer.
- No concurrency beyond two connections was measured; every buffer and plan-shape measurement
  carries across machines, and every timing measurement does not.
