# Graph ergonomics

go-core's `graph` and `lifecycle` behaved correctly but read badly, and their nine consumers
repeated the same workarounds. Goal graph-ergonomics redesigns their API, not their behavior, in one
breaking go-core release followed by one migration task per tier. experiment.graph's spike-graph
proved the API on the full go-web-service and slab, and its intake settled it. go-core v0.11.0
ships it, and every consumer has moved onto it: the libraries' releases, go-database v0.11.0,
go-web-sdk v0.16.0, go-observability v0.3.0, go-storage v0.7.0, and go-cli-sdk v0.3.0, with their
providers; the templates' releases, go-web-sdk-template template/v0.14.0 and go-cli-sdk-template
template/v0.2.0; and go-web-service with its slab CLI, which stay untagged. This note
holds the settled API. The detail behind it (the pain points, the behavior that must not change,
the rejected alternatives, and the sketches) lives in go-core's working context,
`go-core/context/graph-ergonomics.md`.

## The settled API

graph-ergonomics' decisions 9–14, as the releases ship them:

- **Order and groups at Define (9).** A node's place is declared on its Define line with
  `graph.After` and `graph.In`, and groups with `g.Group`; `Scope.After` is removed. An After
  takes a handle defined earlier. `System.Dropped()` reports what a partial Build left out.
  `Replace` swaps only the constructor and keeps the node's After and In. A Graph is safe for
  concurrent use: Builds may run concurrently, a describing call or `Observe` may run beside a
  Build, and concurrent Builds may call the same observers concurrently.
- **Constructors as methods on `*Nodes` (10).** A constructor that needs a setting may return one;
  a constructor that takes no node stays a plain function.
- **Values (11, changed).** `g.Value(name, v)` and the method `g.Fresh[T](name)` (Go 1.27 method
  type parameters) take Define's options, so a value can join a group.
- **Explicit names, checked (12).** Names stay explicit; a fixture check replaces name comments.
  Define, Value, Fresh, and Group refuse an empty name, a name containing a comma or a newline,
  and a name a node or a group already holds.
- **Handles in introspection (13).** `Ref` on a Dependency; `Observe(func(Ref))`, whose observers
  persist for every later Build, and whose panic Build re-panics as a "graph: " message naming
  the node, wrapping the value when it is an error; `LayerOf`, which panics for a node not in the
  System, a nil ref, or a group; `Has(ref) bool`, false for a node not in the System or from
  another Graph; `Dependencies()`, every built node in layer order, then definition order; and
  `String()`, the unquoted layer table, one line per layer as "0: config, logger", with no
  trailing newline.
- **`web.Service` and `cli.Program` (14, changed).** go-web-sdk's runner, `web.Service`, returns
  an exit code as go-cli-sdk's `cli.Program` does, instead of returning an error, and both take
  `lifecycle.Config` the same way: as a node.

## The three details

- **The name check.** A node's or group's name, lowercased with spaces removed, equals its Nodes
  field's name lowercased. go-core gains `graph/graphtest` with `NameErrors(nodes any) []error` and
  `CheckNames(t testing.TB, nodes any)`; Define, Value, Fresh, and Group refuse the empty name
  and a name containing a comma or a newline.
- **`web.Service`.** `web.Service{Graph, LifecycleConfig, Logger, Server, Roots}.Run(ctx, stderr)
  int`, where `Server` is a `*graph.Node[*web.Server]`. Run builds Server and LifecycleConfig
  beside Roots. It logs "server ready", "service failed", and "server stopped" on the logger node;
  a Build failure goes to stderr, and a failed shutdown logs "service failed" and returns 1. A
  service's `App.Run` becomes one `Run` call, and App drops its fallback logger. go-cli-sdk's
  runner is `cli.Program{Root, Graph, LifecycleConfig}.Run(ctx, args, streams) int`.
- **Plain values.** A `*Nodes` method reaches a plain value through a `graph.Value` node, as the
  logger Uses the "writer" node. `g.Value` infers `T` from the argument's static type.

## The node-named run failure

lifecycle's one behavior change: a Monitored value's runtime failure names its node,
`run: <node>: <error>`, where go-core v0.10.0 reports `run: <error>`. go-cli-sdk reports it as one
line, `<path>: run: <node>: <error>`, where the leaf's ended context would otherwise report
"context canceled".

## Not shipped

Arity adapters. The architect decided against them without an adapter tier: the Define-line
changes already made the graph legible and fixed the reliability findings, and a second
constructor form costs one adapter per arity, a projection node or wrapper for each constructor
that takes part of the config, and no-error variants. Methods on `*Nodes` are the only constructor
form.

The release also leaves out `Group.Members`, a way to stop an observer, and a `Replace` that
removes an edge or a group membership.

## Evidence

spike-graph answered yes: both real roots are shorter and their layer placement legible, with
every node's layer stated at its Define line. `mise run measure` against its baseline:

| Root | Code lines | Closures | Wrapper constructors | Name comments | `s.After` sites |
|---|---|---|---|---|---|
| go-web-service | 460 → 411 | 21 → 0 | 14 → 0 | 21 → 0 | 7 → 0 |
| slab | 207 → 202 | 11 → 7 | 3 → 1 | 12 → 0 | 1 → 0 |

The layer tests, the name check, and the runner's "service failed" and "server stopped" logs are
proven only by tests; the rest ran against real infrastructure.

[The answer](https://github.com/JaimeStill/spike-graph/blob/main/context/README.md#the-answer) ·
[spike-graph](https://github.com/JaimeStill/spike-graph) (archived)
