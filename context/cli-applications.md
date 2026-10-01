# CLI applications

A command-line tool is the Elemental Architecture's second application type, beside the web
service. Three tools share one layout:

- slab (`go-web-service/tools/slab`)
- spike-blobfs's `blobfs` command
- spike-harness-driver's `clutch`

This note records that layout, the conventions around it, where the three differ, and the plan
to build the layout into the standard as a template, and possibly an application SDK. The layout
is provisional until the template builds it.

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
- A tool that sits beside a service or library lives in its own module, so cobra never enters
  the service's dependency graph. slab does this, following the architecture's
  `principles/tool-beside-library.md`.
- Tests:
  - The `internal/app` tests execute the command tree over buffers.
  - The domain and scenario tests run over scripted stand-ins. A stand-in that more than one
    test package needs moves into `internal/<pkg>test`, as go-elemental's
    `principles/tests-and-docs.md` says; clutch's is `internal/harnesstest`.

## Where the three differ

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

- **Does cobra stay?** The web service uses plain `net/http` with no framework, and the CLI
  should follow unless cobra earns its place. `goals.cli.tasks.sdk-decision` answers this first:
  - what cobra provides beyond the standard library's `flag`: nested commands, persistent flags
    inherited down the tree, generated help and usage, `PersistentPreRunE`, shell completion;
  - what reproducing the layout's command tree on `flag` costs: a small dispatcher over
    `flag.FlagSet`s, help text, and the persistent-flag inheritance the conventions above lean on.

  If cobra goes, slab moves to the result in the same goal. CLIs in `~/experiments` stay as they
  are, and future spikes start from the template.
- **go-cli-sdk.** Whether CLIs get an application SDK is undecided. The candidates are:
  - the `App` skeleton: the cold and hot starts, the error rendering, and the exit code
  - the scenario runner and its reporter
  - the base of the output

  A piece moves into the SDK when it fits the SDK, meaning it is expressed in the SDK's terms and
  depends on nothing above it (the architecture's `context/promote-on-fit.md`), not because
  several tools repeat it.
- **The template** is planned either way. It scaffolds a tool with the layout above:
  - process entry and the composition root's layer files
  - `output`
  - one domain with one direct command
  - one scenario
  - the tests

  It is named `go-cli-sdk-template` if the SDK exists, following `topology-and-naming.md`, which
  names a template after its SDK. Otherwise it is `go-cli-template`, and the naming principle
  would need to allow a template with no SDK.

## Assumptions

- The layout survives the cobra decision: its layers and conventions are about composition, and
  only the command-library-specific conventions above change.
- The layout still fits once spike-messaging adopts it for its checkpoint programs.
