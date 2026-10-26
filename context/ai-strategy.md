# AI

The planned AI capability builds in four layers, each on the one below (architect,
2026-10-26). `v1.ai` builds them, and go-web-service's AI layer demonstrates them. Two
experiments come first:

- **experiment.hosting** (`ai-hosting.md`) settles layer 1, the self-hosted stack.
- **experiment.ai** settles the layers above it:
  - spike-ai-pipeline proves the client and processing layers now, against the Framework's set
    A, and proves them together in a capstone once its gates clear.
  - spike-go-harness and spike-local-models take the harness layer after experiment.hosting's
    intake, beside spike-harness-driver, which has answered.

The client and processing layers come first because they feed the team's operational
requirements. Most service integrations build on them, not on a harness. The team adopts go-ai
once it is a mature, standard set of packages. Until then spike-ai-pipeline's design is go-ai's
reference.

## The four layers

| Layer | What it covers | Home |
|---|---|---|
| 1 Hosting | The machines that serve local models (the Framework desktop and the two GB10 workstations), covering four things: model selection and configuration, layout across the hardware, a minimal benchmark for a model's compatibility and configuration, and a gateway that serves the pool as one service | `ai-hosting` (planned member repository), plus promoted architecture pages (`ai-hosting.md`) |
| 2 Client | Go primitives over the engines' native APIs: chat with tools and reasoning, vision, embeddings, and transcription, with the wire format as a pluggable layer | `go-ai` (`topology-and-naming.md` reserves the name), its model-client surface |
| 3 Processing | Preparing messages, documents, and streams for native model capabilities, such as a PDF's pages through vision, whether the payload is a whole blob or a stream | settled by experiment.ai's intake; local models for application workloads through `v1.ai.local-models` (planned) |
| 4 Harness | Agentic sessions driven from Go (go-ai's harness-session surface), and local models taking subagent work in Claude Code sessions; lowest priority | `go-ai`, and claude-plugins (`claude-plugins/context/tool-based-skills.md`) through `v1.harness.local-models` (planned) |

go-web-service's AI layer demonstrates the four layers (its capability map, "AI").

## External harnesses in place of tau

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

herald (`references.md`, "herald") is the workload that shaped tau. It is prior art here, not a
workload to rerun. Its `internal/workflow/` is a four-node graph (init, classify, enhance,
finalize). It uses only the Chat and Vision calls and the state graph, on Azure AI Foundry with
managed identity, deployed to commercial Azure and to air-gapped IL6.

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
own tiers once it exists. If the harness-session surface also covers the native capabilities and
the long-running workflows, go-ai has only that surface, and tau retires. tau also retires if
go-ai has both surfaces, because the model-client surface is rebuilt under go-elemental's rules
rather than carried over.

## Experiment: spike-harness-driver

**Question.** Can Go drive an external harness as the infrastructure for agentic work, and what
does that infrastructure need to be? The harnesses are Pi (`--mode rpc` or `json`), Claude Code
(`claude -p --output-format stream-json`, or the Agent SDK), and OpenCode (its server API),
against local and cloud models. The question has four parts:

- **Capabilities.** Which of skills, tool calls, and the native model capabilities beyond chat
  (vision, embeddings, audio) does a harness expose, and which need a direct model client?
- **Sessions.** How is work that spans several agent turns established and managed, and how is a
  session kept beyond a single call and resumed?
- **Exchanges.** How is a payload sent and the response payload interpreted? How is one
  request-and-response block scoped and identified against a persistent session?
- **Workflows.** How are long-running workflows coordinated over those sessions: progress, event
  streaming, cancellation, and concurrency?

**Decision it changes.** go-ai's shape: a harness-session surface only, both surfaces, or a
model-client surface only. It also decides whether the service's container image carries a
harness.

**Evidence.** For each harness and model target, the spike records what the Go interface achieves
on each of the four parts. It also records:

- event streaming to SSE
- tokens and cost
- the container image's footprint
- the Azure managed-identity path, with IL6 checked on paper

**Home.** The experiment lives in `~/experiments/spike-harness-driver`, and its remote is
[JaimeStill/spike-harness-driver](https://github.com/JaimeStill/spike-harness-driver). Its first
step drives Pi against the Framework desktop's router: one session, two scoped exchanges, and a
cancel.

## Experiment: spike-ai-pipeline

**Question.** Can a Go model client and a message, document, and stream processing layer, built
under go-elemental, carry a pipeline that a browser follows live? The pipeline is transactional
and evented: it captures metadata, extracts data by inference, and stores embeddings in
PostgreSQL with pgvector for semantic search, and each stage emits the events the browser
follows. The spike has three repositories, one per layer, as spike-html-packages has:

- **`spike-ai-pipeline-client`** gives primitives over the engines' native APIs. Its baseline
  is OpenAI Chat Completions, with the wire format a pluggable layer beneath a stable request
  and response model, as tau's `protocol` and `format` split it.
- **`spike-ai-pipeline-processing`** prepares messages, documents, and streams for native
  model capabilities. herald's `internal/format` is its prior art.
- **`spike-ai-pipeline`**, the root, holds the capstone: a backend on the members and a web UI
  on experiment.html's stack. It takes messages, uploads, and streams, and shows what each
  stage does as it runs.

It reads tau, spike-harness-driver's `model` and `workflow`, herald, and the members, as
references, not implementations.

**Gates.** The client and processing tasks run now, against the Framework's set A, which serves
chat, vision, audio, transcription, and embeddings. The capstone tasks wait for three things:
those two tasks, spike-model-gateway's answer, and experiment.html's intake. The spike returns
to `planned` in between.

**Decision it changes.** go-ai's model-client surface and its wire-format layer, where the
processing layer lives, `v1.ai.local-models`, and v1.ai's library and service tasks. Its setup
settles five open choices:

- the client's base: spike-harness-driver's `model` or a fresh client
- PDF rendering: herald's ImageMagick and Ghostscript, which is AGPL, or a non-AGPL renderer
- the browser push: SSE or WebSocket
- pgvector as the workspace's first Postgres extension
- its evidence list

**Home.** `~/experiments/spike-ai-pipeline`, remote
[JaimeStill/spike-ai-pipeline](https://github.com/JaimeStill/spike-ai-pipeline), with its two
layer repositories beside it, set up by `plan experiment.ai.spike-ai-pipeline`.

## Experiment: spike-go-harness

**Question.** Does a native Go harness behind spike-harness-driver's driver interface, with
sessions, tool calls, skills, and MCP, reach Pi's fidelity on the self-hosted stack? The spike
measures it beside Pi with spike-harness-driver's conform, and takes the failed cells of
spike-model-hosting's harness-fit profiles as its design input.

**Decision it changes.** Whether go-ai carries its own agent loop, and what replacing the
process-backed drivers costs.

**Home.** `~/experiments/spike-go-harness`, remote
[JaimeStill/spike-go-harness](https://github.com/JaimeStill/spike-go-harness), set up by
`plan experiment.ai.spike-go-harness` once experiment.hosting's intake has run.

## Experiment: spike-local-models

**Question.** On the stack experiment.hosting settles (`ai-hosting.md`), how does a Claude Code
session that stays on Anthropic models route subagent tasks to locally hosted models, and for
which kinds of task is the output acceptable? It is harness-layer work at a later priority,
run after spike-go-harness. Application workloads on local models are spike-ai-pipeline's
(architect, 2026-10-26).

There are two candidates:

- a gateway that routes a subagent profile's model name either to the local router or to
  Anthropic
- a tool, MCP server, or skill that offloads the work to `pi --print` or directly to llama.cpp

The gateway candidate is the gateway spike-model-gateway settles, from agentgateway, LiteLLM, and
Agent Router (`ai-hosting.md`, "spike-model-gateway").
agentgateway's documented Claude Code integration points `ANTHROPIC_BASE_URL` at the gateway and
maps request model names to providers, so it can stand in for a purpose-built router.
agentgateway's documentation shows only a wildcard model sent to one backend,
so the spike first proves the split this candidate depends on: in one configuration, a subagent
profile's model name reaches the local router and the session's own model reaches Anthropic, with
tool use, streaming, and thinking blocks intact on both paths. If experiment.hosting settles on
no gateway, this candidate needs its own gateway in front of the session, which counts against it
when it is compared with the tool candidate.

`claude-plugins/context/tool-based-skills.md` holds the question the spike tests as open:
whether structured, repetitive generation moves to a local model behind a schema-validated
tool, and what would justify it.

**Decision it changes.** `v1.harness.local-models`: the profile format, the routing
mechanism, and which kinds of task may run locally. Whether `ai-hosting` includes a
gateway is spike-model-gateway's decision; this spike routes through what it settles.

**Home.** `~/experiments/spike-local-models`, remote
[JaimeStill/spike-local-models](https://github.com/JaimeStill/spike-local-models), set up by
`plan experiment.ai.spike-local-models` once experiment.hosting's intake has run.

## Constraint: model origin

Models of Chinese origin are avoided where an alternative serves, because of the risk of
censorship and of hidden behavior in their weights. The constraint binds experiment.hosting,
experiment.ai, and v1.ai. A model of Chinese origin is chosen only when no alternative serves, and
the choice records why. A model fine-tuned from a Chinese-origin base takes its base's origin,
whoever publishes the fine-tune.

## Answers · experiment.ai

### Answer · experiment.ai.spike-harness-driver

**Question:** Can Go drive an external agent harness (Pi, Claude Code, OpenCode) as the
infrastructure for agentic work: capabilities, sessions, scoped exchanges, and long-running
workflows?

**Answer:** Yes; go-ai takes both surfaces, harness sessions and a model client, and a service
image carries one pinned harness, Pi by default.

1. Tools and skills run on all three harnesses: a Go tool reaches Pi through its extension and
   Claude Code and OpenCode through MCP. Proven by a running demo: `clutch conform`, every
   capability on Azure and Anthropic for all three harnesses.
2. Of the native capabilities, a harness passes only vision, and drops an image a model can't
   take quietly; embeddings and audio need the `model` client. Proven by a running demo: `model`
   on the router and Azure, and a Go tool giving a Pi session a transcript.
3. A session outlives its process and resumes by ID with its history; `ErrJournalMismatch`
   catches Pi's cwd-scoped sessions. Proven by `clutch conform`'s `resume` and live follow-ups
   from a new process.
4. A session carries one exchange at a time under a driver-assigned ID, each harness's
   cancellation maps to one outcome, and a structured response is a `respond` tool. Proven by
   `clutch conform`'s `exchange`, `cancel`, and `structured`.
5. A standard-library workflow layer of about 1,800 lines runs a DAG of exchanges with a
   concurrency limit, cancellation, SSE with replay, and resume after SIGINT, `kill -9`, and
   `docker stop`. Proven by a running demo: `clutch workflow` and `clutch serve`, in the
   architect-validated checkpoints 1–3.
6. The service image is 215 MB, idles at about 10 MiB, and adds about 95 MiB per Pi session.
   Proven by a running demo: `deploy/`, checkpoint 3.
7. The Azure managed-identity path and IL6 hold. Checked on paper only; nothing ran against them.

[The answer](https://github.com/JaimeStill/spike-harness-driver/blob/main/context/findings.md#the-answer) ·
[spike-harness-driver](https://github.com/JaimeStill/spike-harness-driver)

## Assumptions

- A harness can run as a process inside the service's container image.
- llama.cpp's `/v1/messages` endpoint accepts Claude Code's tool schemas, streaming, and
  thinking blocks.
- The IL6 environment permits a harness process and its model endpoint.
- OpenAI Chat Completions is the baseline wire format: both engines serve it, and llama.cpp also
  serves Anthropic Messages. vLLM's gRPC server speaks raw generation for routers, not chat, so
  it sits below the gateway, not under go-ai.
- Pi, Claude Code, and OpenCode each expose a non-interactive, machine-readable session mode that
  is stable enough to build on.
