# CLI applications

A command-line tool is the Elemental Architecture's second application type, beside the web
service. Four tools came before the standard, all on cobra:

- slab (`go-web-service/tools/slab`)
- spike-blobfs's `blobfs` command
- spike-harness-driver's `clutch`
- spike-messaging's `courier`

spike-cli-architecture rebuilt spike-blobfs's command surface on the standard library's `flag`,
with each command bringing up only the dependencies it declares, and proved the shape this note
records. This note gives the layout, the conventions around it, where the cobra-era tools differ,
and the plan to build the layout into the standard as go-cli-sdk and its template.

## The layout

The application-layer import direction applies as go-elemental's
`principles/topology-and-naming.md` states it: `cmd/*` imports `internal/*`, `internal/*`
imports the root-level packages, and nothing imports in the reverse direction.

- **Process entry.** `cmd/<name>/main.go` derives the signal context, passes it with
  `cli.Streams` and the arguments to the composition root, and exits with the code the run
  returns. It imports only `internal/app` and `cli`.
- **The composition root**, `internal/app`, has one file per layer:
  - `app.go` holds the root command and `App`, with `New` and `Run`. `Run` is one `cli.Run`
    with `WithGraph`, so each command builds only the graph nodes it declares, once each, and
    shuts them down in reverse order. One exported `Nodes` value names every node in the
    graph, and the App publishes its graph, `Nodes`, and root command for tests.
  - `infrastructure.go` defines the infrastructure nodes and the configuration nodes:
    configuration is graph nodes, so there is no `Config` struct. It is the only file that
    names a provider or an adapter.
  - `domain.go` defines the domain nodes and mounts their commands with one
    `root.Add(pkg.Commands(...)...)`. `admin.go` does the same for admin services, where there
    are any.
  - `scenario.go` mounts the narrated scenarios.
  - There is no `config.go`, no `commands.go`, and no initializer: each layer file
    fills its part of the graph and mounts its own commands.
  - Test fixtures live in an internal `apptest` package.
- **Root-level packages:**
  - `domain/<name>` holds one domain's service and its direct commands, which
    `Commands(...) []*cli.Command` builds. `admin/<name>` does the same for an admin service,
    and the two trees are siblings. A direct command does one real operation and prints its
    result.
  - `output` is the renderer every command shares. Results go to stdout, errors to stderr.
  - `scenario` holds the narrated scenarios, a different kind of thing from a direct command. A
    scenario declares its nodes like any other command and checks what it needs first. Then it
    runs its steps, and each step says what it is about to do before doing it.
  - A tool adds shared packages as it needs them, such as slab's `input`, `style`, `httpx`, and
    `env`.

## Conventions

- A command declares the nodes it needs with `Use` and reads them in `Run` with
  `inv.Get`, which panics on a node the command didn't declare.
- `Validate` checks a command's input and builds nothing, so a usage error never brings up a
  dependency.
- A flag that names something the tool can't build fails before any command runs, in the root's
  `PreRun`.
- Requested help and usage errors exit 2, go-core's `ExitUsage`.
- Subcommands are bare words, never prefixed with the capability's name: `scenario resume`.
- Every CLI's scenarios take one shape. One `scenario` package holds the runner and the
  scenarios, and `<tool> scenario <name>` runs one. `<tool> scenario` alone prints its help,
  whose `Footer` lists the scenarios, and the root's help ends with the same listing. A CLI has
  no `list` command.
- A tool that sits beside a service or library lives in its own module, so its dependencies
  never enter the service's graph. slab does this, following the architecture's
  `principles/tool-beside-library.md`.
- Tests:
  - The `internal/app` tests execute the command tree over buffers, with fixtures from
    `apptest`.
  - The domain and scenario tests run over scripted stand-ins. A stand-in that more than one
    test package needs moves into `internal/<pkg>test`, as go-elemental's
    `principles/tests-and-docs.md` says; clutch's is `internal/harnesstest`.

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

## The plan

- **Cobra goes.** CLIs build on the standard library's `flag`. Cobra fails Go Elemental's
  no-frameworks line and the sourcing markers 1 (its type is in every caller's signature) and 5
  (argument parsing is a preference, kept in-house) of
  `architecture/standards/go-elemental/principles/dependencies.md` ("Sourcing"). The tools use a
  small part of it and carry code that works around its defaults: silenced errors,
  `RunE: cmd.Help()` on parents, and tests asserting cobra's message text.
- **go-cli-sdk**, one module, package `cli`, over the standard library and go-core, at the API
  in the `cli` goal's summary: the dispatcher, `Use` with `WithGraph`, `Invocation.Get`,
  `Validate`, `Footer`, `Streams`, and a `Run` that maps usage errors and requested help to
  `ExitUsage`. It leaves out shorthand flags, completion, a `help` command, a counterpart to
  cobra's `Long`, the `App` type (it stays in the template's `internal/app`, as in web), and
  the scenario runner (scaffolded as the tool's own package, promoted only on fit).
- **Per-command composition through the graph.** Each command declares its nodes, and the
  graph brings up only those, once each, closing them in reverse order. The same Coordinator
  runs a web service's graph.
- **go-cli-sdk-template** scaffolds the composition root in "The layout" on go-cli-sdk.
- **Promotions first.** The `go-core-graph` goal promotes the spike's `graph` and `lifecycle`
  into go-core, gives `process/processtest` a one-shot runner for CLIs, and moves the web
  stack onto the graph, before `cli` builds.
- **slab** aligns in its own `slab` goal once `cli` syncs, adopting the scenario shape in
  "Conventions". clutch, courier, and spike-blobfs stay on cobra; future spikes start from the
  template.
- **The evidence** is spike-cli-architecture (archived):
  [the answer](https://github.com/JaimeStill/spike-cli-architecture/blob/main/context/README.md#the-answer).
