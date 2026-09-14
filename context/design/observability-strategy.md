# Observability

The strategy for logs, metrics, and traces across the reference architecture: OpenTelemetry as
the standard tier with no provider the way SQL or identity has one, the shape of the
`go-observability` infrastructure library, where the LGTM stack (Loki, Grafana, Tempo, Mimir)
falls relative to the standard-versus-native split, and how a request's identifier becomes the
correlation key every other signal joins on. Every capability goal beneath `goals.v1.observability`
builds to the contract this record states, and every later v1 goal instruments against it as it
lands.

## 1. Scope and shape

The standard tier is OpenTelemetry: its API and SDK, its semantic conventions, W3C Trace Context
propagation, and OTLP as the wire protocol. The architecture repository's service tiers already
name it alongside ISO/IEC 9075 for SQL and RFC 9110/9457 for HTTP, and `dependency-sourcing.md`'s
marker 5 — a capability whose job is correctness against a specification, not a preference — names
OpenTelemetry as exactly this case. There is nothing to litigate here that auth or storage had to
settle for their own standard tier.

What OpenTelemetry does not give this goal is a provider to declare. SQL has an engine, auth will
have an identity provider, storage will have a blob API; each of those has a second implementation
this architecture names as its anticipated pair. Observability has no such pair, because the OTLP
exporter is already the abstraction a backend sits behind, and nothing about swapping backends
touches the library's shape. `go-observability` is one base module, not a base module with a
declared provider.

The base module does need a second module beside it, and the reason is weight, not a swap axis.
Every OTLP exporter pulls `go.opentelemetry.io/proto/otlp`, which pulls `google.golang.org/grpc`,
`google.golang.org/protobuf`, `github.com/grpc-ecosystem/grpc-gateway/v2`, and both
`google.golang.org/genproto/googleapis/*` modules — a dependency footprint an application that only
wants the tracer and meter interfaces should never have to compile. `minimal-footprint.md` already
states the rule this shape follows: a heavy dependency is isolated behind a sub-module presenting a
light interface, so a consumer that needs only the interface never compiles the weight. The base
module, `github.com/standards-lab/go-observability`, carries the standard library, `go-core`, the
OpenTelemetry API and SDK (`otel`, `otel/trace`, `otel/metric`, `otel/sdk`, `otel/sdk/metric`, all
on a stable v1 line), and `otelhttp` for the HTTP server middleware, admitted at v0 as a stated
exception: the contrib repository has never released its instrumentation modules past v0, and
`otelhttp` still passes every other marker — stdlib types at its boundary
(`func(http.Handler) http.Handler`), one non-OpenTelemetry transitive dependency
(`github.com/felixge/httpsnoop`), and maintenance by the project that defines the ecosystem. The
`otlp` sub-module, `github.com/standards-lab/go-observability/otlp`, pins the exporters and carries
the weight above. This mirrors `sqlate` and `sqlate/postgres`, and `go-database` and
`go-database/postgres` — the same split, for the same reason, with the numbers behind it rather
than an analogy.

The base module builds five things:

1. **`Config`**, on `go-core`'s Merge-and-Finalize contract: the collector endpoint and protocol,
   headers, the trace sampling ratio, and the resource attributes.
2. **`Telemetry`**, a `lifecycle.Service`: it builds the `resource.Resource`, constructs the
   `TracerProvider` and `MeterProvider`, installs them and the W3C `TraceContext` propagator as
   process globals, and flushes them on shutdown.
3. **A trace-correlating `slog.Handler`** wrapping the handler `go-core`'s `logging.New` produces,
   appending `trace_id` and `span_id` from the request's span context when one is valid.
4. **The HTTP server middleware**, `otelhttp.NewMiddleware` behind a constructor over `Config` —
   structurally a `web.Middleware` with no `go-web-sdk` import, per `middleware-sourcing.md`'s
   placement rule for a middleware that collaborates with an infrastructure service.
5. **The request-ID source function** the web SDK's middleware takes (§3): a
   `func(*http.Request) string` returning the request's current trace id.

## 2. Standard versus native: the LGTM stack is neither

`goals.v1.observability` posed this as open; it resolves to a plain answer. Standard-versus-native
is a property of an artifact the service authors — a SQL statement, a request handler — and the
service authors nothing against Loki, Tempo, or Mimir. The collector is the only boundary the
service's code ever crosses, in both topologies:

```
go-web-service --OTLP--> collector --> Loki / Tempo / Mimir   (local, and the demo path)
go-web-service --OTLP--> collector --> a managed OTLP endpoint (deployed)
```

Because the collector stays in the path either way, the service's own configuration is identical
in both, which is what makes a backend swap a configuration change rather than a port (§6). This is
`go-web-service/context/design/stack.md`'s own shape for a provider, applied to a capability whose
provider is the exporter rather than a driver.

