# AI hosting

The hosting layer of the AI capability (`ai-strategy.md`) runs the machines that serve local
models to the organization's consumers: Pi sessions, Claude Code subagents, go-ai, and the
capstone's services. Its home is the member repository `ai-hosting`, which v1.ai.hosting builds
from experiment.hosting's five spikes. This note holds the stack they settled and what
`ai-hosting` must specify; the spikes' answers, each task's inputs, and the experiment rules are in
[docs/ai-hosting.md](../docs/ai-hosting.md).

## The hosts

- **The Framework** (`framework`): AMD Strix Halo, 128 GB of unified memory, 112 GiB of it
  reachable by the GPU as GTT.
- **gb10-1 and gb10-2**: Dell Pro Max GB10 units (DGX Spark class), each aarch64 with an sm_121
  GPU and 128 GB of unified memory at 273 GB/s, about 121 GiB visible. A ConnectX-7 link joins
  them; no model is served across it.

The setup scripts handle three host behaviours ([docs](../docs/ai-hosting.md#the-host-behaviours)).

## The engine: vLLM

Every host serves with vLLM, one model to each container (a row), under its role's alias
(`agent`, `vision`, `audio`, or `embedding`); a request naming the model itself gets a 404. No
workstation runs llama.cpp.

- **Concurrency decides the engine.** The agent row met its latency floor through 12 sessions
  against llama.cpp's 4; vision and audio each held 16 requests against llama.cpp's 1; embedding
  reached 4.4 times llama.cpp's throughput at 16 streams.
- **Models aren't loaded on demand.** GPU compute, not memory, is the bottleneck, and loading and
  unloading add compute on top of inference.
- **Model management stays off the serving API.** A user-device client able to change a host's
  models is a vulnerability.

## The layout

Each role has one host, and nothing fails over between hosts; overflow to Azure is the only
fallback. Purposes sharing one GPU's compute, not memory, decided the placement: every miss
spike-model-layout measured was two purposes on one GPU.

| Host | Role and model | Row, GPU memory fraction | Sequences × context | Load target | Holds alone |
| --- | --- | --- | --- | --- | --- |
| gb10-1 | agent: openai/gpt-oss-120b, FP8 KV | `gpt-oss-120b-agent-fp8`, 0.80 | 12 × 131,072 | 4 sessions | 12 sessions |
| gb10-2 | vision: nvidia/Gemma-4-26B-A4B-NVFP4 | `gemma-4-26B-A4B-NVFP4-vision-gb10`, 0.40 | 16 × 8,192 | 4 requests | 16 requests |
| framework | audio: google/gemma-4-E4B-it, BF16 | `gemma-4-E4B-it-audio-framework`, 0.18 | 4 × 8,192 | 4 requests | 4, beside embedding at 34 req/s |
| framework | embedding: ibm-granite/granite-embedding-english-r2 | `granite-embedding-english-r2-framework`, 0.03 | 128 × 8,192 | 16 streams, at most 34 req/s | knee at 16 streams |

The floors are an agent inter-token latency median of at most 100 ms, a vision request latency
median of at most 10 s, an audio RTFx median of at least 5, and embedding at 4,300 prompt tok/s or
more. With all three hosts loaded to their targets for 240 s, every row met its floor on vLLM
0.33.0; the hosts run 0.34.0, on which the layout is unmeasured. Unlike the GB10s, the Framework
returns from a reboot only once its LUKS passphrase is entered at its console.

## The gateway

agentgateway v1.7.0 fronts the rows from one compose stack on gb10-2, with Postgres holding
budgets and the request log. Each alias has its own base URL, `/<alias>/v1`. The gateway issues
the consumers' keys (401 for a missing, unknown, or revoked key), routes only the eight serving
paths, gives each key a priority, a request-rate bucket, and a token budget per UTC clock hour,
and holds embedding to 34 requests a second with one global bucket (429, `Retry-After: 1`). Past
a row's concurrency cap vLLM queues; past twice the cap the row answers 503 and the gateway evicts
it. Cerebro (bulk) and Athena (chat) each get a key and a budget. The gateway task's inputs are
in [docs](../docs/ai-hosting.md#the-gateway-task).

## Overflow to Azure AI Foundry

When a row is down or refuses, the gateway sends its alias's requests to an Azure AI Foundry
deployment, marks the reply `x-gateway-overflow: azure`, and returns to the row once it is back.
Nothing overflows under IL6. All four aliases overflow:

- **agent** to `gpt-oss-120b`, processed within the data zone;
- **vision** to `gpt-5-mini`, processed globally;
- **audio** to `gpt-audio-mini` on Chat Completions, processed globally. It has two known
  refusals: a text-only chat, and audio inside a Responses request, which the gateway refuses;
- **embedding** to the `embedding` task's model on Foundry managed compute, enabled only once the
  Framework's and Azure's vectors for the same input reach cosine ≥ 0.999.

`text-embedding-3-small` stays deployed but is never routed: its vectors are in another space.

## Set A's embedding model

Set A embeds with granite-embedding-english-r2 (IBM, Apache-2.0, 768 dimensions), chosen because
vLLM couldn't serve EmbeddingGemma 2 on the GB10's SM121. It is English-only; ~/docs is
multilingual.

The `embedding` task (planned) replaces it with a multilingual model that both the Framework's
vLLM and Foundry managed compute serve, trying multilingual-e5-large (MIT, XLM-R base) first. The
model must hold retrieval across ~/docs' languages, fit docinfer's chunks in 512 tokens, and leave
the Framework's audio row at its floor. A Chinese-origin candidate runs only if
multilingual-e5-large fails, with the reason recorded (`ai-strategy.md`, "Constraint: model
origin"). Anything indexed with granite is re-embedded, docinfer's embeddings among them.

## The audio role transcribes over chat

google/gemma-4-E4B-it serves more than speech to text, so it stays the audio model. vLLM serves
`/v1/audio/transcriptions` only for a model implementing `SupportsTranscription`, which
`Gemma4ForConditionalGeneration` doesn't, so the workstations transcribe over chat: go-ai's
`TranscribeChat` sends the clip as input audio in a Chat Completions request. Against the
workstations, the host that builds docinfer's go-ai clients sets `Options.TranscriptionMode` to
`TranscribeChat` in place of go-ai's default `TranscribeRoute`, and docinfer reads it from the
client; a chat-mode reply that isn't completed returns `*ai.IncompleteTranscriptError`. Each
row's refused reasoning efforts are in go-ai's format profiles (go-ai's `docs/concepts.md`,
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

The state of a particular host belongs nowhere in the workspace: a set-up host needs little
attention, so the specification and instance split of
`claude-plugins/context/tool-based-skills.md` has no instance half here.

The `architecture` task (planned) writes four method pages: reading a model card, tool-call
reliability, sizing a vLLM row, and the placement rule. The goal's sync archives personal-agents
([JaimeStill/personal-agents](https://github.com/JaimeStill/personal-agents), the earlier
llama.cpp setup with the `outpost` dispatcher), its README linking forward to `ai-hosting`, and
spike-model-engines, spike-model-gateway, and spike-hosting-upgrades.

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
manage the vLLM rows on the three hosts, with `outpost` as its baseline? It decides `ai-hosting`'s
admin tooling.

## Open for experiment.admin

- **Form.** Keep a bash dispatcher, or write a Go CLI on go-cli-sdk, layered as specification,
  tooling, and skill (`claude-plugins/context/tool-based-skills.md`), with the profile schema as
  its contract. spike-host-admin builds the Go CLI. The root command is `hosting`; `outpost`
  named where the admin sits, not what the tool does.
- **Host-class differences.** How the tool reads GPU use: through `amdgpu_top` on Strix Halo,
  while the GB10s' GPU and memory use can't come from `nvidia-smi`.

## Answers · experiment.admin

## Assumptions

- Production reaches the stack through an Azure Container Apps service that routes between Azure
  AI Foundry and the self-hosted stack, not through the tailnet.
