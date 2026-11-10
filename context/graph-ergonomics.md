# Graph ergonomics

go-core's `graph` and `lifecycle` behave correctly but read badly, and their nine consumers
repeat the same workarounds. Goal graph-ergonomics redesigns their API, not their behavior, in one
breaking go-core release followed by one migration task per tier. experiment.graph's spike-graph
proved the candidate API on the full go-web-service and slab, and its intake settled what the
release ships; this note holds that settled API. The detail behind it (the pain points, the
behavior that must not change, the rejected alternatives, and the sketches) lives in go-core's
working context, `go-core/context/graph-ergonomics.md`, which the release task rewrites to the
shipped API.

## The settled API

graph-ergonomics' decisions 9–14, as the release ships them:

- **Order and groups at Define (9).** A node's place is declared on its Define line with
  `graph.After` and `graph.In`, and groups with `g.Group`; `Scope.After` is removed. An After
  takes a handle defined earlier. `System.Dropped()` reports what a partial Build left out.
- **Constructors as methods on `*Nodes` (10).** A constructor that needs a setting may return one;
  a constructor that takes no node stays a plain function.
- **Values (11, changed).** `g.Value(name, v)` and the method `g.Fresh[T](name)` (Go 1.27 method
  type parameters) take Define's options, so a value can join a group.
- **Explicit names, checked (12).** Names stay explicit; a fixture check replaces name comments.
- **Handles in introspection (13).** `Ref` on a Dependency, `Observe(func(Ref))`, `LayerOf`, and
  `String()`. `LayerOf` panics for a non-member, a nil ref, or a group handle.
- **`web.Run` (14, changed).** go-web-sdk's runner matches `cli.Run`'s exit-code contract instead
  of returning an error.

## The three details

- **The name check.** A node's or group's name, lowercased with spaces removed, equals its Nodes
  field's name lowercased. go-core gains `graph/graphtest` with `NameErrors(nodes any) []error` and
  `CheckNames(t testing.TB, nodes any)`; Define, Value, Fresh, and Group refuse the empty name.
- **`web.Run`.** `web.Run[S Server](ctx, g, lifecycleConfig, logger, server, roots, stderr) int`,
  where `Server` has one method, `Addr() string`. It logs "server ready", "service failed", and
  "server stopped" on the logger node; a Build failure goes to stderr, and a failed shutdown logs
  "service failed" and returns 1. A service's `App.Run` becomes one call, and App drops its
  fallback logger.
- **Plain values.** A `*Nodes` method reaches a plain value through a `graph.Value` node, as the
  logger Uses the "writer" node. `g.Value` infers `T` from the argument's static type.

## The node-named run failure

lifecycle's one behavior change: a Monitored value's runtime failure names its node,
`run: <node>: <error>`, where go-core v0.10.0 reports `run: <error>`. It ships in the release, and
go-cli-sdk's monitored-failure test changes with it in migrate-libraries.

## Not shipped

Arity adapters. The architect decided against them without an adapter tier: the Define-line
changes already made the graph legible and fixed the reliability findings, and a second
constructor form costs one adapter per arity, a projection node or wrapper for each constructor
that takes part of the config, and no-error variants. Methods on `*Nodes` are the only constructor
form.

## Open findings

The spike's findings the intake left to the tasks that ship the code:

- For the release's plan rounds: `LayerOf` as `(int, bool)` or a `Has(ref)`; no
  `Group.Members()`; observers persisting across Builds; `Replace` can't remove an edge.
- For migrate-libraries' plan rounds: `web.Run`'s four handles against an options struct;
  `cli.Command` has no children accessor; sdk/cli never reads `Dropped`.

## Evidence

spike-graph answered yes: both real roots are shorter and their layer placement legible, with
every node's layer stated at its Define line. `mise run measure` against its baseline:

| Root | Code lines | Closures | Wrapper constructors | Name comments | `s.After` sites |
|---|---|---|---|---|---|
| go-web-service | 460 → 411 | 21 → 0 | 14 → 0 | 21 → 0 | 7 → 0 |
| slab | 207 → 202 | 11 → 7 | 3 → 1 | 12 → 0 | 1 → 0 |

The layer tests, the name check, and `web.Run`'s "service failed" and "server stopped" logs are
proven only by tests; the rest ran against real infrastructure.

[The answer](https://github.com/JaimeStill/spike-graph/blob/main/context/README.md#the-answer) ·
[spike-graph](https://github.com/JaimeStill/spike-graph) (archived)