What is genuinely native-tier is real, and it is not Go: the collector's exporter and pipeline
configuration (which backend, in which dialect — `otlp` to Tempo, `prometheusremotewrite` to
Mimir, `otlphttp` to Loki, plus the `filelog` receiver that ingests the service's stdout logs, §4);
Grafana's datasource provisioning, including the Loki derived field and the Tempo
trace-to-logs and trace-to-metrics links; and the dashboards and alert rules built against those
datasources. All three live in `go-web-service`'s compose project, and none of them carries a
`--| tier: native` header the way a SQL statement does, because none of them is a SQL statement.
`stack.md`'s port list gains this as a named artifact class alongside its existing SQL files.

Using the LGTM stack's correlation features in full turns out to add none of these as Go
obligations, because each is produced by standard OpenTelemetry behavior and only consumed by
backend configuration:

- **Log-to-trace correlation** needs `trace_id` on the log record, which the correlating handler
  in §1 already supplies; the Loki derived field that turns it into a link is Grafana's expression
  of an OpenTelemetry convention, not a Loki-specific service obligation.
- **Exemplars** linking a Mimir metric to a Tempo trace need nothing from the service: the OpenTelemetry
  SDK's default exemplar filter attaches one to any measurement recorded inside a sampled span, and
  an exemplar crosses OTLP as a standard field.
- **Trace-to-logs and trace-to-metrics** from the Tempo datasource match on the `service.name`
  resource attribute and the trace id, both already set by §1's `Telemetry` service.

One practical consequence follows from putting the LGTM stack in the compose project at all:
`go-web-service/mise.toml` runs `docker compose up --wait` for the whole project on every
integration run, and a four-service observability stack cannot join that wait unconditionally
without every run blocking on Grafana becoming healthy. The stack is a compose profile, gating
`--wait` on the collector alone; the backends start on demand.

## 3. Correlation: the request id is the trace id

OpenTelemetry's semantic conventions define no generic request-id attribute — only vendor-specific
ones (`aws.request_id` and its kin). Its answer to "identify this request across every record" is
the trace context: `trace_id` and `span_id`, propagated as the W3C `traceparent` header. "Shaped to
OpenTelemetry's conventions," the phrase `v1.web.tasks.middleware`'s summary uses for the request
ID, means exactly this and nothing else.

`go-observability`'s HTTP middleware mints the trace id, because minting one means starting a span,
and starting a span needs a `TracerProvider` — an OpenTelemetry SDK dependency `go-web-sdk`'s
declared line (the standard library and `go-core`) does not admit, and
`middleware-sourcing.md`'s own sourcing table already places tracing in "the infrastructure
module, not the SDK." `go-web-sdk` keeps its cataloged request-ID middleware exactly as designed —
generate if absent, echo a trusted hop, set the response header — and gains one construction
option, a source function the composition root supplies:

```go
func RequestID(opts ...RequestIDOption) web.Middleware
func WithIDSource(fn func(*http.Request) string) RequestIDOption
```

`go-web-service`'s composition root passes `go-observability`'s trace-id source. Neither module
imports the other; the seam is the same extension-point shape `service-organization.md` already
uses for `ErrorWriter`, where the SDK defines the mechanism and the application supplies the
policy. A consumer of `go-web-sdk` alone — the template, before it takes any infrastructure library
— still gets a correlation id, generated rather than trace-derived, so the gap
`middleware-sourcing.md`'s build findings named stays closed regardless of which infrastructure
libraries a given service composes.

The id surfaces in `Problem.Extras`, not `Problem.Instance`: RFC 9457's `instance` identifies the
occurrence by a URI reference, and `Problem.WriteFor` already sets it to the request path, which is
useful on its own and would be lost if overwritten. `Extras` exists precisely for extension
members and merges at the document's top level.

`middleware.RequestLogger`'s field names move to OpenTelemetry's semantic conventions —
`http.request.method`, `url.path`, `http.response.status_code`, `client.address`, and
`http.route` in place of the SDK's current `method`, `path`, `status`, `remote_addr` — so the
observability layer reads them without a collector-side rename. `duration` has no semantic-convention
log attribute (`http.server.request.duration` names a metric, not a log field) and stays the SDK's
own.

None of this needs `go-observability` to exist first: the source-function seam, the `Extras`
placement, and the semconv field names are stdlib-only changes to `go-web-sdk`. `v1.web.tasks.middleware`
is not blocked by this goal, and `next`'s current order — `v1.web` before `v1.observability` — is
correct as it stands; only that task's summary, which currently claims the SDK does the
OpenTelemetry shaping itself, needed correcting.

## 4. Logs at v1: stdout, not OTLP

The service keeps writing structured JSON to stdout, with `trace_id` and `span_id` stamped on by
§1's correlating handler; the collector's `filelog` receiver ingests it and promotes the trace id
onto the record. The OpenTelemetry Logs pipeline — `otel/log`, `otel/sdk/log`, the `slog` bridge,
and `otlploghttp` — is deferred, not adopted at v1.

The reason is independent of OpenTelemetry's own maturity curve, though that curve is also real:
traces and metrics reached a stable v1 line years before logs, and the whole logs path is still
pre-1.0 and moving as one. The decisive reason is operational. A process writing to stdout needs no
live network connection to persist a log line — the runtime has already captured the write before
the process can lose it to a crash or a `SIGKILL`. An OTLP log export depends on a live connection
to the collector at the exact moment a service is unhealthy, which is exactly when the log
matters most. This is the same reasoning that keeps observability off the readiness path (§5): the
failure path must not depend on the thing that might be failing. It also matches the one deployed
precedent already in the estate — herald ships logs to Log Analytics by scraping stdout, with no
OTLP log exporter anywhere in its deployment — and it keeps `go-observability`'s base module on
stable-v1 OpenTelemetry modules only, with the entire logs signal confined to the deferred state
above.

This is a reversible choice, not a permanent one, and the reversal trigger is named so a later
session does not have to re-derive it: `otel/log` and its `slog` bridge reaching a stable v1
release.

## 5. Posture rules

**Observability never gates readiness.** `Telemetry` registers with the process lifecycle through
a `Start` and a `Shutdown` and no health check. A service that cannot reach its collector must
still serve traffic; a readiness probe that fails because the collector is unreachable turns an
observability outage into a service outage, which is the one failure this design exists to
prevent.

**`Telemetry` starts first.** It takes lifecycle stage 0, ahead of the database at stage 1 and
statement verification at stage 2. Nothing should instrument a startup sequence it started after.

## 6. Swap-cost class: interchangeable with review

Moving from the local LGTM stack to a managed backend is a configuration change to the collector's
exporter section; the service's own configuration is untouched, per §2's topology. The review is
narrow and named, the same way auth-strategy.md §1 names Keycloak-to-Entra's review items:

- **Metric temporality.** Prometheus-lineage backends want cumulative measurements; some managed
  backends want delta. The collector's temporality-conversion processor bridges the two, but the
  choice is a review item, not a default.
- **Attribute cardinality.** The SDK's own cardinality limit and a backend's ingestion limit are
  independent; an attribute set a local Mimir accepts can be rejected by a managed tier.
- **OTLP logs ingestion**, once §4's deferred pipeline lands: support and schema vary more across
  backends for logs than for traces or metrics.

None of these touches Go code, which is what keeps this class at "interchangeable with review"
rather than sliding toward schema-bound.

## 7. Alternatives considered

**A backend-named provider sub-module** (`go-observability/loki`, `/tempo`, or `/grafana`).
Rejected. A provider is one implementation of one target API, reached through an adapter beneath a
declared interface; the target API here is OTLP, and the service reaches it through one exporter.
Loki, Tempo, and Mimir implement nothing this library calls — there is no Go-side interface for a
backend-named sub-module to adapt, because the backend never enters the dependency graph.

**No sub-module — one flat module carrying the exporters directly.** Considered, not adopted, and
recorded with its reversal condition rather than dismissed outright. The strongest case against the
split is that `go-web-service`, today's only consumer, always exports and would always compile the
weight regardless. The split is kept anyway, because it is the tier's defined shape for a heavy
dependency behind a light interface, because admitting `grpc` and `protobuf` unconditionally into
every future consumer of this library is a change to the architecture's supply-chain posture and
not a convenience, and because it gives the deferred logs pipeline (§4) a home without a second
restructuring when its trigger fires. If, a year from now, every import of the base module also
imports the `otlp` sub-module, the split will have cost a `go.mod` and a release line for nothing,
and merging the two back down is cheap; that is the condition under which this reverses.

**A per-signal sub-module split** (`/traces`, `/metrics`, `/logs`). Rejected as the wrong axis: the
weight and the version instability both follow the export boundary, not the signal — metrics and
traces share one exporter graph and one version line, and splitting by signal would separate two
things that already move together while leaving the actual weight (§1) undivided.

**Hand-rolling the HTTP instrumentation** instead of sourcing `otelhttp`. Rejected under
`dependency-sourcing.md`'s own test: span naming, low-cardinality route attribution, and the
request-duration histogram's bucket boundaries are correctness against an external specification,
where a wrong answer looks right — the same failure mode the rule was written around for CORS.

**Carrying forward TAU's `Observer`/`Event` abstraction** as the basis for this library. Rejected.
It is a stdlib-only event bus whose severity levels and event shape are deliberately mapped onto
OpenTelemetry's own model, and that mapping is careful, useful work for a library that must not
take an OpenTelemetry dependency. `go-observability`'s whole purpose is to take that dependency, at
which point the mapping becomes the real OpenTelemetry types and the abstraction sitting in front
of it is a layer with nothing left to do.

## Record

This strategy was settled through a `plan` session working `v1.observability`'s task breakdown and
its two named open questions — the standard-versus-native split and the shape of
`go-observability` itself — with the architect. Given the goal's position ahead of every remaining
v1 layer, the design reasoning was escalated to an `opus` agent before anything changed, the same
escalation path `auth-strategy.md` used for its own foundational decisions; the architect reviewed
that reasoning and settled its two most consequential calls — the `otlp` sub-module split, and
stdout logs over OTLP logs at v1 — directly. The full reasoning trace lives in this session's own
record; nothing here restates it a second time.
