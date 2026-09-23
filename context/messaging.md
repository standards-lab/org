# Messaging

The planned layer that makes events and reactors real for a web service. It is one consolidated
contract through which a service emits events and runs reactors without knowing which broker
carries the events between services. `v1.messaging` builds it, and the messaging experiment
proves it first. `messaging-api.md` proposes the API the experiment starts from.

The architecture already fixes the vocabulary (`architecture/architecture.md`, "The elements"):

- A **Reactor** is an entry point driven by an occurrence rather than a caller: a subscription, a
  poll interval, or a schedule. It dispatches to a Domain Service.
- An **event** tells a system outside the domain that a mutation committed. The architecture
  defines emission only, and delivery guarantees belong to the messaging system. Events are never
  used inside the application.

The stack also has these seams in place. `go-web-service/internal/app/reactors.go` is an empty
reactors layer registered on the lifecycle coordinator. `auth-strategy.md` §9 commits grant and
identity-link writes to emit events once this layer lands.

## The experiment's question

Can one broker-agnostic event and reactor contract, built on CloudEvents with outbox emission,
run a service's reactors on NATS JetStream and on an in-memory provider, with no broker import
outside the composition root and the provider?

The answer decides go-messaging's module API, which primitives go-core gains, and how
`v1.messaging` breaks into tasks.

## Decisions

- **The envelope is CloudEvents 1.0.** It is carried in binary mode, with its attributes as
  message headers under the CloudEvents NATS protocol binding. `service-tiers.md` says messaging
  has no formal standard. That is true of broker operations, but not of the event's shape.
  CloudEvents is the standard tier's event vocabulary, the way OpenTelemetry is observability's,
  and the organization establishes only the operations. Rejected: an envelope of our own evolved from signal-lab's
  `Signal`, which is already close to CloudEvents and would diverge from a standard for no gain.
- **Emission goes through a transactional outbox.** The event row is written in the mutation's
  own transaction, and a relay publishes it afterward. Delivery is at least once, and a reactor is
  idempotent, keyed on the event's CloudEvents `id`. Rejected: publishing after commit, which
  loses the event when the process stops between the commit and the publish. That contradicts the
  architecture's definition of an event as a committed mutation.
- **The providers are NATS JetStream and an in-memory provider.** The in-memory provider serves
  as the conformance double and the test double. The standard tier stays provisional until a
  second real broker proves it (`backlog.second-providers`), as `service-tiers.md` requires.
- **The reactor contract is source-agnostic.** A reactor runs one source of occurrences on the
  lifecycle coordinator. A subscription is one kind of source, and an interval or a schedule uses
  the same contract. go-messaging supplies the subscription source.
- **The scope is the standard tier plus one native use.** That native use is request and reply
  through the NATS handle. It proves that native use stays wrapped beneath the standard tier. The
  other capabilities in the ledger below are recorded and not built.

## Outbox sequencing

The outbox follows the two-step protocol spike-blobfs proved for a Postgres commit paired with a
side effect outside it (`blobfs-composition.md`, "The protocols as the service sequences them"):

| blobfs protocol | Outbox counterpart |
|---|---|
| The begin step writes the `pending` intent row in the consumer's transaction, beside the consumer's own row | The emitter writes the event row in the mutation's transaction |
| The side effect runs outside any transaction | The relay publishes outside any transaction |
| The complete step runs on the pool and converges on retry | The relay marks the row published. A retry republishes under the same deduplication id (the event `id`, as `Nats-Msg-Id` on JetStream) |
| There is no failed status, and a retry resumes | An unpublished row stays unpublished until the relay succeeds |
| A step succeeds on a state that is already done | The broker's deduplication window and the reactor's idempotency make a republish harmless |
| A sweeper removes abandoned `pending` rows | The relay is the sweeper: its pass over unpublished rows is the primary path |

One rule carries over, and the experiment tests it: **an event is enqueued in the transaction that
makes the state it reports true.** For a composite operation such as a blobfs upload, that is the
complete step's transaction (the file is available), never the begin step's. Enqueued at the
begin step, the event would report a mutation that has not happened.

## Where the primitives live

The hypothesis follows the architecture's rule that a proven pattern sinks to the lowest level at
which it is generic:

- **go-core** gains what emitters and reactors share, using the standard library alone:
  - a `reactor` package: the source contract, registration on the lifecycle coordinator, and an
    interval source
  - an `event` package: the CloudEvents type, its codec, the emitter interface a domain depends
    on, and the handler outcome that means "never redeliver"

  A domain's emission and a reactor's adapter then import no infrastructure library, and the SDKs
  and the template can name events without taking a broker dependency.
