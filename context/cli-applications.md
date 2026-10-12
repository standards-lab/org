# CLI applications

A command-line tool is the Elemental Architecture's second application type, beside the web
service. Four tools came before the standard, all on cobra:

- slab (`go-web-service/tools/slab`)
- spike-blobfs's `blobfs` command
- spike-harness-driver's `clutch`
- spike-messaging's `courier`

spike-cli-architecture rebuilt spike-blobfs's command surface on the standard library's `flag`,
with each command bringing up only the dependencies it declares, and proved the shape the
architecture's
[CLI applications](https://github.com/standards-lab/architecture/blob/main/standards/go-elemental/principles/cli-applications.md)
page states. This note records where the cobra-era tools differ, the role interfaces across the
stack, and the plan that built the shape into the standard as go-cli-sdk and its template.

## The layout and conventions

The architecture's
[CLI applications](https://github.com/standards-lab/architecture/blob/main/standards/go-elemental/principles/cli-applications.md)
page states the layout and conventions every CLI follows, and go-cli-sdk-template scaffolds
them. The slab goal relies on two of them:

- Scenarios are opt-in. A tool that has them keeps them in one root-level `scenarios` package and
  runs them on go-cli-sdk's `scenario` runner.
- The `output` renderer stays each tool's own; neither go-cli-sdk nor the template ships one.

## Where the tools differ

The record of the cobra-era tools, which the standard replaces:

- **Signal context:** slab uses go-core's `process.SignalContext`. blobfs and clutch use the
  standard library's `signal.NotifyContext`, which keeps go-core out of a spike's
  dependencies.
- **Infrastructure lifetime:** blobfs opens connections during a run, and `Run` closes them.
  Neither slab's nor clutch's infrastructure opens anything that outlives a command.
- **Scenarios:** slab keeps the runner in `scenario` and the scenarios in `demo`, and colors
  its reporter. clutch keeps both in `scenario`, with a plain reporter over `output`.
- **Presentation flags:** each tool reads its color and verbosity flags into the output in its
  own way.

## Role interfaces

go-core's Coordinator finds each graph node value's part from its methods, so a role
interface is a contract a value can meet by accident. The role interfaces across the stack:

| Interface | Home | Methods | Implemented by | Read by |
|---|---|---|---|---|
| Starter | go-core lifecycle | `Start(ctx) error` | database.DB, storage.Store, admin.Service, observability.Telemetry, web.Server, sdk.Reactor | the Coordinator, by assertion |
| Stopper | go-core lifecycle | `Shutdown(ctx) error` | the same except admin.Service | the Coordinator, by assertion |
| Subsystem | go-core lifecycle | Starter + Stopper | DB, Store, Telemetry, Server, Reactor | docs and compile-time proofs only |
| ReadinessChecker | go-core lifecycle | `Ready() bool` | Coordinator, Readiness, DB, Store, admin.Service, Reactor, Waker | `Coordinator.Checks`, through ReadyStarter; `web.Doctor` |
| ReadyStarter | go-core lifecycle | Starter + ReadinessChecker | DB, Store, admin.Service, Reactor | `Coordinator.Checks` |
| Monitored | go-core lifecycle | `Err() <-chan error` | web.Server, Reactor | the Coordinator, by assertion |
| Doctor | go-web-sdk | ReadinessChecker + `Checks()` | Coordinator, Readiness | `RegisterHealth` |
| Mounter | go-web-sdk | `Handle(pattern, handler)` | http.ServeMux, web.Router | `RegisterHealth` |
| Source[T] | go-web-service sdk | `Receive`, `Ready() bool` | Waker, Every | Reactor |

No two packages declare the same method set, and go-cli-sdk declares no role interface of its
own. The rules for the others:

- **Readiness checks come only from a `lifecycle.ReadyStarter`.** go-core v0.8.0 infers a
  readiness check from a node value only when the value is a `lifecycle.ReadyStarter`, a
  `Starter` that implements `Ready`. A value with `Ready()` in another sense, such as a reactor
  source, stays out of /readyz.
- **`web.Doctor` stays in go-web-sdk**, because `RegisterHealth` is its only reader.
- **`Subsystem` stays** as a composite name for Starter plus Stopper, as `io.ReadWriteCloser`
  names its parts, though the Coordinator never asserts it.
- **`Source[T]` keeps `Ready()`**, since a value that is not a Starter no longer joins /readyz.

The Gate's consumer views (`Shared`, `Exclusive`) stay with v1.messaging's core-reactor.

## The plan

- **Cobra goes.** CLIs build on the standard library's `flag`. Cobra fails Go Elemental's
  no-frameworks line and the sourcing markers 1 (its type is in every caller's signature) and 5
  (argument parsing is a preference, kept in-house) of
  `architecture/standards/go-elemental/principles/dependencies.md` ("Sourcing"). The tools use a
  small part of it and carry code that works around its defaults: silenced errors,
  `RunE: cmd.Help()` on parents, and tests asserting cobra's message text.
- **go-cli-sdk** v0.2.0, one module, provides package `cli` and the `scenario` runner over the
  standard library and go-core. Its [README](https://github.com/standards-lab/go-cli-sdk/blob/main/README.md)
  and [`doc.go`](https://github.com/standards-lab/go-cli-sdk/blob/main/doc.go) state the API and
  the features it leaves out. The `App` type stays out of it, in the template's `internal/app`,
  as in web.
- **Per-command composition through the graph.** Each command declares its nodes, and the
  graph brings up only those, once each, closing them in reverse order. The same Coordinator
  runs a web service's graph.
- **go-cli-sdk-template** scaffolds, on go-cli-sdk, the composition root that the
  [CLI applications](https://github.com/standards-lab/architecture/blob/main/standards/go-elemental/principles/cli-applications.md)
  page states.
- **Promotions first.** go-core holds `graph`, the graph-backed `lifecycle` with
  `Coordinator.Exec` as the one-shot form, and `processtest.Run(t, Cmd{Args, Stdin, Env})
  Result{Stdout, Stderr, Code}`, the one-shot runner go-cli-sdk-template's integration suite
  uses.
- **slab** aligns in its own `slab` goal, adopting the conventions in "The layout and
  conventions". clutch, courier, and spike-blobfs stay on cobra; future spikes start from the
  template.
- **The evidence** is spike-cli-architecture (archived):
  [the answer](https://github.com/JaimeStill/spike-cli-architecture/blob/main/context/README.md#the-answer).
