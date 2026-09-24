# reset · v1-storage

- **Status:** closeout (of the `blobfs.admin` session; the lane stays open)
- **Lane:** open. Two of its four tasks are done. It isn't complete until `v1.storage.service`
  and `v1.storage.suite` are.
- **Session:** start
- **Project:** go-database, standards-lab
- **Branch:** `blobfs-admin` in go-database; `v1-storage` at the coordinator (the lane's branch,
  [PR #51](https://github.com/standards-lab/org/pull/51), open until the lane is complete)

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

- **Integrated:** `context/migration-sets.md`, "What the admin surface exposes": go-database's
  `admin/doc.go` and README now document the admin service over several sets. The note's
  opening points there.
- **Retained:** `context/migration-sets.md` (the shipper guarantees, the consumer adoption, and
  the assumptions) and `context/blobfs-composition.md`, both for `v1.storage.service`.
- **Promoted:** nothing. The admin service's policy is go-database's own.
- **Cross-repo:**
  - go-database v0.6.0 ([PR #23](https://github.com/standards-lab/go-database/pull/23)),
    tagged `v0.6.0` once merged; `postgres/` is not re-tagged. It pins sqlate v0.4.0. The admin
    service administers every set of a multi-set migrator:
    - `Status` is `{Ready, Sets []SetStatus}`.
    - `Start` logs the pending migrations by set.
    - `Reset` reverts every set, the last declared first, dropping each history table, then
      reapplies and seeds.
    - `Down`, `Steps`, and `Force` take a required set name, and `ErrUnknownSet` refuses an
      empty or undeclared name before any I/O.
    - `Status` clears Ready on a history it cannot account for.
    - Breaking: the Status shape and the verb signatures.
- **Validated:**
  - Checkpoint 1: a program run on PostgreSQL 17 over blobfs/postgres v0.1.0's set with a
    consumer set above it. Start logged the pending migrations by set. Status reported each set.
    A failed non-transactional migration left `app` dirty, and Up was refused with `ErrDirty`
    naming it. After a manual fix, `Force("app", 1)` and then Up left both sets clean.
    `ErrAboveApplied` refused a Down of `blobfs`, and `ErrUnknownSet` refused an empty name.
    Reset worked across both sets with the seed reapplied. A single-set migrator worked
    unchanged.
  - Checkpoint 2: `mise run build` (GOWORK=off), vet, test with -race, lint, and tidy with no
    diff. The Checkpoint 1 program was rerun. The editor pass ran on Opus. CI was green on PR
    #23.
  - The branch review (reviewer on Opus) returned three defects, fixed as an Adjust (`b2bb413`)
    and rechecked, with CI green:
    - Status left Ready stale on an unknown history row.
    - Start logged "applying" when a dirty set would refuse the run.
    - New's comment was stale.

    The review's test gaps are closed.

## Pending at the fold

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

## Next-focus

`goals.v1.storage.tasks.service`, in go-web-service, with the coordinator's record in the
worktree `.claude/worktrees/v1-storage` on `v1-storage`. It starts once go-database PR #23 has
merged and `v0.6.0` is tagged. PR #51 stays open.
- The organization storage layer registers blobfs's migration set beneath the service's own in
  its one migrator.
- It proves the set through the admin surface, including Reset and Force by set name, on
  go-database v0.6.0.
- `context/migration-sets.md` (consumer adoption) and `context/blobfs-composition.md` are its
  starting notes.
