# reset · observability-instrumentation

- **Status:** closeout
- **Session:** start
- **Project:** go-web-service, go-web-sdk-template
- **Branch:** observability-instrumentation

## Disposition

- **Cross-repo:** `standards-lab/context/roadmap.toml` — deleted the closed
  `goals.v1.observability.tasks.instrumentation` and dropped it from `next`; `goals.v1.observability`
  stays open, empty of tasks, since its layer-goal closing criteria — the extraction record, the
  promoted library and template elements, and the repository documentation — are separate, later,
  unstarted goals, not implied by this one task closing. Added `backlog.service-showcase`,
  sequenced ahead of `v1.middleware` at the architect's request: a way to demonstrate the running
  system to colleagues and leadership, citing the new concept note below.
- **Cross-repo:** `standards-lab/context/design/observability-strategy.md` §1 and §5 corrected —
  `Telemetry` registers through `OnStartup`/`OnShutdown` hooks, not a numbered stage. The record's
  stage-0 claim didn't survive contact with the code: `go-database/admin.Stage = 1` leaves no
  stage number free for telemetry ahead of the pool at 0 without a cross-repository renumbering
  this goal doesn't own, and `lifecycle.Add` panics on a negative stage regardless. The hook
  mechanism satisfies the record's actual invariant — nothing instruments a startup sequence it
  started after — without needing a stage number at all; the full reasoning is in the record now,
  not restated here.
- **Retained:** `go-web-service/context/design/stack.md`'s OpenTelemetry section — unchanged; this
  layer still projects no native-tier deviation, per `observability-strategy.md` §2. The
  service-owned log destination `service-log-destination.md` raised was decided against, on the
  same reasoning one level down, and the concept is deleted; the decision is recorded in
  `go-web-service/compose/README.md`.

`go-web-sdk-template` took the narrow slice the strategy record's §3 described: `RequestID()` wired
ahead of `RequestLogger`, generated rather than trace-derived, since the template stays
provider-free — released as `template/v0.9.0`. `go-web-service` got the rest: the observability
configuration block (a hard requirement, matching how the database block already works); the
telemetry layer, bracketing the lifecycle via hooks rather than a stage, with the shutdown hook
bounded to a short timeout so a flush against an unreachable collector — the integration tier's
normal state — can't hold the drain or fail the process; the middleware chain with tracing
outermost and `RequestID` sourcing the trace id between it and `RequestLogger`; an integration test
proving a trace id reaches both the log record and the problem document, matched rather than
independently checked; and `log.format` defaulting to `json`. Verified against the real compose
stack, not just the hermetic suites: a live request's trace landed in Tempo under
`service.name = go-web-service`, and the matching Loki line carried the same id as both
`request_id` and `trace_id`.

A new concept, `go-web-service/context/concepts/service-showcase.md`, captures a gap the
architect raised while reviewing this step: no way today to hand someone a running composition
and let them see what it does, a gap that widens as messaging and its reactor services land later.
Two directions are weighed there, not chosen between — a narrated demo script, or an OpenAPI spec
with a mounted explorer UI (the architect's read on Go's OpenAPI tooling is dated and worth
revisiting) — and observability itself now belongs in whatever this settles on, alongside
messaging's very different, non-request-shaped signal once it exists.

## Next-focus

`backlog.service-showcase`: a `plan` session, not a `start` — nothing in the concept note is
settled yet. Work out the demo-script-versus-OpenAPI question (and whether it's a choice or a
sequence — a cheap script now, a spec-driven explorer later), how observability's own signals
(a live trace, a correlated log) join the showcase alongside API responses, and how the strategy
holds up against messaging's reactor-driven signals once `v1.messaging` lands, even though that
goal is further out. The architect wants this soon, to brief colleagues and leadership on the
effort's progress.
