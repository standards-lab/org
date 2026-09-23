# AI

The planned AI capability, split into four layers, each in its own home. `v1.ai` builds the
library and service layers. Two experiments come first, one for the library layer and one for the
harness layer. A third experiment, on hosting, is described in `ai-hosting.md`.

## The four layers

| Layer | What it covers | Home |
|---|---|---|
| Hosting | Configuring and running the machines that serve local models: the Framework desktop now, the Dell NVIDIA workstations later | `ai-hosting` (planned member repository), plus promoted architecture pages (`ai-hosting.md`) |
| Harness | Local models taking subagent work in Claude Code sessions | claude-plugins and `architecture/harness/`, through `v1.harness.local-models` (planned) |
| Library | `go-ai`: a Go interface for running models and agent sessions from an application | `go-ai` (`topology-and-naming.md` reserves the name) |
| Service | go-web-service's AI layer, which demonstrates go-ai | go-web-service's capability map ("AI") |

## The pivot away from tau

tau (`references.md`, "Prior R&D — TAU ecosystem") is the organization's earlier agentic
infrastructure. It has four kinds of responsibility:

- **Model-provider plumbing**: `protocol`, `format`, `provider` (azure, ollama, bedrock), and the
  `agent` client's Chat, Vision, and Embed with retry. This is most of the code.
- **Agent-loop concerns**: tool execution, sessions, and memory. These exist only in the
  unreleased `kernel`, and they are exactly what a harness such as Pi, Claude Code, or OpenCode
  already provides.
- **Workflow orchestration**: `orchestrate/state`, a state graph with checkpoints and observer
  events.
- **Domain logic**: this stays in each consumer.

herald is the proof workload (`references.md`, "herald"). Its `internal/workflow/` is a four-node
graph (init, classify, enhance, finalize). It uses only the Chat and Vision calls and the state
graph, on Azure AI Foundry with managed identity, deployed to commercial Azure and to air-gapped
IL6.

`tau-platform/archive/tau-runtime/claude-classify-docs/` in `~/tau` is the prior art that
motivates the pivot. It re-expressed herald's predecessor workflow as a Claude Code project:
skills, subagents, and working-directory state, in about 570 lines of markdown against about 11k
lines of Go. It showed that an effective harness runs the same workflow with no Go infrastructure
at all. The gaps it recorded are what a Go interface to a harness has to close: a REST and SSE
surface, the choice of model, and control of the worker pool.

The hypothesis follows. go-ai may need two surfaces:

- a **harness-session surface**, which runs an agentic session through an external harness and
  streams its events
- a **model-client surface**, a thin client for deterministic pipelines that call a model
  directly

These are surfaces, not the standard and native tiers of `service-tiers.md`. Each surface has its
own tiers once it exists. If the harness-session surface can serve herald's workflow too, go-ai
has only that surface, and tau retires. tau also retires if go-ai has both surfaces, because the
model-client surface is rebuilt under go-elemental's rules rather than carried over.

## Experiment: spike-harness-driver

**Question.** Can one Go interface run agentic sessions through Pi (`--mode rpc` or `json`),
Claude Code (`claude -p --output-format stream-json`, or the Agent SDK), and OpenCode (its server
API), against local and cloud models? And how does herald's classification workflow compare when
it runs through a harness inside the service's container, versus direct model calls in tau's
style?

**Decision it changes.** go-ai's shape: a harness-session surface only, both surfaces, or a
model-client surface only. It also decides whether the service's container image carries a
harness.

**Evidence.** Run herald's workflow both ways on the same document set, and compare:

- accuracy parity
- latency
- tokens and cost
- control of concurrency and cancellation
- event streaming to SSE
- the container image's footprint
- the Azure managed-identity path, with IL6 checked on paper

## Experiment: spike-local-subagents

**Question.** A Claude Code session stays on Anthropic models. How should it delegate subagent
work to framework-hosted models, and for which kinds of task is the output acceptable? There are
two candidates:

- a gateway that routes a subagent profile's model name either to the local router or to
  Anthropic
- a tool, MCP server, or skill that offloads the work to `pi --print` or directly to llama.cpp

`tool-based-skills.md` ("What tooling buys when a model executes a skill") already takes a
position that the spike tests. It moves generation to a local model only when the work is
structured, repetitive, and schema-validated, and it treats the offload as a tool.

**Decision it changes.** The convention `v1.harness.local-models` writes: the profile format,
the routing mechanism, and which kinds of task may run locally. It also decides whether
`ai-hosting` includes a gateway.

## Assumptions

- A harness can run as a process inside the service's container image.
- llama.cpp's `/v1/messages` endpoint accepts Claude Code's tool schemas, streaming, and
  thinking blocks.
- The IL6 environment permits a harness process and its model endpoint.
- Pi, Claude Code, and OpenCode each expose a non-interactive, machine-readable session mode that
  is stable enough to build on.
