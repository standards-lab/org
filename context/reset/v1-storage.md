# reset · v1-storage

- **Status:** closeout (the `v1.storage.service` task; the lane stays open)
- **Lane:** open. `blobfs.build`, `blobfs.admin`, and `v1.storage.service` are done;
  `v1.storage.suite` follows.
- **Session:** start
- **Project:** go-web-service, blobfs, standards-lab
- **Branch:** `storage-service` in go-web-service
  ([PR #33](https://github.com/standards-lab/go-web-service/pull/33)) and in blobfs
  (merged as PR #2, released); `v1-storage` at the coordinator, in the worktree
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

Released and tagged: **blobfs v0.2.0** and **postgres/v0.2.0**
([PR #2](https://github.com/standards-lab/blobfs/pull/2)): version-guarded deletes and marks,
directory status and migration 0003 with two partial indexes, closed deleting branches, hidden
listings, `Store.Sweep` with stale reclaim, and a consolidation (Engine takes a Variant, one wrap
per method, each documented fact one home). The tag was held until go-web-service's C5 proved the
API.

- **Add or sharpen (go-web-service):** `context/README.md` names the storage libraries and the
  object store, and moves object storage into the baseline; `context/domain-architecture.md`
  names both domain layers and records its three proven rules for promotion;
  `context/data-layer.md` marks the coupling rule settled.
- **Add or sharpen (coordinator):** `blobfs-composition.md` follows the validated composition:
  the upload's pending transaction holds no consumer reference, the owner row cascades, a
  recursive removal is mark and sweep, fixed-id seeds in two contribution kinds, and the
  reset/sweep deadlock; `migration-sets.md` gains the from-v1.0 qualifier, the consumer's own
  referential actions, and quiescing background work around schema changes.
- **Promoted:** none landed (a wave's lane lands nothing); the three candidates are under Pending
  at the fold.
- **Retained:** `context/integration-tier.md` (its webtest helpers wait for `v1.data.people`).
- **Validated:**
  - Checkpoint 4b (blobfs v0.2.0): acceptance at every stage; the demonstration program
    (mark, refusals, hidden listings, `Deleting`, one sweep of rows and objects, stale reclaim);
    a consolidation review and a branch review, each applied as an Adjust; CI green on PR #2 and
    acceptance on `main` with the released pins.
  - Checkpoint 4c (go-web-service C1–C6): the slab demonstration (412/428, 202, deleting, 409
    upload, swept 404, a kill -9 mid-sweep finished after restart).
  - Checkpoint 4d (a layering review, S1–S5): the logo failed-put fix proven failing on the old
    code; owner-row cascade; one stage table; the quiesce gate (a real 40P01 deadlock, 0/60 with
    the gate); cross-organization isolation and the download and logo lifecycles in integration.
  - Checkpoint 5 (13–15): storage seeds (7 logos, acme's tree, fixed ids), `slab demo storage`
    16/16 twice live, the docs checked against the code, the editor pass; the architect's own
    walkthrough of the logo and document features through slab.
  - Close: a branch review, all twelve findings fixed in one Adjust (`fa6d099`); vet, `test
    -race`, lint and sqlint, tidy, `mise run integration` (102s), and `slab demo storage` live.
- **Cross-repo:** blobfs released as above; go-web-service `storage-service` published as
  [PR #33](https://github.com/standards-lab/go-web-service/pull/33).

## Pending at the fold

Every edit below is recorded for the wave's fold, not applied. The groups before "Added by the
`v1.storage.service` session" carry forward from `blobfs.admin`.

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
- **References:** unchanged from above.

Added by the `v1.storage.service` close (applied at the fold):

- **Roadmap:**
  - Delete `v1.storage.service`; the lane in `next` becomes `["v1.storage.suite"]`.
  - `v1.storage.suite` grows, as the architect approved at checkpoint 4d: its `repos` gain
    `blobfs`, `go-web-sdk`, and `go-database`, and its summary gains the library promotions,
    each proven by go-web-service in the same step:
    - blobfs v0.3.0: `Store.Write`/`Remove` with an `ObjectPutter` (promoted from
      go-web-service's `data` protocols), the sweep's more-loop, a listing type, a `Deleting`
      error that tells a file from a directory, and scripted-test row builders;
    - go-storage: a distinct `ErrContainerNotFound`, so a missing container on `Open` is a 503;
    - go-web-sdk: the `Content-Disposition` builder, and the error writer logging a 500's cause;
    - go-database: `admin.Stage`'s doc stops naming its consumers' stage;
    - go-web-service adopts each and drops its staged copies.
  - `v1.messaging`'s intake gains the lifecycle evidence from go-web-service, for the go-core
    component interface (spike-messaging `design.md`, "Lifecycle registration"): the sweeper is
    the service's first real reactor, registered with `Add` + `Monitor` and a `Grace` tied to
    the drain timeout, its waker built before the domain; the schema service's stage is declared
    at the call site (`internal/app/stages.go`) rather than taken from `admin.Stage`; the staged
    reactor gained `Wake`, `Draining`, and a Shutdown guard, and `sdk.Gate` quiesces a reactor
    against the admin verbs.
  - A backlog entry: drop `organization_image.active`, now always true, in a schema cleanup.
- **Architecture layer (promotion candidates, landed at the fold):** from go-web-service's
  `context/domain-architecture.md`, "Promotion candidates", now proven by a second domain layer:
  the domain layer as a compositional grouping; the capability-named translation file; and the
  two-way cross-domain coupling rule (downward an SQL check in the consumer's transaction,
  upward an interface the consumer declares and the root injects).
- **Cross-repo notes:** go-web-sdk's `context/README.md` capability map gains the cursor contract
  and the raw-body helpers of v0.11.0 (moved here from the close, since the lane doesn't own
  go-web-sdk's context).

## Next-focus

Start `goals.v1.storage.tasks.suite`, the lane's last task, once go-web-service's
[PR #33](https://github.com/standards-lab/go-web-service/pull/33) has merged. Its scope is the expanded one under Pending
at the fold: the library promotions (blobfs v0.3.0, go-storage, go-web-sdk, go-database), each
proven by go-web-service in the same step, plus the suite's original integration cases (the
pending row a failed put leaves, the deleting row a failed delete leaves and its retry, the 503 on
an Azurite outage) and the coordinated release snapshot. Its close writes `Lane finished.` here.
