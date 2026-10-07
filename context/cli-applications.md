# CLI applications

A command-line tool is the Elemental Architecture's second application type, beside the web
service. Four tools share one layout, all on cobra:

- slab (`go-web-service/tools/slab`)
- spike-blobfs's `blobfs` command
- spike-harness-driver's `clutch`
- spike-messaging's `courier`

This note records that layout, the conventions around it, where the tools differ, and the plan
to build the layout into the standard as go-cli-sdk and its template. The layout is provisional
until `experiment.cli-architecture` proves its replacement and the template builds it.

## The layout

The application-layer import direction applies as go-elemental's
`principles/topology-and-naming.md` states it: `cmd/*` imports `internal/*`, `internal/*`
imports the root-level packages, and nothing imports in the reverse direction.

- **Process entry.** `cmd/<name>/main.go` does these things and nothing else:
  - It derives the signal context.
  - It hands the process's streams to the composition root.
  - It exits with the code the run returns: `os.Exit(app.New(os.Stdout, os.Stderr).Run(ctx))`.

  It imports only `internal/app`.
- **The composition root**, `internal/app`, has one file per layer:
  - `app.go` holds the root command and `App`, which has two starts:
    - `New` is the cold start. It composes the layers in dependency order and performs no I/O.
    - `Run` is the hot start. It executes the command tree under the signal context, renders a
      command's error once through the output, closes what the infrastructure opened, and
      returns the exit code.
  - `config.go` binds the persistent flags into a `Config`.
  - `infrastructure.go` builds the services the layers use from the configuration. It is the
    only file that names a provider or an adapter.
  - `domain.go` builds the domain services and mounts their commands. `admin.go` does the same
    for admin services, where there are any.
  - A scenario file, `demo.go` in slab and `scenarios.go` in clutch, mounts the narrated
    scenarios and the `list` command.
  - `commands.go` is the list of mounts. It composes them and does nothing else.
- **Root-level packages:**
  - `domain/<name>` holds one domain's service and its direct commands, which
    `Commands(svc, out) *cobra.Command` builds. `admin/<name>` does the same for an admin
    service, and the two trees are siblings. A direct command does one real operation and prints
    its result.
  - `output` is the renderer every command shares. Results go to stdout, errors to stderr.
  - `scenario` holds the narrated scenarios, a different kind of thing from a direct command. A
    scenario states what it needs and checks it first. Then it runs its steps, and each step
    says what it is about to do before doing it.
  - A tool adds shared packages as it needs them, such as slab's `input`, `style`, `httpx`, and
    `env`.

## Conventions

- Cobra builds the command tree before it parses the flags. So a layer that reads configuration
  gets a function or the `Config` pointer and reads it at run time, never a value copied when
  the layer was built.
- The root silences cobra's own error and usage reporting, so `App.Run` renders the one error.
  Run with no subcommand, the root prints its help and the scenario listing.
- A flag that names something the tool can't build fails before any command runs: clutch checks
  `--harness` in the root's `PersistentPreRunE`.
- Subcommands are bare words, never prefixed with the capability's name: `demo domain`,
  `scenario resume`.
- Every CLI's scenarios take one shape. One `scenario` package holds the runner and the
  scenarios, and `<tool> scenario <name>` runs one. `<tool> scenario` alone prints its help, whose
  footer lists the scenarios, and the root's help ends with the same listing. A CLI has no `list`
  command.
- A tool that sits beside a service or library lives in its own module, so cobra never enters
  the service's dependency graph. slab does this, following the architecture's
  `principles/tool-beside-library.md`.
- Tests:
  - The `internal/app` tests execute the command tree over buffers.
  - The domain and scenario tests run over scripted stand-ins. A stand-in that more than one
    test package needs moves into `internal/<pkg>test`, as go-elemental's
    `principles/tests-and-docs.md` says; clutch's is `internal/harnesstest`.

## Where the tools differ

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

`cli`'s sdk-decision is settled (`plan-cli`):

