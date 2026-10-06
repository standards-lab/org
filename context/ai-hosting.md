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
- **Serving**: llama.cpp's router in Vulkan mode, run as a systemd service. It binds only to the
  tailnet, and Tailscale's device authentication is the access boundary.
- **Models**: Qwen3-Coder-Next at 131k context and gpt-oss-120b at 32k, sharing four slots and a
  unified KV cache.
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
VRAM instead of unified memory. Their specifications are to be gathered when work on them starts.

## Experiment: spike-model-hosting

The hosting experiment is a new repository, `~/experiments/spike-model-hosting` (remote
[JaimeStill/spike-model-hosting](https://github.com/JaimeStill/spike-model-hosting)), not
personal-agents itself. personal-agents is a read-only reference the spike builds on: nothing is
written to it or archived from it by the experiment. `plan experiment.ai.spike-model-hosting`
records it as a reference when it sets the spike up. The spike's answer lands in
`ai-strategy.md`, "Answers · experiment.ai".

**Question.** Which serving configuration and specification shape hold across both host classes
(Strix Halo with Vulkan, Dell NVIDIA with CUDA) and all three consumers at once?

**Decision it changes.** The `ai-hosting` specification:

- the schema of a host-class profile
- whether a gateway sits in front of the router
- the form of the admin tooling

**Gateway candidate: agentgateway.** [agentgateway](https://github.com/agentgateway/agentgateway)
is the candidate the spike evaluates for the gateway decision, against the bare router. It is an
Apache 2.0 Rust proxy under the Linux Foundation and runs as a standalone binary, so it fits the
router's systemd and tailnet-only shape. Its case is that it answers the deferral in
personal-agents' `capabilities/gateway-proxy.md`: with Claude Code subagents as the second
client, one endpoint in front of the router could give the three consumers what the tailnet
boundary does not:

- a separate identity for each consumer (API keys or JWT, with CEL authorization rules)
- token budgets and rate limits that keep one consumer from exhausting the shared slots
- OpenTelemetry traces and metrics for each request

The spike records whether it holds:

- **Fidelity.** Pi and go-ai's `model` client run through it to llama.cpp unchanged, and Claude
  Code's tool use, streaming, and thinking blocks pass intact. Claude Code's experimental beta
  parameters are rejected ("Extra inputs are not permitted"); the documented workaround,
  `CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1`, is a cost the spike weighs.
- **Cost.** Its memory and latency beside the router, and whether its configuration fits in a
  host-class profile or stands beside it.
- **Both host classes.** The same configuration in front of the Vulkan and the CUDA router.
- **IL6.** Whether a gateway binary is acceptable air-gapped, checked on paper like the harness.

llama.cpp serves the Anthropic Messages API (`/v1/messages`) natively, so the gateway's case
rests on identity, budgets, routing, and observability, not on translating formats. If none of
those is needed across the three consumers, the answer is no gateway.

## Where personal-agents' parts go

- **Into `ai-hosting`**, one specification-level repository:
  - the host-class profiles and presets
  - the admin tooling
  - the systemd unit, which personal-agents describes but doesn't track
  - the pacman restart hook
  - a runbook for each host class

  It states the settings the architect's own implementation uses, generally enough that another
  host of the same class follows it.
- **Into the architecture layer**, the portable method, as pages written once the hosting
  experiment validates it:
  - model tiers
  - context sizing
  - the method for measuring memory footprint
  - the serving conventions: router mode, tailnet-only binding, and presets keyed by capability
    tier rather than by host

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
- **Form.** Either keep the bash dispatcher, or write a Go CLI layered as specification, tooling,
  and skill (`claude-plugins/context/tool-based-skills.md`), with the
  profile schema as its contract.
- **Host-class differences.** How the tool handles them: GPU usage through `amdgpu_top` versus
  `nvidia-smi`, and Vulkan versus CUDA builds.

## Assumptions

- One llama.cpp router per host can serve all three consumers within its slot and KV budget.
- The preset convention (presets keyed by capability tier) carries over to discrete-VRAM hosts.
- llama.cpp stays the serving engine on both host classes.
