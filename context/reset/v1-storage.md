# reset · v1-storage

- **Status:** closeout
- **Lane:** finished. `blobfs.build`, `blobfs.admin`, `v1.storage.service`, and
  `v1.storage.suite` are done.
- **Session:** start
- **Project:** go-storage, go-web-sdk, go-web-sdk-template, go-web-service, standards-lab
- **Branch:** `storage-suite` in go-web-service; `storage-suite-timeouts` in go-storage,
  go-web-sdk, and go-web-sdk-template (merged and released); `storage-suite-close` in go-storage;
  `v1-storage` at the coordinator

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
- This lane is finished: PR #51 merges and the worktree is removed at this close.
- This departs from marathon 0.15's `mechanics/waves.md`, where a lane's record, worktree, and
  coordinator branch are named after its first step, the coordinator branch is per step, and
  the worktree is removed at each close.

## Disposition

This session resumed `goals.v1.storage.tasks.suite` at checkpoint 2c, ran the architect's three
adjustments, and closed the task, which finishes the lane.

- **Integrated:** none at the coordinator; the lane records coordinator edits for the fold.
- **Add or sharpen:**
  - go-web-service `context/domain-architecture.md`: the service exposes its store as
    `Verifier`, the seeder verifies the stores the root lists, and a layer that moves bodies takes
    the server's transfer factory at its construction site.
  - go-storage `context/README.md`: the known limit "`Get` returns the raw body, nothing retries
    it" is dropped; azureblob/v0.4.0 lifted it ([PR #9](https://github.com/standards-lab/go-storage/pull/9)).
- **Promoted:** none at a lane's close; the candidates are under Pending at the fold, now five:
  the three domain-architecture rules, the package-comment inventory rule, and the timeouts
  convention.
- **Promotion evaluation** (what the storage suite moved outward, and what stayed):
  - Moved: the file protocols (`Write`, `Remove`, the `ObjectPutter`) to blobfs; the attachment
    header, the 5xx cause logging, and `PathUUID` to go-web-sdk; `ErrContainerNotFound` to
    go-storage; the per-route transfer deadlines to go-web-sdk (`Transfer`, v0.14.0); the resumed
    and idle-bounded download body to go-storage (v0.4.0).
  - Stayed in go-web-service: `Serve`, the sweep's gate and logging policy, the reactor and the
    gate in `sdk` (they wait on `v1.messaging`'s reconciliation with spike-messaging), and
    `data.ErrBodyTimeout`'s classification, which is the service's own policy.
- **Cross-repo:**
  - go-web-service `storage-suite`: the explicit verifier list, the rootless alias's refusals,
    the transfer deadlines and the 408, the pins, and the close's Adjust
    ([PR #34](https://github.com/standards-lab/go-web-service/pull/34), the architect merges).
  - go-storage v0.4.0 and azureblob/v0.4.0 ([PR #8](https://github.com/standards-lab/go-storage/pull/8)):
    `Config.ReadIdleTimeout`, and a `Get` body that resumes past `try_timeout` with its read
    failures classified; the azureblob pin went straight to `main`, as earlier pins did.
  - go-web-sdk v0.14.0 ([PR #33](https://github.com/standards-lab/go-web-sdk/pull/33)): `Transfer`
    and `Config.TransferRate`, with the read and write timeouts' defaults down to 30s (breaking).
  - go-web-sdk-template template/v0.11.0 ([PR #21](https://github.com/standards-lab/go-web-sdk-template/pull/21)):
    the SDK's new defaults in `config.json`, pinned to go-web-sdk v0.14.0 (the architect's call
    to carry the convention into the SDK and the template).
- **Validated:**
  - Checkpoint A (go-storage): a 48 MiB blob read in 1 MiB chunks over 4.9s through a 1s
    `try_timeout` returned intact across 6 GETs; unit tests cover the idle cut-off; the Azurite
    acceptance suite. Each library then had a branch review (Opus), an Adjust, an editor pass,
    and green CI before its merge and tag.
  - Checkpoint 3 (go-web-service): GOWORK=off build, vet, test -race, lint, sqlint, and tidy for
    the service and slab; `mise run integration`; a live service with `/readyz` all ready and
    `slab demo storage` 16/16. The architect ran a hand walkthrough on the live stack and
    confirmed it: a slow 8 MiB download past `try_timeout` intact, an Azurite pause cut off at
    about 33s with the service still ready, and a paced upload 201 while a slower one answered
    408.
  - The close: the branch review's ten findings fixed as an Adjust (a stalled store's 503 lost
    past the write timeout, fixed by a 5s `try_timeout` and a budget test; the verifier list
    tied to every registered statement by a test; the reset outside the tight timeouts), then
    every check and `mise run integration` again, confirmed by the architect.

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
  go-storage v0.4.0, go-web-sdk v0.14.0, go-database v0.7.0, blobfs v0.5.0, template v0.11.0
  (superseded by the close, below).
- **Architecture layer (candidate, landed at the fold):** the architect decided at the final
  review that each package comment (`doc.go`) stays the authoritative API description, carrying
  an inventory that names every exported identifier with a one-clause role, while each contract
  is stated once, on its symbol. `standards/go-elemental/principles/tests-and-docs.md` says the
  first half; the inventory rule and "one contract, one home" are the addition to land there.
- **Plugin:** unchanged from above.

Added by the `v1.storage.suite` close (applied at the fold):

- **Roadmap:** everything the `v1.storage.suite` session recorded above stands; the task's close
  is this one, so `v1.storage.suite` is deleted, the lane leaves `next`, and `goals.v1.storage`,
  then empty, is deleted. `backlog` gains, beside the pins the libraries owe: go-web-sdk's
  `middleware/rate-limit` pin to go-web-sdk v0.14.0 (left at v0.13.0 rather than cut a release
  for a pin; Go's version selection gives consumers v0.14.0 regardless).
- **References:** the released versions `references.md` and the profiles name are go-core v0.5.0,
  go-storage v0.4.0 with azureblob/v0.4.0, go-web-sdk v0.14.0, go-database v0.7.0 with
  postgres/v0.4.0, blobfs v0.5.0 with postgres/v0.3.0, sqlate v0.4.1, and go-web-sdk-template
  template/v0.11.0.
- **Architecture layer (candidate, landed at the fold):** the timeouts convention, validated
  across go-web-sdk, go-storage, and go-web-service (the architect's call at the close):
  - the server's read and write timeouts are tight, sized for a request that moves no large
    body; a route that moves one sets its own connection deadlines from the body's size, capped
    at the route's limit, and a minimum client rate (go-web-sdk `Transfer`);
  - a store's per-try deadline bounds one operation, never a transfer: a download's body resumes
    past it, and an idle bound on each body read, counted only while a read is in progress, cuts
    off a stalled store without cutting off a slow client (go-storage `ReadIdleTimeout`);
  - a stalled store's whole retry budget fits inside the write timeout, so its 503 is written,
    and a configuration test holds that budget;
  - an upload body's read deadline is the client's (408); the store buffers far enough ahead
    that a stalled store is never charged to the client.
- **Coordinator record:** besides naming this lane `v1-storage`, `context/reset.md`'s lane list
  records it finished.

## Next-focus

Lane finished.

The wave does not fold: the `messaging-experiment` and `ai-experiment` lanes are still open. The
session that finishes the last of them folds the wave and applies this record's Pending at the
fold. The architect names the next session's lane.
