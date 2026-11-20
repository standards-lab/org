# AI hosting

The hosting layer of the AI capability (`ai-strategy.md`) runs the machines that serve local
models to the organization's consumers: Pi sessions, Claude Code subagents, go-ai, and the
capstone's services. The hosts are the Framework desktop and the two GB10 workstations. The
layer's home is the workspace member repository `ai-hosting`, which
v1.ai.hosting builds from experiment.hosting's five spikes; its tasks are in `roadmap/v1.ai.hosting.toml`. This
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
the points its design decides are under "The `gateway` task" below.

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
model origin"). Anything indexed with granite is re-embedded, docinfer's embeddings
(v1.ai.inference) among them.

~/docs' report lists "embeddings and search across languages" as a purpose. Azure's Hugging Face
registry (Foundry managed compute) offers multilingual-e5-large (MIT, XLM-R base, GPU SKU), and
the Chinese-origin Qwen3-Embedding-0.6B, bge-m3, and gte-multilingual-base; it lacks
granite-embedding-278m/107m-multilingual, embeddinggemma-300m, and
multilingual-e5-large-instruct. No benchmark scenario covers embedding in spike-hosting-upgrades'
`bench/`, and spike-model-layout's `bench/levels.jq` `client` mode stops with a jq error on an
embedding result, whose requests carry no first-chunk time.

## The audio role keeps Gemma 4 E4B and transcribes over chat

google/gemma-4-E4B-it serves more than speech to text, so it stays the audio model. vLLM serves
`/v1/audio/transcriptions` only for a model that implements its `SupportsTranscription`
interface, and `Gemma4ForConditionalGeneration` doesn't. The workstations therefore transcribe
over chat: go-ai's `TranscribeChat` sends the clip as input audio in a Chat Completions request.
The reasoning efforts each row refuses are declared in go-ai's format profiles (go-ai's
`docs/concepts.md`, "Profiles").

docinfer transcribes through the transcription route, go-ai's default `TranscribeRoute`, which
the workstations' audio endpoint (vLLM 0.34.0 serving google/gemma-4-E4B-it as `audio` at
framework:8001) doesn't serve. Against the workstations, docinfer sets go-ai's
`Options.TranscriptionMode` to `TranscribeChat`, through Chat Completions; a chat-mode reply that
isn't completed returns `*ai.IncompleteTranscriptError`.

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

## Inputs for v1.ai.hosting's tasks

What experiment.hosting's spikes and intake left for each task, beyond the sections above.

### The `hosts` task

- **A stranded container.** Beyond the nonlocal bind ("The hosts"), the update script can detect
  a running container with no published port, or no network, and print the fix, `docker network
  connect bridge <container>`, instead of waiting out its health check.
- **The Framework's LUKS unlock.** Its root is LUKS-encrypted, so it returns from a reboot only
  after a person enters the passphrase at its console, and it can't be rebooted remotely or
  unattended as the GB10s can. How it unlocks is the architect's security decision: TPM2
  enrollment or a keyfile (no person), or SSH in the initramfs (remote) (spike-model-layout
  `docs/findings/vllm-layout.md`, I.4 and N.3).
- **The Framework's GTT and memory floors.** The GTT ceiling is 112 GiB (`ttm.pages_limit`
  29,360,128, set by spike-model-engines' setup); vllm-layout removed the deprecated amdgpu
  `gttsize` option and kept `ttm.pages_limit` alone. The BIOS VRAM carve-out is 512 MiB. The
  system's 12.5 GiB floor binds before GTT's (capability-vllm, at 0.81 of GTT). The Framework
  stays on Omarchy with Hyprland by the architect's choice, after weighing headless Ubuntu
  Server.
- **Model stores.** Each host's store holds only the models its rows serve: gb10-1
  `openai/gpt-oss-120b`, gb10-2 `nvidia/Gemma-4-26B-A4B-NVFP4`, and the Framework
  `google/gemma-4-E4B-it` and `ibm-granite/granite-embedding-english-r2` (spike-model-layout
  `docs/findings/framework.md`, A; `docs/findings/vllm-layout.md`, G.4 and H).
