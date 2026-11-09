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
