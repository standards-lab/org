# Messaging

The event and reactor layer for a web service: one consolidated contract through which a service
emits events and runs reactors without knowing which broker carries the events between services.
spike-messaging proved it (archived; `references.md`), and `v1.messaging` builds it. The spike's
package documentation and its `context/design.md` hold the detail this note summarizes.

The architecture already fixes the vocabulary (`architecture/architecture.md`, "The elements"):

- A **Reactor** is an entry point driven by an occurrence rather than a caller: a subscription, a
  poll interval, or a schedule. It dispatches to a Domain Service.
- An **event** tells a system outside the domain that a mutation committed. The architecture
  defines emission only, and delivery guarantees belong to the messaging system. Events are never
  used inside the application.

`go-web-service/internal/app/reactors.go` is the service's reactors layer, registered on the
lifecycle coordinator, and `auth-strategy.md` §9 commits grant and identity-link writes to emit
events once this layer lands.

## Decisions

- **The envelope is CloudEvents 1.0**, carried in binary mode with its attributes as message
  headers under the CloudEvents NATS protocol binding. `service-tiers.md` says messaging has no
  formal standard. That is true of broker operations, but not of the event's shape: CloudEvents is
  the standard tier's event vocabulary, the way OpenTelemetry is observability's. Rejected: an
  envelope of our own evolved from signal-lab's `Signal`.
- **The CloudEvents type is go-core's own**, in `event`, on the standard library alone. Rejected:
  `github.com/cloudevents/sdk-go/v2/event`, whose event package imports json-iterator and whose
  one module brings zap, testify, and x/time into the graph.
