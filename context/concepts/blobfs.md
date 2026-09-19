# blobfs

A standalone library that owns a SQL-backed virtual-directory and file-metadata layer over any
object store. It sits adjacent to the standard the way `sqlate` does. `go-storage`'s `Client`
can be the store, but `blobfs` never imports `go-storage`. It exists because the `v1.storage`
demonstration layer needs more than one attachment: a browsable document hierarchy under an
organization. Proving that layer correctly means proving directory listing, hierarchy, and a
guarded single-active-record pattern against real storage, and not only a put and a get.

This is a concept. The design below is provisional until the experiment (`blobfs.experiment`)
has exercised it. The experiment exists to find what is painful, incorrect, or awkward in these
decisions, and the note promotes to `design/` once the shape has held. The assumptions the design
rests on are listed at the end.

## Position and audience

`blobfs` is for any Go project on an engine it ships DDL for, which is Postgres at v1. That is
the reason it is adjacent to the standard and not a member repository of it. Its repository
takes no `go-` prefix and its `references.toml` entry carries no `standard` key, as `sqlate`'s
does. It cannot be an infrastructure library: that tier is one repository per external
technology, named `go-<technology>` for the technology it presents as a service
(`architecture/principles/repository-topology.md`,
`architecture/standards/go-elemental/principles/topology-and-naming.md`), and `blobfs` presents
no external technology. It composes two technologies it does not own, a SQL database and an
object store.

The name `blobfs` stays. The final module path is checked against `repository-topology.md` when
the experiment promotes to its own repository (`blobfs.build`).

### Why a separate library

`dsl-driven-services.md` §2.1 classifies object storage as protocol-driven and SQL as the
organization's one DSL-driven capability. If the virtual-directory metadata lived inside
`go-storage`, that library would own SQL text and schema, duplicating the reason `sqlate` exists.
Keeping `go-storage` protocol-driven (`design/storage-strategy.md`) and putting the SQL-backed
hierarchy in its own library, consuming `sqlate` the way `go-database` does, keeps the
DSL-driven and protocol-driven boundary intact.

Staging the capability as application code in `go-web-service`'s `sdk` package, the existing
promotion-candidate convention (`domain-architecture.md`), was set aside. The architecture's rule
is that a pattern is proven in application code first and then graduates, and `blobfs` departs
from it on the architect's judgment that its shape is general enough to build as its own
repository. The departure has a cost that the `sqlate` precedent does not remove: the `sqlate`
experiment (`standards-lab/experiments/sql-dsl`) redesigned a capability that `go-database` had
already built, released, and reviewed against a real consumer, while `blobfs` has no predecessor
and no consumer. The experiment therefore carries the proving burden, through a consumer-shaped
command-line file system (see The experiment).

## Layers

`blobfs` is one module with three layers, ordered by what a consumer accepts. Each layer imports
the one below it, and a consumer adopts as many as it wants.

1. **The root package** is Go only: the entity types, the status vocabulary, key construction,
   name normalization and validation, the table of allowed state transitions, the key-validation
   interface for the object store, and the error types. It imports neither `sqlate` nor
   `go-storage`. A consumer that keeps its own persistence takes only this layer.
2. **The persistence package** holds the SQL statements, the published pattern namespace, and the
   methods that take a `sqlate.Session`. It introduces the `sqlate` module dependency. A consumer
   that authors its own DDL from the documented schema takes this layer and the root.
3. **The migrations package** embeds the DDL and exports it as a migration source (see
   Migration sources). It imports `sqlate/migrate`, a package in the same `sqlate` module, so it
   adds a commitment and no new dependency: the consumer runs `blobfs`'s schema as part of its
   own migrations.

The organization's rule is that a package splits from its parent by growth or by dependency
weight, never by topic alone. This split is by dependency at the second layer, where the `sqlate`
module enters the import graph, and by commitment at the third. It is an exception to the rule,
taken so that a consumer adopts the capability at the resolution it accepts. A sub-module is
rejected, because `sqlate` has no dependency weight of its own to isolate. The package names are
settled in the experiment. The experiment's `split-check` enforces the import boundaries and
reports how much of the library is `sqlate`-free, and the root may prove too thin to stand alone,
which the experiment measures.

