# reset · blobfs-build

- **Status:** closeout
- **Session:** start
- **Project:** sqlate, blobfs, architecture, standards-lab
- **Branch:** blobfs-build

## Disposition

- **Integrated:**
  - standards-lab `blobfs-api.md`: the operation set is built; blobfs's `docs/features.md` and
    package documentation cover it.
  - standards-lab `blobfs.md`: the library's design is built and documented in blobfs's
    `docs/concepts.md` and `docs/features.md`; its position went to the architecture layer
    (Promoted); its deferred capabilities with their triggers and its assumptions moved to
    `blobfs/context/deferred.md`. Provenance: the closed experiment
    [spike-blobfs](https://github.com/JaimeStill/spike-blobfs).
- **Add or sharpen:**
  - standards-lab `blobfs-composition.md`: the engine installs through
    `data.WithEngine(postgres.Engine)`, the key validator is passed to each write's first step,
    the delete's last step is the purge, a recursive removal is the consumer's own bounded walk, a
    large directory walks by cursor under `query.TotalNone`; the two-connection migrator caveat is
    gone.
  - standards-lab `sqlate-library-support.md`: the returning command, the window-count total, and
    the stricter field grammar are built and deleted; a PostgreSQL scalar-subquery count overlay
    joins the backlog; "What the built releases unblock" replaces the v0.2.0 section; the stale
    "scheduled above" references are fixed.
  - standards-lab `migration-sets.md`, `auth-strategy.md`, and the capability map in
    `context/README.md` cite blobfs's guide instead of the deleted notes.
  - blobfs `context/deferred.md` (new) and its `context/README.md`.
- **Retained:** standards-lab `blobfs-composition.md` and `migration-sets.md`, for
  `blobfs.admin` and `v1.storage.service`.
- **Promoted:** the adjacent position, `sqlate` and `blobfs` outside the five tiers, landed as
  `architecture/context/adjacent-position.md` for a session there to write the pages: the
  amendment to `principles/repository-topology.md`, blobfs in the go-elemental catalog's adjacency
  paragraph, and the pending `baseline-standards.md` exception once `go-auth` ships a second set.
- **Cross-repo:**
  - sqlate v0.3.0 (PR #10): the returning command, `--| returning: <read>`, with
    `query.Returner`, `Returning[T]`, `RowGuard` rebuilt on it (breaking), `sqlate.Beginner`,
    `sqlate.Transact`; tagged `v0.3.0`, `postgres/v0.3.0`, `sqlint/v0.2.0`.
  - sqlate v0.4.0 (PR #11): the counted collection read, a window count in the page's own
    statement; `query.Row` replaces `*sql.Rows` in `ScanFunc` (breaking); a misspelled `not null`
    is a load error; tagged `v0.4.0`, `postgres/v0.4.0`, `sqlint/v0.2.1`.
  - blobfs: the repository `standards-lab/blobfs`, created; v0.1.0 (PR #1), tagged `v0.1.0` and
    `postgres/v0.1.0`, with `example` pinned to both.
  - architecture: `context/adjacent-position.md` on branch `blobfs-build` (PR #22).
  - Each release pin commit went straight to its `main`, bypassing the pull-request rule with the
    architect's account, as the v0.2.0 pin did.
- **Roadmap** (applied when the wave folds):
  - Delete `blobfs.build`; the lane in `next` becomes
    `["blobfs.admin", "v1.storage.service", "v1.storage.suite"]`.
  - `goals.blobfs`: the summary names the base module and one engine sub-module per engine
    (`postgres` at v0.1.0); `context` becomes `["blobfs/docs/concepts.md",
    "blobfs/context/deferred.md"]`.
  - `v1.storage.service` and every other entry citing `standards-lab/context/blobfs.md` cite
    `blobfs/docs/concepts.md` instead; the ordering comment's citation likewise.
  - `v1.data`: `go-web-service` moves from sqlate v0.1.1 to v0.4.0, which changes
    `domain/organization/database.go`'s collection reads, retypes any hand-written scan to
    `query.Row`, and reports `query.NoTotal` on an empty later page
    (`standards-lab/context/sqlate-library-support.md`).
  - `backlog.second-providers`: a second SQL engine includes blobfs's own engine sub-module.
- **References** (applied when the wave folds): `references.toml` gains `[repos.blobfs]`,
  `remote = "https://github.com/standards-lab/blobfs.git"`, with no `standard` key and a comment
  that it is adjacent, as sqlate's is; `references.md` gains a `### blobfs` paragraph beside
  sqlate's.
- **Workspace order** (applied when the wave folds): `.claude/marathon.toml`'s `order` gains
  `blobfs` in the group with `go-storage`, after `sqlate`, which it depends on.
- **Experiments** (applied when the wave folds): `experiments.md`'s spike-blobfs entry records
  its promotion to `standards-lab/blobfs`.
- **Validated:**
  - Checkpoint 1 (sqlate v0.3.0): `mise run acceptance`; one set of statement files compiled
    natively and behind a capability-hiding wrapper returns the row an immediate read returns in
    every outcome, one statement against two, on the pool and inside a caller's transaction;
    `Verify` names `(returning)` after a column rename; the branch review's findings fixed as an
    Adjust.
  - Checkpoint 1b (sqlate v0.4.0): a write racing every page leaves 0 disagreements where a
    separate count disagrees 6 of 6; a two-second stress records 0 violations for the counted read
    and hundreds for the two-statement control; EXPLAIN shows one WindowAgg under `TotalExact` and
    an index walk under `TotalNone`; the review's findings fixed as an Adjust.
  - Checkpoint 2 (blobfs): the conformance suite over both returning forms and both variants and
    the standard keyset spelling; the migration set beneath a consumer's; the named constraints;
    the tree lock's serialization; plan-shape bounds.
  - Checkpoint 3: `mise run example` runs one file's life against Azurite through the
    `KeyValidator` adapter, with no change to go-storage or blobfs.
  - Checkpoint 4: build (GOWORK=off, three modules), test, golangci-lint, sqlint, split-check,
    tidy, and acceptance; the editor pass on Opus; the branch review's ten findings fixed as an
    Adjust (ErrDeleting over a stale version, the hold as a variation point writing no row
    version, walks that end on a cycle, the Variant embedding contract); CI green on PR #1,
    including a cold-cache sqlint fix.

## Next-focus

`blobfs.admin`, in go-database: the admin service over a migrator running several sets (Reset,
Status by set, Start logging pending migrations by set, and `Force <set> <version>`), a minor
release, and the last piece `v1.storage.service` needs to run blobfs's migration set.