- **Emission goes through a transactional outbox.** A domain declares each event with
  `event.Define[T](type)` and raises it into the `Queue` its command receives. `Recorder[Tx].Emit`
  stamps each event (a UUIDv7 id, the service's source, the time) only when the command succeeds,
  and writes it through a `Sink[Tx]` in the command's own transaction. `Tx` is a type parameter
  the composition root fixes (`*sqlate.Tx` for the outbox), so a pool fails to compile and go-core
  names no database. Delivery is at least once. Rejected: publishing after commit, which loses the
  event when the process stops between the two.
- **An inbox table backs idempotency.** `Inbox.Claim` inserts the consumer and event in the
  handler's own transaction, and a repeat changes no row. A domain never sees the event: the
  reactor's adapter hands its command a claim function. Deduplication keys on the event's source
  and id, on the broker (`Nats-Msg-Id` on JetStream, a two-minute window) and in the inbox.
- **The reactor contract is source-agnostic.** A reactor runs one source of occurrences on the
  lifecycle coordinator: a subscription, an interval (`Every`), or a demand (`Wake`, with the
  interval as backstop). The source decides what a handler error means: a subscription
  redelivers, and `event.Permanent` terminates. `reactor` is its own package, importing neither
  `lifecycle` nor `messaging`. A reactor's grace is half the drain timeout
  (`reactor.GraceWithin`).
- **A subscription's `Name` is its durable consumer and its delivery group.** Rejected: a
  separate `Group` field, since JetStream has no second level of grouping within a durable.
- **A subscription carries a binding start position**, `StartAll` (the zero value) or `StartNew`.
  It places only a new consumer, and a mismatch under an existing `Name` fails the bind. A service
  binds `StartNew`.
- **`MaxDeliver` is set per subscription**, never as a default: a subscription whose input can
  arrive early bounds its redelivery. Exhausting the bound reaches the `Runtime`'s error hook.
- **The providers are NATS JetStream and an in-memory provider.** The in-memory provider is the
  conformance double and the test double. The standard tier stays provisional until a second real
  broker proves it (`second-providers`), as `service-tiers.md` requires.
- **A broker constructs without I/O and starts as a stage-0 lifecycle component.** `Start`
  connects, reconnecting without limit, so an outage makes the broker not ready rather than ending
  the process.
- **A service's messaging is one `messaging.Runtime`**, built from its `messaging` configuration
  block, the broker, the engine's statements, and the drain timeout. `Runtime.Relay` builds the
  relay reactor and `Runtime.Consume[T]` a consuming reactor. The provider has a configuration
  block of its own beside `messaging`.
- **Failures the runtime survives reach one error hook**, `func(ctx, Failure)` on the `Runtime`:
  relay and source errors and `MaxDeliver` exhaustion. The handler's context carries the delivery
  attempt, and an outbox row that fails past a bound is quarantined. Deferred: a dead-letter
  stream, until a consumer needs replay.
- **A stream has one owner.** A broker binds a stream it does not own without provisioning it.
  The spike's last-writer-wins `CreateOrUpdateStream` on every broker is the case this removes.
- **The scope is the standard tier plus one native use**: request and reply through the `nats`
  provider's `Conn()`, in the composition root alone. The rest of the ledger below is recorded and
  not built.

## Outbox sequencing

The relay polls (250ms by default) and is a `reactor.Source`; its pass is the primary path and its
own sweeper over unpublished rows. Each row is handled in a transaction of its own: claim the
oldest unpublished row with `FOR UPDATE SKIP LOCKED`, publish while holding it, and mark it
published in the same transaction. A failure leaves the row unpublished, and a later pass
republishes it under the same source and id. Holding the claim across the publish lets several
relays share the outbox, at the cost of a row lock and a connection held for the publish.
Rejected: LISTEN/NOTIFY, which needs a pinned connection outside `database/sql` and only lowers
latency; and spike-blobfs's two-step, publishing and then marking in separate steps, which the
spike's `TestStopBetweenCommitAndPublishLosesNoEvent` fails when the mark comes first.

The relay sits below every reactor that commits events, so the drain stops the producers first,
and `outbox.Drain` gives it a last pass at a quarter of the drain timeout.

The rule: **an event is enqueued in the transaction that makes the state it reports true.** For a
composite operation such as a blobfs upload, that is the complete step's transaction, never the
begin step's (`blobfs-composition.md`, "The protocols as the service sequences them").

## Where the primitives live

A proven pattern sinks to the lowest level at which it is generic:

- **go-core** gains what emitters and reactors share, on the standard library alone:
  - `reactor`: the source contract, `Every`, `Wake`/`Waker`, `Draining`, `GraceWithin`, and
    `Gate`, the context-aware readers-writer gate that quiesces background work around the
    schema-changing admin verbs (`migration-sets.md`).
  - `event`: the CloudEvents type, its codec, `CheckType`, `Permanent`, `Define`/`EventKind`,
    `Queue`, and `Recorder[Tx]`/`Sink[Tx]`.
  - `lifecycle`: component registration (below).

  A domain's emission and a reactor's adapter import no infrastructure library, and the SDKs and
  the template can name events without taking a broker dependency.
- **go-messaging** holds the broker tier: the broker contract, subscriptions, `Runtime`, the
  engine-agnostic `outbox` and `inbox`, the `memory` provider, and `messagingtest`. The `nats`
  provider and the `postgres` engine are modules of their own; the engine authors the outbox's and
  the inbox's statements and ships both tables in one migration set (`migration-sets.md`).
- **The general sweeper** reclaims published outbox rows and inbox rows (`v1.messaging.sweeper`).

## Lifecycle registration

Infrastructure joins the lifecycle through `Start`, `Shutdown`, and `Ready` (go-database's pool,
go-storage's store, the broker), and every composition root copied them into a `lifecycle.Service`
by hand. `lifecycle` gains `Component` with those three methods, `Monitored`, a component that adds
`Err`, and `lc.Register(name, stage, c)`, which adds the component with its readiness check and
monitors `Err` when the component is `Monitored`, found by type assertion. Every spike service
registered its database, its broker, and its reactors through it. The stage stays at the
composition root's call site, because it is the process's dependency order, which a library can't
know; there is no `Stage` type. The coordinator's two error-drop windows (after the signal, and at
the drain deadline) stay as they are, since the reactor covers both itself.

Stages order the drain by what commits events: the database and the broker lowest, then the
schema and verification stages, then the relay below every reactor that commits events, and the
server and the producers at the root.

## The capability ledger

signal-lab (`references.md`, "Prior R&D — event-driven architecture") proved more capabilities than
the standard tier holds. Each capability is classed through the standard and native split, and
some turn out to strengthen a different architecture layer:

| Capability | Class | Reason |
|---|---|---|
| Durable publish and subscribe | Standard | The core of the tier |
| Competing consumers: work assignment across a delivery group | Standard: a subscription's `Name` is its durable consumer and its delivery group | Every target broker has it, and it is how a reactor scales across replicas |
| Acknowledge, redeliver with delay, terminate | Standard, expressed through the handler's return value | Every broker has it. The redelivery and ordering policy is the "interchangeable with review" part |
| Request and reply, multi-reply discovery | Native, through the `nats` provider's handle | Not common across brokers. A request between services is an HTTP call (`service-organization.md`) |
| Subject wildcards and hierarchy | Native in filter syntax. The standard tier filters on event `type` only | Topic syntax differs by broker |
| Key-value store with watch and compare-and-set | A candidate provider for another service: caching (`backlog.response-caching`) | Enhances another layer, not messaging |
| Object store | A candidate go-storage provider, judged against go-storage's standard tier | Enhances another layer |
| WebSocket bridge, NATS-native chat | Belongs to `v1.client` | Client transport, not events between services |

## The evidence

spike-messaging answered its question, whether one broker-agnostic event and reactor contract on
CloudEvents with outbox emission runs a service's reactors on JetStream and in memory with no
broker import outside the composition root and the provider: yes. Four services played a 30-round
exercise on JetStream through the contract, and the same contract passed the conformance suite in
memory.

1. The conformance suite passes on both providers. Proven only by tests (`messagingtest.Run`).
2. A stop between commit and publish loses no event. Proven only by tests.
3. A redelivered event is handled once. Proven only by tests.
4. Two replicas in one delivery group share the work. Proven by a validate task.
5. A shutdown drains in-flight handling through the coordinator, in about 110ms. Proven by
   validate tasks.
6. No provider import outside the composition root and the provider. Proven by a standing import
   check (`split-check`).
7. An event is emitted only in its command's transaction. Proven only by tests; the composite
   Postgres-and-blob case is unproven, and `v1.messaging.service-events` proves it.
8. The native request and reply stays inside the provider and the root. Proven by a running demo.

## Open

- Trace context as the CloudEvents `traceparent` extension, which `v1.observability` owns;
  `Event.Extensions` already carries it.
- Migrating a durable's binding configuration (`Start`, `MaxDeliver`, `AckWait`, the types) across
  a rolling deploy. Changing it fails the bind today, so every change is a recreation.
- Reaping an abandoned durable consumer (`InactiveThreshold`).
- Whether go-messaging names the change-only event pattern: an event that carries its producer's
  whole state and a monotonic sequence, so a consumer that skips stale input loses nothing.
- A dead-letter stream, deferred until a consumer needs replay.

## Assumptions

- Assumes JetStream's deduplication window covers the relay's retry interval.
- Assumes every event a service emits reports a mutation in its own database, so an outbox table
  in that database suffices.
- Assumes the service stays on `database/sql`-shaped sessions (sqlate's `Session`).
