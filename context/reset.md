# reset · service-on-sqlate

- **Status:** closeout
- **Session:** start
- **Project:** go-web-service, standards-lab, go-database, go-web-sdk, go-web-sdk-template
- **Branch:** service-on-sqlate

## Disposition

- **Integrated:** `v1.data.sql.integration.service`. go-web-service runs on authored SQL end
  to end over go-database v0.4.0 with postgres/v0.3.0, go-web-sdk v0.6.0, and sqlate v0.1.0
  with postgres/v0.1.1, built in seven stages: the pins and the root-level `data` package (the
  session with the pattern catalog and the statements registry, the migration set, the `app`
  pattern namespace, the seeder, the lock-name registry with the one lock statement behind
  `Database.Lock`, the `web.Query` to `query.Directives` lowering, and the shared status
  matcher); the `sdk` package re-seeded with `PathID` and `Command`; the organization domain on
  seven statement files with `Validate` on each command, edit on `PUT /{id}`, transfer
  requiring the `parent_id` key; `admin/database` as the HTTP half over go-database's admin
  service with detail on 403 and 409; the `admin.seed` config block (`APP_ADMIN_SEED`); the
  composition root as one file per layer with the pool at stage 0, the schema at stage 1, and
  the domains at stage 2; `sqlint.toml` with `go tool sqlint` in mise's lint and CI, the
  `cmd/db` tasks gone, the README and changelog rewritten. Every package proves its SQL over
  `sqlate/sqltest`. The run-and-verify pass against a fresh compose stack proved startup
  migration and seeding, the probes with the schema check, the filter grammar (equality,
  membership, `like`, an unknown field, an unknown operator, and a malformed value as typed
  400s), create with Location and the duplicate 409, PUT with 200, 400, 412, 428, and 404,
  transfer with the required key, the cycle 409, path recomposition, the root move, delete with
  204 and 412 and the children 409, every admin endpoint, the idempotent seed with counts of
  zero, 403 with seeding off, and a clean drain to exit 0. The service stays versionless.
- **Integrated:** the service's `design/domain-architecture.md`, `design/composition-root.md`,
  and `design/stack.md` are rewritten for the code as built (the superseded blocks gone; the
  verb rule, the matcher composition, the lock rule, the three stages, the port list as the
  native-tier files plus the migrations); `concepts/data-layer.md`'s settled-layout paragraph
  decayed and its evaluation section names the `sdk` tenants, the `data` package's
  non-scaffoldable pieces, and the seed helper; `concepts/retrospective-findings.md`'s
  domain-rewrite section is consumed; `context/README.md`'s baseline names the admin service.
- **Culled:** nothing. Soft delete, recommended by the prototype's review and not in the task,
  is recorded as a deferred convention under `concepts/data-layer.md`'s domain direction.
- **Retained:** the 503 on a database outage is wired in `data.Status` and unproven live; the
  integration tier asserts it. For the docs pass: the landing zone's topology-and-naming
  principle and the go-web-sdk-template pages still name `internal/infrastructure` and
  `internal/domain`, and both index pages list go-web-service as planned.
- **Cross-repo:** at the coordinator, `roadmap.toml` deletes the `service` task, opens the
  `suite` task's gate and adds the 503 to its list, marks the `integration` goal's summary
  landed but for the listener, and advances `next` to `v1.data.sql.tasks.suite`;
  `design/dsl-driven-services.md` §6.1 records the lock's and the guarded-command read's
  placements and edit on PUT, §8 marks the service done, and §11 records 2026-09-06;
  `context/README.md` dates the service. At go-database, `context/README.md` and
  `design/infrastructure-service.md` say the reference service built the admin HTTP half. At
  go-web-sdk, `concepts/error-handling.md`'s service section says the conversion landed and
  names the two staged candidates. At go-web-sdk-template, `context/README.md`'s candidate
  direction says the service proved the patterns.

## Next-focus

`v1.data.sql.tasks.suite`, a `start` session at go-web-service: the integration tier per
`standards-lab/context/design/testing-hierarchy.md`, the `//go:build integration` suite
black-box through the API against the compose stack, the CI job booting the stack on merge to
main and `workflow_dispatch`, and the mise task. The assertion list is the run-and-verify pass
above plus the hierarchy's list (two concurrent transfers under the lock, migration DDL via
startup, seed idempotency on a second run) and the 503 on an outage. Once the first run is
live, the docs amendments the hierarchy names (tests-and-docs, release-and-ci) land in
whichever session is next in the docs repository. After it, `v1.data.sql.integration.listener`.
