# reset · messaging-experiment

- **Status:** closeout
- **Session:** plan
- **Project:** standards-lab
- **Branch:** messaging-experiment

## Disposition

- **Add or sharpen:**
  - New `context/messaging.md`. It holds:
    - the messaging experiment's question, and the decision the answer changes
    - five decisions: the CloudEvents 1.0 envelope, the transactional outbox, NATS JetStream plus
      an in-memory provider, a source-agnostic reactor contract, and a scope of the standard tier
      plus one native request-and-reply use
    - the outbox sequencing, carried over from spike-blobfs's two-step protocol
    - the go-core and go-messaging split, as the experiment's hypothesis
    - the capability ledger, which classes signal-lab's capabilities through the standard and
      native split
    - the proof list and the experiment's home
  - New `context/messaging-api.md`: the provisional API the experiment starts from.
  - `context/README.md` lists both notes.
- **Retained:**
  - signal-lab stays in `references.md` as prior R&D. The ledger cites it and doesn't copy it.
  - For the architecture layer after the experiment validates, two candidates. First, sharpening
    `service-tiers.md`'s "no formal standard" for messaging to "CloudEvents for the envelope, and
    operations the organization establishes". Second, the reactor contract's home in go-core.
    Neither is promoted, because both notes are provisional.
- **Roadmap** (recorded for the wave fold, not applied):
  - `goals.v1.messaging`: add `context = ["standards-lab/context/messaging.md"]`. The summary
    should also name the CloudEvents envelope, outbox emission, and the source-agnostic reactor
    contract, with go-core gaining the `reactor` and `event` primitives as the experiment's
    hypothesis.
  - `goals.v1.messaging.tasks.experiment`: the plan half is done, so its summary reads as the
    `experiment` session setting up spike-messaging and cataloging it. The task stays in `next`.
  - Cross-layer captures from the ledger, one sentence each, citing `messaging.md`:
    - `backlog.response-caching`: NATS key-value as a candidate provider.
    - `backlog.second-providers`: the NATS object store as a candidate go-storage provider, judged
      against go-storage's standard tier.
    - `goals.v1.client`: the WebSocket bridge and NATS-native chat as candidate client transports.
- **Validated:**
  - Checkpoint 1 was a walkthrough that traced a grant write's event through the notes: emission
    in the mutation's transaction, the relay recovering after a stop, a delivery group across two
    replicas, and the adapter stopping the event at the process boundary. The architect
    confirmed it.
  - Final: every cited path and roadmap entry resolves, the notes contradict nothing in
    `architecture/architecture.md` or `service-tiers.md`, and the edit pass was done by the
    session itself.

## Next-focus

`goals.v1.messaging.tasks.experiment`, second session. This is an `experiment` session in
standards-lab that sets up spike-messaging:

- the local directory is `~/experiments/spike-messaging`
- the remote is `github.com/JaimeStill/spike-messaging`
- the founding decisions and the question come from `context/messaging.md`
- its first spike step starts from `context/messaging-api.md`

The catalog entry for `experiments.md` goes in this lane's Disposition and is applied at the wave
fold. After that session, the lane is finished.
