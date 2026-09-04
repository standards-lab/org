# Testing hierarchy

The workspace's testing strategy: the tiers, what each proves, and the cadence each runs at.
Settled at the `v1.testing` session (2026-09-01) from the questions captured at the 2026-08-31
retrospective. The docs landing zone's
[tests-and-docs](https://github.com/standards-lab/docs/blob/main/standards/go-elemental/principles/tests-and-docs.md)
and
[release-and-ci](https://github.com/standards-lab/docs/blob/main/standards/go-elemental/principles/release-and-ci.md)
principles state the shipped posture; this note is the decision record for the hierarchy,
including the tier that is not yet built. The strategy is deliberately no more complex than
implementing integration testing through `goals.v1.data` requires; refinements wait for the
cost data that would justify them.

## Two tiers

- **Unit tier** — per pull request, every layer, touching no service, network, or disk. The
  existing gate: `go vet`, `gofmt`, `go mod tidy -diff`, `go test -race ./...`, and lint, on
  every PR push and merge to main; green licenses merge. Needing no external service is the
  tier's contract, not a ceiling under review: fakes, scripted drivers, and port-0 listeners
  are how a package proves its behavior here. The tier is also the home of every cheap gate
  that needs no container: the `GOWORK=off` per-module build steps in go-database (the v0.3.0
  tag proved CI blind to pin breakage) and `sqlint` from the DSL strategy join it as
  `v1.data.sql` lands them.
- **Integration tier** — the composed service, black-box, on merge. One build tag,
  `//go:build integration`, marks the suite; it runs against the service's own compose stack
  and exercises the service through its API. Triggers: push to main and `workflow_dispatch` —
  below the per-PR unit rate by design. Green licenses release: a release is cut only from a
  main whose integration run has passed.

## The integration tier mirrors the developer

The integration stack is the compose stack — the same definition that runs the service
holistically (`compose.yml` and its includes), not a parallel container list maintained for
CI. The CI job boots it the way a developer does (`docker compose up -d --wait`, the
`mise run db-up` gesture) and then runs the tagged suite; locally, `mise run test-integration`
does the same. When a capability lands, its backing service joins the compose stack and the
integration stack follows automatically; there is nothing separate to keep in sync.

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
found proven only by hand: transfer cycle rejection; two concurrent transfers under the
advisory lock; the guard's 404-versus-412 split on absent and stale rows; root-code
uniqueness (`NULLS NOT DISTINCT`) as the conflict response; path recomposition after
transfer; migration DDL via startup verify/apply; and seed idempotency on a second run. The
suite absorbs the manual compose-stack ritual — there is no third tier: the compose stack
remains dev tooling, and the serve-probes-drain check becomes a documented README step
rather than a CI tier.

## A prepare-capable scripted driver is a unit-tier asset

Prepare-based verification is provable on the unit tier because the scripted driver can
prepare. The prototype built that driver as `sqlate/sqltest` (`concepts/sqlate.md`), a public
package, so every consumer's unit tests run over it; it replaced go-database's driver fakes,
which were duplicated across four packages and could not prepare (go-database
`context/concepts/v0.4-findings.md` item 6). The gap closed on the unit tier rather than being
worked around at the integration tier.

## Sequencing and the docs rule

The tier does not gate the DSL rewrite: `v1.data.sql` proceeds on the settled strategy, and
the integration suite lands once the capability gate opens. The docs amendments —
tests-and-docs' "CI needs no database container" claim and release-and-ci's CI section —
land once the first integration run is live, in whichever session that is true for: the
landing zone states what exists, and until then this note is where the decision lives.
