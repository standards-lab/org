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
- **Stack**: DGX OS 7.6.0, driver 580.178.04, CUDA 13.0, kernel 7.0.0-1025-nvidia, Docker
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
Framework baseline, then spike-model-engines, spike-hosting-upgrades, spike-model-layout, and
spike-model-gateway; its intake runs before capstone.data-intelligence (`data-intelligence.md`).
spike-host-admin moved to experiment.admin, after the capstone, which doesn't need the admin tool
(plan, 2026-11-09); its answer lands in "Answers · experiment.admin". The benchmarks spike, which spike-hosting-upgrades
replaced (architect, 2026-10-29), and the host-admin spike moved here from the client and processing experiment, so that this experiment owns the whole hosting layer (`ai-strategy.md`, "The
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
  Scripting stays minimal: only what must be idempotent (install, update, uninstall), and step
  helpers that a session sources and calls one step at a time in the architect's view, each
  command logged with its timestamp so the docs quote the log; a helper never runs a benchmark
  on its own (architect, 2026-11-06).
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
granted once, by the architect, as a sudoers rule naming that one command exactly, such as the
page-cache drop, granted on all three hosts (`/usr/sbin/sysctl vm.drop_caches=3` on the GB10s,
`/usr/bin/sysctl vm.drop_caches=3` on the Framework). Every session starts by bringing
its hosts up to date, and every measurement records the engine version, the host's stack, and the
laptop's network path to the host (`tailscale ping`): a DERP relay adds its round trip to TTFT
and request latency, but not to inter-token latency or the server's own counters.

**experiment.hosting runs on all three hosts.** gb10-2 was reserved for spike-ai-pipeline from
2026-10-26 until that spike completed (2026-11-09); it now serves
experiment.hosting's remaining spikes beside gb10-1 and the Framework, which spike-model-layout
needed to lay models out across every pool before spike-model-gateway (architect, 2026-11-09). Each
session brings all three current. spike-model-layout's `hosts` task moved gb10-2 onto
spike-model-engines' engines suite and shared preset, carrying the two vision keys below into that
preset; spike-ai-pipeline's repository stays untouched. After spike-model-gateway, gb10-2 goes to
capstone.data-intelligence's design measurements and build, and then to experiment.ocr (plan,
2026-11-09).

**gb10-2 serves vision alone** on vLLM v0.33.0 (`gemma-4-26B-A4B-NVFP4-vision-gb10`) from
spike-model-engines' engines suite, and no workstation runs llama.cpp (spike-model-layout
`docs/layout.md`). spike-ai-pipeline's `llama.env` and `host-update` are stale. spike-model-engines'
shared preset keeps two keys, for every host, in its section for Google's Gemma 4 26B-A4B QAT Q4_0
(google/gemma-4-26B-A4B-it-qat-q4_0-gguf, with its own projector), which set A names in place of
ggml-org's post-training Q4_0, since that fails the quantization floor:

- `ub = 1120`, since llama.cpp caps a non-causal image's tokens at n_ubatch (default 512) where
  the model allows 1120, at a cost of 800 MiB.
- `image-min-tokens = 560`, so a small image of dense text gets enough tokens to be read.

**Every host lets a container bind its Tailscale address before Tailscale is up.** A container
that publishes a port on the host's Tailscale address races Tailscale at boot: docker's start
fails ("cannot assign requested address") and leaves the container on no network, and a later
`docker start` runs it with no published port. All three hosts set `net.ipv4.ip_nonlocal_bind = 1`
persistently, so the bind succeeds first: gb10-1 and gb10-2 in
`/etc/sysctl.d/90-nonlocal-bind.conf`, the Framework in `60-tailnet-bind.conf`. After a reboot,
every vLLM row bound to the tailnet came back by itself (spike-model-layout
`docs/findings/vllm-layout.md`, I.1 and I.3). No workstation runs the llama container.
`docker network connect bridge <container>` fixes a container already stranded.

**llama.cpp's `/v1/messages` and `/v1/responses` rebuild a request as Chat Completions,** so each
drops what Chat Completions can't carry:

- At b11669, `/v1/messages` drops output_config, a tool's strict, and the none tool choice. It
  maps named and allowed_tools choices to required, yet gpt-oss-120b made no call under either
  and repeated its answer to the 1024-token bound. Its stream error frame carries no
  `"type":"error"`.
- At b11669, `/v1/responses` drops text, reasoning's summary, store, include, and an image's
  detail, and treats named and allowed_tools choices as auto. It refuses previous_response_id, a
  content part other than text, image, or file, and a reasoning item without content. Every reply
  reports completed, even at the token bound. It streams reasoning under OpenAI's
  `response.reasoning_text` names, with no sequence numbers or indexes, and its stream error frame
  is `{"error":{...}}` with no type.
