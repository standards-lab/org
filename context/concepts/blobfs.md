# blobfs

A standalone library, adjacent to the standard the way `sqlate` is, that owns a SQL-backed
virtual-directory and file metadata layer over any object store — `go-storage`'s `Client`
included, but `blobfs` never imports it. It exists because the `v1.storage` demonstration layer
turned out to need more than one attachment: a browsable document hierarchy under an
organization, proven correctly means proving directory listing, hierarchy, and a guarded
single-active-record pattern against real storage, not just a put and a get. This is a concept:
nothing below is decided past what's stated as settled, and `blobfs`'s own `plan` session
(`blobfs.design`) settles the rest.

## Why a separate library, and why not staged in `go-web-service`

`dsl-driven-services.md` §2.1 classifies object storage as protocol-driven and SQL as the
organization's one DSL-driven capability. If the virtual-directory metadata lived inside
`go-storage`, that library would own SQL text and, almost certainly, schema and migrations —
duplicating `sqlate`'s own reason for existing rather than using it. Keeping `go-storage` strictly
protocol-driven (`design/storage-strategy.md`) and putting the SQL-backed hierarchy in its own
library, consuming `sqlate` the way `go-database` does, keeps the DSL-driven/protocol-driven
boundary intact instead of blurring it for one capability's convenience.

Staging it as application code in `go-web-service`'s `sdk` package — the existing promotion-candidate
convention (`domain-architecture.md`) — was considered and set aside. The architect's read is that
this capability's shape is clear enough, and general enough beyond this one service, to be worth
building as its own repository from the start, proven first by an `experiment` rather than staged
inside the reference service. `sqlate` took the identical path: prototyped at
`standards-lab/experiments/sql-dsl` before it existed as a repository.

## The opaque key convention

`go-storage`'s own keys stay opaque (`design/storage-strategy.md` §6, §7) — not because
authorization ever parses a key (it never does; `auth-strategy.md` §8 already routes
authorization through the owning SQL row), but because object stores have no atomic rename. A key
that encodes hierarchical position turns an ordinary reorganization into a non-atomic
copy-and-delete storm across everything beneath the moved node — the same lesson
`auth-strategy.md` §10 already recorded once, rejecting a materialized path for organization
lineage for the identical structural reason.

`blob_file.id`, a UUID minted when the row is created — the `pending` phase of
`storage-strategy.md` §6's two-phase write already establishes this — becomes the key's prefix:
`[blob_file.id]/[filename]`. The filename segment is frozen at upload time, sanitized against
`go-storage`'s `Capabilities` for the active provider, and never kept in sync with a later rename
of the row's own display name — a rename is a SQL `UPDATE`, touching no object. The segment exists
only so a raw container is legible to an operator browsing it directly; nothing in `blobfs` or the
application ever parses it back out.

Directory hierarchy lives entirely in SQL: `blob_directory`, self-referencing on `parent_id`, the
same shape `organization`'s own tree already uses, with the virtual path a read-time projection —
a recursive CTE, mirroring `organization`'s own lineage query — rather than a materialized string.
A folder move or rename is one SQL update; the object store is never touched, because no object's
key ever encoded where it lived. One consequence worth naming: since directory listing is a SQL
query against `blob_file`/`blob_directory` with real pagination and filtering, `go-storage`'s own
`List` operation may end up serving only reconciliation and sweep use once `blobfs` exists, rather
than being how an application actually lists a directory's contents.

## Composition-based ownership

`blobfs` owns no notion of what a file belongs to — no `owner_kind`/`owner_id` polymorphic
columns, no domain vocabulary at all. A consuming domain composes ownership through its own join
table with its own constraints. The worked example from the session that produced this note:
`organization`'s own `org_image` row maps `organization` → `blob_file`, with a partial unique
index enforcing one active image per organization (the same guard shape `inventory`'s custody
ledger already uses for one open row per held instance) and a content-type check restricting it to
images. `organization` imports `blobfs`; `blobfs` never imports back, the same downward-only
relationship `devices` already has to `inventory`.

## Sequencing

`go-storage`'s base library, then `azureblob` (`v1.storage.library`, `.azureblob`) — neither
depends on `blobfs`. Then `blobfs`'s own goal in full: `blobfs.design` (the dedicated `plan`
session settling the open questions below), `.experiment` (the spike, at
`standards-lab/experiments/blobfs` — coordinator-level, per
`references/workspace-coordination.md`, never inside a member repository), and `.build`
(promotion into `blobfs`'s own repository at the experiment's close). Only then
`v1.storage.service`: the organization logo and document hierarchy, consuming both `azureblob`
and the real `blobfs` library.

## Open questions for `blobfs`'s own `plan` session

None of these are settled here — they're why the goal has a dedicated `design` task before the
experiment:

- **How a consuming service's domain read models join against `blobfs`-owned schema.** An
  organization's document listing needs to join `blob_directory`/`blob_file`, tables a library
  owns rather than tables the application authored. Nothing in this organization's conventions
  currently addresses a domain read model joining across a schema boundary like this.
- **How `blobfs` ships its schema and migrations for a consuming application to adopt.** No
  existing library here ships migrations — `go-database` ships none; every migration in this
  organization is application-authored. Whether `blobfs` publishes a migration set the consumer
  applies alongside its own, or some other mechanism, is undecided.
- **Whether `blobfs` needs its own engine sub-module structure** (`blobfs/postgres`, mirroring
  `sqlate/postgres`) or simply authors portable-tier SQL compiled through the consumer's own
  `sqlate` instance, needing no sub-module of its own.
- **The final module path and taxonomy placement.** `blobfs` is the working name. It sits outside
  `topology-and-naming.md`'s closed infrastructure-library list, the same way `sqlate` does — that
  placement needs checking against `repository-topology.md` before the name and module path are
  final, not assumed from the working name alone.

## Assumptions

- Assumes `blobfs` consumes `sqlate` directly for its own authored SQL, the same way `go-database`
  does, rather than inventing a second SQL mechanism.
- Assumes the `org_image`-style composition pattern generalizes to other domains (`people`,
  `inventory`) once they exist; nothing here commits a second consumer, and the pattern is
  evidence, not a decision, until one actually adopts it.
