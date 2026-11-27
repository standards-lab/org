# AI

The planned AI capability builds in four layers, each on the one below. `v1.ai` builds them, and
go-web-service's AI layer demonstrates them. Around the capstone, capstone.data-intelligence
(`data-intelligence.md`):

- **v1.ai.hosting** (`ai-hosting.md`) built layer 1, the self-hosted stack, before the capstone.
- **v1.ai** builds the client and processing layers before the capstone, in go-ai, docproc, and
  docinfer.
- **experiment.ocr**, after the capstone: spike-scanned-documents.
- **experiment.harness**, after the capstone: spike-go-harness and spike-local-models, beside
  spike-harness-driver's answer. The capstone needs no harness.

The client and processing layers come first: they feed the team's operational requirements, and
most service integrations build on them, not on a harness. The team adopts go-ai
once it is a mature, standard set of packages; until then spike-ai-pipeline's design is its
reference. Detail for the coming goals, and [candidates for `marathon
retro`](../docs/ai-strategy.md#candidates-for-marathon-retro), are on [the AI strategy
page](../docs/ai-strategy.md).

## The four layers

| Layer | What it covers | Home |
|---|---|---|
| 1 Hosting | The machines serving local models (the Framework desktop and two GB10 workstations): model selection, layout, upgrade evaluation, and a gateway serving the pool as one service, Torii (agentgateway) | `ai-hosting`, plus promoted architecture pages (`ai-hosting.md`) |
| 2 Client | Go primitives over the engines' native APIs (chat with tools and reasoning, vision, embeddings, transcription), the wire format pluggable | `go-ai`'s model-client surface |
| 3 Processing | Preparing messages, documents, and streams, whole or streamed, for native model capabilities | `docproc` and its system tools (`v1.ai.processing`); `docinfer` (`v1.ai.inference`) |
| 4 Harness | Agentic sessions driven from Go, and local models taking Claude Code subagent work; lowest priority | `go-ai`'s harness-session surface; claude-plugins (`claude-plugins/context/tool-based-skills.md`) through `v1.harness.local-models` (planned); Kannushi, the planned production harness service, a gateway consumer beside Bunrei |

## External harnesses in place of tau

tau (`references.md`, "Prior R&D — TAU ecosystem") is the organization's earlier agentic
infrastructure: model-provider plumbing (most of its code), agent-loop concerns (tool execution,
sessions, and memory), workflow orchestration (`orchestrate/state`), and domain logic, which stays
in each consumer. herald, the workload that shaped tau, is prior art, not a workload to rerun.

A harness such as Pi, Claude Code, or OpenCode already provides the agent loop.
`tau-platform/archive/tau-runtime/claude-classify-docs/` in `~/tau` motivates the pivot: herald's
predecessor workflow as a Claude Code project, about 570 lines of markdown against about 11k lines
of Go. Its recorded gaps are what a Go interface to a harness must close: a REST and SSE surface,
the choice of model, and control of the worker pool.

go-ai may need two surfaces: a **harness-session surface**, which runs an agentic session through
an external harness and streams its events, and a **model-client surface**, a thin client for
deterministic pipelines. These are surfaces, not `architecture/principles/service-tiers.md`'s
tiers; each has its own. If the harness-session surface also covers the native capabilities and
the long-running workflows, go-ai has only that surface and tau retires; tau also retires if go-ai
has both, since the model-client surface is rebuilt under go-elemental's rules.
experiment.harness's intake decides.

## The client and processing layers

v1.ai builds the client and processing layers from spike-ai-pipeline's answer. go-ai is the model
client; docproc extracts, with the three command-line tools nested inside it, and docinfer infers
over what docproc extracts. The spike proved both layers over a real share, ~/docs; the
transactional pgvector pipeline and the browser that follows it move, unproven, to the capstone.

