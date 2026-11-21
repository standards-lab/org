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
page states. CLIs build on `flag`, not cobra: cobra fails Go Elemental's no-frameworks line and
the sourcing markers 1 (its type is in every caller's signature) and 5 (argument parsing is a
preference, kept in-house) of `architecture/standards/go-elemental/principles/dependencies.md`
("Sourcing"). This note records where the cobra-era tools differ and the role interfaces across
the stack. The evidence is spike-cli-architecture (archived):
[the answer](https://github.com/JaimeStill/spike-cli-architecture/blob/main/context/README.md#the-answer).

## The layout and conventions

The architecture's
[CLI applications](https://github.com/standards-lab/architecture/blob/main/standards/go-elemental/principles/cli-applications.md)
page states the layout and conventions every CLI follows, and go-cli-sdk-template scaffolds
them. slab follows them, including these two:

- Scenarios are opt-in. A tool that has them keeps them in one root-level `scenarios` package and
  runs them on go-cli-sdk's `scenario` runner.
- The `output` renderer stays each tool's own; neither go-cli-sdk nor the template ships one.

## Where the tools differ

The record of the tools that came before the standard. slab runs on go-cli-sdk, with a
composition root modeled on go-cli-sdk-template's, and follows the scenario convention: its
root-level `scenarios` package runs on go-cli-sdk's `scenario` runner as `slab scenario <name>`,
and the root's and the `scenario` parent's help both end with the listing. clutch, courier, and
spike-blobfs stay on cobra; future spikes start from the template. Where blobfs and clutch differ
from slab:

- **Signal context:** slab uses go-core's `process.SignalContext`. blobfs and clutch use the
  standard library's `signal.NotifyContext`, which keeps go-core out of a spike's
  dependencies.
- **Infrastructure lifetime:** blobfs opens connections during a run, and `Run` closes them.
  Neither slab's nor clutch's infrastructure opens anything that outlives a command.
- **Scenarios:** clutch keeps both its runner and its scenarios in `scenario`, with a plain
  reporter over `output`.
- **Presentation flags:** each tool reads its color and verbosity flags into the output in its
  own way. slab reads color from its `no color` node (`--no-color`, `SLAB_NO_COLOR`) and from
  `NO_COLOR`, and colors only the blocks a scenario step prints through Show, never a heading, a
  note, or a caption.

## Role interfaces

go-core's Coordinator finds each graph node value's part from its methods, so a role
interface is a contract a value can meet by accident. The role interfaces across the stack:

| Interface | Home | Methods | Implemented by | Read by |
|---|---|---|---|---|
| Starter | go-core lifecycle | `Start(ctx) error` | database.DB, storage.Store, admin.Service, observability.Telemetry, web.Server, reactor.Reactor, slab's scenarios.Service and scenarios.Grafana | the Coordinator, by assertion |
| Stopper | go-core lifecycle | `Shutdown(ctx) error` | the same except admin.Service | the Coordinator, by assertion |
| Subsystem | go-core lifecycle | Starter + Stopper | DB, Store, Telemetry, Server, Reactor | docs and compile-time proofs only |
| ReadinessChecker | go-core lifecycle | `Ready() bool` | Coordinator, Readiness, DB, Store, admin.Service, Reactor, Waker | `Coordinator.Checks`, through ReadyStarter; `web.Doctor` |
| ReadyStarter | go-core lifecycle | Starter + ReadinessChecker | DB, Store, admin.Service, Reactor | `Coordinator.Checks` |
| Monitored | go-core lifecycle | `Err() <-chan error` | web.Server, Reactor | the Coordinator, by assertion |
| Doctor | go-web-sdk | ReadinessChecker + `Checks()` | Coordinator, Readiness | `RegisterHealth` |
| Mounter | go-web-sdk | `Handle(pattern, handler)` | http.ServeMux, web.Router | `RegisterHealth` |
| Source[T] | go-core reactor | `Receive`, `Ready() bool` | Waker, Every | Reactor |

No two packages declare the same method set, and go-cli-sdk declares no role interface of its
own. The rules for the others:

- **Readiness checks come only from a `lifecycle.ReadyStarter`.** go-core infers a readiness
  check from a node value only when the value is a `lifecycle.ReadyStarter`, a `Starter` that
  implements `Ready`. A value with `Ready()` in another sense, such as a reactor source, stays out
  of /readyz.
- **`web.Doctor` stays in go-web-sdk**, because `RegisterHealth` is its only reader.
- **`Subsystem` stays** as a composite name for Starter plus Stopper, as `io.ReadWriteCloser`
  names its parts, though the Coordinator never asserts it.
- **`Source[T]` keeps `Ready()`**, since a value that is not a Starter stays out of /readyz.

go-core's `reactor.Gate` is a struct; a consumer declares the view it holds, as go-messaging's
`messaging.Gate` and `outbox.Gate` declare `Shared` alone.