- **go-messaging** holds the broker tier: publishing, subscriptions and delivery groups, the
  `nats` and `memory` providers, and the outbox relay, with the outbox table shipped as a
  migration set (`migration-sets.md`).

Open: where the outbox writer belongs. It needs only `database/sql`-shaped execution, which
sqlate's `Session` already satisfies, so it could live in go-core. Its table is go-messaging's
migration set, though, which argues for go-messaging. Either way, the domain depends only on the
emitter interface, and the composition root injects the implementation.

## The capability ledger

signal-lab (`references.md`, "Prior R&D — event-driven architecture") proved more capabilities than
the standard tier holds. Each capability is classed through the standard and native split, and
some turn out to strengthen a different architecture layer:

| Capability | Class | Reason |
|---|---|---|
| Durable publish and subscribe | Standard | The core of the tier |
| Competing consumers: work assignment across a delivery group | Standard, as a named delivery group on a subscription | Every target broker has it, and it is how a reactor scales across replicas |
| Acknowledge, redeliver with delay, terminate | Standard, expressed through the handler's return value | Every broker has it. The redelivery and ordering policy is the "interchangeable with review" part |
| Request and reply, multi-reply discovery | Native, through the `nats` provider's handle | Not common across brokers. A request between services is an HTTP call (`service-organization.md`) |
| Subject wildcards and hierarchy | Native in filter syntax. The standard tier filters on event `type` only | Topic syntax differs by broker |
| Key-value store with watch and compare-and-set | A candidate provider for another service: caching (`backlog.response-caching`) | Enhances another layer, not messaging |
| Object store | A candidate go-storage provider, judged against go-storage's standard tier | Enhances another layer |
| WebSocket bridge, NATS-native chat | Belongs to `v1.client` | Client transport, not events between services |

## What the experiment settles

- Adopt `github.com/cloudevents/sdk-go/v2/event`, or write a spec-conformant type of our own.
  Existing solutions come first, weighed against Go Elemental's dependency line, and the loser is
  recorded as a rejected alternative.
- Where the outbox writer lives. Whether the relay polls or listens for notifications. Whether a
  consumer-side inbox table backs idempotency.
- Whether `reactor` is its own go-core package or part of `lifecycle`.
- Whether go-core's `lifecycle` gains a component interface: `Start`, `Shutdown`, and `Ready`,
  registered by name and stage, with a `Stage` type. go-storage's `Store` and go-database's pool
  already have those methods, and every composition root copies them into a `lifecycle.Service`
  by hand. The stage stays with the composition root, because a library can't know a process's
  dependency order (go-database's `admin.Stage` constant is the counterexample). The spike tests
  this against published go-core, with the reactor as the next component. It also settles which
  stage a reactor takes: `StageRoot` beside the server, or a stage of its own. The change isn't
  a prerequisite. It would fix the API before the evidence exists, and it touches go-web-service,
  where the blobfs-build lane works.
- Who provisions a stream, since signal-lab solved startup ordering with a retry. How readiness and
  drain run through the coordinator.
- How trace context propagates as the CloudEvents `traceparent` extension alongside
  go-observability.

## The proof

The experiment's final validation answers its question with this evidence:

1. The conformance suite passes on both providers.
2. A stop between commit and publish loses no event.
3. A redelivered event is handled once.
4. Two replicas in one delivery group share the work.
5. A shutdown drains in-flight handling through the coordinator.
6. An import check finds no provider import outside the composition root and the provider, and
   no go-messaging import in a domain package.
7. A composite Postgres and blob operation emits its event only from its complete step's
   transaction.
8. The native request-and-reply use stays inside the `nats` provider and the composition root.

## The experiment's home

The experiment follows the hosting convention in `../experiments.md`:

- local directory: `~/experiments/spike-messaging`
- remote: `github.com/JaimeStill/spike-messaging`

It keeps each package in a directory named for its intended home (`core/reactor`, `core/event`,
`messaging/…`), so promoting a package is a move, and the import graph proves the split.

## Assumptions

- Assumes JetStream's deduplication window covers the relay's retry interval.
- Assumes every event a service emits reports a mutation in its own database, so an outbox table
  in that database suffices.
- Assumes the service stays on `database/sql`-shaped sessions (sqlate's `Session`).
