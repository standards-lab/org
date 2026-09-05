# reset · websdk-v0.6

- **Status:** closeout
- **Session:** start
- **Project:** go-web-sdk, standards-lab, go-web-service
- **Branch:** websdk-v0.6

## Disposition

- **Integrated:** `v1.data.sql.integration.websdk`. go-web-sdk v0.6.0 is built and validated in
  six stages: `ErrorWriter.Detail` over a built-in detail set of 400, 413, and 428; `IfMatch`
  and `PreconditionError` promoted verbatim from the service's `sdk` staging; `DecodeJSON` and
  `BodyError`, answering 413 for a body over its limit where the service's `decode` answered
  400 and rejecting anything after the first value; the bracket operator grammar
  `field[op]=value` in `ParseQuery`, `Query.Filters` now an ordered `[]Filter` with the operator
  lexical; and the adapter core pulled forward from `v1.web.adapter` in place of a respond
  helper — `HandlerFunc`, `Handle`, `Group.SetErrorWriter`, `Group.HandleErr`, and a
  committed-tracking `recorder` so the adapter never writes a second response, reporting a
  post-commit error through `ErrorWriter.Log`. The SDK's own error types carry their status
  through the unexported `statusError` interface, sealed on purpose: consumer policy stays the
  matcher list. The unit suite is green with race and lint; the end-to-end test drives ten
  outcomes through router, module, group, and the three request helpers. The root regrouped by
  doc.go section (`errors.go`, `request.go`, `response.go`, `handler.go`; `env.go` into
  `config.go`), 14 files. The tag `v0.6.0` is cut on merge.
- **Integrated:** `go-web-sdk/context/concepts/error-handling.md` §1 and §2.5 decayed into
  `handler.go`, `request.go`, and `group.go`; the note now holds only what `v1.web.adapter`
  still owns, plus the idiom line: wiring-time methods for optional behavior, struct options for
  construction-time configuration, per the go-patterns constructors reference. The capability
  map and `concepts/direction.md` say v0.6.0.
- **Culled:** the claim "never functional options" (the same note, its former line 65) — it
  overstated the go-patterns rule, which reserves struct configs for production constructors
  and allows variadic or method-style variation for genuinely optional behavior.
- **Retained:** the landing-zone pages v0.6.0 moved out from under, for the docs pass:
  `go-web-sdk/problems.md` (detail on 400 only; one built-in type), `reads.md` (filters as
  `url.Values`, exact match), `routing.md` (no `HandleErr`), and a request-helpers page that does
  not exist; `docs/context/concepts/dsl-docs-pass.md` is left for that pass to inventory.
  `go-web-service/sdk/web.go` and its `doc.go` describe staging for a promotion that has landed;
  they retire at `integration.service` with the handlers they serve. The service's context
  describes its code at the pinned v0.5.0 until that rewrite.
- **Cross-repo:** at the coordinator, `design/dsl-driven-services.md` §8 marks go-web-sdk done,
  §10 drops the operator-syntax question, and History records 2026-09-05;
  `design/service-organization.md` says the adapter is built. The roadmap deletes the `websdk`
  task, rewords `v1.web.tasks.adapter` to its remaining items with `go-web-sdk` as its one
  repository, names the go-web-sdk pages in the `docs` task, and advances `next` to `template`.
  At go-web-service, `design/domain-architecture.md`, `concepts/data-layer.md`, and
  `concepts/retrospective-findings.md` no longer route the adapter and the decode promotion
  through `v1.web.adapter`; they landed in v0.6.0 and the service adopts them at
  `integration.service`.

## Next-focus

`v1.data.sql.integration.template`, a `start` session at go-web-sdk-template: pin go-web-sdk
v0.6.0 and go-database v0.4.0 with sqlate, then scaffold `internal/data` with its directories
and the seeder skeleton, `admin/database` over go-database's admin package as error-returning
handlers, the composition root as one file per layer with `routes.go` the list of mounts,
`internal/sdk` with the directives lowering onto sqlate's operators, `sqlint.toml`, the mise
tasks, and the entity conventions. SETTLE opens with the engine question: the template is
engine-free today, and scaffolding the data layer means declaring Postgres or parameterizing
the provider. The template and the service are sized as separate sessions on purpose.
