# AI hosting

The hosting layer of the AI capability (`ai-strategy.md`) covers configuring and running the
machines that serve local models to the organization's consumers:

- Pi sessions
- Claude Code subagents
- the go-ai service

Its planned home is a new workspace member repository, working name `ai-hosting`, built by
`v1.ai.hosting`. It is written fresh from what personal-agents proved, rather than transferred
with that repository's history.

## What exists

personal-agents (`github.com/JaimeStill/personal-agents`, local `~/code/personal-agents`) is a
marathon context project outside the workspace. It configures one host class today, the
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
VRAM instead of unified memory. Their specifications are gathered when their work starts.

## Experiment: personal-agents as the hosting spike

personal-agents becomes the hosting experiment rather than a new `spike-*` repository. It is
already a standalone marathon project, and `v1.ai.hosting` rebuilds its result fresh, so nothing
is lost by skipping a copy.

**Question.** Which serving configuration and specification shape hold across both host classes
(Strix Halo with Vulkan, Dell NVIDIA with CUDA) and all three consumers at once?

**Decision it changes.** The `ai-hosting` specification:

- the schema of a host-class profile
- whether a gateway sits in front of the router
- the form of the admin tooling

## The decomposition

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

  Their place in the layer is decided when they are promoted. The harness pages are the nearest
  neighbor.
- **Nowhere**: the state of a particular host. Once a host is set up it needs little attention,
  and anything a build or tuning effort keeps falls outside the workspace. There is no instance
  repository, so the specification/instance split of `tool-based-skills.md` ("How a
  specification and an instance separate") has no instance half here.

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
  and skill (`tool-based-skills.md`, "The three layers a tooling domain is built in"), with the
  profile schema as its contract.
- **Host-class differences.** How the tool handles them: GPU usage through `amdgpu_top` versus
  `nvidia-smi`, and Vulkan versus CUDA builds.

## Assumptions

- One llama.cpp router per host can serve all three consumers within its slot and KV budget.
- The preset convention (presets keyed by capability tier) carries over to discrete-VRAM hosts.
- llama.cpp stays the serving engine on both host classes.
