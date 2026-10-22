# AI hosting

The hosting layer of the AI capability (`ai-strategy.md`) covers configuring and running the
machines that serve local models to the organization's consumers:

- Pi sessions
- Claude Code subagents
- the go-ai service

Its planned home is a new workspace member repository, working name `ai-hosting`.
`v1.ai.hosting` builds it fresh from what spike-model-hosting proves on personal-agents'
setup, rather than transferring that repository's history.

## The existing setup: personal-agents

personal-agents (`github.com/JaimeStill/personal-agents`, local `~/code/personal-agents`) is a
marathon context project outside the workspace. It configures one host class, the
Framework desktop:

- **Hardware**: AMD Strix Halo, 128GB of unified memory, about 96GB of it exposed to the GPU.
- **Serving**: llama.cpp's router in Vulkan mode, from upstream llama.cpp's release build b11529,
  run as a systemd service. It binds only to the tailnet, and Tailscale's device authentication
  is the access boundary.
- **Models**: spike-model-hosting's starting set, set A, sharing four slots and a unified KV cache:
  - gpt-oss-120b MXFP4, at 131072 context
  - Gemma 4 26B-A4B Q4_0, with vision, at 32768 context
  - gemma-4-E4B Q8_0, with audio, at 32768 context
  - EmbeddingGemma 2 Q8_0, for embeddings

  With all four loaded and four requests in flight per model, 3.30 GiB of the 96GB GPU pool stays
  free.
- **Client**: Pi reaches the router directly through `LLAMA_BASE_URL`.

Its `reference/` directory holds the measured method:

- model selection and tiers by memory budget
- context sizing
- the memory footprint on unified memory
- the preset convention (a recipe per model, and instantiated copies per capability tier)

`admin/` holds `outpost`, a bash dispatcher over `outpost-<group>-<verb>` scripts. Its
`capabilities/gateway-proxy.md` defers a gateway until a second client needs the router, and
Claude Code subagents are that second client.

The Dell NVIDIA workstations are a second host class, with CUDA instead of Vulkan and discrete
VRAM instead of unified memory. Their specifications are gathered by spike-model-hosting's `cuda`
task, on the first workstation, available from 2026-10-14.

## Experiment: experiment.hosting