## The opaque key convention

`go-storage`'s own keys stay opaque (`design/storage-strategy.md` §6, §7). Authorization never
parses a key, because `auth-strategy.md` §8 routes it through the owning SQL row. The reason is
that object stores have no atomic rename: a key that encodes hierarchical position turns a
reorganization into a non-atomic copy-and-delete across everything beneath the moved node.
`auth-strategy.md` §10 rejected a materialized path for organization lineage for the same reason.

A file's key is `[blobfs_file.id]/[filename]`. The `id` is a UUID that `blobfs` mints in Go with
the standard library's `uuid.NewV7()` when it inserts the `pending` row. The filename segment is
frozen at upload, validated against the provider through the key-validation method on
`blobfs`'s object-store interface, and never kept in sync with a later rename of the row's
display name. A rename is a SQL `UPDATE` and touches no object. The segment exists so an
operator browsing a raw container can read it, and `Capabilities` in `go-storage` exists partly
for this case (`design/storage-strategy.md` §2). Nothing parses the segment back out.

Directory hierarchy lives entirely in SQL. `blobfs_directory` references itself through
`parent_id`, the same shape as `organization`, and a file's virtual path is a read-time
recursive query at standard tier, as `organization_view.sql` is. A folder move or rename is one
SQL update, because no object's key encodes where it lives.

## The schema

The schema is two tables, `blobfs_directory` and `blobfs_file`.

- **`blobfs_directory`** carries `id`, `parent_id`, `name`, a version guard, and timestamps.
- **`blobfs_file`** carries `id`, `directory_id` (`NOT NULL`), `name`, `status` (`pending`,
  `available`, or `deleting`, the vocabulary of `design/storage-strategy.md` §6), `key`, and the
  facts `go-storage`'s `Object` carries (size, content type, entity tag), plus a version guard
  and timestamps.
- **Uniqueness** is per table and exact match: `(parent_id, name)` on directories and
  `(directory_id, name)` on files. `blobfs` normalizes names to NFC in Go with
  `golang.org/x/text/unicode/norm`, a direct dependency of the base module, because the Go
  standard library has no normalizer. A file and a directory may share a name. Root directories
  need no constraint, since the consumer's table anchors every root.

The column set is a starting point that the experiment finalizes.

### The schema is public API

The tables, columns, constraint names, referential actions, and migration set are public API
under semantic versioning, and the documentation says so. The refusal to delete a non-empty
directory is a foreign key: the documented schema requires the foreign keys from `parent_id` and
`directory_id` to have no cascading action. A consumer who writes `ON DELETE CASCADE` removes
file rows whose objects still exist, and the objects cannot be found again, because keys carry
no directory prefix and the interface has no `List`. `blobfs` maps a violation of its own
documented constraint to a sentinel error through `sqlate`'s `ConstraintError.Constraint`, and
leaves violations of a consumer's join-table constraints unclassified.

## Migration sources

`blobfs` ships its DDL as a migration source: a named set of migrations with its own version line
and its own history table, which a consumer adds to its one migrator. A source is self-contained:

- It owns its objects. Every object a source owns starts with the source name and an underscore:
  the tables `blobfs_directory` and `blobfs_file`, their indexes and constraints, and the history
  table `blobfs_schema_version`. A Postgres schema per source is rejected, because `sqlate`'s
  migration catalog targets MySQL and MariaDB, where a schema is a database, so a
  schema-qualified statement is not portable and a foreign key across databases fails.
- It never references a consumer's objects. This is `downward-dependencies` applied to schema.
  The consumer's migrations reference the source's tables freely.
- Its released migrations never change in text or name, and each ships a down text, so a
  consumer's `Reset` can revert it. A new migration applies over any data that was valid under
  the previous version, and a breaking schema change is a major release. A golden-hash test in
  `blobfs`'s repository pins the released text.

