# AI hosting

The hosting layer of the AI capability (`ai-strategy.md`) runs the machines that serve local
models to the organization's consumers: Pi sessions, Claude Code subagents, go-ai, and the
capstone's services. Its home is the member repository `ai-hosting`, which v1.ai.hosting built
from experiment.hosting's five spikes. This note holds the stack they settled and what
`ai-hosting` must specify; the spikes' answers, the inputs for later goals, and the experiment
rules are in [docs/ai-hosting.md](../docs/ai-hosting.md).

## The hosts

- **The Framework** (`framework`): AMD Strix Halo, 128 GB of unified memory, 112 GiB of it
  reachable by the GPU as GTT.
- **framework-2**: a second Strix Halo, set up as `framework` is.
- **gb10-1 and gb10-2**: Dell Pro Max GB10 units (DGX Spark class), each aarch64 with an sm_121
  GPU and 128 GB of unified memory at 273 GB/s, about 121 GiB visible. A ConnectX-7 link joins
  them; no model is served across it.

Each Strix Halo host keeps its model store on its second NVMe at `/srv/models`, plain ext4, and
unlocks its LUKS root at the console or remotely, through dropbear in the initramfs, reached on
the LAN through a host that is up (ai-hosting's `classes/strix-halo/runbook.md`, "Remote unlock").

The setup scripts handle three host behaviours ([docs](../docs/ai-hosting.md#the-host-behaviours)).

## The engine: vLLM

Every host serves with vLLM, one model to each container (a row), under its role's alias
(`agent`, `vision`, `audio`, or `embedding`); a request naming the model itself gets a 404. No
workstation runs llama.cpp.

- **Concurrency decides the engine.** The agent row met its floor at 12 sessions to llama.cpp's
  4, vision and audio held 16 requests to its 1, and embedding reached 4.4 times its throughput.
- **Models aren't loaded on demand.** GPU compute, not memory, is the bottleneck, and loading and
  unloading add compute on top of inference.
- **Model management stays off the serving API.** A user-device client able to change a host's
  models is a vulnerability.

## The layout

Each role has one host, and nothing fails over between hosts; overflow to Azure is the only
fallback. Rows are placed by the GPU compute they share, not the memory they fit
([ai-hosting's placement rule](https://github.com/standards-lab/ai-hosting/blob/main/docs/policy.md#placement)).
The table reads ai-hosting's class profiles on vLLM v0.34.0.

| Host | Role and model | Row, GPU memory fraction | Sequences × context | Load target | Holds alone |
| --- | --- | --- | --- | --- | --- |
| gb10-1 | agent: openai/gpt-oss-120b, FP8 KV | `vllm-agent`, 0.80 | 12 × 131,072 | 4 sessions | 12 sessions |
| gb10-2 | vision: nvidia/Gemma-4-26B-A4B-NVFP4 | `vllm-vision`, 0.55, queue limit 64 (32 waiting) | 32 × 8,192, an image budget of 1120 soft tokens | 32 requests | 13.4 images a minute of dense pages at 32 streams |
| framework | audio: google/gemma-4-E4B-it, BF16 | `vllm-audio`, 0.30, queue limit 192 | 48 × 8,192 | 48 streams | 2.133 req/s of 30 s windows at 48 streams, its throughput peak |
| framework-2 | embedding: intfloat/multilingual-e5-large | `vllm-embed`, 0.03 | 16 × 512 | 16 streams, at most 78 req/s | 78 req/s at 16 streams alone |

The floors are an agent inter-token latency median of at most 100 ms, vision's throughput peak at
the accuracy-first image budget, audio's throughput peak at full transcription, and embedding at
4,300 prompt tok/s or more. Vision found no throughput peak within the KV cache that holds
full-context requests (ai-hosting's `docs/findings/vision.md`, `#dense-1120-high`,
`#start-1120-055`). Audio's model transcribes at most 30 s of a clip (`docs/findings/audio.md`,
`#audio-real-30s`, `#audio-real-30s-64`).

The four rows ran together on v0.34.0. Agent, audio, and embedding matched their rows alone
within their hosts' run noise. Vision matched at 2, 4, and 8 streams; at 32 its decode rate (total
output, ITL) matched, while its finished-page count read 3–4% under, past gb10-2's 1.5% run noise
(`docs/findings/vision.md#layout`, `#dense-noise-32`).

## The gateway

agentgateway v1.7.0 fronts the rows from one compose stack on gb10-2, with Postgres holding only
the request log. Each alias has its own base URL, `/<alias>/v1`. The gateway issues the
consumers' keys (401 for a missing, unknown, or revoked key), routes only the eight serving paths,
gives each key a priority and a request-rate bucket, and holds embedding to 78 requests a second
with one global bucket (429, `Retry-After: 1`). Past a row's concurrency cap vLLM queues; past its
queue limit (running plus waiting) the row answers 503 and the gateway evicts it. Priority orders
only the waiting queue.

Cloud spend is held in Azure, by each deployment's capacity (agent 133, vision 41, and audio 30,
its quota) and a $50 monthly Cost Management budget alert.

## Overflow to Azure AI Foundry

When a row is down or refuses, the gateway sends its alias's requests to an Azure AI Foundry
deployment, marks the reply `x-gateway-overflow: azure`, and returns to the row once it is back.
Nothing overflows under IL6. The gateway rewrites each request for its deployment, dropping or
renaming the fields the deployment refuses, and sets vision's `reasoning_effort` to `minimal`, so
a client addresses an alias without knowing which backend answers (ai-hosting's
`docs/gateway.md#overflow`). Three aliases overflow:

- **agent** to `gpt-oss-120b`, processed within the data zone;
- **vision** to `gpt-5-mini`, processed globally; and
- **audio** to `gpt-audio-mini` on Chat Completions, processed globally. It has two known
  refusals: a text-only chat, and audio inside a Responses request, which the gateway refuses.

Embedding has no cloud fallback, since a cloud model's vectors are in another space;
`text-embedding-3-small` is deleted.

## Set A's embedding model

Set A embeds with multilingual-e5-large (Microsoft, MIT, 1,024 dimensions, a 512-token context),
which replaced granite-embedding-english-r2, an English-only model; ~/docs is multilingual. Callers
prefix a query with `query: ` and a stored passage with `passage: `. Served on vLLM, it scored
within 0.02 nDCG@10 of its published score on all 37 BelebeleRetrieval subsets (ai-hosting's
`docs/findings/embedding.md#e5-retrieval`). Anything indexed with granite is re-embedded,
docinfer's embeddings among them.

## The audio role transcribes over chat

google/gemma-4-E4B-it serves more than speech to text, so it stays the audio model. vLLM serves
`/v1/audio/transcriptions` only for a model implementing `SupportsTranscription`, which
`Gemma4ForConditionalGeneration` doesn't, so the workstations transcribe over chat: go-ai's
`TranscribeChat` sends the clip as input audio in a Chat Completions request; how a client selects
it is in [docs](../docs/ai-hosting.md#inputs-for-later-goals). Each row's refused reasoning
efforts are in go-ai's format profiles (go-ai's `docs/concepts.md`,
"Profiles").

## Production

Production has no tailnet, which fences only the dev workstations. A service on Azure Container
Apps will route inference between Azure AI Foundry and the self-hosted stack
by request and availability. The stack takes requests, spreads them across the pool, and queues
them when capacity is full, and its access control can't depend on the tailnet. Making the
gateway the only network path to the rows is for the networking and platform teams.

## What `ai-hosting` holds

`ai-hosting`, a peer of claude-plugins in the workspace's lowest layer, states the settings the
architect's hosts use, generally enough that another host of the same class follows it: host-class
profiles, vLLM rows, the gateway's stack, runbooks, the upgrade-evaluation convention rebuilt on
vLLM, and the admin tooling after experiment.admin's intake. It runs as one command suite,
`hosting <group> <verb>`, and is the only hosting repository on the workstations; the suite's
design is `ai-hosting/context/hosting-suite.md`.

A particular host's state stays out of tracked files, so the specification and instance split of
`claude-plugins/context/tool-based-skills.md` has no instance half here: a survey report is host
state, kept untracked in ai-hosting's `.state/upgrades/<host>/<UTC date>T<HHMM>Z.md`. A
measurement is a findings cell in `docs/findings/`, which the profiles' reasons cite.

Architecture's `principles/model-cards.md` and `principles/tool-call-reliability.md` cover reading
a model card and tool-call reliability; sizing and placing rows stay in ai-hosting's
`docs/policy.md`, "The tuning rule" and "Placement".

`ai-hosting` is public. Tailnet identifiers (the MagicDNS suffix and addresses) stay out of it,
and its docs name hosts by short name.

## Hosts and access

The hosts are on the tailnet under `tag:inference`, reached by Tailscale SSH in accept mode while
the experiments run. gb10-2 also carries capstone.data-intelligence's measurements and build, then
experiment.ocr. Session rules are in [docs](../docs/ai-hosting.md#hosts-and-access).

## Experiment rules

Every hosting spike, experiment.admin's included, keeps the rules in
[docs](../docs/ai-hosting.md#experiment-rules).

## Experiment: spike-host-admin

experiment.admin's one spike, run after capstone.data-intelligence: can one CLI on go-cli-sdk
manage the vLLM rows on the four hosts, with ai-hosting's `hosting` suite as its baseline? It
decides `ai-hosting`'s admin tooling.

## Open for experiment.admin

- **Form.** Keep a bash dispatcher, or write a Go CLI on go-cli-sdk, layered as specification,
  tooling, and skill (`claude-plugins/context/tool-based-skills.md`), with the profile schema as
  its contract (ai-hosting's `classes/README.md`). spike-host-admin builds the Go CLI. The root command is `hosting`; `outpost`
  named where the admin sits, not what the tool does.
- **Host-class differences.** How the tool reads GPU use: through `amdgpu_top -p` and GTT on
  Strix Halo; on the GB10s, `nvidia-smi --query-compute-apps used_memory` gives each process's GPU
  memory, though its totals report N/A. ai-hosting's `hosting engine memory` reads both.

## Answers · experiment.admin

## Assumptions

- Production reaches the stack through an Azure Container Apps service that routes between Azure
  AI Foundry and the self-hosted stack, not through the tailnet.
