# AI hosting

The hosting layer of the AI capability (`ai-strategy.md`) runs the machines that serve local
models to the organization's consumers: Pi sessions, Claude Code subagents, go-ai, and the
capstone's services. The hosts are the Framework desktop and the two GB10 workstations. The
layer's home is the workspace member repository `ai-hosting`, which
v1.ai.hosting builds from experiment.hosting's five spikes; its tasks are in `roadmap.toml`. This
note holds the stack those spikes settled and what `ai-hosting` must specify.

## The hosts

- **The Framework** (`framework`): AMD Strix Halo with 128 GB of unified memory, 112 GiB of it
  reachable by the GPU as GTT.
- **gb10-1 and gb10-2**: Dell Pro Max GB10 units of the DGX Spark class, each aarch64 with an
  sm_121 GPU, 20 cores, 3.6 TB of NVMe, and 128 GB of unified memory at 273 GB/s, of which about
  121 GiB is visible. A QSFP link over ConnectX-7 joins them at about 185 Gb/s of RDMA; no model
  is served across it.

Three host behaviours shape the setup scripts:

- **Memory reporting on the GB10s.** `nvidia-smi` reports no memory use, and `cudaMemGetInfo`
  counts the page cache as used. vLLM reads the OS's available memory instead, which counts the
  page cache as free.
- **The CDI spec on the GB10s.** An apt upgrade without a reboot can leave the container
  toolkit's CDI spec (`/var/run/cdi/nvidia.yaml`) naming a moved driver library, and GPU
  containers then fail to start until `sudo nvidia-ctk cdi generate
  --output=/var/run/cdi/nvidia.yaml` regenerates it.
- **Binding the Tailscale address at boot.** A container that publishes a port on the host's
  Tailscale address races Tailscale at boot: docker's start fails ("cannot assign requested
  address"), leaving the container on no network, and a later `docker start` runs it with no
  published port. All three hosts set `net.ipv4.ip_nonlocal_bind = 1` persistently, so the bind
  succeeds first: the GB10s in `/etc/sysctl.d/90-nonlocal-bind.conf`, the Framework in
  `60-tailnet-bind.conf`. `docker network connect bridge <container>` fixes a container already
  stranded.

## The engine: vLLM

Every host serves its models with vLLM, one model to each container. A container serves its model
under its role's alias (`agent`, `vision`, `audio`, or `embedding`), and a request that names the
model itself gets a 404. The spikes call each container a row.

- **Concurrency decides the engine.** On gb10-1 the agent's FP8 KV row met its latency floor
  through 12 sessions, against llama.cpp's 4; on gb10-2 vision and audio each held 16 requests
  against llama.cpp's 1, and embedding reached 4.4 times llama.cpp's throughput at 16 streams.
- **Models aren't loaded on demand.** GPU compute, not memory, is the bottleneck, and loading and
  unloading add compute on top of inference.
- **Model management stays off the serving API.** A client on a user device that can change a
  host's model configuration through the serving API is a vulnerability, so the gateway routes
  only the serving paths.

No workstation runs llama.cpp; vLLM supersedes spike-model-engines' llama.cpp answer ("The
spikes' answers").

## The layout

Each role has one host, and nothing fails over between hosts; overflow to Azure is the only
fallback. Purposes sharing one GPU's compute, not memory, decided the placement: every miss
spike-model-layout measured was two purposes on one GPU, with memory far above the floors.

| Host | Role and model | Row, GPU memory fraction | Sequences × context | Load target | Holds alone |
| --- | --- | --- | --- | --- | --- |
| gb10-1 | agent: openai/gpt-oss-120b, FP8 KV | `gpt-oss-120b-agent-fp8`, 0.80 | 12 × 131,072 | 4 sessions | 12 sessions |
| gb10-2 | vision: nvidia/Gemma-4-26B-A4B-NVFP4 | `gemma-4-26B-A4B-NVFP4-vision-gb10`, 0.40 | 16 × 8,192 | 4 requests | 16 requests |
| framework | audio: google/gemma-4-E4B-it, BF16 | `gemma-4-E4B-it-audio-framework`, 0.18 | 4 × 8,192 | 4 requests | 4, beside embedding at 34 req/s |
| framework | embedding: ibm-granite/granite-embedding-english-r2 | `granite-embedding-english-r2-framework`, 0.03 | 128 × 8,192 | 16 streams, at most 34 req/s | knee at 16 streams |