`sqlate/migrate` v0.1.1 cannot host a source. `Migration.Version` is a fixed integer and
`checkPrefix` requires the applied rows to match the set positionally by version and name, so
two sets cannot be merged. `go-database`'s admin service takes one `*migrate.Migrator`. The change
is to let one `Migrator` run several sets, each with a source name and its own history table,
under one lock, in the order the consumer declares them with its own set last. The default set
keeps the existing table, so a v0.1.1 database needs no history migration. Upgrading is a
`go.mod` bump: the next start finds `blobfs`'s pending migrations and applies them. There is no
checksum column, and a changed text under an unchanged name is caught by the producer's golden
test and not by the consumer's migrator. The change ships as `sqlate` v0.2.0, with a minor
`go-database` admin release so `Reset` reverts across all sets and `Status` reports each source's
head. `go-auth` introduces its own schema (`goals.v1.auth`), so this is a `sqlate` capability
and not a `blobfs` one-off.

A consumer with its own numbered migrations adopts `blobfs` in four steps: register
`blobfs`'s patterns in its catalog, build one migrator over `blobfs`'s set followed by its own,
add `blobfs`'s statements to its verify stage, and write its own migrations referencing
`blobfs_file` and `blobfs_directory`. `blobfs`'s set is always at its head before the consumer's
runs. A consumer on another migration tool takes the first two layers and authors the DDL from the
documented schema.

The migrations are Postgres DDL at v1: the `uuid` type and partial unique indexes appear in
them. `blobfs`'s statements remain standard tier, checked by its own `sqlint.toml`, and the
migrations package selects a directory per engine (`Migrations(dialect)`), so a second engine adds
a directory and not a module.

## Writing, deleting, and moving

### The write is a sequence of steps

`blobfs` never calls the object store. It exposes the steps of `design/storage-strategy.md`
§6's two-phase write, and the consumer sequences them:

1. `blobfs` inserts the `pending` row in the caller's session, in the same transaction as the
   consumer's own rows.
2. The consumer puts the object.
3. `blobfs` marks the row `available`, or marks the write failed.

Every method takes a `sqlate.Session` and passes it through unwrapped, because the
`transaction: required` header check is a type assertion on `*sqlate.Tx` and a wrapper fails it.
A crash between steps one and two leaves a `pending` row that a query finds. No sweeper ships
in v1; `v1.messaging` is the trigger.

`blobfs` declares its own object-store interface, holding key validation and a maximum key
length. Beginning a write stores the key in the row, so `blobfs` must validate it. A small
adapter at the consumer's composition root wires `*storage.Store`'s key validation to that
interface. Key construction from an id and a filename, filename sanitizing, path resolution, and
the subtree and cycle-check statements are `blobfs`'s own API and not part of the interface.

### Deleting

`blobfs` has no cascade and no recursive delete. It deletes files and directories individually,
and a consumer that wants a recursive delete layers the operations itself.

- **A directory** is deleted by one `DELETE` that the foreign key refuses while the directory has
  children. A violation of `blobfs`'s own keys maps to a not-empty sentinel.
- **A file** is deleted in two steps around the object delete of §6. Begin marks the row
  `deleting`, is idempotent on a row already `deleting`, is permitted from `pending` or
  `available`, and returns the key. The consumer deletes the object, and the object store treats a
  missing key as success, so the step is safe to repeat. Complete removes the row, succeeds when
  the row is already gone, and refuses a row that is not `deleting`.
- **Every other mutation** (move, rename, complete-write, fail-write) refuses a `deleting` row, so
  a concurrent move cannot relocate a row whose object is gone.
- **Listings include** `pending` and `deleting` rows, so a recursive walk sees the rows it must
  finish. A walk refetches the first page until it is empty.

A `deleting` row keeps its `(directory_id, name)` slot until it is removed, so a `put` of the same
name in the window fails as name-taken. A child inserted during a walk is safe: the foreign key
refuses the directory delete, and the walker lists again. The recursive delete is not atomic and
can loop under sustained writes. A consumer that needs exclusion takes its own advisory lock.

### Moving

A directory move uses a cycle-check statement, and the caller takes an advisory lock around the
move. `blobfs` cannot take the lock itself: lock names live in the application's registry
(`domain-architecture.md`), and taking one is a native statement. An unchecked cycle makes every
path read loop forever, so the documentation states the lock requirement.

## Reading and composing ownership

