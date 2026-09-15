# reset · observability-stack

- **Status:** closeout
- **Session:** start
- **Project:** go-web-service
- **Branch:** observability-stack

## Disposition

- **Retained:** `go-web-service/context/design/stack.md`'s OpenTelemetry section — unchanged;
  nothing this step built earns a port-list entry, since the LGTM stack's configuration is
  operational configuration for chosen tools rather than a native-tier deviation, exactly as
  `observability-strategy.md` §2 already reasoned.
  `standards-lab/context/design/observability-strategy.md` also stays intact — the
  instrumentation task still needs it in full.
- **Cross-repo:** `standards-lab/context/roadmap.toml` — deleted the closed
  `goals.v1.observability.tasks.stack` and dropped it from `next`; the parent goal
  `goals.v1.observability` stays open (`.tasks.instrumentation` remains). Added a citation to the
  new concept note below on that task's `context` list.

The observability compose profile: an OpenTelemetry Collector and a local Loki, Tempo, Mimir, and
Grafana stack, gated behind `profiles: ["observability"]` so `db-up` and `integration` stay
unaffected. The collector is wired for the service's future traces and metrics over OTLP and
already receives its stdout logs over a TCP receiver, though the service itself sends nothing
yet. Grafana's datasources cross-link by structured metadata (log-to-trace, trace-to-logs,
trace-to-metrics, exemplars once something emits them); a first dashboard proves the pipeline
alive against the collector's own self-telemetry, the only live signal before instrumentation.
`mise run otel-up`/`otel-down`/`otel-reset` manage the profile — `otel-up` also polls Mimir past a
real, reproducible ready-before-its-ring-joins race compose's own `--wait` cannot see. `serve`
streams its stdout to the collector over a TCP redirect, a documented hard dependency (the
collector must already be running; it going away mid-run kills the dev server via SIGPIPE) rather
than something engineered around. `compose/README.md` is the full reference. A new concept,
`go-web-service/context/concepts/service-log-destination.md`, captures the better long-term shape
— the service owning a configurable log destination itself, instead of a shell redirect reaching
in from outside it — for the instrumentation task to design once the service exists to decide
against.

## Next-focus

`v1.observability.tasks.instrumentation`: wire `go-web-service` to `go-observability`, flip
`log.format` to `json`, and build the composition-root telemetry layer (`Telemetry` at lifecycle
stage 0, ahead of the database) per `observability-strategy.md`. Design is settled there and in
`go-web-sdk/context/concepts/middleware-sourcing.md`'s request-id source seam; the new
`service-log-destination.md` concept is an open question this task should resolve, not assume.
