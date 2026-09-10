# Testing hierarchy

The workspace's testing strategy: the tiers, what each proves, and the cadence each runs at.
Settled at the `v1.testing` session (2026-09-01) from the questions captured at the 2026-08-31
retrospective. The architecture repository's
[tests-and-docs](https://github.com/standards-lab/architecture/blob/main/standards/go-elemental/principles/tests-and-docs.md)
and
[release-and-ci](https://github.com/standards-lab/architecture/blob/main/standards/go-elemental/principles/release-and-ci.md)
principles state the shipped posture; this note is the decision record for the hierarchy. Both
tiers are built: the integration tier landed in go-web-service at `v1.data.sql.tasks.suite`
(2026-09-07) as the root `integration` package. The strategy is deliberately no more complex
than implementing integration testing through `goals.v1.data` requires; refinements wait for
the cost data that would justify them.

## Two tiers

- **Unit tier** — per pull request, every layer, touching no service, network, or disk. The
  existing gate: `go vet`, `gofmt`, `go mod tidy -diff`, `go test -race ./...`, and lint, on
  every PR push and merge to main; green licenses merge. Needing no external service is the
  tier's contract, not a ceiling under review: fakes, scripted drivers, and port-0 listeners
  are how a package proves its behavior here. The tier is also the home of every cheap gate
  that needs no container: the `GOWORK=off` per-module build step in go-database (the v0.3.0
  tag proved CI blind to pin breakage; the step landed with v0.4.0) and `sqlint` from the DSL
  strategy, which runs in sqlate's own lint task and joined the service's at
  `v1.data.sql.integration.service` (2026-09-06).
- **Integration tier** — the composed service, black-box, on merge. One build tag,
  `//go:build integration`, marks the suite; it runs against the service's own compose stack
  and exercises the service through its API. Triggers: push to main and `workflow_dispatch` —
  below the per-PR unit rate by design. Green licenses release: a release is cut only from a
  main whose integration run has passed.

## The integration tier mirrors the developer

The integration stack is the compose stack — the same definition that runs the service
holistically (`compose.yml` and its includes), not a parallel container list maintained for
CI. `mise run integration` boots that definition as its own compose project on its own port
(`docker compose up -d --wait` under a project name), runs the tagged suite, and tears the
project down with its volume on exit, so the developer's stack and data are never touched and
every run starts from an empty database; the CI job runs the same task. When a capability
lands, its backing service joins the compose stack and the integration stack follows
automatically; there is nothing separate to keep in sync.

The suite tests the way a developer tested manually: operations against the running
composition through the API surface. A cross-service domain behavior — a command that writes
a blob and synchronizes its URL in the database atomically — therefore has the same home as a
single-service one, because the tier is keyed to the composed service, not to any backing
service's identity. Black-box through the API is also the durable shape: the suite asserts
behavior, so it survives the internal rewrites (`v1.data.sql` first) that would invalidate
package-level tests written against the plumbing.

## Integration testing is an application-layer concern

Layers below the application stay strictly on the unit tier. The reference service is the
first consumer of the infrastructure libraries (`design/service-organization.md`), and the
composed application is where a library claim meets a real engine: a black-box test that
creates a duplicate root and receives 409 proves the whole chain — the authored SQL, the
driver, the error classification, the matcher, the handler. A standing library-level
integration suite would re-prove a slice of that chain in isolation at the cost of a second
integration surface; the hierarchy does not carry one.

Two consequences follow:

- A library claim the service surface can express moves up into the service suite. Concurrent
  starters is the worked example: two composition-root starts against one database is a
  service-level test, and the truer form of the claim.
- A library claim only a real engine proves and the service surface cannot express — dirty
  migration state, non-transactional DDL, force semantics — is a **session-time acceptance
  proof**: demonstrated against a real engine during the session that lands the claim,
  recorded in that session's notes, and not re-proven continuously in CI. A regression
  slipping through this gap is the cost data that would justify revisiting.

## Capability gate

A capability enters the integration suite only when its API surface is complete enough to
exercise it end-to-end; the suite never tests half-landed surfaces from the side. The first
capability gates on the organization-domain rewrite (`v1.data.sql.tasks.suite` follows
`v1.data.sql.integration.service`), and later backing services — auth's Keycloak, storage's
azurite, messaging's NATS — join the compose stack when their service layer is testable
through the API, not when their library lands.

## What the first integration suite asserts

The go-web-service suite asserts, through the API, the behaviors the 2026-08-31 evaluation
found proven only by hand:

- transfer cycle rejection
- two concurrent transfers under the advisory lock
- the guard's 404-versus-412 split on absent and stale rows
- root-code uniqueness (`NULLS NOT DISTINCT`) as the conflict response
- path recomposition after transfer
- migration DDL via startup verify/apply
- seed idempotency on a second run and across a second start
- two concurrent composition-root starts against one empty schema
- every admin verb
- the 503 on a database outage, which the suite proved was half wired (a read surfaced the
  driver's raw error) and sqlate v0.1.1 closed The suite absorbs the manual
compose-stack ritual — there is no third tier: the compose stack remains dev tooling, and the
serve-probes-drain check is a documented README step rather than a CI tier. A dirty migration
history stays a session-time acceptance proof.

## The harness

The harness runs the service as the binary and drives it only through production surfaces:
configuration by `APP_*` environment variables, the API and the admin mount for state control,
the network through a loopback relay for fault injection, and signals and the exit code for
lifecycle. Nothing in the runtime exists for the tests' sake. Its rules, each learned from a
stall the first suite hit, are the decision record the toolkit was built to and the docs pass
reads from:

- A stall is a finding, never a sleep. Every wait is a bounded poll on an observable
  condition; an unexplained second is traced to its cause and removed.
- A race-instrumented service drops the race runtime's exit sleep (`atexit_sleep_ms=0`); races
  are reported during the run.
- One client per process, one connection per client. The harness mirrors the runtime's
  single-instance shape, and a transport that could dial a second connection leaves the server's
  graceful shutdown a request-less connection to wait five seconds on.
- Cleanup drains before it kills: interrupt first, kill only a process that does not exit, so
  backing services see every connection close.
- Readiness is observed through the API, on a port the harness chose; the log is diagnostic
  output, never parsed.
- State control goes through the operator's surface; a case starts from a state it made.
- Fault injection is at the network: the relay severs and restores deterministically, and the
  service cannot tell it from an outage.
- Diagnostics ride with the failure: every process's output is captured and attached.
- The harness proves itself hermetically on the unit tier, against loopback stand-ins.
- Timing is a first-class diagnostic; per-call timing found every cause above.

## The library-toolkit convention

A library whose infrastructure is exercised by integration testing ships its integration
toolkit beside it, the way `sqltest` ships beside sqlate for the unit tier. Built at
`v1.data.sql.tasks.toolkit` (2026-09-07): the reference service's harness was written with its
promotion boundaries one file each, and each piece moved to the layer that owns what it
exercises.
go-core's `process/processtest` is the process half and go-web-sdk's `webtest` the HTTP half;
the template ships the wiring engine-free (its `integration` package, task, and CI job), and a
generated service adds its compose stack. Each package's `doc.go` states its API. What the
reference service's tier grows next is its `context/concepts/integration-tier.md`. The relay
moved on fit, not on a second consumer (`design/service-organization.md`).

## A prepare-capable scripted driver is a unit-tier asset

Prepare-based verification is provable on the unit tier because the scripted driver can
prepare. The prototype built that driver as `sqlate/sqltest`, a public
package, so every consumer's unit tests run over it; it replaced go-database's driver fakes,
which were duplicated across four packages and could not prepare. The gap closed on the unit tier rather than being
worked around at the integration tier.

## The docs rule

The tier is live, and the standard's tests-and-docs and release-and-ci principles state the two tiers, the toolkit convention, and the green-integration-licenses-release rule (2026-09-08). The toolkit packages' documentation states their APIs; this note keeps the decision record and the harness rules.