The design reviews read [what spike-ai-pipeline
built](../docs/ai-strategy.md#what-spike-ai-pipeline-built) and [the spike's
answer](../docs/ai-strategy.md#the-spikes-answer), whose evidence numbers the notes cite.

### Repositories and placement

- **go-ai** is the model client, an infrastructure library beside go-storage in `order`, its base
  module on the standard library only: one Open Responses `protocol`, the three wire formats the
  engines serve natively (Chat Completions, Anthropic Messages, Open Responses), and a `/models`
  listing. Its CLI, `infer`, is the nested module `tools/infer`.
- **docproc** is the extraction library, an enabling library adjacent to the standard, beside
  sqlate. Its library module requires `golang.org/x/text` only and depends on no model concept.
  Its stores are interfaces with filesystem and memory implementations, its providers nested
  modules: `docproc/storage` (over go-storage) and `docproc/postgres` (over sqlate). The nested
  `cmd/docproc` writes a run directory through the public `rundir` and `rundir/schema`.
- **docinfer** is the inference library, adjacent and above go-ai's tier: it depends on docproc
  and go-ai. Its nested `docinfer/postgres` holds its cache in PostgreSQL, and `cmd/docinfer` is
  its command; docinfer's package docs are its built API's one home, and its command notes
  (`context/command.md`, `command-api.md`, `command-report.md`) hold cmd/docinfer's design.
- **docproc's system tools** live in docproc under `tools/<format>`, each on its own toolchain:
  docproc-ooxml (docx, xlsx, and pptx), docproc-mail, and docproc-onenote. Each has one versioned
  JSON Lines contract (`system-tools.md`, "The docproc-* tools and their contracts") and is tagged
  `tools/<format>/v<version>`, so a contract change lands with its decoder in one pull request.
  The Go module zip leaves `tools/` out.

An adjacent library may depend on a stack library, which pins it above that library's tier
(`architecture/principles/repository-topology.md`, "Adjacent libraries"): only layers above
go-ai's tier use docinfer. A provider sub-module pins only its own level, and one over sqlate pins
no tier. A repository's place in `order` follows its base module; nested command modules and
system tools don't move it. A cross-repository task orders its own exceptions in its brief.

### How v1.ai builds them

v1.ai's sub-goals have all shipped: v1.ai.client built go-ai,
v1.ai.processing built docproc and its system tools, v1.ai.inference built docinfer, and
v1.ai.hosting built ai-hosting. The design
tasks build from docproc's
[`docs/layers.md`](https://github.com/standards-lab/docproc/blob/main/docs/layers.md), the
signatures of every layer of Bunrei's stack. Each sub-goal opens with a `design` review, led by
a Fable 5.2 evaluation of each promoted artifact against the standards, the layers note, and
Bunrei's requirements, and closes with an `acceptance` task that reruns the real workloads and
fixes what it finds, never redesigning.

Local models for application workloads have no sub-goal of their own: ai-hosting serves set A's
roles for them (vision with reasoning off, transcription, and embeddings), and v1.ai.hosting's
`embedding` task replaced set A's embedding model (`ai-hosting.md`, "Set A's embedding model").

Evidence 5, settled, bears on the layering. docproc prepares long audio, cutting windows at
silences and releasing the join, and the layer above transcribes it and owns the inference
guards: Gemma 4 E4B answered a short window with its own instruction, which the join can't tell
from speech. No LLM pass smooths the seams; a gpt-oss pass over the seam words made every run
worse. Windowing costs accuracy on multi-speaker audio, so whether audio that fits one model
request goes unsplit is the host's choice, set through cmd/docproc's `DOCPROC_AUDIO_LIMIT`
(starting 30s; 4 s or less refused); docproc's default stays 30 s (docproc's
`docs/acceptance.md`).

The detail page lists [what docproc's library left
open](../docs/ai-strategy.md#what-docprocs-library-left-open).

## Open experiments

- **spike-scanned-documents** (experiment.ocr): does vision transcription hold at scale, and does
  a dedicated OCR model beat it? Decides whether the document stage needs an OCR tier, and the
  vision tier's model, image-token budget, and quantization
  ([detail](../docs/ai-strategy.md#experiment-spike-scanned-documents)).
- **spike-go-harness** (experiment.harness): does a native Go harness reach Pi's fidelity on the
  self-hosted stack? Decides whether go-ai carries its own agent loop
  ([detail](../docs/ai-strategy.md#experiment-spike-go-harness)).
- **spike-local-models** (experiment.harness, last): how does a Claude Code session route subagent
  tasks to local models, and for which tasks is the output acceptable? Decides
  `v1.harness.local-models` ([Experiment:
  spike-local-models](../docs/ai-strategy.md#experiment-spike-local-models)).

## Constraint: model origin

No model of Chinese origin is admitted unless cleared, because of the risk of censorship and of
hidden behavior in their weights. The constraint binds experiment.ocr, experiment.harness, and
v1.ai. A model fine-tuned from a Chinese-origin base takes its base's origin, whoever publishes the
fine-tune, and a model takes its teacher's origin, the generator of its training targets. A model
is cleared when NIST's CAISSI or the UK AISI evaluated that exact release, assessed covert
behavior, backdoors, and agent hijacking, and found parity with its U.S. references, and the
architect admits it
([ai-hosting's model-origin rule](https://github.com/standards-lab/ai-hosting/blob/main/docs/policy.md#model-origin)).

## Answers · experiment.ocr

## Answers · experiment.harness

### Answer · experiment.harness.spike-harness-driver

**Question:** Can Go drive an external agent harness (Pi, Claude Code, OpenCode) as the
infrastructure for agentic work?

**Answer:** Yes; go-ai takes both surfaces, harness sessions and a model client, and a service
image carries one pinned harness, Pi by default. A harness passes only vision of the native
capabilities; embeddings and audio need the model client. The Azure managed-identity path and IL6
are checked on paper only ([evidence](../docs/ai-strategy.md#spike-harness-drivers-evidence)).

## Assumptions

- The IL6 environment permits a harness process and its model endpoint.
- OpenAI Chat Completions is the baseline wire format vLLM serves; vLLM's gRPC server speaks raw
  generation for routers, not chat, so it sits below the gateway, not under go-ai.
- Pi, Claude Code, and OpenCode each expose a stable, non-interactive, machine-readable session
  mode.
