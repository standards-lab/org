# reset · v1-storage

- **Status:** closeout
- **Session:** plan
- **Project:** standards-lab
- **Branch:** v1-storage

## Disposition

- **Authored:** `standards-lab/context/design/storage-strategy.md` — the go-storage strategy
  record: the standard-tier `Client` interface (five operations plus `Probe`, no conditional
  writes, no object metadata beyond `ContentType`), `Capabilities` for provider key constraints,
  the base-plus-`azureblob`-plus-`admin` module layout, the interchangeable-with-review swap
  class with its three named review items, and the two-phase write (no transaction spans
  Postgres and an object store). Settled through an `opus` escalation, then substantially
  revised with the architect: the escalation's literal virtual-path key was rejected in favor of
  an opaque UUID-prefixed key, for the same reason `auth-strategy.md` §10 already rejected a
  materialized path for organization lineage — object stores have no atomic rename.
- **Authored:** `standards-lab/context/concepts/blobfs.md` — the virtual-directory metadata
  library the demonstration layer surfaced as needing, structured like `sqlate` rather than
  staged inside `go-storage` or `go-web-service`. Concept tier: its SQL-consumption mechanism,
  the cross-schema-join question, and its sub-module shape are explicitly left open for
  `blobfs.design`.
- **Integrated:** `standards-lab/context/design/testing-hierarchy.md` — corrected a claim that a
  blob write and its database row sync "atomically"; no transaction spans Postgres and an object
  store, and the text now names the two-phase write instead.
- **Promoted:** `standards-lab/context/roadmap.toml` — `goals.v1.storage` gained four tasks
  (`library`, `azureblob`, `service`, `suite`); a new `goals.blobfs` goal (tasks `design`,
  `experiment`, `build`) was added outside the `v1` tree, the same standing `goals.slab` has;
  `next` now interleaves both goals' tasks in dependency order in place of the two goal-level
  entries: `v1.storage.library`, `.azureblob`, `blobfs.design`, `.experiment`, `.build`,
  `v1.storage.service`, `.suite`.
- **Retained:** `backlog.second-providers` — the storage counterpart (S3/minio) stays post-1.0,
  unchanged this session.

## Next-focus

`v1.storage.library`: build go-storage's base module — `Config`, `Store`, the `Client`
interface, `Capabilities`, and the error sentinels — proven on the unit tier over an in-memory
fake. Depends on none of `blobfs`'s still-open questions, so it runs before that goal.

The `go-storage` repository doesn't exist yet; the next session creates it (`marathon init`, a
`code` project) before building, and the coordinator's `.claude/marathon.toml` `[workspace]
order`/`paths` may need a new entry once it's checked out alongside its siblings.