`blobfs` owns no notion of what a file belongs to: no `owner_kind` or `owner_id` columns and no
domain vocabulary. A consuming domain composes ownership through its own join table with its
own constraints. `organization` imports `blobfs`, and `blobfs` never imports back, the same
downward-only relationship `devices` is planned to have to `inventory`.

The scope predicate of `auth-strategy.md` §2 ends with `c.descendant_id = o.unit_id`, so it
needs a `unit_id` on the row it filters, and `blobfs`'s tables have none. The consumer's join
table therefore drives every authorized listing, and `blobfs_file` and `blobfs_directory` are the
joined detail.

`blobfs` publishes a `blobfs` pattern namespace (`query.Publish` over an `fs.FS`, and a `sqlint`
`[export]` entry). The consumer registers it in its one catalog and includes the patterns for
the column list and the path projection, the same way `auth-strategy.md` §2 treats an
infrastructure library as a narrow third publisher. Directories and files have separate listing
patterns, each paged and filtered independently. An interleaved listing is deferred.

A projection base in `sqlate` binds no parameters of its own today, so a directory-scoped listing
declares `directory_id` as a `--| field:` and filters through a request directive. `blobfs`'s
listing function takes the directory id as a required argument, appends the filter, and returns
`UnknownFieldError` when the base did not declare the field, so a forgotten filter fails loudly.
This is interim. The trigger for replacing it is the arity-one projection-base lift that
`auth-strategy.md` §4 records.

The worked example is `organization`'s `org_image` row, mapping `organization` to `blobfs_file`,
with a partial unique index that enforces one active image per organization. That guard has the
shape `data-layer.md` describes for the unbuilt `inventory` domain's custody ledger. A `CHECK`
constraint cannot restrict the file to images, because `content_type` lives on `blobfs_file` in
another table, so the consumer's command enforces it or the join row carries a denormalized
content type. An organization's document hierarchy also needs a root directory to hang from, which
the consumer holds as a second join table or a column on its own table.

## The experiment

`blobfs.experiment` builds a simple command-line file system over blob storage at
`standards-lab/experiments/blobfs`, coordinator-level and never inside a member repository
(`references/workspace-coordination.md` in the marathon plugin). Its purpose is to validate the
ergonomics of the decisions above and expose what is painful, incorrect, or awkward, so the
design adjusts before promotion. It also tests whether shipping the Go and SQL layers integrated
works, and the documentation-only retreat stays viable.

- **Setup.** Its own `go.mod` and `mise.toml`, depending on published `sqlate` v0.1.1 and
  `go-storage` v0.1.0 with `azureblob`, with no `replace` to a sibling checkout. A compose file
  runs Postgres and Azurite with `--skipApiVersionCheck`.
- **Layout.** The three `blobfs` layers are promotion candidates. The multi-source migrator is a
  fourth: a shim over published `sqlate` v0.1.1 that holds one `migrate.Migrator` per set with its
  own table, takes an outer lock through `sqlate.Locker`, and runs the sets in canonical order, so
  its code moves into `sqlate` almost unchanged. `cmd/` is the command-line consumer. A
  `split-check` task, the technique `experiments/sql-dsl` used, fails the build if the root
  package imports `sqlate` or `go-storage`, if the persistence package imports `go-storage`, or if
  the migrator shim imports more than `sqlate`.
- **Consumer side.** A `volume` table (id, name, root directory, a stand-in `unit_id`) anchors
  listings and includes the published patterns, and a `volume_bookmark` table (volume id, file
  id) references `blobfs_file`. A `--unit` flag rehearses only the directive-filter shape of the
  scope predicate.
