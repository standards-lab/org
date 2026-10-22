# AI hosting

The hosting layer of the AI capability (`ai-strategy.md`) covers configuring and running the
machines that serve local models to the organization's consumers:

- Pi sessions
- Claude Code subagents
- the go-ai service

The hosts are the Framework desktop and the two GB10 workstations. The layer's planned home is a
new workspace member repository, working name `ai-hosting`. `v1.ai.hosting` builds it fresh from
experiment.hosting's answers, rather than transferring personal-agents' history.

## The existing setup: personal-agents

personal-agents (`github.com/JaimeStill/personal-agents`, local `~/code/personal-agents`) is a
marathon context project outside the workspace. It configures one host class, the
Framework desktop:

- **Hardware**: AMD Strix Halo, 128GB of unified memory, about 96GB of it exposed to the GPU.
- **Serving**: llama.cpp's router in Vulkan mode, run as a systemd service from
  spike-model-hosting's rendered presets and unit. It runs upstream build b11734 (at
  spike-model-hosting's screen task) and is held to a minimum build of b11452. It binds only to
  the tailnet, and Tailscale's device authentication is the access boundary on the dev
  workstations. Production has no tailnet ("Experiment: experiment.hosting", "Production").
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

The two GB10 workstations, `gb10-1` and `gb10-2`, are a second host class: Dell Pro Max GB10
units of the DGX Spark class. Like Strix Halo, they have unified memory and no discrete VRAM.

- **Hardware**: aarch64 with an sm_121 GPU, 20 cores, 3.6 TB of NVMe, and 128 GB of unified
  memory at 273 GB/s, of which about 121 GiB is visible and about 117 GiB available.
- **Stack**: DGX OS reports 7.4.0 but carries a newer stack: driver 580.178.04, CUDA 13.0,
  kernel 7.0, Docker 29.6.2, and nvidia-container-toolkit 1.20.1.
- **Interconnect**: a QSFP link over ConnectX-7 joins the two units, at about 185 Gb/s of RDMA
  across both port halves.
- **Memory reporting**: `nvidia-smi` reports no memory use, and `cudaMemGetInfo` counts the page
  cache as used.

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

### spike-model-hosting

spike-model-hosting (`~/experiments/spike-model-hosting`, remote
[JaimeStill/spike-model-hosting](https://github.com/JaimeStill/spike-model-hosting)) is the
Framework's llama.cpp baseline. Its tasks established:

- **align** moved the Framework's router to llama.cpp's upstream Vulkan release serving set A, and
  brought personal-agents and spike-harness-driver in line with it.
- **profile** defined the host-class profile schema in TOML, with a validator and a renderer, and
  the Strix Halo profile, which takes each model's sampling and reasoning settings from its card.
- **consumers** ran Pi, Claude Code, and go-ai's model client at once against the router, recorded
  the baseline later measurements compare with, and measured tool-call reliability per model and
  consumer.
- **main-model** added named model sets, swapped over the router's API, and disqualified Mistral
  Small 4 on Vulkan.
- **evaluation** built `evaluate`'s gated stages on Inspect AI and MTEB.
- **screen** added `evaluate`'s screen and the pool of admissible models, listed on paper.

Every consumer and instrument runs from the architect's laptop; the Framework only hosts and
serves, under the access rules in "Hosts and access" above. A sweep measures each model's slots
and context size per host. Any open model licence is accepted: Apache-2.0, MIT, OpenMDW-1.1, the
NVIDIA Open Model licences, the LFM Open License 1.0, the Llama 3.1 and 4 Community licences, and
the Gemma Terms of Use, under the model-origin constraint (`ai-strategy.md`).

The spike closes through its `answer` task, which takes no new measurement, and its answer lands
in "Answers · experiment.hosting" below. Its other questions moved:

- the engine (llama.cpp or vLLM) and hosting method (native or container) to spike-model-engines
- the model layout to spike-model-layout
- the gateway and access control to spike-model-gateway
- the admin CLI to experiment.ai's spike-host-admin (`ai-strategy.md`)
- benchmarking and model candidates to experiment.ai's spike-model-benchmarks (`ai-strategy.md`)

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
  - how to read a model card (spike-model-hosting's `docs/model-cards.md`); `ai-hosting`'s
    per-model presets cite the card values this method reads
  - the serving conventions: router mode, tailnet-only binding (the dev workstations' boundary), presets keyed by capability tier
    rather than by host, and the tool-call reliability convention (spike-model-hosting's
    `docs/findings/tool-reliability.md`)

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
  profile schema as its contract. experiment.ai's spike-host-admin builds the Go CLI, with
  outpost as its baseline.
- **Host-class differences.** How the tool handles them: GPU usage through `amdgpu_top` on Strix
  Halo, while the GB10s' GPU and memory use can't come from `nvidia-smi`; and Vulkan versus CUDA
  builds.

## Answers · experiment.hosting

## Assumptions

- One serving stack per host can serve all three consumers within its slot and memory budget.
- The preset convention (presets keyed by capability tier) carries over to the GB10s' unified
  memory and to vLLM.
- Production reaches the stack through an Azure Container Apps service that routes between Azure
  AI Foundry and the self-hosted stack, not through the tailnet.
