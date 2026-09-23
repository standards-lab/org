# reset · messaging-experiment

- **Status:** closeout
- **Session:** experiment
- **Project:** standards-lab
- **Branch:** spike-messaging

## Disposition

- **Add or sharpen:**
  - `context/messaging.md`: "What the experiment settles" adds the lifecycle registration
    question. It asks whether go-core's `lifecycle` gains a component interface with the stage
    kept at the composition root, and which stage a reactor takes. The spike tests it; the
    change isn't a prerequisite to the lane.
  - `context/messaging-api.md`: `reactor.Register` takes `stage int`, because go-core has no
    `lifecycle.Stage` type. The note now says spike-messaging's `api.md` owns the API.
- **Retained:** both notes stay as the org's record of the question and the starting API. The
  spike carries them over as its own notes and never edits them here.
- **Cross-repo:**
  - spike-messaging set up with `marathon init` at `~/experiments/spike-messaging` and pushed to
    `github.com/JaimeStill/spike-messaging`, which is public. Its `[experiment]` table serves
    `org`. Its context holds the question and the eight proofs (`README.md`), the decisions and
    open questions (`design.md`), and the starting API (`api.md`). `references.toml` lists the
    repositories it reads, and the gitignored `references.local.toml` maps them.
  - The spike's `README.md` lists its path, five `start` steps to the final validation. Its
    `api.md` recasts the reactor as a lifecycle component (`New`, `Start`, `Shutdown`, `Ready`)
    that the composition root registers at a stage it chooses. Its `design.md` records the
    lifecycle registration hypothesis.
- **Catalog** (recorded for the wave fold, not applied): add this row to `experiments.md`:

  ```
  | spike-messaging | Can one broker-agnostic event and reactor contract, built on CloudEvents with outbox emission, run a service's reactors on NATS JetStream and on an in-memory provider? | [JaimeStill/spike-messaging](https://github.com/JaimeStill/spike-messaging) |
  ```

- **Roadmap** (recorded for the wave fold, not applied):
  - `goals.v1.messaging.tasks.experiment` stays in `next`. Its summary becomes the spike's setup
    and its run through its path, followed by the intake session. The intake session's close
    records the task's deletion.
  - The `next` header comment: a wave's lane is a goal, and its tasks run in order, not a single
    task. A lane finishes only when its goal's work is done, which for an experiment means the
    intake session, since the spike can't edit this repository. The wave waits for every lane. The
    manifest format already allows a goal as a wave member (marathon-roadmap
    `references/manifest.md`). A candidate for the plugin itself: say this in marathon's wave
    rules, which today describe a lane as "a single step, or a sequence of steps".
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
  - Final walkthrough: spike-messaging's `main` has the setup commit and the path commit, pushed
    and tracking `origin/main`. `gh repo view` reports the repository `PUBLIC`, and `references.local.toml` is
    untracked. A fresh `start` there finds `context/reset.md` at closeout, and its Next-focus and
    path settle the first step without opening standards-lab. Every local path in
    `references.local.toml` resolves, and go-core's `lifecycle` API was checked against the
    corrected signature.

## Next-focus

The lane stays open until the spike finishes and its result is taken in. Its remaining steps, in
order:

1. The spike's five `start` sessions, run in `~/experiments/spike-messaging` from that project's
   own reset file, following the path in its `context/README.md`. Step 1 builds `core/event` and
   `core/reactor`. The spike never edits this repository, so this record doesn't change while
   they run.
2. After the spike's final close, a `plan` session here, on `v1.messaging.experiment`. It reads
   the spike's close Disposition (the question, the answer, and the evidence). It decides with the
   architect what `v1.messaging` and go-core take from it, including the lifecycle registration
   question in `context/messaging.md`. It records that as notes, and as roadmap edits for the
   fold. It confirms the spike is pushed, archives its remote, and writes `Lane finished.` here.
   If that finishes the wave's last lane, it folds the wave.