The floors are an agent inter-token latency median of at most 100 ms, a vision request latency
median of at most 10 s, an audio real-time factor (RTFx) median of at least 5, and embedding at
4,300 prompt tok/s or more. With all three hosts loaded at once to their targets for 240 s, every
row met its floor on vLLM 0.33.0. The hosts now run vLLM 0.34.0, on which the layout is
unmeasured. The GB10s come back from a reboot unattended; the Framework comes back only after a
person enters its LUKS passphrase at its console.

## The gateway

agentgateway v1.7.0 fronts the rows from one compose stack on gb10-2, beside vision's row, with
Postgres holding budgets and the request log. Each alias has its own base URL, `/<alias>/v1`,
whose `/models` lists that alias alone. The gateway:

- issues the consumers' keys, answering a missing, unknown, or revoked key with 401;
- routes only the eight serving paths, so no vLLM management route is open to a consumer key;
- gives each key a priority, a request-rate bucket, and a token budget per UTC clock hour;
- holds embedding to 34 requests a second with one global bucket, answering the excess with 429
  and `Retry-After: 1`;
- adds nothing beyond run noise to agent, vision, or audio, and at most 3.4 ms (median) to an
  embedding request.

Past a row's concurrency cap vLLM queues the request; past its queue limit, twice the cap, the
row answers 503, and the gateway evicts it. go-ai v0.3.0's and infer's integration tiers pass
through the gateway on all four aliases, and go-ai retries the gateway's 429 and 503, honouring
`Retry-After`. Cerebro (bulk work) and Athena (chat) each get a key and a budget. The gateway's
requirements are spike-model-gateway's
[docs/gateway.md](https://github.com/JaimeStill/spike-model-gateway/blob/main/docs/gateway.md);
the points its design decides are comments on v1.ai.hosting's `gateway` task.

## Overflow to Azure AI Foundry

When a row is down or refuses, the gateway sends its alias's requests to an Azure AI Foundry
deployment, marks the reply `x-gateway-overflow: azure`, and returns to the row once it is back.
spike-model-gateway proved the route with each row down. Nothing overflows under IL6.

All four aliases overflow. v1.ai.hosting's `gateway` task builds the routes, the audio and
embedding routes for the first time:

- **agent** goes to `gpt-oss-120b`, processed within the data zone.
- **vision** goes to `gpt-5-mini`, processed globally.
- **audio** goes to `gpt-audio-mini` on Chat Completions, processed globally. It has two known
  refusals: a text-only chat, and audio inside a Responses request, which the gateway refuses.
- **embedding** goes to the `embedding` task's model on Foundry managed compute, enabled only
  once the Framework's and Azure's vectors for the same input reach cosine ≥ 0.999.

The Foundry account keeps all four of its deployments. `text-embedding-3-small` stays deployed
but is never routed, because its vectors are in another space.

## Set A's embedding model

Set A embeds with granite-embedding-english-r2 (IBM, Apache-2.0, 768 dimensions), chosen because
vLLM couldn't serve EmbeddingGemma 2 on the GB10's SM121. It is English-only, and ~/docs holds
text in Bulgarian, Greek, Polish, Arabic, Hebrew, Japanese, Russian, and Chinese, and audio in nine
languages.

v1.ai.hosting's `embedding` task is planned to replace it with a multilingual model that both the
Framework's vLLM and Foundry managed compute serve, trying multilingual-e5-large (MIT, on an
XLM-R base) first. The model must hold retrieval across ~/docs' languages, fit docinfer's chunks
in 512 tokens, and leave the Framework's audio row at its floor. A Chinese-origin candidate runs
only if multilingual-e5-large fails, with the reason recorded (`ai-strategy.md`, "Constraint:
model origin"). Anything indexed with granite is re-embedded.

## The audio role keeps Gemma 4 E4B and transcribes over chat

google/gemma-4-E4B-it serves more than speech to text, so it stays the audio model. vLLM serves
`/v1/audio/transcriptions` only for a model that implements its `SupportsTranscription`
interface, and `Gemma4ForConditionalGeneration` doesn't. The workstations therefore transcribe
over chat: go-ai's `TranscribeChat` sends the clip as input audio in a Chat Completions request.
The reasoning efforts each row refuses are declared in go-ai's format profiles (go-ai's
`docs/concepts.md`, "Profiles").

