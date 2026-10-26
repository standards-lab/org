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
models: which engine and hosting method on each, how a model's compatibility and configuration
are settled, how the models are laid out across their memory pools, which gateway routes requests
across the pool and restricts access to it, and what tool administers them? Its intake
decides what `v1.ai.hosting` builds. The spikes run one at a time: spike-model-hosting's
Framework baseline, then spike-model-engines, spike-model-benchmarks, spike-model-layout,
spike-model-gateway, and spike-host-admin. The benchmarks and host-admin spikes moved here from
experiment.ai, so that this experiment owns the whole hosting layer (`ai-strategy.md`, "The four
layers"; architect, 2026-10-26). Their answers land in "Answers · experiment.hosting" below.

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
- Sessions are guided, slower, and methodical, so the architect learns each tool by running it
  (architect, 2026-10-23). A slice's docs and scripts are written on the branch, then the session
  walks the architect through the host steps one at a time from those docs and records each
  output; the last slice is the architect running the whole procedure from the docs. Scripting
  stays minimal: only what must be idempotent (install, update, uninstall).
- A benchmark is built and run a step at a time, typed by the architect from the commands the
  brief agreed, never scripted out of the architect's sight.
- Engines stay current. Only cells run in the same session on the same versions are compared; a
  task that compares with an earlier one first reruns one anchor cell per engine, and an anchor
  that moved beyond run noise is a finding.
- The quantization floor: a format is accepted when its publisher evaluated it (native MXFP4, a
  QAT Q4_0, or an NVFP4 or FP8 checkpoint whose card evals are within about 1% of BF16).
  Otherwise nothing goes below Q5_K_M or FP8, and embedding, ASR, and dense models under 8B run at
  Q8_0 or FP8 or better.

**Hosts and access.** The Framework (`framework`) and the two GB10s (`gb10-1`, `gb10-2`) are on the
tailnet under `tag:inference`, reached by Tailscale SSH, which stays in accept mode while the
experiments run. Sessions do on any of them whatever needs no sudo, the docker group included;
root stays the architect's, run from exact commands. Every session starts by bringing its hosts up
to date, and every measurement records the engine version and the host's stack.

**gb10-2 is reserved** for experiment.ai's spike-ai-pipeline from 2026-10-26 (`ai-strategy.md`),
so the two spikes can run at once without sharing a host. experiment.hosting runs on gb10-1 and
the Framework, and gb10-2 is out of its host set and its every-session upgrade; the pipeline
spike's sessions keep gb10-2 current. A hosting task that needs gb10-2, such as two-node serving
over QSFP, names the borrow in its task brief. If a pipeline task is in progress, the architect
and the session agree when to borrow it so the running task isn't disrupted, and the borrowing
session restores gb10-2's pipeline engine afterward.

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

The spike has answered, with no new measurement in its `answer` task: "Answer ·
experiment.hosting.spike-model-hosting", under "Answers · experiment.hosting" below. Its other
questions moved:

- the engine (llama.cpp or vLLM) and hosting method (native or container) to spike-model-engines
- the model layout to spike-model-layout
- the gateway and access control to spike-model-gateway
- the admin CLI to spike-host-admin
- benchmarking and model candidates to spike-model-benchmarks

### spike-model-engines

spike-model-engines (`~/experiments/spike-model-engines`, remote
[JaimeStill/spike-model-engines](https://github.com/JaimeStill/spike-model-engines)): which
engine, llama.cpp or vLLM, serves each workstation, and is it hosted in a container or on bare
metal? Measures raw capability by model shape (vision included), quantization, and concurrency,
with no model sets, and documents each engine as a guide to setup, models, APIs, updates, and
uninstall. The GB10s go first, containers before bare metal; the Framework follows with
llama.cpp's ROCm and Vulkan builds and vLLM's ROCm image. Load runs use GuideLLM, and llama-bench
measures llama.cpp's backends. The QSFP link was measured during the GB10s' setup (dotfiles), and
two-node serving stays with spike-model-layout. IL6 is out of its scope (architect, 2026-10-23).
Decides the engine and hosting method per host class.

### spike-model-benchmarks

What is the smallest benchmark that quickly settles a candidate model's compatibility with a host
and engine and its initial configuration: context, slots, and the settings its card names? It
runs after spike-model-engines and screens the pool document's candidates, and a host keeps only
the models that pass on it. The configuration is a starting point: requirements drive any tuning
beyond it. The large-scale suite waits in the backlog as `model-evaluation`: composable
benchmark elements, results by model, build, preset, and host, a run queue per host, and a
management interface. Decides the benchmark method.

### spike-model-layout

How are models placed across the three memory pools, and what context and how many concurrent
sessions does each placed model hold? Decides the layout.

### spike-model-gateway

Which gateway spreads requests across the pool and queues them at capacity, and how is access
restricted without the tailnet? Decides whether a gateway fronts the stack, which, and its access
control.

### spike-host-admin

Can one CLI on go-cli-sdk manage every engine and host experiment.hosting keeps: install,
update, service status and restart, models, metrics, and GPU and memory use? outpost is its
baseline ("The existing setup: personal-agents"). It runs last, managing what the other spikes
settled. Decides `ai-hosting`'s admin tooling ("Open for v1.ai.hosting's SETTLE").

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
  profile schema as its contract. spike-host-admin builds the Go CLI, with
  outpost as its baseline.
- **Host-class differences.** How the tool handles them: GPU usage through `amdgpu_top` on Strix
  Halo, while the GB10s' GPU and memory use can't come from `nvidia-smi`; and Vulkan versus CUDA
  builds.

## Answers · experiment.hosting

### Answer · experiment.hosting.spike-model-hosting

**Question:** What does llama.cpp's Vulkan router on the Framework establish for the
self-hosted stack: a host-class profile, Pi, Claude Code, and go-ai's model client at once,
tool-call reliability, a memory budget on unified memory, and a measurement method?

**Answer:** llama.cpp's Vulkan router on the Framework serves set A from one rendered
host-class profile to Pi, Claude Code, and go-ai's model client at once, with no answer crossed
and the pool's floor held; with gpt-oss-120b only Pi meets the tool-call bar, and the
measurement method stands on set A's two baseline runs, its screen proven by tests only.

1. A host-class profile: one TOML profile holds set A alone (gpt-oss-120b, Gemma 4 26B-A4B,
   gemma-4-E4B, EmbeddingGemma 2) and validates and renders the router's presets and systemd
   unit for it, each model at its card's sampling and reasoning settings, naming no host.
   Proven by the `profile` package's tests and by a running demo: the committed render's
   SHA-256 (`96732ef9…`) equals the installed presets', and the probe passes, on b11734,
   2026-10-23.
2. Three consumers at once: Pi, Claude Code, and the model client on gpt-oss-120b's 4 slots,
   beside an embedding stream, sent 40 chat requests and 1032 embeddings in 51.7 s with no
   answer crossed; all three harnesses (Pi, Claude Code, OpenCode) send the reasoning back on
   tool turns. Proven by a running demo, `evaluate concurrent`, and the `wire` package's
   round-trip integration tests, on b11572, 2026-10-12; set A's role models under each other's
   load crossed nothing in 18 and 22 rounds, on b11715, 2026-10-17 and 2026-10-18.
3. Tool-call reliability: with gpt-oss-120b, Pi on `tool_choice` `"required"` meets OpenAI's
   bar (pass^5 9 of 9, 0 invalid); Claude Code answers in text where a call is due and makes up
   the skill's motto, and OpenCode on `auto` gets the parser's peg-native error in its stream.
   The layered convention (server, profile, client, harness, gateway, measurement) is recorded
   with its sources. Proven by running demos: `evaluate conform -k 5` on b11572, 2026-10-12,
   and harness fit in both baseline agent runs on b11715, 2026-10-17 and 2026-10-18 (Pi 1.3.0,
   Claude Code 2.1.301, OpenCode 1.19.3).
4. Speed: gpt-oss-120b generates 48.8 tok/s alone and 21.2 per stream at four; the Gemmas
   can't hold four 8k agentic requests at c 32768; a shared 8192-token prefix cuts TTFT from
   4-11 s to under 0.55 s; a router restart loads all four in about 21 s, warm or cold. Proven
   by running demos (GuideLLM 0.8.1, `evaluate prefix`, `evaluate coldload`) on b11572,
   2026-10-12; the deep stages reran the grid and prefix reuse on b11715, 2026-10-17 and
   2026-10-18, gpt-oss-120b's short cells within 3%, and the move to b11715 read 20.8 and
   21.0 s cold, 2026-10-17.
5. A memory budget on unified memory: set A leaves 3.30 GiB of the 96 GiB pool free with 4 in
   flight on every model (b11565, 2026-10-12); each model keeps its high-water mark until
   reloaded, so the floor (2816 MiB) is judged after a reload at the warmed peak, 3068 MiB free
   (b11734, 2026-10-19); a cached set swap takes about 23 s (b11669, 2026-10-15). Proven by
   running demos: `footprint.sh`, evaluate's reload and warmed-peak reading, and `swap`.
6. A measurement method: `evaluate` gates admission, fitness, capacity, and quality on
   absolute floors, and set A's two full runs set the bars (b11715, 2026-10-17 and
   2026-10-18); the sweep found set A supports 2 concurrent agent sessions (b11734,
   2026-10-20), not confirmed by a set stage. Proven by running demos; the screen, full run,
   and results index by tests only. On paper, the pool counts 47 models, 34 admissible
   (b11734, 2026-10-19); Mistral Small 4 is disqualified on Vulkan (3.36 tok/s, CPU fallback
   until llama.cpp's open PR #30105; b11669, 2026-10-15 and 2026-10-16).

The original evidence's other items moved:

- 2, one schema rendering every platform on both host classes: spike-model-engines (engine
  and hosting method per host class); the profile format is settled at experiment.ai's
  spike-host-admin's setup.
- 3, how each platform is set up and managed: spike-model-engines (scripted install and setup)
  and experiment.ai's spike-host-admin.
- 6, a capability matrix per platform: spike-model-engines (raw capability by model shape,
  embedding and ASR among them).
- 7, presets, context sizing, and the footprint method beyond the Framework:
  spike-model-layout (context × sessions per placed model) and spike-model-engines (vLLM).
- 8, each gateway approach: spike-model-gateway, budgets, rate limits, OpenTelemetry traces,
  memory and latency overhead, and one configuration on both host classes included.
- 9, one admin tool over every platform and host: experiment.ai's spike-host-admin.
- 10, IL6 on paper: spike-model-gateway; the engines' IL6 check was dropped from
  spike-model-engines (architect, 2026-10-23).
- The model candidates and the benchmark method: experiment.ai's spike-model-benchmarks.

[The answer](https://github.com/JaimeStill/spike-model-hosting#the-answer) ·
[spike-model-hosting](https://github.com/JaimeStill/spike-model-hosting)

## Assumptions

- One serving stack per host can serve all three consumers within its slot and memory budget.
- The preset convention (presets keyed by capability tier) carries over to the GB10s' unified
  memory and to vLLM.
- Production reaches the stack through an Azure Container Apps service that routes between Azure
  AI Foundry and the self-hosted stack, not through the tailnet.
