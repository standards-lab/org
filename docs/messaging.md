# Messaging

The evidence behind the messaging contract and the capabilities it classes. The decisions are in
the coordinator's [messaging note](../context/messaging.md); the contract's behavior is in the
`go-messaging` README, its docs/, and package documentation.

## The evidence

spike-messaging (archived; `references.md`) answered whether one broker-agnostic event and
reactor contract on CloudEvents with outbox emission runs a service's reactors on JetStream and in
memory with no broker import outside the composition root and the provider: yes. Four services
played a 30-round exercise on JetStream through the contract, and the same contract passed the
conformance suite in memory. The spike's package documentation and its `context/design.md` hold
the detail.

1. The conformance suite passes on both providers. Proven only by tests (`messagingtest.Run`).
2. A stop between commit and publish loses no event. Proven only by tests.
3. A redelivered event is handled once. Proven only by tests.
4. Two replicas in one delivery group share the work. Proven by a validate task.
5. A shutdown drains in-flight handling through the coordinator, in about 110ms. Proven by
   validate tasks.
6. No provider import outside the composition root and the provider. Proven by a standing import
   check (`split-check`).
7. An event is emitted only in its command's transaction. Proven by tests, and the composite
   Postgres-and-blob case by go-web-service's integration tier.
8. The native request and reply stays inside the provider and the root. Proven by a running demo.

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
| Key-value store with watch and compare-and-set | A candidate provider for another service: caching (`response-caching`) | Enhances another layer, not messaging |
| Object store | A candidate go-storage provider, judged against go-storage's standard tier | Enhances another layer |
| WebSocket bridge, NATS-native chat | Belongs to `v1.client.reference` | Client transport, not events between services |

## A stream-backed root

Planned. In capstone.data-intelligence's Cerebro, each job is a root Entity whose state is folded
from the events on its own subject, and appending an Event, guarded by an
expected-last-subject-sequence check, is the transaction that makes the Command's change true. Its
other events go through the outbox in PostgreSQL (docproc's `context/cerebro.md`, "State and
subjects"). go-messaging gains, through the capstone's own tasks
(`roadmap/capstone.data-intelligence.toml`, its `messaging-append` and `messaging-work-queue`
tasks): that expected-sequence append beside the outbox, and a
work-queue package (JetStream work-queue retention: heartbeat, `MaxDeliver`, backoff, a
dead-letter subject) whose contract a Postgres `SKIP LOCKED` provider could also meet
(`data-intelligence.md`, "Cerebro and Athena").