## Production

Production has no tailnet; the tailnet fences only the dev workstations. A containerized service
on Azure Container Apps will route inference between Azure AI Foundry and the self-hosted stack
by request and availability. The stack takes requests, spreads them across the pool, and queues
them when capacity is full, and its access control can't depend on the tailnet. Making the
gateway the only network path to the rows is production's concern, for the networking and
platform teams.

## What `ai-hosting` holds

`ai-hosting` states the settings the architect's own hosts use, generally enough that another
host of the same class follows it:

- the host-class profiles, from spike-model-hosting's `profile` package;
- each host class's vLLM rows, with install, update, and uninstall, from spike-model-engines'
  `host/`;
- the gateway's compose stack, from spike-model-gateway;
- a runbook for each host class;
- the upgrade-evaluation convention and its skill, rebuilt on vLLM;
- the admin tooling, after experiment.admin's intake.

It runs as one command suite, `hosting <group> <verb>`, symlinked into `~/.local/bin` on each
host. A command that differs by host class detects the class once and calls its own private
function for that class. v1.ai.hosting builds the read-only operator commands (a row's status,
its served model, its metrics, and GPU memory), and ai-hosting is the only hosting repository on
the workstations (v1.ai.hosting's plan, 2026-11-19).

The state of a particular host belongs nowhere in the workspace: once set up, a host needs little
attention, and there is no instance repository, so the specification and instance split of
`claude-plugins/context/tool-based-skills.md` has no instance half here.

Four method pages are planned for the architecture layer, written by v1.ai.hosting's
`architecture` task: reading a model card, tool-call reliability, sizing a vLLM row, and the
placement rule.

personal-agents ([JaimeStill/personal-agents](https://github.com/JaimeStill/personal-agents)), the
architect's earlier llama.cpp setup on the Framework with the `outpost` admin dispatcher, is
archived at v1.ai.hosting's sync, its README linking forward to `ai-hosting`.

`ai-hosting` is public in the standards-lab organization. Tailnet identifiers (the MagicDNS
suffix and the addresses) stay out of it, and its docs name hosts by their short name.

## The spikes' answers

- **spike-model-hosting** ([answer](https://github.com/JaimeStill/spike-model-hosting#the-answer)):
  one rendered host-class profile served set A to Pi, Claude Code, and go-ai's model client at
  once with no answer crossed, and with gpt-oss-120b only Pi, on `tool_choice` `"required"`,
  met the tool-call bar. Its `docs/model-cards.md` and `docs/findings/tool-reliability.md` feed
  the method pages.
- **spike-model-engines** ([answer](https://github.com/JaimeStill/spike-model-engines#the-answer)):
  answered llama.cpp in a container on both host classes. vLLM supersedes the engine; the
  container stands, since on gb10-1 both engines ran at native speed in their containers.
- **spike-hosting-upgrades**
  ([answer](https://github.com/JaimeStill/spike-hosting-upgrades#the-answer),
  [convention](https://github.com/JaimeStill/spike-hosting-upgrades/blob/main/docs/convention.md)):
  a skill surveys each host weekly and on events, ranking every upgrade from cited public
  sources against the host's load targets; the host measures only what no source settles, and
  an adoption leaves a chain of dated records. It was measured on llama.cpp hosts only.
- **spike-model-layout**
  ([answer](https://github.com/JaimeStill/spike-model-layout/blob/main/README.md#the-answer)):
  the layout above.
- **spike-model-gateway** ([answer](https://github.com/JaimeStill/spike-model-gateway#the-answer)):
  agentgateway, as above.

## Hosts and access

The Framework (`framework`) and the two GB10s (`gb10-1`, `gb10-2`) are on the tailnet under
`tag:inference`, reached by Tailscale SSH, which stays in accept mode while the experiments run.
gb10-2 also carries capstone.data-intelligence's design measurements and build, then
experiment.ocr. Sessions do on any host whatever needs no sudo, the docker group included; root
stays the architect's, run from exact commands. The architect may grant a root command a session
must repeat as a sudoers rule naming that one command exactly, such as the page-cache drop,
granted on all three hosts (`/usr/sbin/sysctl vm.drop_caches=3` on the GB10s,
`/usr/bin/sysctl vm.drop_caches=3` on the Framework). Every session starts by bringing its hosts
up to date. Every measurement records the engine version, the host's stack, and the laptop's
network path to the host (`tailscale ping`): a DERP relay adds its round trip to TTFT and request
latency, but not to inter-token latency or the server's own counters.

The hosts keep their bare names tailnet-only by sending no hostname in DHCP. Serving on tailnet
IPv6 as well (`net.ipv6.ip_nonlocal_bind = 1`) waits until MagicDNS serves AAAA records for
dual-stack nodes (tailscale#1152).

## Experiment rules

Every hosting spike keeps to these rules, experiment.admin's included:

- A pinned standard tool is adopted before anything is written. No Python is authored, and mise
  tasks wrap plain commands. Install and setup may be scripted, as named, idempotent steps in a
  clean, scalable layout (the pattern: `dotfiles/platforms/dgx/system-setup.sh`).
- A task introduces at most one new tool, and its brief and walkthrough say what the tool is, how
  it is set up, and what it provides. A task has at most four slices; its last is a walkthrough
  the architect runs before the next task is planned.
- Every benchmark is agreed with the architect in the task's plan round: the tool, how it runs
  (configuration, load, duration), and the decision it informs. A run takes at most 10 minutes
  unless the brief decides otherwise, sized to how critical the question is. BUILD runs no
  benchmark the brief doesn't name; a new or widened one is an escalation.
- Benchmarking looks for a solid starting point, not perfection: the configuration is tuned as
  the stack is used.
- Sessions are guided where the architect is learning: a new tool's or concept's first walk,
  every root step, and the last slice's walkthrough. Routine host steps that need no root
  (downloads and image pulls, engine install, update, and uninstall, model loads, readiness
  waits, cold-start timing, log and memory read-outs, and health and version checks) the session
  runs itself over Tailscale SSH and records. Every command a session or the architect runs
  appears in the docs exactly as run. Scripting stays minimal: only what must be idempotent
  (install, update, uninstall), and step helpers that a session sources and calls one step at a
  time in the architect's view, each command logged with its timestamp so the docs quote the
  log; a helper never runs a benchmark on its own.
- A benchmark is built and run a step at a time from the commands the brief agreed, by the
  session in the architect's view or by the architect, never scripted out of the architect's
  sight.
- Engines stay current. Only cells run in the same session on the same versions are compared; a
  task that compares with an earlier one first reruns one anchor cell per engine, and an anchor
  that moved beyond run noise is a finding.
- The quantization floor: a format is accepted when its publisher evaluated it (native MXFP4, a
  QAT Q4_0, or an NVFP4 or FP8 checkpoint whose card evals are within about 1% of BF16).
  Otherwise nothing goes below Q5_K_M or FP8, and embedding, ASR, and dense models under 8B run
  at Q8_0 or FP8 or better.

## Experiment: spike-host-admin

experiment.admin's one spike. Can one CLI on go-cli-sdk manage the vLLM rows on the three hosts:
install, update, service status and restart, models, metrics, and GPU and memory use?
personal-agents' `outpost` is its baseline. It runs after capstone.data-intelligence and decides
`ai-hosting`'s admin tooling ("Open for experiment.admin").

## Open for experiment.admin

**The admin tooling:**

- **Name.** The suite's root command is `hosting` (v1.ai.hosting's plan, 2026-11-19); `outpost`
  named where the admin sits, not what the tool does.
- **Form.** Either keep the bash dispatcher, or write a Go CLI on go-cli-sdk, layered as
  specification, tooling, and skill (`claude-plugins/context/tool-based-skills.md`), with the
  profile schema as its contract. spike-host-admin builds the Go CLI, with outpost as its
  baseline.
- **Host-class differences.** How the tool reads GPU use: through `amdgpu_top` on Strix Halo,
  while the GB10s' GPU and memory use can't come from `nvidia-smi`.

## Answers · experiment.admin

## Assumptions

- Production reaches the stack through an Azure Container Apps service that routes between Azure
  AI Foundry and the self-hosted stack, not through the tailnet.