- **Commands.** `mkdir`, `put`, `cat`, `ls`, `mv`, `rm`, `rmdir`, `rm -r` (consumer code layered
  over the public API), `stat`, `bookmark`, `schema status|up|down|reset` (standing in for
  `go-database`'s admin surface), and a fault-injection flag that stops between the steps of a
  write and of a file delete.

It proves, in order of risk:

1. A paged, filtered, sorted read model anchored on the consumer's table composes through the
   published patterns with a correct total, and the path query's cost per page is measured.
2. Every `blobfs` statement compiles at standard tier and prepares against the schema `blobfs`'s
   own migration set created.
3. The two-phase write composes into the consumer's transaction, and a crash between steps
   leaves a queryable `pending` row.
4. A directory move with the cycle check under a caller-held lock closes the race against a real
   Postgres.
5. The multi-source migrator handles fresh replay, an upgrade rehearsal (a new `blobfs` migration
   added mid-experiment, then a restart), reset order with `volume_bookmark`'s foreign key, and
   dirty-state refusal. Its measures decide integrated versus documentation-only: the lines in the
   consumer's composition root under each model, the consumer edits an upgrade needs, whether
   reset and status stay one operation, whether the shim needed anything outside the public API,
   and whether a consumer without `go-database` operates the schema in a few lines.
6. A file delete converges under retry after a crash at each step, a consumer-layered recursive
   delete is correct under a concurrent insert, and a `volume_bookmark` key into `blobfs_file`
   meets a delete with a classifiable error.
7. The consumer's key-validation wiring is small.
8. Every awkward call site the command-line consumer needs is a finding against a decision.

It does not prove authorization, which needs `go-auth` and is proven in the storage domain's
auth sweep under `v1.auth`. It does not prove the migration path through `go-web-service`'s admin
surface and `Reset`, which `v1.storage.service` proves.

## Sequencing

`go-storage` and `azureblob` are built and released, and neither depends on `blobfs`. `blobfs`'s
goal runs `.experiment`, then `.sources` (promoting the migrator into `sqlate` v0.2.0), then
`.build`, which promotes the experiment into `blobfs`'s own repository and first release against
`sqlate` v0.2.0. The `go-database` admin release follows, and only then does `v1.storage.service`
build the organization logo and document hierarchy, consuming `azureblob` and the real `blobfs`.

## Deferred, with triggers

- **Soft delete and a recycle bin.** The trigger is the first domain that needs the convention
  `data-layer.md` defers.
- **Content replacement and versioning.** The trigger is a consumer that overwrites a file's
  content.
- **File checksums.** `go-storage` exposes none, so `blobfs` would compute one while streaming.
- **A checksum for migration text.** The trigger is a released migration changed in place that the
  golden test missed. It would need one history table with a source column.
- **The sweeper.** The trigger is `v1.messaging`, as `design/storage-strategy.md` §6 records. It
  is a candidate command beside the library.
- **A `blobfstest` toolkit.** The trigger is what the experiment shows a consumer's tests need.
- **An interleaved directory-and-file listing.** The trigger is a consumer that builds a folder
  browser.
- **MySQL and MariaDB DDL.** The trigger is the second SQL engine (`backlog.second-providers`).
- **A Postgres schema per source.** The trigger is dropping MySQL and MariaDB portability.
- **Two architecture-layer promotions.** With `sqlate` and `blobfs`, two repositories sit outside
  the five tiers, which makes `repository-topology.md`'s "every module repository of a standard
  belongs to exactly one" false unless adjacency is named as a position; the trigger is
  `blobfs.build`. A library shipping its own object namespace as a migration source amends
  `baseline-standards.md`, which says nothing above the standard tier may harden into a library's
  contract; the trigger is `go-auth` as the second shipper.

## Assumptions

- `sqlate` pattern publication carries a hierarchy recursive query at standard tier, as
  `organization_view.sql` and `query/projection.go` indicate.
- A multi-source migrator built as a shim over the published `migrate` API reproduces the
  per-source design fully, so the shim moves into `sqlate` almost unchanged.
- The consumer's key-validation wiring stays a few lines.
- The directive-filter workaround is acceptable until the projection-base lift.
- The file delete steps are idempotent as stated, and the write's failure step leaves the row in a
  state a retry recovers.
- The path-composing recursive query is affordable per page. It walks from the roots, as
  `organization_view.sql` does.
- A file and a directory sharing a name stays unambiguous for path resolution.
- The three-layer structure is worth its exception to the split rule: the root stands alone
  usefully for a consumer with its own persistence.
- The `org_image`-style composition generalizes to other domains once they exist. Nothing here
  commits a second consumer, and the pattern is evidence and not a decision until one adopts it.
