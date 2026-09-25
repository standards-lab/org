# reset · v1-storage

- **Status:** handoff (the `v1.storage.service` session, partway through; the lane stays open)
- **Lane:** open. `blobfs.build` and `blobfs.admin` are done; `v1.storage.service` is in progress;
  `v1.storage.suite` follows.
- **Session:** start
- **Project:** go-web-service, blobfs, go-web-sdk, sqlate, go-database, standards-lab
- **Branch:** `storage-service` in go-web-service (unpublished, last commit `cd35d5a`) and in blobfs
  (unpublished, last commit `6a2cd30`, a WIP); `v1-storage` at the coordinator, in the worktree
  `.claude/worktrees/v1-storage` ([PR #51](https://github.com/standards-lab/org/pull/51) stays open)

## Orchestration

This lane is rooted in a goal, not in its first step. Every later session of the lane follows
these rules until the workflow carries them (see "Plugin" at the end of Pending at the fold):

- The lane's goal is `v1.storage`. Its tasks run in order: `blobfs.build` (done),
  `blobfs.admin` (done), `v1.storage.service`, then `v1.storage.suite`. A task's close finishes
  that task, never the lane. The lane is complete only when the whole sequence is, and only
  `v1.storage.suite`'s close writes `Lane finished.` here.
- This record is named after the goal, `context/reset/v1-storage.md`, and persists across the
  lane's sessions. It was `blobfs-build.md`, named after the lane's first step.
- The lane's worktree at the coordinator is `.claude/worktrees/v1-storage`, on the lane's own
  branch `v1-storage`. Both persist across the lane's sessions. Each session commits its record
  and note changes there and pushes to the open
  [PR #51](https://github.com/standards-lab/org/pull/51). The PR merges, and the worktree is
  removed, only when the lane is complete.
- In a member repository, branches stay per task, named after the task. A task starts only once
  the previous task's member branch has merged and any release it cut is tagged.
- Each close rewrites the Disposition for its own step and carries every unapplied fold edit
  forward under Pending at the fold, so nothing a session records for the fold is lost.
- This departs from marathon 0.15's `mechanics/waves.md`, where a lane's record, worktree, and
  coordinator branch are named after its first step, the coordinator branch is per step, and
  the worktree is removed at each close.

## Disposition

This session is a handoff, so the notes are untouched: every note edit waits for the close, and
this Disposition records what the session released and decided. Released and tagged:

- **go-web-sdk v0.11.0** ([PR #30](https://github.com/standards-lab/go-web-sdk/pull/30)) and
  `middleware/rate-limit/v0.1.1` (a pin commit on `main`). The read contract's cursor (opt-in through
  `Limits.Cursor`; `Page` gains `more` and `next`; `Total` is omitted when uncounted; `NewPage` takes
  `Paging`), `ReadUpload`/`UploadError` (415/411/413), `WriteObject` (an opener called only when bytes
  are sent; ETag and date revalidation; nosniff), and `webtest.Raw`.
- **sqlate v0.4.1** ([PR #12](https://github.com/standards-lab/sqlate/pull/12)): `Force` writes the
  set's whole history prefix through the version. Found when the app set gained a second migration.
- **go-database v0.6.1** (a pin commit on `main`): it requires sqlate v0.4.1.

Decided with the architect (they bind the rest of the task):

- The organization path read is `GET /organizations/lookup?path=`: ServeMux refuses
  `/path/{path...}` beside any `GET /{id}/<sub>`.
- The logo fallback and a base64 read are deferred to `v1.client`.
- The rest are the re-plan's settled decisions, listed in Next-focus.

**Promotion candidates for the close** (at a handoff, nothing is promoted):
- the domain-coupling rule (downward: an SQL check in the consumer's transaction; upward: an
  interface the consumer declares, injected);
- the capability-named translation file, now proven by two layers.

## Pending at the fold

Every edit below is recorded for the wave's fold, not applied. The groups before "Added by the
`v1.storage.service` session" carry forward from `blobfs.admin`.

Every edit below is recorded for the wave's fold, not applied. The first four groups carry
forward from the `blobfs.build` close.

- **Roadmap:**
  - Delete `blobfs.build` and `blobfs.admin`; the lane in `next` becomes
    `["v1.storage.service", "v1.storage.suite"]`. `goals.blobfs`, left empty and with no
    criteria, is deleted with them; the `blobfs.build` close's edits to its summary and context
    lapse, since blobfs's own `docs/concepts.md` and `context/deferred.md` carry the library.
  - `v1.storage.service` and every other entry citing `standards-lab/context/blobfs.md` cite
    `blobfs/docs/concepts.md` instead; the ordering comment's citation likewise.
  - `v1.storage.service`: go-web-service moves to go-database v0.6.0. Its
    `admin/database` handler adopts the per-set `Status` and the set-named `down`, `steps`, and
    `force`, maps `admin.ErrUnknownSet` to a 4xx, and puts `reset` behind an explicit
    confirmation.
  - `v1.data`: go-web-service moves from sqlate v0.1.1 to v0.4.0, which changes
    `domain/organization/database.go`'s collection reads, retypes any hand-written scan to
    `query.Row`, and reports `query.NoTotal` on an empty later page
    (`standards-lab/context/sqlate-library-support.md`).
  - `backlog.second-providers`: a second SQL engine includes blobfs's own engine sub-module.
  - The `next` header comment: the storage lane is the goal `v1.storage`, with blobfs's two
    tasks as its first steps.
- **References:** `references.toml` gains `[repos.blobfs]`,
  `remote = "https://github.com/standards-lab/blobfs.git"`, with no `standard` key and a comment
  that it is adjacent, as sqlate's is; `references.md` gains a `### blobfs` paragraph beside
  sqlate's.
- **Workspace order:** `.claude/marathon.toml`'s `order` gains `blobfs` in the group with
  `go-storage`, after `sqlate`, which it depends on.
- **Experiments:** `experiments.md`'s spike-blobfs entry records its promotion to
  `standards-lab/blobfs`.
- **Coordinator record:** `context/reset.md`'s lane list names this lane `v1-storage`
  (`context/reset/v1-storage.md`) instead of `blobfs-build`.
- **Plugin:** not a fold edit. After the wave finishes, the architect takes up the workflow in a
  separate session that refines the ontology of its concepts. It is input to that session,
  together with the messaging lane's recorded candidate (a lane is a goal). This lane's
  Orchestration rules are the working example:
  - a lane rooted in a goal whose tasks run in sequence
  - a record, worktree, and coordinator branch named after the goal, persisting until the goal
    is done
  - per-task member branches
  - unapplied fold edits carried forward at each close

Added by the `v1.storage.service` session (applied at the fold, alongside the task's close):

- **Roadmap:**
  - Delete `v1.storage.service` at its close.
  - Remove the sqlate move from `v1.data`: this task absorbed it, and the service is on sqlate v0.4.1.
  - `v1.client` gains the logo fallback and the `<img>`-with-cookie versus bearer-token fetch
    question.
  - `v1.messaging`'s intake gains the reactor reconciliation: the demand and schedule flavors staged
    in go-web-service's `sdk/reactor.go` against spike-messaging's `core/reactor`, before go-core
    takes a reactor.
  - A backlog entry for the general sweeper, carrying its activation rule (below).
- **Notes (new at the coordinator):**
  - `context/reactor.md`: the reactor as the async primitive; the event flavor is the spike's, the
    demand and schedule flavors are this lane's.
  - `context/sweeper.md`, the general sweeper concept:
    - a data-oriented garbage collector, a level-triggered reconciler whose durable state lives with
      the data owner;
    - the `Reclaimer` contract `Pass(ctx, budget) (more bool, err error)`: idempotent, convergent,
      bounded;
    - registration by name with a policy, as lifecycle's services register;
    - multi-replica claiming (`SKIP LOCKED` or an advisory lock);
    - backlog metrics, not readiness;
    - the runner as a reactor;
    - blobfs's `Sweep` as the first reclaimer.

    **Activation rule (binding):** the moment any other layer needs sweeper-like reclamation
    (outbox cleanup, expired sessions, soft-delete purge, or any cascade beyond pruning SQL rows),
    that step builds the general sweeper and moves blobfs's sweep onto it as its first reclaimer.
    It never builds a second standalone reactor. It stays out of `architecture/` until it
    graduates.
  - The sqlate ledger (`context/sqlate-library-support.md`): classify a connection lost mid-read
    (`io.ErrUnexpectedEOF`) as `ErrConnectionFailed`. go-web-service's `data.Status` maps it to 503
    meanwhile.
  - At their next release, go-database's, blobfs's, sqlate/postgres's, and sqlint's sqlate pin
    moves to v0.4.1.
- **At the close** (go-web-service, no fold needed):
  - `context/domain-architecture.md` gains the coupling rule and the promotion evaluation now that
    a second layer exists.
  - go-web-sdk's `context/README.md` capability map gains the cursor contract and the raw-body
    helpers.
  - `blobfs-composition.md` and `migration-sets.md` are tended against what validation proved.
- **References:** unchanged from above.


## Next-focus

Resume `goals.v1.storage.tasks.service` at **Checkpoint 4b, stage B1**, from this record. Run the
`start` handoff: check out `storage-service` in blobfs and go-web-service, then continue.

**Position:**
- Checkpoints A, 1, 2, and 3 are confirmed.
- Checkpoint 4 (the owner-row migration `69a55ce`, `domain/document` `cbfb379`, `slab docs`
  `cd35d5a`) was reported and then re-planned, not confirmed. Its behavior is superseded by 4b and 4c
  below.
- blobfs `6a2cd30` is a **WIP**: B1 is written, and its unit tier passes, but `mise run acceptance`
  (the conformance suite live, over both engines) has not run. Finish B1 first: run acceptance and
  fix what it finds.

**Settled decisions** (from the architect's review of Checkpoint 4):
- **Reactors:** a reactor is the background-service primitive; no new service kind.
- **Promote on fit:** the reactor is staged in go-web-service, shaped like spike-messaging's
  `core/reactor`.
- **Reads:** deleting branches are hidden from every listing; only a read by id shows
  `status: deleting`; a listing of a deleting directory is 404.
- **Batches:** by record count (about 100), with no total bound; a marked branch takes no new writes.
- **409:** `data.Status` sends a curated `Detail`, never raw error text.
- **Deletes:** both document deletes take If-Match.
- **The sweep:** blobfs owns it, including pending rows past a threshold; the service supplies the
  trigger and the owner-row hook.
- **The general sweeper:** captured as a concept with its activation rule (Pending at the fold).

**Stages.** The delegation is noted per stage. Executors run on Opus and are read firsthand; each
commit fires `on-commit`, which declares no hook.

- **Checkpoint 4b: blobfs v0.2.0** (branch `storage-service`)
  - **B1. Version-guarded deletes (WIP).**
    - `VersionOption`/`AtVersion` on `Files.Delete` and `Directories.Delete`, in one guarded statement
      each.
    - Check: `mise run acceptance`.
  - **B2. Directory status** (executor, with B3).
    - Migration `postgres/migrations/0003_directory_status`: `status text NOT NULL DEFAULT 'active'`,
      `blobfs_cc_directory_status`.
    - Golden hashes (`postgres/golden_test.go`); the tests that assume two migrations; the constraint
      constant.
    - `blobfs.DirectoryStatus` (active, deleting) and `Directory.Status`; `directory_columns` gains
      `d.status` (breaking for 0.x); `postgres/statements/resolve_path.sql` updated.
  - **B3. Mark and refuse.**
    - `Directories.MarkDeleting(ctx, tx, id)`: under `LockTree`, it refuses the root, then marks the
      directory, every descendant, and every file beneath `deleting` in one recursive update, and
      returns the counts.
    - Refused with `ErrDeleting`: a create, ensure, or move of a directory or file whose
      parent or destination is deleting, and a move out of a deleting branch.
    - Inserts take the form `INSERT … SELECT … WHERE parent.status = 'active'`, with a parent read
      to classify a refusal. `data/directories_move.go`'s `ErrRefused → ErrRootDirectory` mapping is
      reworked.
    - Stragglers from the race are caught by the sweep, not locks.
  - **B4. Listings** (executor).
    - A Go-appended filter excludes deleting entries; `IncludeDeleting()` opts out.
    - `List`/`Continue` of a deleting directory is `ErrDeleting`.
    - `Directories.Deleting(ctx, sess, limit)` returns the branch roots.
    - The listing, keyset, refusals, and cost tests move to these defaults.
  - **B5. `Store.Sweep(ctx, sess, objects ObjectDeleter, opts ...SweepOption) (SweepResult, error)`**
    (executor).
    - One stateless, bounded pass with `Batch(n)` (default 100), `OnRemoveDirectory(func(ctx, tx,
      blobfs.Directory) error)` (run in the removal's transaction), and `PendingOlderThan(d)` (off by
      default).
    - The pass: files (object delete, then purge), stragglers marked, directories removed deepest
      first, the branch root last.
    - `SweepResult{Files, Directories, Pending int; More bool}`; convergent.
    - Conformance cases: a full sweep, a crash midway then a finishing pass, stragglers, the hook,
      pending reclaim, a fake deleter.
    - It answers `context/deferred.md`'s sweeper.
  - **B6. Docs and release.**
    - `docs/concepts.md` (a new "Deleting a branch") and `docs/features.md` (every section the
      surface touches); `data/doc.go`, `postgres/doc.go`, `context/deferred.md`; both changelogs at
      `[v0.2.0]`.
    - The editor pass, the branch review, and an Adjust; a PR, CI, merge; tags `v0.2.0` and
      `postgres/v0.2.0`.
  - **Demonstration:** acceptance, plus a scratch program:
    - a stale `AtVersion` returns `ErrVersionMismatch`;
    - mark a three-level branch;
    - a create, an upload, and a move into it are each `ErrDeleting`;
    - the listings hide the branch while `Find` shows `deleting`;
    - `Deleting()` returns the branch root;
    - one `Sweep` removes the branch's rows and objects and calls the hook once;
    - a pending row past the threshold is reclaimed.
- **Checkpoint 4c: go-web-service** (branch `storage-service`)
  - **C1.** Pin blobfs and blobfs/postgres v0.2.0; directory `status` on a read by id; a listing
    within a deleting branch is 404.
  - **C2.** `data.Status` curated details: "an entry with that name already exists", "the directory
    is not empty", "the directory is being deleted", "the file is referenced", and "the request
    conflicts with the current state". Tests show no raw text reaches the wire.
  - **C3.** Both document deletes take If-Match (428 when missing, 412 when stale).
    - `?recursive=true` runs `MarkDeleting` guarded by the version, answers 202 with `Location`,
      and nudges the sweeper.
    - `maxWalk` and the synchronous walk are deleted.
  - **C4. `sdk/reactor.go`** (executor, with C5), bound for go-core.
    - Match spike-messaging's `/home/jaime/experiments/spike-messaging/core/reactor` and its
      `design.md`: `Source`/`Func`, `New` with `Grace`, `Start`/`Shutdown`/`Ready`/`Err`, `Every`.
    - New: `Wake(d)` with a coalescing `Nudge()`.
    - The coordinator adapter is `lc.Add` plus `lc.Monitor(r.Err())`.
  - **C5. The sweeper reactor** in `internal/app/reactors.go`.
    - `Wake(interval)`, calling `Store.Sweep` with `data.Objects` as the deleter and the document
      layer's owner-row unbind as `OnRemoveDirectory`; it loops while `More` is true.
    - Stage 3 (below root, after the domains); a `sweep` config block (interval 30s, batch 100,
      pending age 1h).
    - Its doc comment carries the general sweeper's activation rule.
    - Tests include a crash midway and a finishing pass.
  - **C6.** slab: `--version` on both document deletes, `--recursive` printing the 202,
    `dirs get` showing `status`. `usage.md`, the README, and the CHANGELOG.
  - **Demonstration (slab):**
    1. Build a tree and delete it with `--recursive --version N`: a 202, then `dirs get` shows
       `deleting` and the listings hide it.
    2. An upload into it is a 409 with the curated detail.
    3. After a sweep interval it is a 404, and the container holds none of its objects.
    4. A stale version is a 412; a missing one a 428.
    5. Kill the service mid-sweep and restart it; the sweep finishes.
- **Checkpoint 5 (final)**
  - **13. Seeder:** `data.NewSeeder` takes the storage.
    - `default` seeds a logo per organization from `data/seeds/fixtures/<code>.png`, plus a small
      acme document tree, through `Ensure` with caller-supplied ids; idempotent.
    - The logos: 80×80 PNGs sliced from the architect's `~/Pictures/org-logos.png` (a 3×3 grid:
      acme, engineering, platform; product, operations, logistics; finance at the bottom middle),
      each cropped to its content, padded square with 12% margin, whitened, and downscaled with
      LANCZOS. The session's scratchpad copies are gone, so re-slice them.
  - **13b. `slab demo storage`** (executor): the sets, the logo replace cycle, the document tree
    with a cursor walk, the scoped refusals, a recursive delete swept to 404, and a Reset.
  - **14.** README: Stack, the port list (the partial index), the logo and document API, the admin
    storage group, `APP_STORAGE_*`, the reactor, and tests. The CHANGELOG with its pins; `doc.go`
    files.
  - **15.** The editor pass.
  - **Validation:** build, vet, `test -race`, lint, sqlint, both modules' `tidy -diff`, and
    `mise run integration`; `slab demo storage`; CI green on the PR.
- **Then `close`:** the branch review, tending the notes (Pending's close items), the record at
  closeout, and publishing go-web-service `storage-service`. Next is `v1.storage.suite`.
