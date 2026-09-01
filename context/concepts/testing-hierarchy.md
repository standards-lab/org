# The testing hierarchy

Captured at the 2026-08-31 retrospective; `v1.testing` in the roadmap cites this note as the
session brief. The session reviews the full testing hierarchy for the workspace and settles
principles and guidelines per layer — all decisions settle there; this note carries the
questions and the evidence.

## What the session settles

- **The layer taxonomy and a principle per layer**: what each tier proves, what it may touch
  (no services, one service, the composed stack), and what "green" at each tier licenses.
- **Cadence versus cost**: the current unit tier runs at an aggressive rate (every PR push and
  merge to main) and stays there. Service-backed integration tiers run less frequently by
  design — the session decides the trigger model (per-merge, nightly, pre-release, on-demand
  label, or path-filtered) so cost is optimized without losing the value.
- **The postgres-in-CI approach**: a `postgres` service container in the workflow, a
  build-tagged suite (e.g. `//go:build database`), and a `mise run test-db` task were the
  shapes sketched at the retrospective; the session confirms or replaces them, and sets the
  pattern the later service integrations (auth's Keycloak, messaging's NATS, storage's
  azurite) will reuse — this decision is the template for every service-backed tier, not a
  one-off.
- **What the against-database suite asserts**: the things only a database can prove. From the
  evaluation: the transfer cycle rejection, two concurrent transfers under the advisory lock,
  the guard's 404-vs-412 split on absent/stale rows, `NULLS NOT DISTINCT` root-code
  uniqueness, path recomposition after transfer, migration DDL, seed idempotency on a second
  run. (The full coverage evidence is in go-web-service
  `context/concepts/retrospective-findings.md`, "For the testing session.")
- **The prepare-capable fakes**: the hermetic scripted-driver harness cannot exercise
  prepare-based verification (go-database `context/concepts/v0.4-findings.md` item 6); the
  session places the shared harness in the hierarchy.
- **Where the manual compose-stack ritual goes**: today's end-to-end proof is undocumented
  shell history; the session decides whether it becomes a scripted smoke tier or is absorbed
  by the against-database suite.

## Standing constraints

- The tier does not gate the DSL rewrite: `v1.data.sql` proceeds on the settled strategy, and
  the suite lands per this session's decisions in whatever order they schedule.
- Hermetic-by-default stays the baseline for the unit tier — the workspace's black-box,
  no-network, no-disk posture is an asset the hierarchy builds on, not a policy under review.
