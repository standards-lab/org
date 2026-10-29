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

- **Hardware**: AMD Strix Halo, 128GB of unified memory, 112 GiB of it reachable by the GPU as
  GTT (spike-model-engines, llama-strix-rocm).
- **Serving**: llama.cpp's router in ggml's Vulkan image, run as spike-model-engines' `llama`
  container at b11960 ([setup, "12. Serve set A"](https://github.com/JaimeStill/spike-model-engines/blob/main/docs/engines/llama.cpp/setup.md#12-serve-set-a)).
  It binds only to the tailnet, and Tailscale's device authentication is the access boundary on
  the dev workstations; production has no tailnet ("Experiment: experiment.hosting",
  "Production"). spike-model-hosting's systemd router, `llama-router`, was retired on 2026-10-29.
- **Models**: set A, on the 112 GiB pool: gpt-oss-120b MXFP4, on four slots sharing a unified KV
  cache at 131072 context; Google's Gemma 4 26B-A4B QAT Q4_0, with vision; gemma-4-E4B Q8_0, with
  audio; and granite-embedding-english-r2 BF16, for embeddings. Loaded, the four hold 92.1 GiB of
  GTT.
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
- **Stack**: DGX OS 7.6.0, driver 580.178.04, CUDA 13.0, kernel 7.0.0-1019-nvidia, Docker
  29.6.2, and nvidia-container-toolkit 1.20.1.
- **Interconnect**: a QSFP link over ConnectX-7 joins the two units, at about 185 Gb/s of RDMA
  across both port halves.
- **Memory reporting**: `nvidia-smi` reports no memory use, and `cudaMemGetInfo` counts the page
  cache as used. On the integrated GPU, vLLM reads the OS's available memory instead, which counts
  the page cache as free: with 18 GiB cached and 99.7 GiB MemFree, v0.32.0 reported 113.42 of
  121.63 GiB free at startup.
- **CDI spec**: an apt upgrade without a reboot can leave the container toolkit's CDI spec
  (`/var/run/cdi/nvidia.yaml`) naming a moved driver library, and GPU containers then fail to
  start until `sudo nvidia-ctk cdi generate --output=/var/run/cdi/nvidia.yaml` regenerates it.

## Experiment: experiment.hosting

**Question.** How do the workstations (the Framework's Strix Halo and the two GB10s) serve local
models: which engine and hosting method on each, how a model's compatibility and configuration
are settled, how the models are laid out across their memory pools, which gateway routes requests
across the pool and restricts access to it, and what tool administers them? Its intake
decides what `v1.ai.hosting` builds. The spikes run one at a time: spike-model-hosting's
Framework baseline, then spike-model-engines, spike-hosting-upgrades, spike-model-layout,
spike-model-gateway, and spike-host-admin. The benchmarks spike, which spike-hosting-upgrades
replaced (architect, 2026-10-29), and the host-admin spike moved here from experiment.ai, so that this experiment owns the whole hosting layer (`ai-strategy.md`, "The
four layers"; architect, 2026-10-26). Their answers land in "Answers · experiment.hosting" below.

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
- Sessions are guided where the architect is learning: a new tool's or concept's first walk,
  every root step, and the last slice, a walkthrough the architect runs from the docs (architect,
  2026-10-23 and 2026-10-26). Routine host steps that need no root (downloads and image pulls,
  engine install, update, and uninstall, model loads, readiness waits, cold-start timing, log and
  memory read-outs, and health and version checks) the session runs itself over Tailscale SSH and
  records. Every command a session or the architect runs appears in the docs exactly as run.
  Scripting stays minimal: only what must be idempotent (install, update, uninstall).
- A benchmark is built and run a step at a time from the commands the brief agreed, by the
  session in the architect's view or by the architect, never scripted out of the architect's
  sight.
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
root stays the architect's, run from exact commands. A root command a session must repeat may be
granted once, by the architect, as a sudoers rule naming that one command exactly, such as
gb10-1's page-cache drop (`/usr/sbin/sysctl vm.drop_caches=3`). Every session starts by bringing
its hosts up to date, and every measurement records the engine version, the host's stack, and the
laptop's network path to the host (`tailscale ping`): a DERP relay adds its round trip to TTFT
and request latency, but not to inter-token latency or the server's own counters.

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
- upgrade evaluation and model candidates to spike-hosting-upgrades

### spike-model-engines

spike-model-engines (`~/experiments/spike-model-engines`, remote
[JaimeStill/spike-model-engines](https://github.com/JaimeStill/spike-model-engines)): which
engine, llama.cpp or vLLM, serves each workstation, and is it hosted in a container or on bare
metal? Measures raw capability by model shape (vision included), quantization, and concurrency,
with no model sets, and documents each engine as a guide to setup, models, APIs, updates, and
uninstall, and whether it loads models on demand. The GB10s go first, containers before bare
metal; the Framework follows with llama.cpp's ROCm and Vulkan builds and vLLM's official ROCm
image (vllm/vllm-openai-rocm; AMD's rocm/vllm images are deprecated). Load runs use GuideLLM, and
llama-bench measures llama.cpp's backends. The QSFP link was measured during the GB10s' setup
(dotfiles), and two-node serving stays with spike-model-layout. IL6 is out of its scope
(architect, 2026-10-23). The spike has answered: "Answer ·
experiment.hosting.spike-model-engines", under "Answers · experiment.hosting" below.

### spike-hosting-upgrades

What convention evaluates an upgrade to the hosting stack (a host's hardware configuration, its
model host, or its models) from what others publish rather than benchmarks of our own? An agent
skill gathers the current public benchmarks, engine releases, and model cards for each host class,
engine, and model, cites them, and writes a dated report of ranked upgrades; anchor cells A and B
verify each on our host before it is adopted. The spike screens the pool document's candidates
the same way. The large-scale suite waits in the backlog as `model-evaluation`, for the workloads
only we can measure: our agents, and our role sets under our own sessions. Decides the
upgrade-evaluation convention and its cadence.

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

**Set A's embedding model.** It should no longer be EmbeddingGemma 2 (architect, 2026-10-26).
vLLM v0.32.0 didn't serve it on the GB10 (SM121): its default Triton attention needs more shared
memory than SM121 has, and vLLM issue #60847 reports FLASH_ATTN's head size unsupported on GB10
and FLASHINFER without encoder-only attention; the fixes #60631, #60848, and #60942 are open and
unmerged. llama.cpp serves it. granite-embedding-english-r2 (IBM, Apache-2.0, BF16) serves on both
engines (spike-model-engines, `docs/findings/bench-gb10-container.md`, "Embeddings" and "Engine
limits"). v1.ai.hosting and the goals that serve
set A make the change.

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
  and experiment.hosting's spike-host-admin.
- 6, a capability matrix per platform: spike-model-engines (raw capability by model shape,
  embedding and ASR among them).
- 7, presets, context sizing, and the footprint method beyond the Framework:
  spike-model-layout (context × sessions per placed model) and spike-model-engines (vLLM).
- 8, each gateway approach: spike-model-gateway, budgets, rate limits, OpenTelemetry traces,
  memory and latency overhead, and one configuration on both host classes included.
- 9, one admin tool over every platform and host: experiment.hosting's spike-host-admin.
- 10, IL6 on paper: spike-model-gateway; the engines' IL6 check was dropped from
  spike-model-engines (architect, 2026-10-23).
- The model candidates and the upgrade-evaluation convention: experiment.hosting's
  spike-hosting-upgrades.

[The answer](https://github.com/JaimeStill/spike-model-hosting#the-answer) ·
[spike-model-hosting](https://github.com/JaimeStill/spike-model-hosting)

### Answer · experiment.hosting.spike-model-engines

**Question:** Which engine, llama.cpp or vLLM, serves each workstation (the GB10s, then the
Framework's Strix Halo), and is it hosted in a container or on bare metal?

**Answer:** llama.cpp in a container on both host classes, in ggml's CUDA image on the GB10s and
in ggml's Vulkan image on the Framework; vLLM's leads are kept as evidence, and slots are a
per-scenario tuning setting, not a limit.

The answer is drawn from two host classes, so it is stated as rules keyed by a host's traits,
with the GB10s and the Framework as worked instances. No figure is compared across sessions or
versions.

1. On NVIDIA, llama.cpp on CUDA. On gb10-1 llama.cpp led gpt-oss-120b at 1 stream (50.0
   against 34.0 output tok/s) and the Gemma 4 shapes at 4 streams; vLLM led gpt-oss-120b from
   4 streams (168 against 129 at 16), kept as evidence, since one engine serves a host and a
   split host wasn't measured. Proven by measured runs: `docs/findings/bench-gb10-container.md`
   (llama.cpp b11669, vLLM v0.32.0, 2026-10-26).
2. On AMD, llama.cpp on Vulkan. On the Framework ggml's Vulkan image led its ROCm image at both
   anchors (by 6.39% and 5.21%) and on every llama-bench row; ROCm's sooner cold loads and its
   vision leads are kept as evidence (`docs/findings/llama-strix-rocm.md`, b11669,
   2026-10-27). vLLM's ROCm image served 22.4% and 20.6% of llama.cpp's output at anchors A and
   B (`docs/findings/vllm-strix.md`, vLLM v0.32.0, llama.cpp b11960, 2026-10-28). Proven by
   measured runs.
3. Unified memory: llama.cpp takes the model's buffers, and vLLM reserves 0.80 of the pool
   whatever the model, so one llama.cpp router holds several models; set A's four hold 92.1 of
   the Framework's 112 GiB GTT ceiling. Proven by measured runs:
   `docs/findings/bench-gb10-container.md` (2026-10-26), `docs/findings/vllm-strix.md`
   (2026-10-28), and `docs/findings/answer.md` (b11960, 2026-10-29).
4. Streams: the measured curves are the evidence, and the answer sets no slot limit.
   gpt-oss-120b on 16 slots served 48.5, 77.1, 129.0, and 126.2 output tok/s at 1, 4, 16, and
   32 streams on gb10-1 (b11669, 2026-10-26), and 43.6, 69.5, 81.6, and 80.1 on the Framework
   over a DERP relay (b11960, 2026-10-29); the two curves ran in different sessions on
   different builds and are not compared. The Framework sweep gives gpt-oss-120b an initial
   setting of 4 streams, where set A starts. Proven by measured runs:
   `docs/findings/bench-gb10-container.md` and `docs/findings/answer.md`.
5. Embeddings at concurrency: vLLM's lead is kept as evidence. On granite-embedding-english-r2
   at 16 streams vLLM served 295.1 requests/s against llama.cpp's 55.5 on gb10-1 (2026-10-26)
   and 215.3 against 40.5 on the Framework (2026-10-28). Proven by measured runs:
   `docs/findings/bench-gb10-container.md` and `docs/findings/vllm-strix.md`.
6. Models on demand: llama.cpp's router loads and unloads models through its API in seconds,
   and vLLM serves one model a start, in minutes; on gb10-1 gpt-oss-120b loaded in 48 s against
   vLLM's 577 s cold start. Proven by measured runs: `docs/findings/bench-gb10-container.md`
   (2026-10-26) and `docs/findings/vllm-strix.md` (2026-10-28).
7. A container on both host classes. On gb10-1 llama.cpp ran at the same speed, startup, and
   memory in ggml's container and natively, and vLLM at parity, its container starting 137 s
   sooner. Proven by measured runs: `docs/findings/llama-gb10-native.md` (b11669) and
   `docs/findings/vllm-gb10-native.md` (v0.32.0), 2026-10-27. The Framework's bare metal is
   unproven: ggml's native Vulkan build led the image at prompt processing by 4.49 to 9.49%,
   likely from the host's newer Mesa, untested (`docs/findings/llama-strix-rocm.md`,
   b11669, 2026-10-27).
8. The Framework serves set A from the `llama` container in ggml's Vulkan image at b11960, and
   spike-model-hosting's router is retired. Proven by a running demo: the walkthrough's
   retirement and Pi's tool calls on set A (`docs/findings/answer.md`, 2026-10-29).

Gaps in the answer: the Framework's API probes, not measured; the Framework's bare metal, not
measured as a server; and vLLM's slow NVFP4 decode on SM121, cause open.

Requirements for the intake to route:

- Workload-based tuning: slots and context tuned per model, workload, and host, owned by the
  backlog goal model-evaluation and by spike-model-layout; not measured.
- Request priority: llama-server queues past its slots first come, first served, with no
  limit, priority, or 429, while vLLM has a priority scheduler; not measured.
- Model management separated from inference and held by an admin: llama-server's router
  exposes load, unload, download, and delete to any client that reaches it, `?autoload=true`
  overrides `--no-models-autoload`, and every API key has the same access, by the source,
  untested. Today the tailnet, ufw, and `tag:inference` bound it.

[The answer](https://github.com/JaimeStill/spike-model-engines#the-answer) ·
[spike-model-engines](https://github.com/JaimeStill/spike-model-engines)

## Assumptions

- One serving stack per host can serve all three consumers within its slot and memory budget.
- The preset convention (presets keyed by capability tier) carries over to the GB10s' unified
  memory and to vLLM.
- Production reaches the stack through an Azure Container Apps service that routes between Azure
  AI Foundry and the self-hosted stack, not through the tailnet.
