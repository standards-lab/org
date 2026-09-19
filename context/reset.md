# reset · blobfs-design

- **Status:** closeout
- **Session:** plan
- **Project:** standards-lab
- **Branch:** blobfs-design

## Disposition

- **Settled:** the `blobfs` design, in `concepts/blobfs.md` (`blobfs.design`), as a provisional
  concept. `blobfs` is one module in three layers a consumer adopts by what it accepts: a Go-only
  root package, a persistence package over `sqlate`, and a migrations package. It ships its DDL as
  a migration source with its own version line and history table, and every object it owns starts
  with the source name (`blobfs_directory`, `blobfs_file`, `blobfs_schema_version`). Deletes have
  no cascade: a directory delete is refused by the foreign key while children exist, a file
  delete is two steps around the object delete, and a consumer layers recursive delete itself.
  Writes are steps the consumer sequences, and `blobfs` never calls the object store. It publishes
  a `sqlate` pattern namespace, and the consumer's join table anchors every authorized listing.
  The experiment is a command-line file system over blob storage with a multi-source migrator
  shim, and its proofs are ordered by risk in the concept.
- **Integrated:** nothing decayed.
- **Promoted:** nothing. The shape was designed this session, so it waits for the experiment to
  exercise it before the note moves to `design/`.
- **Culled:** the concept's open-questions section, and the claims the reviews found wrong: the
  `sqlate` precedent (its spike redesigned a capability `go-database` had already built), the
  placement rationale (the `go-<technology>` naming rule, not the list alone), the `inventory`
  custody ledger cited as existing precedent, the `org_image` content-type check that a `CHECK`
  constraint cannot express across tables, and the `List` aside that implied a sweeper nothing
  owns.
- **Retained:** `concepts/blobfs.md`, provisional, with its assumptions named: pattern publication
  carries a hierarchy query at standard tier, the migrator shim moves into `sqlate` almost
  unchanged, the key-validation wiring stays small, the directive filter is acceptable until the
  projection-base lift, the file delete steps are idempotent as stated, and the three-layer split
  earns its exception to the split rule.
- **Corrected:** `design/auth-strategy.md` §8 no longer sketches an `owner_kind`, `owner_id`
  attachment table; the consumer's join table carries `unit_id` and drives the listing.
  `design/storage-strategy.md` §6 assigns the row and schema to `blobfs` as a migration source and
  ownership to the consumer's join table. The `context` path on `backlog.sql-meta-language`
  pointed at a `docs` repository that no longer exists and now points at
  `architecture/context/concepts/sql-meta-language.md`.
- **Roadmap:** `blobfs.sources` (the `sqlate` v0.2.0 promotion of the migrator) and `blobfs.admin`
  (the `go-database` admin release over several sets) were added, and `next` now runs
  `blobfs.experiment`, `blobfs.sources`, `blobfs.build`, `blobfs.admin`, then
  `v1.storage.service`. The `blobfs`, `blobfs.experiment`, `blobfs.build`, `v1.storage.service`,
  and `v1.auth` summaries state what each now proves or waits on. `blobfs.design` is deleted with
  its `next` entry.
- **Cross-repo:** none written. Two architecture-layer promotions wait on later triggers: the
  adjacent-repository position, when `blobfs.build` closes, and a library shipping its own object
  namespace as a migration source, when `go-auth` is the second shipper. The `google/uuid` example
  in `architecture`'s `go-elemental/principles/dependencies.md` is still true; Go 1.27's standard
  library now also has `uuid`.

## Next-focus

`blobfs.experiment`, an `experiment` session in `standards-lab`, at
`standards-lab/experiments/blobfs`: the command-line file system that `concepts/blobfs.md`
defines, built against published `sqlate` v0.1.1 and `go-storage` v0.1.0 with `azureblob`, and
never a `replace` to a sibling checkout. Start with the consumer-shaped read model (a paged,
filtered, sorted listing anchored on the `volume` table through the published patterns), because
it can invalidate composition-based ownership. The multi-source migrator shim follows. The session
settles its own stage list at SETTLE. It changes no member repository, and `blobfs.sources`
promotes the shim into `sqlate` afterward.
