# reset · integration-suite

- **Status:** closeout
- **Session:** start
- **Project:** sqlate, go-web-service, standards-lab
- **Branch:** integration-suite

## Disposition

- **Integrated:** `v1.data.sql.tasks.suite`. go-web-service's root `integration` package is
  the integration tier, built in nine stages: the harness in untagged files (`Main` builds
  `cmd/server` once with the race detector; `Launch`, `Ready`, `Start`, and `Stop` run it as a
  subprocess on a reserved port configured by `APP_*` variables and read its exit code; the
  `Forwarder` relays TCP to a backing service and severs and restores it; the `Client` reads
  responses and RFC 9457 problems; `Reset`, `Revert`, `Schema`, and `Seed` drive state through
  the admin mount) with its own hermetic tests on the unit tier, and the tagged suite: the
  lifecycle (startup migration and seeding from an empty schema, the three readiness checks,
  seed idempotency on a second run and a second start, 403 with seeding off, two concurrent
  starters, the drain to exit 0), the organization API (reads, paging and sort, the filter
  grammar's typed 400s, create with Location and the sibling, root, and absent-parent 409s,
  edit's and delete's precondition ladders, transfer's required key, cycle 409s, path
  recomposition, and root move, two sibling transfers at once serialized by the tree lock), every
  admin verb, and the outage (503 on readiness, a read, and a command while liveness stays 200,
  then recovery without a restart). `mise run integration` runs the suite against the compose
  definition as its own project on port 5433 and tears it down on exit; CI gains
  `workflow_dispatch` and an `integration` job on merge to main; vet and lint carry the tag; the
  fixed `container_name` is gone so two compose projects coexist. The README, CLAUDE.md, and the
  changelog state the tier. Validation: the whole-module vet, unit tier, lint, tidy, and two
  suite runs (fresh and populated volumes) green; the first CI run is after publish.
- **Integrated:** the 503 on a database outage, retained unproven at the previous close, was
  half wired: sqlate wrapped `ErrConnectionFailed` only on `Conn` and `Begin`, so a read against
  an unreachable engine surfaced the driver's raw error as a 500 while a command in a
  transaction answered 503. sqlate v0.1.1 classifies a `net.Error` or `driver.ErrBadConn` from
  every session call before the dialect, context errors excluded; merged and tagged mid-session,
  and the service pins it. The suite fails against v0.1.0 on the read, so it proves the claim.
- **Integrated:** three timing causes found and removed in the harness, none a service defect,
  recorded as harness rules in `design/testing-hierarchy.md`: the race runtime's one-second exit
  sleep (`GORACE=atexit_sleep_ms=0`), a request-less connection the default transport could
  leave for the server's graceful shutdown to wait five seconds on (one connection per host, one
  client per process), and killed processes leaving connections for Postgres to reap (cleanup
  interrupts first).
- **Integrated:** go-web-service `design/composition-root.md`'s unit-test contract names the
  tier as built and the README's serve, probe, and drain step as the one hand check;
  `internal/app/app_test.go`'s comment and `context/README.md`'s baseline follow.
- **Promoted:** nothing.
- **Culled:** nothing.
- **Retained:** for the docs pass (`v1.data.sql.tasks.docs`): tests-and-docs' "CI needs no
  database container" claim and release-and-ci's CI section are moved out from under, alongside
  the landing-zone and template pages the previous close listed. `design/testing-hierarchy.md`
  stays as the decision record the docs pass reads from, its harness section included. A dirty
  migration history stays a session-time acceptance proof. The retrospective's compose-variable
  drift item stands; the harness honors `APP_DATABASE_*` but the drift itself is unaddressed.
- **Cross-repo:** at the coordinator, `roadmap.toml` deletes `suite`, adds
  `v1.data.sql.tasks.toolkit` (go-core, go-web-sdk, go-web-sdk-template, go-web-service) and
  `v1.data.sql.tasks.states` (go-database, go-web-service), repoints `hardening` and dates the
  `docs` task's tier sentence, and advances `next` to `toolkit`; `design/testing-hierarchy.md`
  states both tiers built, the isolated compose project, the assertion list as built, the
  harness rules, and the planned library-toolkit convention; `design/dsl-driven-services.md` §9
  and §11 record 2026-09-07; `context/README.md` dates the tier and bumps sqlate to v0.1.1. At
  go-web-service, the new `concepts/integration-tier.md` carries the direction for `toolkit`
  (the seams by ownership layer, the convention), `states` (the fixture feature), and the
  per-domain protocol helpers due at the second domain. At sqlate, the v0.1.1 release; its
  context needed no change.

## Next-focus

`v1.data.sql.tasks.toolkit`, a `start` session spanning go-core, go-web-sdk,
go-web-sdk-template, and go-web-service in that order: extract the process runner to go-core
beside `process`, the client and problem decoding to `go-web-sdk/webtest`, decide the
forwarder's home by its second consumer, wire the template's engine-free `integration` package,
mise task, and CI job, release, and re-pin the service with its harness thinned to the toolkit;
record the library-toolkit convention in `design/testing-hierarchy.md`. The API is taken from
the harness as built (go-web-service `context/concepts/integration-tier.md`), not redesigned.
After it, `v1.data.sql.tasks.states`, then `v1.data.sql.integration.listener`.
