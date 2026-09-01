# reset · testing

- **Status:** closeout
- **Session:** start
- **Project:** standards-lab (org), go-web-service
- **Branch:** testing

## Disposition

- **Promoted:** `concepts/testing-hierarchy.md` → `design/testing-hierarchy.md`. `v1.testing`
  executed: the two-tier strategy settled. The hermetic per-PR unit tier covers every layer
  and is the home of the cheap hermetic gates (`GOWORK=off` module builds, the SQL header
  check) as `v1.data.sql` lands them. The integration tier is an application-layer concern
  that mirrors the developer: the compose stack is the integration stack, booted in CI the
  way `mise run db-up` does; the `//go:build integration` suite runs black-box through the
  API on merge to main plus `workflow_dispatch`; green licenses release. Layers below the
  application stay unit-only — service-expressible library claims move up (concurrent
  starters as two composition-root starts), engine-only claims (dirty state,
  non-transactional DDL, force semantics) become session-time acceptance proofs. A
  capability enters the suite only when its API surface is testable end to end. No third
  tier: the suite absorbs the manual compose-stack ritual; serve-probes-drain becomes a
  README step. `context/README.md` gained the capability entry.
- **Integrated:** roadmap — `v1.testing` deleted; `v1.data.sql.tasks.suite` added (the
  integration tier, gated on `tasks.organization` by the capability gate); one line each
  appended to `v1.data.sql.tasks.query` (the shared prepare-capable fake package and the
  `GOWORK=off` CI steps), `.migrate` (acceptance-proof form for the engine-only criteria),
  `.hardening` (the template seeds the tier's scaffolding), and `.docs` (amend
  tests-and-docs and release-and-ci once the tier is live); `next` advanced.
- **Cross-repo:** go-web-service `context/concepts/retrospective-findings.md` — the "For the
  testing session" section consumed; the header records the consumption and points at the
  design note.
- **Retained:** the docs landing zone's tests-and-docs and release-and-ci pages,
  intentionally unamended — they state the shipped posture, which is still one tier; the
  design note is the decision record until the first integration run is live.

## Next-focus

`v1.data.sql.plan` — the SQL planning session settling what the DSL strategy defers (its
§9): query signatures, Source loading, placeholder rebinding; catalog composition; row
mapping; Guard shape; the migration version table and force/dirty semantics (the
acceptance-proof form for the engine-only cases is settled in
`standards-lab/context/design/testing-hierarchy.md`). Context:
`standards-lab/context/design/dsl-driven-services.md`. Runs in go-database.
