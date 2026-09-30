# reset · v1-storage

- **Status:** handoff (the `v1.storage.suite` task, before its close; the lane stays open)
- **Lane:** open. `blobfs.build`, `blobfs.admin`, and `v1.storage.service` are done;
  `v1.storage.suite` is at its last checkpoint.
- **Session:** start
- **Project:** go-core, go-storage, go-web-sdk, go-database, blobfs, go-web-sdk-template,
  go-web-service, standards-lab
- **Branch:** `storage-suite` in go-web-service (open, unpublished, at `b70f184`); every library
  branch this task opened is merged and released; `v1-storage` at the coordinator, in the worktree
  `.claude/worktrees/v1-storage` ([PR #51](https://github.com/standards-lab/org/pull/51) stays
  open)

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

This session ran `v1.storage.suite` through checkpoints 1, 2, 2b, and 2c; the task's close is next.

- **Released** (each tagged on a green `main`, after a branch review, an Adjust, and an editor pass):
  - Checkpoint 1: go-storage v0.2.0 + azureblob/v0.2.0, go-web-sdk v0.12.0, go-database v0.6.2,
    blobfs v0.3.0 + postgres/v0.3.0.
  - Checkpoint 2b: go-storage v0.2.1, blobfs v0.4.0.
  - Checkpoint 2c (the final whole-suite review, the architect's scope "everything, API
    included"): go-core v0.5.0; go-storage v0.3.0 + azureblob/v0.3.0; go-web-sdk v0.13.0 +
    middleware/rate-limit/v0.2.0; go-database v0.7.0 + postgres/v0.4.0; blobfs v0.5.0 (postgres
    and example pinned, postgres untagged: comments and tests only); go-web-sdk-template
    template/v0.10.0.
- **go-web-service** `storage-suite` (unpublished): adopts every release above; the storage
  suite's integration cases (paging, a refused upload and delete, a storage outage); the
  final review's service findings. Checks at `b70f184`: GOWORK=off build, vet, test -race, lint,
  sqlint, tidy (service and slab); `mise run integration` 65s; live `/readyz` all ready and
  `slab demo storage` 16/16.
- **Add or sharpen:** each member repository's `context/` notes were brought current in its own
  branches (go-web-service's `context/domain-architecture.md`, `data-layer.md`,
  `integration-tier.md`; blobfs's and go-storage's capability maps). No coordinator note changed:
  the lane records coordinator edits for the fold.
- **Promoted:** none at a handoff. Candidates for the close: the three domain-architecture rules
  already under Pending at the fold, and the package-comment inventory rule (below).
- **Validated:** see Next-focus for the checkpoint 2c evidence; the close's own validation is
  still to run.

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

Added by the `v1.storage.suite` session (applied at the fold, alongside the task's close):

- **Roadmap:**
  - Delete `v1.storage.suite`; the lane leaves `next`; `goals.v1.storage`, then empty, is
    deleted.
  - Everything the `v1.storage.service` close recorded for `v1.storage.suite`'s growth is done
    (the library promotions shipped) and lapses.
  - `v1.messaging`'s intake keeps the reactor and the gate staged in go-web-service's `sdk`
    (the architect's call at this task's final review): reconcile with spike-messaging's
    `core/reactor` before go-core takes either.
  - Backlog: the pin moves the libraries still owe (sqlate/postgres and sqlint to sqlate v0.4.1);
    a scalar-subquery count so a page past the last reports a total (sqlate); blobfs's orphan
    reconciler stays deferred to `v1.messaging` (blobfs `context/deferred.md`).
- **References:** `references.md` and the profiles name the released versions: go-core v0.5.0,
  go-storage v0.3.0, go-web-sdk v0.13.0, go-database v0.7.0, blobfs v0.5.0, template v0.10.0.
- **Architecture layer (candidate, landed at the fold):** the architect decided at the final
  review that each package comment (`doc.go`) stays the authoritative API description, carrying
  an inventory that names every exported identifier with a one-clause role, while each contract
  is stated once, on its symbol. `standards/go-elemental/principles/tests-and-docs.md` says the
  first half; the inventory rule and "one contract, one home" are the addition to land there.
- **Plugin:** unchanged from above.

## Next-focus

Resume `goals.v1.storage.tasks.suite` in go-web-service on `storage-suite` (at `b70f184`), with
two adjustments the architect approved after checkpoint 2c, then close the task and the lane.

1. **Statement verification from an explicit list** (the architect's call): the composition root
   hands the seeder every store as an explicit verifier list (both domain stores and blobfs's
   `FS`), separate from the seed contributions, so a store that seeds nothing is still verified.
   Verification stays one pass at the schema stage (go-database v0.7.0's `Start` runs the
   seeder's `Verify` whenever a seeder is configured). `data.NewSeeder` gains the list;
   `data.Contribution.Verifiers` goes. Test: a store with no seed contribution is verified.
2. **The document root alias refuses what a real root refuses** (fix now): before its first
   write, the `root` alias answers 200 with an empty page for a bad sort, filter, or cursor,
   where a real root answers 400. Validate the query (directives and cursor) on the alias path
   as the listing would (`domain/document/storage.go` `rootless`).
3. **Timeouts by the Go convention** (the architect's call, "full convention"):
   - the server: a short `ReadHeaderTimeout`; `ReadTimeout` and `WriteTimeout` tight
     server-wide; the upload and download routes widen their own deadlines through
     `http.ResponseController` (`SetReadDeadline`, `SetWriteDeadline`), sized from the route's
     size limit and a minimum client rate;
   - the store: `try_timeout` sized for a single operation, not a whole transfer; a download
     body gets a read-idle (progress) timeout, reset on each read that makes progress, so a
     stalled store is cut off and a slow, steady client is not. That is a go-storage feature (a
     `read_idle_timeout`-style option on azureblob, or a Store-level reader): a go-storage
     release, then the pin;
   - classification by the deadline that fired: the request body's read deadline is the client's
     (408), the store's context or idle deadline is the upstream's (503 or 504); `data.ErrBodyRead`
     and the timeout ambiguity in `PutObject` resolve into that.
   Each library change runs as before: branch `storage-suite-timeouts` (or similar), a branch
   review, an Adjust, an editor pass, a PR, CI, merge, tag; go-web-service pins it.
4. Then checkpoint 3's final validation (GOWORK=off checks, `mise run integration`,
   `slab demo storage` on a live service) and `close`: the branch review, go-web-service's PR
   (the architect merges), this record rewritten as the closeout with the promotion evaluation
   (what moved outward: the file protocols to blobfs, the attachment and 5xx logging and
   `PathUUID` to go-web-sdk, `ErrContainerNotFound` to go-storage; what stayed: `Serve`, the
   sweep's gate and logging policy, the reactor and gate in `sdk`), `Lane finished.`, PR #51
   merged, and the worktree removed. No other lane has finished, so the wave does not fold.

Stages: the task's list (1–28) is committed and released; checkpoint 2c was reported and the
architect answered it with items 1–3 above, which run before checkpoint 3. The dev stack
(go-web-service's compose Postgres and Azurite) is up; a dev server built from `b70f184` was
started for the checkpoint and may still be running on :8080.