- **The vLLM guides.** spike-model-engines' guides (`docs/engines/vllm/`) describe v0.32.0 and one
  `vllm` container on port 8000. They don't cover a row's own container, port, and image
  (4ad76e6), `VLLM_ROCM_BUILD` and the `-rocm72` fallback, building the Framework's audio image,
  or the Framework's ufw route rules for ports 8001 and 8002, and they list none of the standing
  rows or their caps: `gpt-oss-120b-agent-fp8` (12) and `gpt-oss-120b-agent` (8) on gb10-1,
  `gemma-4-26B-A4B-NVFP4-vision-gb10` (16), `gemma-4-E4B-it-audio-framework` (4), and
  `granite-embedding-english-r2-framework` (128, 65,536 batched tokens). `api.md`'s examples
  send the Hugging Face repository as `"model"`, while each standing row serves only its alias;
  `models.md`, "The rows", lists the four rows with their queue limits and scheduling policy
  (#19, 683b78e).
- **torchcodec.** The Framework's audio image lacks torchcodec, so every audio request logs
  `torchcodec unavailable ... falling back to soundfile/PyAV`; the image keeps that fallback
  (spike-model-layout `docs/findings/vllm-layout.md`, L).
- **The `-rocm72` fallback's tag.** `engines vllm update` removes every official image no
  container was created from, so the Framework's update for granite removed the staged
  `vllm/vllm-openai-rocm:v0.33.0-rocm72` fallback's tag; its layers stay under the `-rocm72`
  audio image, and `docker pull` restores it (spike-model-layout `docs/findings/vllm-layout.md`,
  B).
- **Per-row tracing.** `VLLM_TRACE=on VLLM_MODEL=<row id> host/engines vllm update` adds
  `--otlp-traces-endpoint grpc://gb10-2:4317` (`VLLM_TRACE_COLLECTOR`), `--add-host
  gb10-2:<tailnet address>` resolved at run time, and `OTEL_SERVICE_NAME=vllm-<alias>`; the same
  update without `VLLM_TRACE` turns it off (spike-model-engines #20). The update guide doesn't
  document it. `vllm update` compares only the image and the command, so a changed collector
  address or alias in `--add-host` or `OTEL_SERVICE_NAME` doesn't restart a traced row.
- **The laptop-side update.** spike-ai-pipeline's `mise run host-update` fails at its clone pull
  once the clone is gone; it touches nothing first.

### The `gateway` task

From spike-model-gateway (its answer: spike-model-gateway's README, "The answer"; the
requirements: spike-model-gateway `docs/gateway.md`):

- **Observability.** Metrics and logs go through the same Collector into the full LGTM stack (the
  architect), on go-web-service's observability profile (its Collector, contrib 0.165.0, with
  Loki, Tempo, Mimir, and Grafana). One central Collector takes every service's telemetry:
  - traces pushed over OTLP, as agentgateway and the four rows do in the spike;
  - metrics by its `prometheus` receiver scraping agentgateway's stats port (15020) and each vLLM
    row's `/metrics` (queue depth, KV cache, latency histograms), exported to Mimir;
  - container logs by Docker's `fluentd` logging driver on each service (dual logging keeps
    `docker logs`), sent across hosts to the Collector's `fluentforward` receiver (24224), with a
    `transform` processor mapping each container's tag to its `service.name`, so Grafana's
    trace-to-logs link searches the name Tempo uses. A per-host `filelog` agent is the
    alternative.

  Joining the gateway's 503s and evictions to a row's queue and KV cache is the evidence the
  `/metrics` admission gap needs. Unverified: whether agentgateway v1.7.0's access log carries the
  trace id (if so, go-web-service's `json_parser` trace operators apply), and whether vLLM's
  histograms carry exemplars; vLLM's plain-text logs carry no trace id.
- **Route `ai`.** The gateway runs route `ai` backends, the form agentgateway's docs recommend for
  path-based LLM routing. The UI labels them "legacy", and since 1.7 the Kubernetes model form is
  called the recommended one (#4307); agentgateway has no deprecation policy (#2767, closed as
  premature), and it deprecated `binds` without parity (#3204). The spike's watch register
  (`docs/watch.md`) tracks route `ai`, and the `llm` form, trialled in task `routing` where it
  passed, is the fallback (`docs/gateway.md`, "Priority's gaps").
- **Embedding overflow.** Every Azure-answered reply carries an overflow marker
  (`x-gateway-overflow: azure`). The spike gave embedding no Azure group, since
  text-embedding-3-small's replies come from another vector space; it stays deployed and
  unrouted. A client that stores vectors refuses or re-routes a marked reply from a model whose
  vectors don't match the row's (`docs/gateway.md`, requirements 8 and 9); acceptance's
  vector-parity cell gates embedding's overflow.
- **Embedding past its limit.** spike-model-layout's `docs/layout.md` requirement 3 says traffic
  past embedding's 34 req/s limit "queues or overflows"; agentgateway answers it with 429 and
  `Retry-After: 1` instead, and the client waits (`docs/gateway.md`, requirement 6).
- **Priority's gaps.** vLLM's queue limit counts every key alike, so a bulk flood also refuses
  interactive requests, which then overflow. Under saturation a row's 503 with eviction sends
  nearly the whole load to Azure, and agentgateway's eviction multiplier has no cap, so after a
  long outage or overload a row can stay out for minutes after it is back (`docs/gateway.md`,
  "Priority's gaps"). Priority orders only vLLM's waiting queue: interactive vision's median
  latency stayed 0.47 s over its 10 s floor on a saturated row, so holding it under bulk load
  needs more, such as a cap on bulk's concurrency.
- **Admission on `/metrics`.** agentgateway reads no row's `/metrics` and queues nothing itself,
  and no candidate does it natively (`docs/comparison.md`, "Scoring").
- **A sliding budget window** is wanted: v1.7.0's `rolling: 1h` is a fixed UTC clock hour.
- **Authenticated OTLP ingress and sampling below 100%** are production requirements. The spike's
  Collector takes spans on gb10-2's tailnet address unauthenticated, and the gateway and rows
  sample every request (`docs/findings/answer.md`, "For production"; `docs/gateway.md`,
  requirement 10).
- **`/models` through the gateway** is the gateway's own answer,
  `{"id":"<alias>","object":"model","owned_by":"gateway"}`, so go-ai v0.3.0's `Model.Native` and
  `infer models`' root column carry no served model, while the row's own listing gives `root`
  (`openai/gpt-oss-120b` on gb10-1). Whether to pass `root` through or keep it hidden is the
  design's call; a consumer reading `Native` through the gateway gets no `root`
  (`docs/findings/answer.md`, "At v0.3.0"; `docs/gateway.md`, requirement 1).
- **Path hardening.** agentgateway doesn't normalize dot segments (`docs/findings/routing.md`,
  "Realignment on route `ai`").
- **Smaller limits.** A body over 64 KiB isn't retried, each attempt to a powered-down host costs
  about 11 s, the UI's Logs and Analytics are reached only by URL on route `ai`, and the gateway's
  `Retry-After` reads as rounded down, so a client allows one attempt more.
- **A measured cold start**, a candidate for the restart and capacity notes in place of an
  estimate. vLLM's startup traces give each row's cold start by phase, read from its `Overall
  Loading` trace: weights' load, model preparation, KV cache allocation, and warmup. The agent's
  46.2 s is led by its 17.8 s weights' load, audio's 50.0 s by its 20.5 s warmup; vision 31.3 s,
  embedding 13.0 s, one restart each with the weights cached (`docs/findings/answer.md`, "The
  rows' startup traces and cold start").
- **An agent Messages stream that overflows to Azure streams.** Through the rows-down overlay
  (agentgateway v1.7.0, Foundry `gpt-oss-120b`), three sends counting to 300 at `max_tokens` 1024
  each gave 61 text deltas in bursts spread over the reply, never all at the end: the first at
  0.57–0.66 s, and the last with `message_stop` at 1.12–1.36 s. The translated stream's
  `message_start` reports 0 input tokens, because agentgateway builds it from the first chunk,
  before any usage; only `message_delta` carries the real count, so a consumer must read input
  usage there. Azure's `azureai-fe-is-streaming` header doesn't show streaming: it reads `False`
  on a request streamed directly to Azure too. The cause of an earlier stall at `--max-tokens 16`
  is open; it didn't recur in three reruns (`docs/findings/priority.md`, the addendum on the
  16-token Messages stall).
- **The compose stack's build.** The stack bind-mounts its config read-only, an exception to
  `~/architecture`'s convention, to weigh for the capstone's stack. `gateway/compose.sh` also
  exports `BUILDX_NO_DEFAULT_ATTESTATIONS=1`: Compose v5.2.0 ignores `provenance: false`, and on
  Docker's containerd image store each build's new attestation changes the image id, so compose
  recreates the built service on every `up --build`. The convention may want the same
  (`docs/gateway.md`, "Operational notes for the intake").
- **Audio overflow.** gpt-audio-mini on Chat Completions answered the priority walkthrough's clip
  with 200, "The access code is 7429." (`docs/findings/priority.md`, the walkthrough). It
  processes globally, as vision's overflow does.

What the gateway owes Cerebro is in `data-intelligence.md`, "What it asks of the goals beneath
it".

### The `upgrades` task

From spike-hosting-upgrades and spike-model-layout:

- **The host descriptors** follow the layout (spike-model-layout `docs/layout.md`, "Requirements
  for the intake: the host descriptors"). gb10-1 serves the agent alone on
  `gpt-oss-120b-agent-fp8` (4 sessions, 12 at most), with 15.07 GiB idle and 14.87 GiB at its
  lowest against its 12.2 GiB floor; its compiling start dips to 11.39 GiB. gb10-1's descriptor
  says gb10-2 is reserved for spike-ai-pipeline. gb10-2 has no descriptor: vision alone (4
  requests, 16 at most), with the hosts task's within-session spread on gb10-2 (output tok/s: A
  2.37%, B 0.52%) as its first run-noise figure. The Framework serves audio (4 requests, 4 at
  most) and embedding (16 streams, admitted at up to 34 requests a second).
- **The convention's measurements.** It was measured on llama.cpp hosts only. The weekly cadence
  has never run, the measure mode has run once, as a settings check, and no register entry has
  reached "cleared on the host".

### The `acceptance` task

From experiment.hosting's intake:

- **vLLM 0.34.** Every row runs v0.34.0, and the layout's measurements are v0.33.0's. v0.34
  carries the GB10 weight-loading fix (#58868), which bears on gb10-1's 437 s and gb10-2's 128 s
  weight reads after a reboot, and the extensible KV cache (spike-model-layout
  `docs/findings/vllm-layout.md`, A.2 and N.6).
- **The agent's FP8 KV** runs uncalibrated at scale 1.0. The accuracy check found no significant
  loss against BF16, but GPQA diamond's 198 questions put the paired difference's 95% interval at
  −10.7 to +1.6 points, so a loss of up to about 10 points isn't excluded. Calibration or a
  larger set would narrow it (spike-model-layout `docs/findings/vllm-layout.md`, C.5 and N.1).
- **Vision's load from streamed video** is unmeasured: every vision cell sent image questions.
- **Audio has no headroom** above its target of 4; 8 streams beside embedding at 8.4 requests a
  second is untested. Sessions of 131,072 tokens are computed from the KV cache, not run.
- **Benchmarks over the network.** Production clients sit on the same direct network as the GPUs,
  so the layout is sized on LAN figures. A cell run from the laptop over a Tailscale relay carries
  the relay's trip in its client-side figures (TTFT, request latency, RTFx), about 0.4 s for a 6 s
  audio clip; its server-side figures are unaffected (spike-model-layout
  `docs/findings/role-split.md`, "Stack, engine and network path" and L.7).
- **uv.** spike-model-layout's `mise.bench.toml` pins uv 0.13.7, behind 0.13.8, left to the next
  currency step.

## Inputs for later goals

- **The hosting-upgrades plugin** (v1.harness's `hosting-upgrades` task). The skill resolves every
  input and output against the project it runs in (`${CLAUDE_PROJECT_DIR}`), so the plugin says
  which repository that is and how its `docs/` is initialized there (the architect, at
  spike-hosting-upgrades' session brief). The host descriptors, the policy, the reports, the
  measure records, the blocker register, and the research cache live beside the scripts and
  settings an adoption changes, in ai-hosting.
- **Model evaluation at scale** (model-evaluation), from experiment.hosting's intake
  (spike-hosting-upgrades' and spike-model-engines' answers):
  - The candidate backlog, which one screen a session can't clear: the Framework's second survey
    lists over twenty agent candidates as needing measurement.
  - Workload-based tuning: concurrency and context tuned per model, workload, and host.
  - A `max_tokens` cap per tier in the agent-tier harness (spike-model-hosting's
    `evaluation/suites/agent_quality.py`), recorded with the tier's list, so a runaway reply is a
    counted cap hit, not a lost sample. BFCL sends none: on gb10-1 one multi-turn sample generated
    23,306 output tokens, and the tier hit its 45-minute cap at 19 of 20 samples.
  - An embedding scenario: spike-hosting-upgrades' `bench/` has none, so no embedding candidate
    can be measured.

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
