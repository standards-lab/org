# AI hosting: the spikes' answers and inputs for later goals

The detail behind `context/ai-hosting.md`, in five sections: the spikes' answers, the host
behaviours the setup scripts handle, the inputs for later goals, hosts and access, and the hosting
experiment rules.

## The spikes' answers

- **spike-model-hosting** ([answer](https://github.com/JaimeStill/spike-model-hosting#the-answer)):
  one rendered host-class profile served set A to Pi, Claude Code, and go-ai's model client at
  once with no answer crossed, and with gpt-oss-120b only Pi, on `tool_choice` `"required"`,
  met the tool-call bar. Its `docs/model-cards.md` and `docs/findings/tool-reliability.md` fed
  architecture's `principles/model-cards.md` and `principles/tool-call-reliability.md`.
- **spike-model-engines** ([answer](https://github.com/JaimeStill/spike-model-engines#the-answer)):
  answered llama.cpp in a container on both host classes. vLLM supersedes the engine; the
  container stands, since on gb10-1 both engines ran at native speed in their containers.
- **spike-hosting-upgrades**
  ([answer](https://github.com/JaimeStill/spike-hosting-upgrades#the-answer),
  [convention](https://github.com/JaimeStill/spike-hosting-upgrades/blob/main/docs/convention.md)):
  a skill surveys each host weekly and on events, ranking every upgrade from cited public
  sources against the host's load targets; the host measures only what no source settles, and
  an adoption leaves a chain of dated records.
- **spike-model-layout**
  ([answer](https://github.com/JaimeStill/spike-model-layout/blob/main/README.md#the-answer)):
  the layout in the note.
- **spike-model-gateway** ([answer](https://github.com/JaimeStill/spike-model-gateway#the-answer)):
  agentgateway. Each alias's `/models` lists that alias alone. The gateway adds nothing beyond run
  noise to agent, vision, or audio, and at most 3.4 ms (median) to an embedding request, and the
  spike proved the overflow route with each row down. go-ai v0.3.0's and infer's integration tiers
  pass through it on all four aliases, and go-ai retries its 429 and 503, honouring `Retry-After`.
  Its requirements are
  [docs/gateway.md](https://github.com/JaimeStill/spike-model-gateway/blob/main/docs/gateway.md).

## The host behaviours

Beyond the note's summary, each GB10 has 20 cores and 3.6 TB of NVMe, and their QSFP link over
ConnectX-7 carries about 185 Gb/s of RDMA. Three host behaviours shape the setup scripts:

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
  published port. All four hosts set `net.ipv4.ip_nonlocal_bind = 1` persistently, so the bind
  succeeds first: the GB10s and framework-2 in `/etc/sysctl.d/90-nonlocal-bind.conf`, the
  Framework in `60-tailnet-bind.conf`. `docker network connect bridge <container>` fixes a
  container already stranded.

## Inputs for later goals

- **The hosting-upgrades plugin** (v1.harness's `hosting-upgrades` task). The skill resolves every
  input and output against the project it runs in (`${CLAUDE_PROJECT_DIR}`), so the plugin says
  which repository that is and how its `docs/` is initialized there (the architect's decision).
  The plugin initializes the project's `docs/` with the convention (`docs/upgrades.md`), the
  policy (`docs/policy.md`), and the host descriptors (`docs/hosts/`). Reports go untracked to
  `.state/upgrades/<host>/<UTC date>T<HHMM>Z.md`, measurements are findings cells, the watch
  register (`docs/watch.md`) replaces the blocker register, and there is no research cache.
- **Model evaluation at scale** (model-evaluation), from spike-hosting-upgrades' and
  spike-model-engines' answers:
  - The candidate backlog, which one screen a session can't clear: the Framework's second survey
    lists over twenty agent candidates as needing measurement.
  - Workload-based tuning: concurrency and context tuned per model, workload, and host.
  - A `max_tokens` cap per tier in the agent-tier harness (spike-model-hosting's
    `evaluation/suites/agent_quality.py`), recorded with the tier's list, so a runaway reply is a
    counted cap hit, not a lost sample. BFCL sends none: on gb10-1 one multi-turn sample generated
    23,306 output tokens, and the tier hit its 45-minute cap at 19 of 20 samples.
  - The current models stay until the capstone completes, and the surveys' candidates go
    unscreened till then. EmbeddingGemma 2 is embedding's next candidate, if vLLM serves it.
- **The agent's FP8 KV** runs uncalibrated at scale 1.0. The accuracy check found no significant
  loss against BF16, but GPQA diamond's 198 questions put the paired difference's 95% interval at
  −10.7 to +1.6 points, so a loss of up to about 10 points isn't excluded. Calibration or a
  larger set would narrow it (spike-model-layout `docs/findings/vllm-layout.md`, C.5 and N.1).
  Sessions of 131,072 tokens are computed from the KV cache, not run.
- **Vision's load from streamed video** is unmeasured: every vision cell sent image questions.
- **Benchmarks over the network.** Production clients sit on the same direct network as the GPUs,
  so the layout is sized on LAN figures. A cell run from the laptop over a Tailscale relay carries
  the relay's trip in its client-side figures (TTFT, request latency, RTFx), about 0.4 s for a 6 s
  audio clip; its server-side figures are unaffected (spike-model-layout
  `docs/findings/role-split.md`, "Stack, engine and network path" and L.7).
- **Transcription mode.** Against the workstations, the host that builds docinfer's go-ai clients
  sets `Options.TranscriptionMode` to `TranscribeChat` in place of go-ai's default
  `TranscribeRoute`, and docinfer reads it from the client; a chat-mode reply that isn't completed
  returns `*ai.IncompleteTranscriptError`.
- **The gateway's trace id.** Unverified: whether agentgateway v1.7.0's access log carries the
  trace id (if so, go-web-service's `json_parser` trace operators apply), and whether vLLM's
  histograms carry exemplars; vLLM's plain-text logs carry no trace id (spike-model-gateway
  `docs/gateway.md`).

## Hosts and access

Sessions do on any host whatever needs no sudo, the docker group included; root stays the
architect's, run from exact commands. The architect may grant a root command a session must repeat
as a sudoers rule naming that one command exactly, such as the page-cache drop, granted on all
four hosts (`/usr/sbin/sysctl vm.drop_caches=3` on the GB10s, `/usr/bin/sysctl vm.drop_caches=3`
on the Strix Halo hosts). Every session starts by bringing its hosts up to date. Every measurement
records the engine version, the host's stack, and the laptop's network path to the host
(`tailscale ping`): a DERP relay adds its round trip to TTFT and request latency, but not to
inter-token latency or the server's own counters.

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
- Any quantization or format is eligible. A candidate is judged by measurement on the host it
  would serve on: accuracy and capability first, then throughput at the row's cap and queue
  limit. A card's evals only rank candidates (ai-hosting's
  [judged on the host](https://github.com/standards-lab/ai-hosting/blob/main/docs/policy.md#judged-on-the-host)).
