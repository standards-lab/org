# Messaging

The event and reactor layer for a web service: one consolidated contract through which a service
emits events and runs reactors without knowing which broker carries the events between services.
go-core and go-messaging implement it, and their behavior is documented in the `go-messaging`
README and the package documentation of go-messaging and go-core's `event`, `reactor`, and
`lifecycle`; this note holds the decisions. spike-messaging's evidence and the
capability ledger are in [the messaging docs](../docs/messaging.md).

The architecture fixes the vocabulary, Reactor and event (`architecture/architecture.md`, "The
elements"): it defines emission only, and delivery guarantees belong to the messaging system.
`go-web-service/internal/app/reactors.go` is the service's reactors layer, and `auth-strategy.md`
§9 commits grant and identity-link writes to emit events.

## Decisions

- **The envelope is CloudEvents 1.0**, in binary mode with its attributes as message headers under
  the CloudEvents NATS protocol binding. `service-tiers.md` says messaging has no formal standard;
  that is true of broker operations, not of the event's shape: CloudEvents is the standard tier's
  event vocabulary, the way OpenTelemetry is observability's. Rejected: an envelope of our own
  evolved from signal-lab's `Signal`.
- **The CloudEvents type is go-core's own**, in `event`, on the standard library alone. Rejected:
  `github.com/cloudevents/sdk-go/v2/event`, whose event package imports json-iterator and whose
  module brings zap, testify, and x/time into the graph.
- **Emission goes through a transactional outbox**, written through `Recorder[Tx]` in the
  command's own transaction and stamped only when the command succeeds. `Tx` is a type parameter
  the composition root fixes, so a pool fails to compile and go-core names no database. Delivery
  is at least once. Rejected: publishing after commit, which loses the event when the process
  stops between the two.
- **An inbox table backs idempotency**, claimed in the handler's own transaction. A domain never
  sees the event: the reactor's adapter hands its command a claim function. Deduplication keys on
  the event's source and id, on the broker (`Nats-Msg-Id`) and in the inbox.
- **The reactor contract is source-agnostic.** A reactor runs one source of occurrences, and the
  source decides what a handler error means. `reactor` is its own package, importing neither
  `lifecycle` nor `messaging`.
- **A subscription's `Name` is its durable consumer and its delivery group.** Rejected: a
  separate `Group` field, since JetStream has no second level of grouping within a durable.
- **A subscription carries a binding start position**, which places only a new consumer; a
  mismatch under an existing `Name` fails the bind. A service binds `StartNew`.
- **`MaxDeliver` is set per subscription**, never as a default: a subscription whose input can
  arrive early bounds its redelivery.
- **The providers are NATS JetStream and in-memory**, the conformance and test double. The tier
  stays provisional until a second real broker proves it (`second-providers`, `service-tiers.md`).
- **A broker constructs without I/O and starts as a lowest-layer graph node**, so an outage makes
  it not ready rather than ending the process.
- **A service's messaging is one `messaging.Runtime`**; the provider's configuration block sits
  beside its `messaging` block.
- **Failures the runtime survives reach one error hook**, which logs by default and which a
  service replaces to throttle, count, or alert.
- **A stream has one owner**, which creates or updates the stream; every other broker writes
  nothing and binds it. This removes the spike's last-writer-wins `CreateOrUpdateStream` on every
  broker.
- **An event's NATS subject is its type, unchanged**, and the nats block's prefix is the namespace
  every type must start with, not a token prepended to it. This and the one owner capturing
  `<prefix>.>` govern a web service's events for committed mutations of executed commands. A job's
  progress and result stream, such as Cerebro's (`cerebro.job.<job>`, one subject per job for the
  job's gap-free sequence), is a different use case these conventions don't govern.
- **The scope is the standard tier plus one native use**: request and reply through the `nats`
  provider's `Conn()`, in the composition root alone. The rest of the capability ledger is not
  built.

## Outbox sequencing

The relay is a polling `reactor.Source` and its own sweeper over unpublished rows. Each row is
handled in a transaction of its own: claim the oldest row that is neither published nor
quarantined with `FOR UPDATE SKIP LOCKED`, publish while holding it, and mark it published in the
same transaction. A transient failure leaves the row unpublished and ends the pass, and a later
pass republishes it under the same source and id. A row that can never be published is
quarantined at once and the pass goes on. Holding the claim across the publish lets
several relays share the outbox, at the cost of a row lock and a connection held for the publish.
Rejected: LISTEN/NOTIFY, which needs a pinned connection outside `database/sql` and only lowers
latency; and spike-blobfs's two-step, publishing and then marking in separate steps, which the
spike's `TestStopBetweenCommitAndPublishLosesNoEvent` fails when the mark comes first.

The relay sits below every reactor that commits events, so the drain stops the producers first.

The rule: **an event is enqueued in the transaction that makes the state it reports true.** For a
composite operation such as a blobfs upload, that is the complete step's transaction, never the
begin step's (`blobfs-composition.md`, "The protocols as the service sequences them").

## Which commands emit events

Every public command that changes state emits exactly one event, in the transaction that makes the
state true. A retry of a delete already under way, such as a repeated file delete or branch mark,
emits none.

- **The type** is `<service>.<domain>.<operation>` for an operation on the domain itself, and
  `<service>.<domain>.<element>.<operation>` for one on a sub-element.
- **The data** is the entity's stored state after the command, with no derived path, timestamps,
  or previous state.
- **Every seed write** emits its command's event.
- **The database admin's state reset** emits one `<service>.database.reset` through go-database's
  admin reset hook, ahead of its seed events. Its CloudEvents subject is the state's name, and its
  data is the name as a JSON string. A reset loses unpublished events, since its revert drops the
  outbox. The schema verbs emit nothing.

## Where the primitives live

A proven pattern sinks to the lowest level at which it is generic:

- **go-core** holds what emitters and reactors share, `event` and `reactor`, on the standard
  library alone, so a domain's emission and a reactor's adapter import no infrastructure library,
  and the SDKs and the template name events without a broker dependency.
- **go-messaging** holds the broker tier.
- **The general sweeper**, go-database's `sweep`, runs the outbox and inbox retentions;
  go-web-service sets both ages to 7 days.

## Lifecycle registration

Infrastructure (go-database's pool, go-storage's store, the broker) joins the lifecycle as a graph
node's value whose part the coordinator infers from its methods, so nothing registers. The
coordinator's two error-drop windows (after the signal, and at the drain deadline) stay as they
are, since the reactor covers both itself. The graph's layers order the drain, so the server
drains first and the database and the broker last.

## Open

- Trace context as the CloudEvents `traceparent` extension, which `v1.observability` owns;
  `Event.Extensions` already carries it.
- Migrating a durable's binding configuration (`Start`, `MaxDeliver`, `AckWait`, the types) across
  a rolling deploy. Changing it fails the bind, so every change is a recreation.
- Reaping an abandoned durable consumer (`InactiveThreshold`), since nats-server 2.15 limits a
  stream to 1000 consumers by default.
- `natstest.Scratch` hands out a random prefix, so a service harness can't run its real event
  types on a scratch stream, and owned streams under one namespace can't run in parallel on one
  server.
- Whether go-messaging names the change-only event pattern: an event that carries its producer's
  whole state and a monotonic sequence, so a consumer that skips stale input loses nothing.
- A dead-letter stream, deferred until a consumer needs replay.
- **A stream-backed root** (planned). In capstone.data-intelligence's Cerebro, each job is a root
  Entity folded from the events on its own subject, so a guarded append is the transaction that
  makes the Command's change true (`architecture/architecture.md`, "The elements", asks for that
  atomicity, not for SQL). go-messaging gains an expected-sequence append beside the outbox and a
  work-queue package ([design](../docs/messaging.md#a-stream-backed-root)).

## Assumptions

- Assumes JetStream's deduplication window covers the relay's retry interval.
- Assumes every event a service emits reports a mutation in its own database, so an outbox table
  in that database suffices. Cerebro breaks it on purpose: a job's events are its root Entity,
  appended to a JetStream subject outside any outbox ("A stream-backed root", under Open).
- Assumes the service stays on `database/sql`-shaped sessions (sqlate's `Session`).
