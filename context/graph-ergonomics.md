# Graph ergonomics

go-core's `graph` and `lifecycle` behave correctly but read badly, and their nine consumers
repeat the same workarounds. Goal graph-ergonomics redesigns their API, not their behavior, in one
breaking go-core release followed by one migration task per tier. The design direction (the pain
points, the behavior that must not change, the candidate API, the open fork, the rejected
alternatives, and the migration) lives in go-core's working context,
`go-core/context/graph-ergonomics.md`; this note holds the experiment that settles it and its
answers.

## Experiment: spike-graph

experiment.graph has one spike, `experiment.graph.spike-graph`, at `~/experiments/spike-graph`
(`github.com/JaimeStill/spike-graph`). Its question, evidence, and path are in the spike's
`context/README.md`.

- **The question.** Does the candidate API, built on today's graph semantics, make
  go-web-service's real composition root and slab's real CLI shorter, and their layer placement
  legible, run against real infrastructure, with arity adapters and methods on `*Nodes` compared
  on one tier?
- **The decision it changes.** The exact API go-core's breaking release ships: which of
  graph-ergonomics' decisions 9–14 survive as written, whether arity adapters ship, and the three
  details the design note leaves open (the rule the name check applies; what `web.Run` returns
  and reports, and where the ready, failed, and stopped logs go; how a `*Nodes` method reaches a
  plain value).

The spike is a prerequisite of capstone.data-intelligence: the capstone's composition root is
written against the API experiment.graph's intake settles and graph-ergonomics ships.

## Answers · experiment.graph

### Answer · experiment.graph.spike-graph

**Question:** Does the candidate API, on today's graph semantics, make go-web-service's composition
root and slab's CLI shorter, and their layer placement legible, run against real infrastructure,
with arity adapters and methods on `*Nodes` compared on one tier?

**Answer:** Yes: both real roots are shorter and their layer placement legible, with every node's
layer stated at its Define line, no constructor body ordering anything, and the service root's
wrapper constructors, closures, and name comments gone; arity adapters don't ship, so methods on
`*Nodes` are the only constructor form.

1. The candidate `graph` and `lifecycle` packages: sdk/graph's and sdk/lifecycle's tests, go-core's
   ported and the candidate's own, under `mise run check`; both roots run on them.
2. The service on the candidate: internal/app's tests under `mise run check`; `mise run integration`
   passed on the spike-graph-integration stack (web task); the migrated service served slab's
   scenarios on the spike-graph stack (cli task).
3. `web.Run`: sdk/web's tests and internal/app's Run tests; the running service logged "server
   ready" (cli task); "service failed" and "server stopped" are proven only by tests.
4. slab migrated: `mise run slab` ran all six scenarios against the migrated service on the stack,
   all exiting 0 (cli task); help and usage errors building nothing is proven only by tests.
5. Layer tests on `Roots()`, `LayerOf`, and handles, with `Dropped()` empty: proven only by tests
   (internal/app's layer tests, slab's `TestGraph_ScenarioLayers`).
6. The name check: proven only by tests (`graphtest.CheckNames`, run on both roots' Nodes).
7. A `*Nodes` method reaches a plain value through a `graph.Value` node (the logger's "writer"):
   internal/app's tests; the running service logs through it.
8. Arity adapters: decided, not measured. The architect decided they don't ship: the Define-line
   changes already made the graph legible and fixed the reliability findings, so a second
   constructor form isn't worth its cost (one adapter per arity, a projection node or wrapper for
   each constructor that takes part of the config, no-error variants). No adapter tier was written.
9. Before/after, from `mise run measure` against baseline 2999659: service code lines 460 → 411,
   closures 21 → 0, wrapper constructors 14 → 0, name comments 21 → 0, `s.After` sites 7 → 0; slab
   code lines 207 → 202, closures 11 → 7, wrappers 3 → 1, name comments 12 → 0, `s.After` 1 → 0.
   Decisions 9, 10, 12, 13 kept; 11 and 14 changed; the open fork dropped.

[The answer](https://github.com/JaimeStill/spike-graph/blob/main/context/README.md#the-answer) ·
[spike-graph](https://github.com/JaimeStill/spike-graph)
