# reset · messaging-experiment

- **Status:** closeout
- **Session:** experiment
- **Project:** standards-lab
- **Branch:** spike-messaging

## Disposition

- **Retained:** `context/messaging.md` and `context/messaging-api.md` stay as the org's record of
  the question and the starting API. The spike carries both over as its own notes and never edits
  them here.
- **Cross-repo:**
  - spike-messaging set up with `marathon init` at `~/experiments/spike-messaging` and pushed to
    `github.com/JaimeStill/spike-messaging`, which is public. Its `[experiment]` table serves
    `org`. Its context holds the question and the eight proofs (`README.md`), the decisions and
    open questions (`design.md`), and the starting API (`api.md`). `references.toml` lists the
    repositories it reads, and the gitignored `references.local.toml` maps them.
  - The spike's `api.md` corrects one signature: `reactor.Register` takes `stage int`, because
    go-core's `lifecycle.Service.Stage` is an int and no `lifecycle.Stage` type exists. The org's
    `messaging-api.md` still says `lifecycle.Stage`. The `plan` session that reads the result
    fixes it or retires the note.
- **Catalog** (recorded for the wave fold, not applied): add this row to `experiments.md`:

  ```
  | spike-messaging | Can one broker-agnostic event and reactor contract, built on CloudEvents with outbox emission, run a service's reactors on NATS JetStream and on an in-memory provider? | [JaimeStill/spike-messaging](https://github.com/JaimeStill/spike-messaging) |
  ```

- **Roadmap** (recorded for the wave fold, not applied):
  - Delete `goals.v1.messaging.tasks.experiment`, and remove `"v1.messaging.experiment"` from
    `next`'s wave. The wave stays while its other lanes are open.
  - Carried from the lane's `plan` session:
    - `goals.v1.messaging`: add `context = ["standards-lab/context/messaging.md"]`. The summary
      should also name the CloudEvents envelope, outbox emission, and the source-agnostic reactor
      contract, with go-core gaining the `reactor` and `event` primitives as the experiment's
      hypothesis. It should add that the experiment runs in spike-messaging, and that its intake
      is the rest of the goal.
    - `backlog.response-caching`: NATS key-value as a candidate provider, citing `messaging.md`.
    - `backlog.second-providers`: the NATS object store as a candidate go-storage provider,
      judged against go-storage's standard tier, citing `messaging.md`.
    - `goals.v1.client`: the WebSocket bridge and NATS-native chat as candidate client
      transports, citing `messaging.md`.
- **Validated:**
  - Final walkthrough: spike-messaging's `main` has one setup commit, pushed and tracking
    `origin/main`. `gh repo view` reports the repository `PUBLIC`, and `references.local.toml` is
    untracked. A fresh `start` there finds `context/reset.md` at closeout, and its Next-focus and
    capability map settle the first step without opening standards-lab. Every local path in
    `references.local.toml` resolves, and go-core's `lifecycle` API was checked against the
    corrected signature.

## Next-focus

Lane finished.

The spike's own sessions run in `~/experiments/spike-messaging`, starting with `start` on
`core/event` and `core/reactor`. When it closes, a `plan` session here reads the result, per
`experiments.md`.
