# reset · observability-library

- **Status:** closeout
- **Session:** start
- **Project:** go-observability
- **Branch:** observability-library

## Disposition

- **Retained:** `standards-lab/context/design/observability-strategy.md` — stays intact, not
  decayed. Its §1 (the base module's shape) is now built and expressed in `go-observability`'s
  own README and package `doc.go`, but the note still grounds two unbuilt tasks —
  `v1.observability.tasks.stack` and `.tasks.instrumentation` — so trimming it now would cut a
  reference those sessions still need; a `review` session is the better place to reassess it
  once the whole `v1.observability` goal closes.
- **Cross-repo:** `standards-lab/context/roadmap.toml` — deleted the closed
  `goals.v1.observability.tasks.library` and dropped it from `next`; the parent goal
  `goals.v1.observability` stays open (`.tasks.stack` and `.tasks.instrumentation` remain).
  `standards-lab/.claude/marathon.toml`'s `order` list and `references.toml`/`references.md`
  gained a `go-observability` entry, grouped with `go-database`/`go-web-sdk` at the same
  dependency depth (built on `go-core` alone) — done at the step's start, carried here for
  completeness.

`go-observability` v0.1.0 built per `observability-strategy.md`: `Config`, `Telemetry`,
`NewTraceHandler`, `NewMiddleware`, and `RequestIDSource` in the base module; `NewTraceExporter`
and `NewMetricExporter` in the `otlp` sub-module, connecting in plain text unconditionally
(`Config` has no TLS field yet — deferred until a deployed or managed backend needs it). No
`Protocol` field on `Config`: the `otlp` sub-module ships gRPC exporters alone for this release.

## Next-focus

`v1.observability.tasks.stack`: the OpenTelemetry collector and the LGTM stack (Loki, Grafana,
Tempo, Mimir) in `go-web-service`'s compose project, as a compose profile so
`docker compose up --wait` doesn't gate on Grafana. Design is settled in
`standards-lab/context/design/observability-strategy.md` §2 and §4; `v1.observability.tasks.instrumentation`
(wiring `go-observability` into `go-web-service` and `go-web-sdk-template`) follows once the
stack exists.