- **Cobra goes.** CLIs build on the standard library's `flag`. Cobra fails Go Elemental's
  no-frameworks line and the sourcing markers 1 (its type is in every caller's signature) and 5
  (argument parsing is a preference, kept in-house) of
  `architecture/standards/go-elemental/principles/dependencies.md` ("Sourcing"). The tools use a
  small part of it and carry code that works around its defaults: silenced errors,
  `RunE: cmd.Help()` on parents, and tests asserting cobra's message text.
- **go-cli-sdk** exists, one module, package `cli`, over the standard library and go-core. It
  holds only the dispatcher:
  - flags after positionals, root flags at any depth, a root pre-run hook
  - NoArgs and ExactArgs, required and mutually exclusive flag groups
  - a set-flag query and a repeatable string flag
  - generated help: a parent run alone prints help, an unknown subcommand is a usage error
  - a Run that maps usage errors to go-core's ExitUsage

  It leaves out shorthand flags, completion, the `App` type (it stays in the template's
  `internal/app`, as in web), and the scenario runner (scaffolded as the tool's own package,
  promoted only on fit).
- **Per-command composition.** `New` builds every layer for every command today, so each
  command pays for the whole stack. The composition root starts only what every run needs;
  each command declares the dependencies it requires, and one central initializer brings up
  only those, once each, closing them in reverse order.
- **go-cli-sdk-template** scaffolds the layout on go-cli-sdk.
- **The experiment first.** `experiment.cli-architecture` builds one spike,
  `spike-cli-architecture`, on real dependencies (the `blobfs` library, go-storage, Postgres)
  as close to the template as possible, adapting spike-blobfs as a read-only reference. Its
  intake writes `cli`'s tasks.
- **Promotions before `cli`.** Every library promotion the experiment identifies runs once the
  experiment completes and before `cli` builds. The `go-core-graph` goal promotes the spike's
  `graph` and `lifecycle` packages into go-core, and gives `process/processtest` a one-shot
  runner for CLIs.
- **slab** aligns in its own `slab` goal once `cli` syncs, adopting the scenario shape in
  "Conventions". clutch, courier, and spike-blobfs stay on cobra; future spikes start from the
  template.

## Assumptions

- The layout survives the cobra decision: its layers and conventions are about composition, and
  only the command-library-specific conventions above change, along with the composition root's
  per-command dependencies.

## Answers · experiment.cli-architecture

### Answer · experiment.cli-architecture.spike-cli-architecture

**Question:** Do a stdlib-`flag` dispatcher with the planned feature set and a per-command
dependency initializer hold up in a real CLI over the `blobfs` library, go-storage, and Postgres?

**Answer:** Yes; a dispatcher on the standard library's `flag` carries spike-blobfs's whole
command surface, each command brings up only the graph nodes it declares, and the same
Coordinator runs go-web-service's staged graph.

1. A dispatcher on the standard library's `flag` covers go-cli-sdk's planned feature set. Proven
   by the running binary; `cli`'s tests alone prove root flags, PreRun, and Exclusive, which
   blobfs doesn't use.
2. A CLI with no cobra or pflag in its module graph reproduces spike-blobfs's whole command
   surface. Proven by the integration suite's `TestScript` over the built binary, and by
   `go mod graph`.
3. Each command brings up only the graph nodes it declares. Proven by help, version, and a usage
   error running with the stack down, and by `internal/app`'s build-recording tests.
4. Each node comes up once per run and shuts down in reverse, on success, error, and
   cancellation. Proven by `cli`'s tests and the integration suite, whose `TestAnInterruptedPut`
   sends SIGINT to the built binary and gets one report.
5. A dependency that fails to come up is reported once, and what had started is shut down.
   Proven by `TestUse_FailuresReportedOnce`,
   `TestInfrastructureIntegration_StoreUnreachableClosesTheDatabase`, and the integration
   suite's `TestTheStoreUnreachable`.
6. A record lists each cobra convention and feature spike-blobfs used, with what replaced it.
   Proven by running both binaries' help and exit codes: [the cobra record](https://github.com/JaimeStill/spike-cli-architecture/blob/main/context/cobra-conventions.md).
7. The layout's test conventions hold without cobra. Proven only by tests: `internal/app`'s over
   buffers and `domain/files`'s over `storagetest.Fake`.
8. A scenario package declares its nodes like any other command. Proven by both scenarios
   running twice against the development stack, and by the integration suite's `TestScenarios`.
9. The graph expresses go-web-service's stage table, and the CLI and a service run on one
   Coordinator. Proven only by tests: `TestWebServiceStageOrder` and `TestWebServiceSubsetBuild`
   (graph) and `TestRunServesAGraphShapedLikeTheWebService` (lifecycle).

**go-cli-sdk:** the `cli` package's API is the candidate. It adds `Use` with `WithGraph`,
`Invocation.Get`, `Validate`, `Footer`, and `Streams`, and departs from the plan three times:
requested help exits 2, there is no `help` or `completion` command, and cobra's `Long` has no
counterpart.

**go-cli-sdk-template:** `internal/app`'s shape is the candidate composition root. One exported
`Nodes` value describes the graph, configuration is graph nodes, there is no `config.go`,
`commands.go`, or central initializer, fixtures live in an internal `apptest` package, and
`main` imports `cli` for `cli.Streams`.

**Promotion:** promote `graph` and `lifecycle` into go-core at the API as validate left it.

[The answer](https://github.com/JaimeStill/spike-cli-architecture/blob/main/context/README.md#the-answer) ·
[spike-cli-architecture](https://github.com/JaimeStill/spike-cli-architecture)
