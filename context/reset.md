# reset · blobfs-experiment

- **Status:** closeout
- **Session:** experiment
- **Project:** standards-lab
- **Branch:** blobfs-experiment

## Disposition

- **Promoted:** `context/concepts/blobfs.md` deconstructed into five concept documents, all
  concept-tier since nothing built from the experiment yet exists: `blobfs.md` re-scoped to what
  the library is; `blobfs-api.md` (new), the proposed `Directories`/`Files` operation set and
  repository layout for `blobfs.build`'s own SETTLE; `blobfs-composition.md` (new), how a consumer
  builds around the library; `migration-sets.md` (new), the multi-set migrator model
  `blobfs.sources` promotes into `sqlate`; `sqlate-library-support.md` (new), the coordinator's
  ledger of every other `sqlate` adjustment, split into what `sources` schedules and what stays an
  unscheduled backlog.
- **Integrated:** `experiments/blobfs/REVIEW.md`, `DECISIONS.md`, and `NOTES.md` merged into one
  closed-experiment record, describing the experiment as it stood at close (stage 32 and five
  follow-up commits) rather than at stage 16; `DECISIONS.md` and `NOTES.md` removed, and every
  stale reference to either repointed at `REVIEW.md`. `design/auth-strategy.md` §8 amended to
  state the directory grain alongside the file grain (the read anchored on a directory an owner
  row authorized, not a join row per file, and why a move never crosses one top-level directory
  into another). `design/storage-strategy.md`'s "migration source" corrected to "migration set."
  Two claims `REVIEW.md` had made stale by later stages were corrected in the same pass:
  `go-storage`'s object-store interface has had `List` since v0.1.0 (the concept's contrary claim
  was wrong), and the bookmark-versus-delete race is closed, not open (adjustment 10, stage 23).
- **Culled:** the `blobfs.experiment` roadmap task, finished, and its entry in `next`. The
  original `blobfs.md`'s CLI-shaped and ownership-shaped content (the directive-filter workaround,
  the interim "path projection" pattern, the consumer's ownership vocabulary) is gone from the
  library concept; it was never the library's, only the tool's, and now lives, correctly
  attributed, in `blobfs-composition.md`.
- **Retained:** every other design note and concept the review touched only by citation, unchanged.
- **Roadmap:** `blobfs.sources`'s summary and proof grew to six areas of `sqlate` (the collection
  read, the guard, verification and headers, the mapper, errors, and `migrate`), not the migrator
  alone — the architect's call, sorted by whether each adjustment strengthens `sqlate` and its
  consumers generally rather than by how much work it is. `blobfs.build`'s summary states that
  `blobfs-api.md` is input to its own SETTLE, not a finished design. `blobfs.admin` and
  `v1.storage`'s two tasks pick up small corrections (`Force` named explicitly, the seed step, the
  new documents cited).
- **Architecture layer:** nothing promoted at this close. The candidates the earlier handoff named
  (navigation one directory at a time; engine packages owning their DDL and native variants;
  migration sets as layers; ids as the primary handle) stay recorded as deferred, with their
  triggers, in `concepts/blobfs.md`'s "Deferred, with triggers" section; `blobfs.build` is the
  named trigger for the adjacency position, and `go-auth` as the second shipper for the
  object-namespace amendment.
- **Cross-repo:** none; every edit this close made lands in `standards-lab`, the coordinator.

## Next-focus

`blobfs.sources`, in the `sqlate` repository: promote the experiment's multi-set migrator shim into
`sqlate` v0.2.0 alongside the `sqlate` adjustments the architect chose to fold in at close — a
parameterized projection base, a total mode and keyset cursor on `Directives`, a guard that returns
the row and can carry a status predicate, verified field types, a resolved library namespace, a
multi-line native declaration (with `sqlint` updated in lockstep, not trailing), a schema-qualified
history check, and a mapper that flattens embedded structs. Start from
`context/concepts/migration-sets.md` and `sqlate-library-support.md`'s "Scheduled in
`blobfs.sources`" section; both already carry the exact shapes settled in this session's SETTLE.
`sqlate`'s own repository does not yet have a marathon `context/`; the first session there should
check whether to initialize one before planning the task in earnest.
