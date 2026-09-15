# reset · problem-vocabulary-migration

- **Status:** closeout
- **Session:** start
- **Project:** go-web-service, go-web-sdk-template
- **Branch:** problem-vocabulary-migration

## Disposition

- **Integrated:** closed `goals.v1.web.tasks.migration` — both consumers moved onto go-web-sdk
  v0.8.0. `go-web-service`: `data.Status`, `admin/database`'s matcher, and
  `domain/organization`'s matcher now return `web.Problem` as `web.ProblemMatcher`s, each
  carrying only the status the domain already decided; `RegisterHealth` is called with the zero
  `Problem` in both `go-web-service` and `go-web-sdk-template`, since neither names a problem
  type of its own yet and no type-URI namespace exists anywhere in the workspace (RFC 9457
  treats `about:blank` as no semantics beyond the status code, so a title without a type would be
  non-conforming). `go-web-sdk-template`'s readiness-log test fixed to assert the renamed
  `url.path` attribute instead of matching the old `path` name by coincidence.
- **Retained:** nothing new deferred this session — the matchers stay status-only by design
  (typing the domain vocabulary is gated on `go-web-sdk`'s own deferred `statusError`-precedence
  item, a consumer-driven decision, not this task's).
- **Cross-repo:** `standards-lab/context/roadmap.toml` — deleted `goals.v1.web.tasks.migration`
  (closed) and, with it, the now-emptied `goals.v1.web` goal in full. At the architect's
  direction, `next`'s single `v1.observability` entry is replaced with its three tasks in
  sequence — `v1.observability.tasks.library`, `.tasks.stack`, `.tasks.instrumentation` — since
  the goal's design is already settled (`design/observability-strategy.md`) and what remains is
  executing its tasks in order.

## Next-focus

`go-observability`: `v1.observability.tasks.library` — the infrastructure library: the base
module over the OpenTelemetry API and SDK, the `otlp` sub-module, repository creation, the
README's dependency-line statement admitting the v0 otelhttp exception, and the first release.
Design is settled in `standards-lab/context/design/observability-strategy.md`; this is execution,
not a fresh planning concept. The repository doesn't exist in the workspace yet and isn't in the
coordinator's `.claude/marathon.toml` `order` list — creating it and adding it to `order` is part
of this task's own scope, not a separate `init` step to settle first.

Beyond this: `v1.observability.tasks.stack` (the collector and LGTM compose stack, in
`go-web-service`) and `v1.observability.tasks.instrumentation` (wiring both `go-web-service` and
`go-web-sdk-template` to the library) follow in that order once the library exists.