- At b12098, `/v1/messages` ignores `thinking: disabled` and `reasoning_effort`; only
  `chat_template_kwargs.enable_thinking = false` stops the reasoning, so vision sent through
  Messages reasons.

**spike-model-engines' `host/` is owned by experiment.hosting** until its intake. Its spikes may
change the repository's `host/` (scripts and presets) through pull requests, each citing the
finding behind it, so the host tooling stays in step with the findings; every other reference
stays read-only. A spike that changes it lists spike-model-engines in its `repos`, so its lock
covers the change. After the intake, the de facto management repository for the workstations is
established in `~/architecture`, from spike-model-engines' `host/` as it then stands. The lean for
its shape: one repository that adapts to its host through an explicit host-class profile
(strix-halo, gb10), checked against what the host detects, with the per-platform parts kept to
those profiles (dotfiles' `platforms/<class>/` pattern). Rejected: extracting `host/` into a new
repository now, which the intake would only move again (architect, 2026-11-02).

**Dual-stack serving waits.** Publishing the router on tailnet IPv4 and IPv6
(`net.ipv6.ip_nonlocal_bind = 1` beside the IPv4 key, on the Framework and the GB10s) is taken up
once MagicDNS serves AAAA records for dual-stack nodes (tailscale#1152, open). Until then the hosts
keep their bare names tailnet-only by sending no hostname in DHCP (architect, 2026-11-02).

**vLLM is the preferred engine on the workstations,** chosen for concurrency first; its open
issues are addressed as the stack is used. The architect's preference (2026-11-12):

- Each vLLM instance serves one model. gb10-1 serves the agent, gb10-2 vision, and the Framework
  audio and embedding, and no purpose has a second host (spike-model-layout `docs/layout.md`, "The
  layout").
- Models aren't loaded on demand. GPU compute, not memory, is the bottleneck, and loading and
  unloading add compute on top of inference.
- Model management stays off the serving API. A client on a user device, such as Pi, that can
  change a workstation's model configuration through the serving API is a vulnerability.
- llama.cpp is removed from the workstations' layouts and kept in spike-model-engines.

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
- the admin CLI to spike-host-admin, now experiment.admin's
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
(dotfiles), and two-node serving is not pursued (spike-model-layout). IL6 is out of its scope
(architect, 2026-10-23). The spike has answered: "Answer ·
experiment.hosting.spike-model-engines", under "Answers · experiment.hosting" below.

### spike-hosting-upgrades

What convention evaluates an upgrade to the hosting stack (a host's hardware configuration, its
model host, or its models) from what others publish rather than benchmarks of our own? A
project-local Claude Code skill in the spike, whose plugin form the intake routes to
claude-plugins, gathers the current public benchmarks, engine releases, and model cards for each host class,
engine, and model, cites them, and writes a dated report of ranked upgrades; anchor cells A and B
verify each on our host before it is adopted. The spike screens the pool document's candidates
the same way. The large-scale suite waits in the backlog as `model-evaluation`, for the workloads
only we can measure: our agents, and our role sets under our own sessions. Decides the
upgrade-evaluation convention and its cadence. The spike has answered: "Answer ·
experiment.hosting.spike-hosting-upgrades", under "Answers · experiment.hosting" below.

### spike-model-layout

How are models placed across the three memory pools, and what context and how many concurrent
sessions does each placed model hold? It runs on gb10-1, gb10-2 and the Framework, and measures, in
capped GuideLLM runs: host parity across the two GB10s; memory per model per host class; context ×
sessions per placed model; set A on one GB10 against the roles split across both GB10s (one serving
the agent model alone, the other vision, audio and embedding, with requests directed by
capability); the Framework in the role that comparison points to; and the agent and the
capability models on vLLM, in containers, against llama.cpp. Its last task, `vllm-layout`, tuned
the layout the architect chose: vLLM instances, one model each, with gb10-1 serving the agent,
gb10-2 vision, and the Framework audio and embedding, the embedding rate-limited by the gateway.
On-demand loading and two-node serving over QSFP are not pursued:
GPU compute, not memory, is the bottleneck, and one model per instance across the GB10s replaces
pooling them (architect, 2026-11-12). Routing stays on paper, handed to spike-model-gateway as
requirements. Decides the layout v1.ai.hosting specifies. Its
evidence and path are in the spike's `context/README.md`. The spike has answered: "Answer ·
experiment.hosting.spike-model-layout", under "Answers · experiment.hosting" below.

### spike-model-gateway

Which gateway spreads requests across the pool and queues them at capacity, and how is access
restricted without the tailnet? Decides whether a gateway fronts the stack, which, and its access
control.

### spike-host-admin

experiment.admin's one spike. Can one CLI on go-cli-sdk manage every engine and host
experiment.hosting keeps: install, update, service status and restart, models, metrics, and GPU
and memory use? outpost is its baseline ("The existing setup: personal-agents"). It runs after
capstone.data-intelligence, managing what experiment.hosting settled. Decides `ai-hosting`'s admin tooling ("Open for v1.ai.hosting's SETTLE").

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
limits"). v1.ai.hosting and the goals that serve set A make the change. gb10-2 serves
granite-embedding-english-r2 BF16 from llama.cpp (spike-ai-pipeline, client-core).

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
  and hosting method per host class); the profile format is settled at experiment.admin's
  spike-host-admin's setup.
- 3, how each platform is set up and managed: spike-model-engines (scripted install and setup)
  and experiment.admin's spike-host-admin.
- 6, a capability matrix per platform: spike-model-engines (raw capability by model shape,
  embedding and ASR among them).
- 7, presets, context sizing, and the footprint method beyond the Framework:
  spike-model-layout (context × sessions per placed model) and spike-model-engines (vLLM).
- 8, each gateway approach: spike-model-gateway, budgets, rate limits, OpenTelemetry traces,
  memory and latency overhead, and one configuration on both host classes included.
- 9, one admin tool over every platform and host: experiment.admin's spike-host-admin.
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

### Answer · experiment.hosting.spike-hosting-upgrades

**Question:** What convention evaluates an upgrade to the hosting stack (a host's hardware
configuration, its model host, or its models) from what others publish rather than benchmarks
of our own?

**Answer:** A skill surveys each host weekly and on events, ranking every upgrade from cited
public sources against the host's load targets; the host measures only what no source settles,
and an adoption leaves a chain of dated records.

The answer is drawn from two hosts, framework (Strix Halo, llama.cpp Vulkan b11960) and gb10-1
(GB10, llama.cpp CUDA b12098). No figure is compared across sessions, and two builds are
compared only within one session.

1. The skill: one project-local Claude Code skill, `hosting-upgrades`, with three modes. `setup`
   prints the permission rules it needs and never writes settings. `survey <host>` probes the
   host read-only and without root and applies nothing. `measure <host>` runs local cells under
   one harness, the engine rule for an engine move and the keep rule for a candidate. The skill
   reads the host descriptors and the policy and cites every source with its URL and read time.
   Proven by its runs: the probes
   ([findings/skill.md](https://github.com/JaimeStill/spike-hosting-upgrades/blob/main/docs/findings/skill.md)),
   seven dated reports, and one measure record
   ([skill](https://github.com/JaimeStill/spike-hosting-upgrades/tree/main/.claude/skills/hosting-upgrades)).
2. Dated, cited reports of ranked upgrades per host, each item given verify, take, hold, or
   skip. Each covers four kinds: hardware configuration, system updates and debt, the engine,
   and the models, with a flag-by-flag configuration review and model discovery. The two second
   surveys of 2026-11-06 state what moved since the host's last report. Both researched in full,
   because the host stack had moved. gb10-1's cited 23 sources and 29 model entries from the
   cache and read only 14 fresh. Proven by the reports:
   [framework 2026-11-06](https://github.com/JaimeStill/spike-hosting-upgrades/blob/main/docs/reports/framework/2026-11-06.md)
   and
   [gb10-1 2026-11-06-2](https://github.com/JaimeStill/spike-hosting-upgrades/blob/main/docs/reports/gb10-1/2026-11-06-2.md),
   with the earlier reports beside them.
3. Each host's top upgrade, verified by anchors A and B before and after, then adopted or
   rolled back. framework's E4B QAT as the audio model was rolled back on ASR B (−5.0%).
   gb10-1's move from b11669 to b12063 was adopted on the architect's overrule of the engine
   rule (ASR B −11.07%), to stay current. Under the revised skill, framework's settings check
   confirmed set A's no-cache keys (20.1% less time, 19.2% faster long-reply decode), and the
   host now serves them. Proven by measured runs:
   [verify-framework](https://github.com/JaimeStill/spike-hosting-upgrades/blob/main/docs/findings/verify-framework.md)
   (b11960, 2026-11-02),
   [verify-gb10-1](https://github.com/JaimeStill/spike-hosting-upgrades/blob/main/docs/findings/verify-gb10-1.md)
   (b11669 and b12063, 2026-11-03), and
   [framework's measure](https://github.com/JaimeStill/spike-hosting-upgrades/blob/main/docs/measures/framework/2026-11-06.md)
   (b11960, 2026-11-06).
4. The pool screened per host class. The gates ran on every candidate. Gemma 4 26B-A4B QAT,
   serving agent and vision, was measured beside gpt-oss-120b and dropped on both hosts: on
   framework at the engine gate (b11960's Vulkan backend aborts with several slots), and on
   gb10-1 on BFCL multi-turn's floor. Each host keeps set A, and the other candidates are hold
   until measured. Proven by measured runs:
   [framework's screen](https://github.com/JaimeStill/spike-hosting-upgrades/blob/main/docs/screens/framework/2026-11-04.md)
   (b11960, 2026-11-04) and
   [gb10-1's screen](https://github.com/JaimeStill/spike-hosting-upgrades/blob/main/docs/screens/gb10-1/2026-11-05.md)
   (b12098, 2026-11-05).
5. The published claim beside our anchors, thin: three claims, all in the verify findings.
   spike-model-engines' +56.6% for E4B QAT held within a point (+55.4%), but it said nothing of
   the model's unprompted reasoning, which cost ASR B its lead. llama.cpp #31188's +17.2% MoE
   prefill held in direction only (TTFT −0.6% to −3.2%). #29948's +7.1% dense prefill didn't
   show (TTFT +5.2% and +23.3%). The screens set no published claim beside their cells. Sources
   predicted a gain's direction more often than its size. Proven by measured runs: the verify
   findings' "The claim beside the anchors" and "Cells".
6. The convention and its cadence, each statement citing its record. It states when survey
   runs (weekly per host, and on a host update, a register entry's fix reaching an image, a
   changed purpose or load target, and before any change to the host), what triggers a
   measure (a verify item or a "Needs measurement" entry, one measure a session, its cells
   agreed in the plan round), and the record an adoption leaves: the report,
   the measure record quoting its command log, then the descriptor, the pin, and the register
   entry's state. It also covers how proposals are settled between surveys, the blocker
   register's four states, the research cache's 14-day rule, per-host load targets, which
   spike-model-layout owns, and what stays with model-evaluation. Proven by the two second
   surveys and the records it cites:
   [docs/convention.md](https://github.com/JaimeStill/spike-hosting-upgrades/blob/main/docs/convention.md).

Gaps in the answer:

- No repeat survey has run on an unmoved stack. Both second surveys researched in full, because
  the host stack had moved, so "only what moved" is shown by the research cache's reuse, not by
  a run that skipped research. The weekly cadence was decided on 2026-11-06 and hasn't run yet.
- No embedding scenario in `bench/`, so no embedding candidate can be measured on either host.
- The project's own conversions aren't built: Nemotron 3.5 Transcribe to Q8_0 (no publisher or
  ggml-org GGUF), and granite-embedding-english-r2 from IBM's stored safetensors.
- The engines are held. framework is on b11960 and gb10-1 on b12098, behind the newest imaged
  build, b12169, because builds from b12100 don't resolve Google's QAT GGUFs (#31620, #31639,
  open in the register). #30137's fix is in b12169, and its move is skip until that clears.
- The vision section's prompt-cache keys (`cache-ram 0`, `cache-idle-slots false`,
  `ctx-checkpoints 2`) are still to verify on both hosts, the one verify item of each second
  survey.
- The measure mode has run once, as a settings check. No engine move or candidate screen has run
  under it, and no register entry has reached "cleared on the host".
- The published claims beside our anchors are three, from two sessions (evidence 5).

Requirements for the intake to route:

- spike-model-hosting's agent-tier harness (`evaluation/suites/agent_quality.py`): a `max_tokens`
  cap per tier, recorded with the tier's list, so a runaway reply is a counted cap hit, not a
  lost sample. BFCL sends none today. On gb10-1 one multi-turn sample generated 23,306 output
  tokens, and the tier hit its 45-minute cap at 19 of 20 samples (this goal's pending edit for
  the harness, and gb10-1's screen of 2026-11-05).
- spike-model-engines' `scripts/currency.sh`: name the newest imaged build from ghcr.io's tag
  list, as this spike's `fix-in-image.sh` does. Walking GitHub's releases list, it names b12124
  and can't see b12169, an image built with no release (framework's report of 2026-11-06,
  "Blockers").
- The skill's plugin form, routed to claude-plugins, with the convention for where its `docs/`
  initializes. The plugin targets the model-hosting repository that succeeds personal-agents,
  spike-model-engines' `host/` and spike-model-hosting (the workstations' conventions and
  script suite), so the host descriptors, the policy, the reports, the measure records, the
  blocker register and the research cache live beside the scripts and presets an adoption
  changes. Today the skill resolves every input and output against the project it runs in
  (`${CLAUDE_PROJECT_DIR}`), so the plugin must say which repository that is and how its
  `docs/` is initialized there (the architect, at the session brief, 2026-11-09).
- spike-model-layout: the descriptors' purposes and load targets follow its layout, and a change
  there changes them. It also weighs hosting the agent model alone on vLLM for concurrent
  sessions, this goal's other pending edit.
- model-evaluation: workloads only we can measure (our agents, and our role sets under our own
  sessions), and tuning at scale. That includes the candidate backlog, which one screen a session
  can't clear: framework's second survey lists over twenty agent candidates as needing
  measurement.

[The answer](https://github.com/JaimeStill/spike-hosting-upgrades#the-answer) ·
[spike-hosting-upgrades](https://github.com/JaimeStill/spike-hosting-upgrades)

### Answer · experiment.hosting.spike-model-layout

**Question:** How are models placed across the three workstations' memory pools to make the best
use of memory, and what context and how many concurrent sessions does each placed model hold?

**Answer:** Each model runs in its own vLLM v0.33.0 instance, one heavy purpose to a GPU: gb10-1
serves the agent, gb10-2 vision, and the Framework audio beside embedding rate-limited by the
gateway. Compute shared on one GPU, not memory, decides the placement.

The architect chose the layout, and the spike gives its evidence. Every figure is a LAN figure,
since production clients sit on the same direct network as the GPUs, and no figure is compared
across sessions.

| Host | Purpose and model | vLLM row, fraction | Sequences × context | Load target | Standing cell, all three hosts at once | Holds alone |
| --- | --- | --- | --- | --- | --- | --- |
| gb10-1 | agent: gpt-oss-120b, FP8 KV | `gpt-oss-120b-agent-fp8`, 0.80 | 12 × 131,072 | 4 sessions | ITL 48.13 ms | 12 sessions (ITL 99.07 ms) |
| gb10-2 | vision: Gemma 4 26B-A4B, NVIDIA's NVFP4 | `gemma-4-26B-A4B-NVFP4-vision-gb10`, 0.40 | 16 × 8,192 | 4 requests | 4.49 s | 16 requests (9.64 s) |
| framework | audio: Gemma 4 E4B, BF16 | `gemma-4-E4B-it-audio-framework`, 0.18 | 4 × 8,192 | 4 requests | RTFx 5.78 | 4, beside embedding at 34 req/s (RTFx 5.84) |
| framework | embedding: granite-embedding-english-r2 | `granite-embedding-english-r2-framework`, 0.03 | 128 × 8,192 | 16 streams, at most 34 req/s | 17,442 prompt tok/s | knee at 16 streams, 98,325 prompt tok/s |

The floors: agent ITL median ≤100 ms, vision request latency median ≤10 s, audio RTFx median ≥5,
embedding ≥4,300 prompt tok/s.

1. Purposes kept apart, not memory, decide the layout. Every miss the spike measured was
   purposes sharing one GPU's compute and bandwidth, with memory far above the floors. Set A on
   one GB10 took the agent to ITL 237.67 ms with 15.86 GiB free at its lowest. The capability
   host on llama.cpp missed vision (38.90 s) and audio (RTFx 0.98) with 68.69 GiB free, and two
   capability hosts held 0 of 4 vision and 0 of 4 audio streams at the targets. On vLLM, audio
   beside vision on gb10-2 missed at every pair level from 2 + 2 to 8 + 8 streams (RTFx 2.86 to
   3.36), with 35.53 GiB free in the pair cell, while each held alone. Proven by measured runs:
   [role-split](https://github.com/JaimeStill/spike-model-layout/blob/main/docs/findings/role-split.md)
   (llama.cpp b12098, 2026-11-10),
   [framework](https://github.com/JaimeStill/spike-model-layout/blob/main/docs/findings/framework.md)
   (b12098, 2026-11-10),
   [capability-vllm](https://github.com/JaimeStill/spike-model-layout/blob/main/docs/findings/capability-vllm.md)
   (vLLM v0.33.0, 2026-11-11) and
   [vllm-layout](https://github.com/JaimeStill/spike-model-layout/blob/main/docs/findings/vllm-layout.md),
   E and K (v0.33.0, 2026-11-12).
2. vLLM over llama.cpp, for concurrency. On gb10-1 the FP8 KV row met the agent's floor through
   12 sessions, the BF16 row through 8, and llama.cpp through 4. Each capability purpose alone
   held 16 vision and 16 audio requests on gb10-2 against llama.cpp's 1 and 1, with 4.4 times
   llama.cpp's embedding throughput at 16 streams (c). Proven by measured runs:
   [agent-vllm](https://github.com/JaimeStill/spike-model-layout/blob/main/docs/findings/agent-vllm.md),
   F, and capability-vllm (v0.33.0 against b12098, 2026-11-11).
3. The agent's FP8 KV row. lm-evaluation-harness 0.4.13 found no significant loss against BF16
   at 95%: GPQA diamond 0.6212 against 0.6667 (b 24, c 15, p 0.20); RULER's needle at 32,768
   tokens 0.80 against 0.88 (p 0.22) and at 65,536 0.92 against 0.90 (p 1). FP8 holds 1,733,910
   tokens of KV in the same bytes. Proven by measured runs: vllm-layout, C (2026-11-12).
4. NVFP4 vision. mmmu_val scored 0.6111 on NVIDIA's NVFP4 on gb10-2 against 0.6178 on Google's
   BF16 on the Framework (b 78, c 72, p 0.68), no significant loss; CUDA against ROCm is a limit.
   Proven by measured runs: vllm-layout, G (2026-11-12).
5. Audio beside embedding on the Framework. Audio alone reached RTFx 6.54. Beside embedding run
   flat out at 16 streams it fell to 2.44; beside embedding at a steady 8.4 and 34 requests a
   second it held 6.43 and 5.84; at 8 streams beside 34 requests a second it missed at 4.94, so
   its cap is 4. Embedding's knee is 16 streams at 98,325 prompt tok/s, about 23 times its floor
   (c). Proven by measured runs: vllm-layout, F (2026-11-12).
6. The standing layout at the load targets. All three hosts at once for 240 s met every floor:
   the agent at 4 sessions (ITL 48.13 ms), vision at 4 (4.49 s, 212 of 212 ≤10 s), audio at 4
   (RTFx 5.78), and embedding at 34 requests a second (17,442 prompt tok/s). At their lowest
   gb10-1 read 14.87 GiB and gb10-2 65.21 GiB (floor 12.2), and the Framework 90.24 GiB of GTT
   (11.2) and 87.27 GiB of system memory (12.5). After one reboot each, gb10-2 served vision in
   3 min 49 s and gb10-1 the agent in 8 min 58 s, unattended; the Framework served after the
   architect entered its LUKS passphrase. llama.cpp is removed from all three hosts. Proven by
   measured runs: vllm-layout, H, I and J (2026-11-12).
7. Host parity and the capacity table. gb10-1 and gb10-2 run anchors A and B within run noise of
   each other (A −1.37%, B −0.99% output tok/s), so cells compare across the GB10s. Each set A
   model's memory on a GB10 and on the Framework gives the capacity table the placements are
   sized from. Proven by measured runs:
   [hosts](https://github.com/JaimeStill/spike-model-layout/blob/main/docs/findings/hosts.md)
   and
   [footprint](https://github.com/JaimeStill/spike-model-layout/blob/main/docs/findings/footprint.md),
   I (b12098, 2026-11-09).

Not pursued:

- On-demand loading. GPU compute, not memory, is the bottleneck, and loading and unloading add
  compute on top of inference (architect, 2026-11-12).
- Two-node serving over QSFP with vLLM and Ray. One model per vLLM instance across the GB10s
  replaces pooling them; the agent holds 12 sessions of 131,072 tokens on gb10-1 alone
  (architect, 2026-11-12).
- Redundancy and failover. Each purpose has one host, by the architect's choice. The Framework's
  staged vision row (Gemma 4 12B QAT w4a16, never measured) and audio failover row were dropped
  when audio moved to the Framework (architect, 2026-11-12).
- CUDA MPS and other concurrency tools. Placement and vLLM's per-row limits keep the purposes
  apart.

Gaps in the answer:

- The agent's FP8 KV runs uncalibrated at scale 1.0. GPQA's 198 questions put the paired
  difference's 95% interval at −10.7 to +1.6 points (c), so a loss of up to about 10 points isn't
  excluded.
- Audio has no headroom above its target of 4. A fifth stream queues or overflows; 8 streams
  beside embedding at 8.4 requests a second is untested.
- The Framework returns from a reboot only after a person enters its LUKS passphrase at its
  console, and can't be rebooted remotely.
- The FP8 row's compiling start dipped under gb10-1's floor (11.39 GiB for 21 s); a start with the
  compile cache warm stayed above it.
- vLLM v0.34.0 was unreleased, so every host stays on v0.33.0, unmeasured on v0.34.
- Vision's load from streamed video is unmeasured: every vision cell sent image questions.
- Sessions of 131,072 tokens are computed from the KV cache, not run; the deepest measured were 8
  concurrent 65,603-token requests, held on both agent rows.

Requirements for the intake to route:

- spike-model-gateway: admission on the engines' live latency and queue signals per purpose
  floor, queueing past each row's cap, embedding held to at most 34 requests a second, overflow
  to cloud models, model management only through the gateway, no failover, and the Framework
  treated as down after a reboot until its console unlock
  ([docs/layout.md, "Requirements for spike-model-gateway"](https://github.com/JaimeStill/spike-model-layout/blob/main/docs/layout.md#requirements-for-spike-model-gateway)).
- spike-hosting-upgrades' host descriptors: each host's purposes and load targets from the
  layout, and a new descriptor for gb10-2
  ([docs/layout.md, "Requirements for the intake: the host descriptors"](https://github.com/JaimeStill/spike-model-layout/blob/main/docs/layout.md#requirements-for-the-intake-the-host-descriptors)).

[The answer](https://github.com/JaimeStill/spike-model-layout/blob/main/README.md#the-answer) ·
[spike-model-layout](https://github.com/JaimeStill/spike-model-layout)

### Intake notes · experiment.hosting

spike-model-layout's sync left these for experiment.hosting's intake. The first list concerns the
hosts and the coordinator's notes; the second concerns spike-model-engines, whose repository the
sync didn't change.

The hosts and the coordinator's notes:

- **gb10-1's descriptor.** spike-hosting-upgrades' descriptor for gb10-1 still says gb10-2 is
  reserved for spike-ai-pipeline.
- **The host descriptors.** Their purposes and load targets follow the layout (spike-model-layout
  `docs/layout.md`, "Requirements for the intake: the host descriptors"). gb10-1 serves the agent
  alone on `gpt-oss-120b-agent-fp8` (4 sessions, 12 at most), with 15.07 GiB idle and 14.87
  GiB at its lowest against its 12.2 GiB floor; its compiling start dips to 11.39 GiB.
  gb10-2 has no descriptor in spike-hosting-upgrades and needs one: vision alone (4 requests, 16
  at most), with the hosts task's within-session spread on gb10-2 (output tok/s: A 2.37%, B
  0.52%) as its first run-noise figure. The Framework serves audio (4 requests, 4 at most) and
  embedding (16 streams, admitted at up to 34 requests a second).
- **The Framework's GTT and memory floors.** The GTT ceiling was already 112 GiB
  (ttm.pages_limit 29,360,128, set by spike-model-engines' setup). vllm-layout removed the
  deprecated amdgpu gttsize option, which the kernel warned about, and kept ttm.pages_limit alone.
  The BIOS VRAM carve-out is 512 MiB. The system's 12.5 GiB floor binds before GTT's
  (capability-vllm, at 0.81 of GTT). The Framework stays on Omarchy with Hyprland by the
  architect's choice, after weighing headless Ubuntu Server.
- **The Framework's LUKS unlock.** The Framework's root is LUKS-encrypted, so it returns from a
  reboot only after a person enters the passphrase at its console, and it can't be rebooted
  remotely or unattended as the GB10s can. How it unlocks is the architect's security decision,
  on either OS: TPM2 enrollment or a keyfile (no person), or SSH in the initramfs (remote)
  (spike-model-layout `docs/findings/vllm-layout.md`, I.4 and N.3).
- **The Framework's disk after the GGUFs' removal.** The Framework's df didn't drop when
  llama.cpp's GGUFs (81.87 GiB of store) were removed. Its btrfs snapshots likely hold the blocks
  until they rotate out, and reading them needs root. The architect left the snapshots as they
  are (spike-model-layout `docs/findings/vllm-layout.md`, H and N.5).
- **The hosts' model stores.** Each host's store holds only the models its rows serve: gb10-1
  `openai/gpt-oss-120b`, gb10-2 `nvidia/Gemma-4-26B-A4B-NVFP4`, and the Framework
  `google/gemma-4-E4B-it` and `ibm-granite/granite-embedding-english-r2`. Earlier tasks cleared
  the GB10s' and the Framework's extras with `hf cache rm`, and vllm-layout removed set A's GGUFs
  and the BF16 26B-A4B (spike-model-layout `docs/findings/framework.md`, A;
  `docs/findings/vllm-layout.md`, G.4 and H).
- **vLLM v0.34.** v0.34.0 was not released when vllm-layout's BUILD started (GitHub's latest was
  v0.33.0, with tags v0.34.0rc0 to rc2 only), so every host stays on v0.33.0. v0.34 carries the
  GB10 weight-loading fix (#58868), which bears on gb10-1's 437 s and gb10-2's 128 s weight reads
  after a reboot, and the extensible KV cache (spike-model-layout `docs/findings/vllm-layout.md`,
  A.2 and N.6).
- **The agent's FP8 KV.** The agent's FP8 KV runs uncalibrated at scale 1.0. The accuracy check
  found no significant loss against BF16, but GPQA diamond's 198 questions put the paired
  difference's 95% interval at −10.7 to +1.6 points, so a loss of up to about 10 points isn't
  excluded. Calibration or a larger set would narrow it (spike-model-layout
  `docs/findings/vllm-layout.md`, C.5 and N.1).
- **Vision from streamed video.** Vision's load from streamed video is unmeasured: every vision
  cell sent image questions (the architect raised it).
- **The vision section's prompt-cache keys.** spike-hosting-upgrades' gap leaves these keys to
  verify. The shared preset caps the host prompt cache in the 26B-A4B QAT and E4B sections
  (`cache-ram 0`, `cache-idle-slots false`, `ctx-checkpoints 2`; role-split's pull request,
  21b5e8c). Uncapped, the cache took gb10-1 under its floor under vision load
  (spike-model-layout `docs/findings/footprint.md`, G and K). No workstation runs llama.cpp now,
  so the verify item has no host to run on.
- **Benchmarks over the network.** Production clients sit on the same direct network as the
  GPUs, so the layout is sized on LAN figures (the architect). A cell run from the laptop over a
  Tailscale relay carries the relay's trip in its client-side figures (TTFT, request latency,
  RTFx), about 0.4 s for a 6 s audio clip; its server-side figures are unaffected
  (spike-model-layout `docs/findings/role-split.md`, "Stack, engine and network path" and L.7).
  The `tailscale ping` each run records shows which path it took. This bears on the rules'
  "Benchmarks".
- **The agent's engine.** For the agent, the architect accepts a longer cold start in exchange for
  better concurrency and overall performance, and finds vLLM the more attractive agent engine,
  chiefly for its concurrency (spike-model-layout `docs/findings/agent-vllm.md`, the conclusion
  and N).
- **spike-model-layout's bench environment.** `mise.bench.toml` pins uv 0.13.7, behind 0.13.8,
  left to the next currency step. Its `bench/levels.jq` `client` mode stops with a jq error on an
  embedding result, whose requests carry no first-chunk time.
- **spike-ai-pipeline's `host-update`.** The laptop-side `mise run host-update` fails at its clone
  pull once the clone is gone; it touches nothing first.

spike-model-engines:

- **The vLLM guides.** The guides in `docs/engines/vllm/` (README.md, setup.md, models.md,
  api.md, update.md, uninstall.md; outside `host/`) still describe v0.32.0 and one `vllm`
  container on port 8000. They don't cover a row's own container, port and image
  (capability-vllm's `host/` change, 4ad76e6), `VLLM_ROCM_BUILD` and the `-rocm72` fallback,
  building the Framework's audio image, or the Framework's ufw route rules for ports 8001 and
  8002. They list none of the standing rows or their caps: `gpt-oss-120b-agent-fp8` (12) and
  `gpt-oss-120b-agent` (8) on gb10-1, `gemma-4-26B-A4B-NVFP4-vision-gb10` (16),
  `gemma-4-E4B-it-audio-framework` (4) and `granite-embedding-english-r2-framework` (128, 65,536
  batched tokens). vllm-layout removed gb10-2's audio and embedding rows and the Framework's
  vision rows (spike-model-layout `docs/findings/agent-vllm.md`, N;
  `docs/findings/capability-vllm.md`, A.2, A.3 and N; `docs/findings/vllm-layout.md`, B).
- **The llama.cpp guides.** The guides in `docs/engines/llama.cpp/` (outside `host/`) describe
  llama.cpp serving the workstations, and no workstation runs it: vllm-layout removed its
  container, images and GGUFs from all three hosts, and spike-model-engines keeps every llama.cpp
  feature. They also name strix-halo's pin as b11960 (README.md:14, update.md:35, setup.md:303
  and :591, and context/README.md "Held pins"). The GB10 setup guide ("GB10, in a container")
  says gb10-2 is reserved and puts the clone in the home directory, not ~/repos, and `models.md`
  lists neither role-split's three sections nor the Gemma cap keys (spike-model-layout
  `docs/findings/vllm-layout.md`, H).
- **measure-lib's `gate` on Vulkan.** On a Vulkan host, `gate` on a clone carrying role-split's
  sections would load the 16-slot 26B-A4B section, which aborts on Vulkan past 4 slots.
- **granite's cache keys.** granite's llama.cpp section sets no cache keys, so every embedding
  request saves its idle slot to the prompt cache (177,657 saves in one 2.5-min sweep on gb10-2).
  `cache-idle-slots = false` and `cache-ram = 0` would stop it (spike-model-layout
  `docs/findings/role-split.md`).
- **The 16-slot section's Vulkan comment.** The preset's
  `gemma-4-26B-A4B-it-qat-Q4_0-16-slots` comment (host/llama-preset/models.ini:217, "GB10 only:
  on Vulkan (the Framework) the section aborts past 4 slots") is broader than measured. The aborts
  were at 16 x 131,072 (#29082) and 15 x 131,072 (#29221), while 4 x 8,192 runs; 5-14 slots and
  16 x 8,192 never ran on Vulkan. #29221's report crashes at -np 4 with 262,144-token slots, and
  the E4B past 4 slots never ran on Vulkan either. The framework task left both untested, since
  the fix (#25584) is in no build and more slots add no vision throughput (spike-model-layout
  `docs/findings/framework.md`, K.4).
- **The audio image's torchcodec.** The Framework's audio image (`host/`) lacks torchcodec, so
  every audio request logs `torchcodec unavailable ... falling back to soundfile/PyAV`; the image
  keeps that fallback (spike-model-layout `docs/findings/vllm-layout.md`, L).
- **`vllm_remove_images`.** In `host/`, `engines vllm update` removes every official image no
  container was created from, so the Framework's update for granite removed the staged
  `vllm/vllm-openai-rocm:v0.33.0-rocm72` fallback's tag. Its layers stay under the `-rocm72` audio
  image, and `docker pull` restores it (spike-model-layout `docs/findings/vllm-layout.md`, B).

## Answers · experiment.admin

## Assumptions

- One serving stack per host can serve all three consumers within its slot and memory budget.
- The preset convention (presets keyed by capability tier) carries over to the GB10s' unified
  memory and to vLLM.
- Production reaches the stack through an Azure Container Apps service that routes between Azure
  AI Foundry and the self-hosted stack, not through the tailnet.
