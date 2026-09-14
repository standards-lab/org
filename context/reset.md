# reset · observability-strategy

- **Status:** closeout
- **Session:** plan
- **Project:** standards-lab, go-web-sdk, go-web-service
- **Branch:** observability-strategy

## Disposition

- **Integrated:** authored `standards-lab/context/design/observability-strategy.md` —
  go-observability's shape (a base module over the stable OpenTelemetry API and SDK, an `otlp`
  sub-module isolating the exporters' dependency weight), the standard-versus-native resolution
  (this layer stays pure standard tier; nothing is projected for `stack.md`'s port list), the
  request id as the OpenTelemetry trace id surfaced through `Problem.Extras`, logs on stdout JSON
  with the OTLP logs pipeline deferred, the posture rules, and the swap-cost class. Escalated to
  an `opus` agent given the goal's position ahead of every remaining v1 layer; the architect
  settled the `otlp` sub-module split and the stdout-over-OTLP-logs call directly.
- **Integrated:** seeded `goals.v1.observability.tasks.library/stack/instrumentation` in
  `roadmap.toml`, and corrected `goals.v1.observability`'s and `goals.v1.web.tasks.middleware`'s
  summaries (the latter had claimed OpenTelemetry-shaping work the SDK cannot do on its own).
- **Integrated:** added `goals.v1.web.tasks.migration`, tracking the
  `go-web-service`/`go-web-sdk-template` migration onto `go-web-sdk`'s new
  `ProblemMatcher`/`Readiness` shapes, gated on the SDK's next release past v0.7.0.
- **Integrated:** corrected `dependency-sourcing.md`'s rule — a library that clears the standard
  markers is imported as a declared dependency, never copied into the tree — and split the
  sourced middleware set (CORS, real client IP, compression, rate limiting) out of
  `v1.web.tasks.middleware` into its own goal, `v1.middleware`, inserted into `next` after
  `v1.web`.
- **Integrated:** added an Observability row to `service-organization.md`'s anticipated-services
  table.
- **Retained:** `concepts/admin-listener.md`'s assignment of the admin mount's audit record to
  `goals.v1.observability` — examined, found wrong (nothing in this settled scope supplies an
  audit record), left as `v1.admin-listener`'s own decision rather than fixed here.
- **Cross-repo:** `go-web-sdk/context/concepts/middleware-sourcing.md` — corrected the
  correlation id's home (`Problem.Extras`, not `Problem.Instance`), corrected CORS and real-IP to
  imported rather than copied, and noted the split to `v1.middleware`.
- **Cross-repo:** `go-web-service/context/design/stack.md` — restructured the port list into
  per-layer subsections (SQL, OpenTelemetry), stating the OpenTelemetry layer's own entry: pure
  standard tier, nothing projected.

## Next-focus

`go-web-sdk`: finish `v1.web.tasks.adapter`'s remaining items (router 404/405 hooks, the
`ErrorLog` bridge with `MaxHeaderBytes` and the swallowed-encoder-error decision, the per-block
config env segment) together with `v1.web.tasks.middleware`'s hand-rolled remainder (the
request-id source seam `WithIDSource` takes, the semconv field renames, and the rest of the
hand-rolled catalog: timeout, content-type gate, body limit, fixed headers, conditional wrap,
path hygiene). This is what `goals.v1.observability.tasks.library` needs before it can start.
`v1.middleware` (the sourced set) and the rest of `v1.observability` wait behind it in `next`.