**Question.** How do the workstations (the Framework's Strix Halo and the two GB10s) serve local
models: which engine and hosting method on each, how the models are laid out across their memory
pools, and which gateway routes requests across the pool and restricts access to it? Its intake
decides what `v1.ai.hosting` builds. The spikes run one at a time: spike-model-hosting's
Framework baseline, then spike-model-engines, spike-model-layout, and spike-model-gateway. Their
answers land in "Answers · experiment.hosting" below.

**Production.** Production has no tailnet; the tailnet only fences the dev workstations. A
containerized service on Azure Container Apps will route inference between Azure AI Foundry and
the self-hosted stack by request and availability. The stack has to take requests, spread them
across the pool, and queue them when capacity is full, and its access control can't depend on the
tailnet.

**The rules.** Every spike of the experiment keeps to them:

- A pinned standard tool is adopted before anything is written. No Python is authored, and mise
  tasks wrap plain commands. Install and setup may be scripted, as named, idempotent steps in a
  clean, scalable layout (the pattern: `dotfiles/platforms/dgx/system-setup.sh`).
- A task introduces at most one new tool, and its brief and walkthrough say what the tool is, how
  it is set up, and what it provides. A task has at most four slices; its last is a walkthrough the
  architect runs before the next task is planned.
- Every benchmark is agreed with the architect in the task's plan round: the tool, how it runs
  (configuration, load, duration), and the decision it informs. A run takes at most 10 minutes
  unless the brief decides otherwise, sized to how critical the question is. BUILD runs no
  benchmark the brief doesn't name; a new or widened one is an escalation.
- Benchmarking looks for a solid starting point, not perfection: the configuration is tuned as the
  stack is used, with no premature optimization.

**Hosts and access.** The Framework (`framework`) and the two GB10s (`gb10-1`, `gb10-2`) are on the
tailnet under `tag:inference`, reached by Tailscale SSH, which stays in accept mode while the
experiments run. Sessions do on any of them whatever needs no sudo, the docker group included;
root stays the architect's, run from exact commands. A host is brought up to date right before a
spike begins and frozen while it runs, and every measurement records the host's stack.

### spike-model-engines

Which engine, llama.cpp or vLLM, serves each workstation, and is it hosted natively or in a
container? Measures raw capability by model shape, quantization, and concurrency, with no model
sets. Decides the engine and hosting method per host class.

### spike-model-layout

How are models placed across the three memory pools, and what context and how many concurrent
sessions does each placed model hold? Decides the layout.

### spike-model-gateway

Which gateway spreads requests across the pool and queues them at capacity, and how is access
restricted without the tailnet? Decides whether a gateway fronts the stack, which, and its access
control.

## Experiment: spike-model-hosting

The hosting experiment is a new repository, `~/experiments/spike-model-hosting` (remote
[JaimeStill/spike-model-hosting](https://github.com/JaimeStill/spike-model-hosting)), not
personal-agents itself. It reads personal-agents, spike-harness-driver, tau-examples, and
tau-protocol, all in the references catalog, and archives none of them. It wrote to
personal-agents and spike-harness-driver only in its `align` task. The spike's answer lands in
`ai-strategy.md`, "Answers · experiment.ai".

**Question.** Which serving platform, configuration, and specification shape hold across both
host classes (Strix Halo with Vulkan or ROCm, Dell NVIDIA with CUDA) and all three consumers at
once?

**Decision it changes.** The `ai-hosting` specification:

- the serving platform on each host class
- the schema of a host-class profile
- whether a gateway sits in front of the serving, and which one
- the form of the admin tooling

**Serving platforms.** Only llama.cpp has been stood up so far. The spike compares, live:

- llama.cpp: Vulkan and ROCm on Strix Halo, CUDA on the workstation, behind its own router
- vLLM: ROCm on Strix Halo, CUDA on the workstation, behind llama-swap
- SGLang: CUDA on the workstation, and time-boxed on Strix Halo from its nightly gfx1151
  images, behind llama-swap

For each, it records how the platform is set up and managed, and what it makes possible: model
swapping, prefix caching, structured output, batching, embeddings, vision, audio, and the
Anthropic Messages API. Lemonade serves only as an install path for vLLM on Strix Halo. Ollama
and LM Studio wrap llama.cpp's ggml, and TGI is archived; all three are recorded on paper. The
main-model pool is gpt-oss-120b (the lead), Mistral Small 4 (a swap candidate), and Gemma 4
26B-A4B. The shared model set, set A, is gpt-oss-120b, Gemma 4 26B-A4B, gemma-4-E4B, and
EmbeddingGemma 2, each in its platform's native format, under the model-origin constraint
(`ai-strategy.md`).
Every engine and gateway runs its latest upstream release, and each document records the exact
version in use when it is written.

**Gateway approaches.** The spike compares no gateway with agentgateway and LiteLLM in front of
the serving. Envoy AI Gateway (now Agent Router) and vLLM's production-stack are
Kubernetes-first, and are recorded on paper. A gateway's case is what the tailnet boundary
doesn't give the three consumers:

- a separate identity for each consumer
- token budgets and rate limits that keep one consumer from exhausting the shared slots
- OpenTelemetry traces and metrics for each request

For each gateway, the spike records:

- **Fidelity.** Pi and go-ai's `model` client run through it unchanged, and Claude Code's tool
  use, streaming, and thinking blocks pass intact. agentgateway rejects Claude Code's
  experimental beta parameters ("Extra inputs are not permitted"); the documented workaround,
  `CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1`, is a cost the spike weighs.
- **Cost.** Its memory and latency beside the serving, and whether its configuration fits in a
  host-class profile or stands beside it.
- **Both host classes.** The same configuration in front of both hosts' serving.
- **IL6.** Whether it is acceptable air-gapped, checked on paper.

llama.cpp, vLLM, and SGLang each serve the Anthropic Messages API natively, so a gateway's case
rests on identity, budgets, routing, and observability, not on translating formats. If none of
those is needed across the three consumers, the answer is no gateway.

**Hosts.** The spike runs on the Framework desktop first. The Dell NVIDIA workstation is
available from 2026-10-14, and the tasks that need it run last. The spike may change anything on
either host, reached by Tailscale SSH in accept mode until it completes. It never touches the
architect's laptop beyond the repository, its check, read-only probes, and HTTP.

## Where personal-agents' parts go

- **Into `ai-hosting`**, one specification-level repository:
  - the host-class profiles and presets
  - the admin tooling
  - the systemd unit, which personal-agents describes but doesn't track
  - a runbook for each host class

  It states the settings the architect's own implementation uses, generally enough that another
  host of the same class follows it.
- **Into the architecture layer**, the portable method, as pages written once the hosting
  experiment validates it:
  - model tiers
  - context sizing
  - the method for measuring memory footprint
  - the serving conventions: router mode, tailnet-only binding, presets keyed by capability tier
    rather than by host, and the tool-call reliability convention (spike-model-hosting's
    `context/tool-reliability.md`)

  Their place in the layer is decided when they are promoted. claude-plugins'
  tool-based-skills note is the nearest neighbor.
- **Nowhere**: the state of a particular host. Once a host is set up it needs little attention,
  and anything a build or tuning effort keeps falls outside the workspace. There is no instance
  repository, so the specification/instance split of `claude-plugins/context/tool-based-skills.md` has no instance half here.

personal-agents is archived once `ai-hosting` lands, and its README gains a forward link to it.

## Visibility

`ai-hosting` lives in the standards-lab organization and is public, like the rest of the working
context. Tailnet identifiers (the MagicDNS suffix and the addresses) stay out of it, and the
docs name hosts by their short name.

## Open for v1.ai.hosting's SETTLE

These are decided once the experiments show what the tooling has to serve.

**The repository's name.** `ai-hosting` is the working name. The criteria:

- It names a facet the architecture standardizes, not one person's setup.
- It stands outside the five tiers, beside the harness repositories (`repository-topology.md`).
- It leaves room for adjacent concerns, such as a gateway, a retrieval store, or a chat UI, only
  if the facet includes them.

**The admin tooling:**

- **Name.** `outpost` names where the admin sits, not what the tool does.
- **Form.** Either keep the bash dispatcher, or write a Go CLI on go-cli-sdk, layered as
  specification, tooling, and skill (`claude-plugins/context/tool-based-skills.md`), with the
  profile schema as its contract. spike-model-hosting builds the Go CLI, with outpost as its
  baseline.
- **Host-class differences.** How the tool handles them: GPU usage through `amdgpu_top` versus
  `nvidia-smi`, and Vulkan versus CUDA builds.

## Answers · experiment.hosting

## Assumptions

- One serving stack per host can serve all three consumers within its slot and memory budget.
- The preset convention (presets keyed by capability tier) carries over to discrete-VRAM hosts
  and to engines other than llama.cpp.
