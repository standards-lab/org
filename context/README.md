# Workspace context

The contextual index of the Standards Lab organization. This repository holds the
organization's context about itself — how it presents, the direction records, and the catalog
of repositories the effort is built from — and coordinates the workspace as marathon's declared
coordinator. Authoritative context lives at its single home and is linked from here:

- **The vision** — the opening of the profiles: [the authored baseline](https://github.com/standards-lab/.github/blob/main/profile/README.md),
  mirrored into the extended profile at `../private/profile/README.md` (member checkout; the
  mirror convention is in `../CLAUDE.md`).
- **The definitions** — the three-level hierarchy (architecture → standard → module) and the
  narrowing rule that binds it: the [architecture repository](https://github.com/standards-lab/architecture),
  home of the Elemental Architecture. `go-elemental` is the first standard, the Go
  implementation of the architecture.
- **The path** — the root `roadmap/`, its index and goal files: the goal tree of what remains.
- **Prior work** — most of the architecture already exists across prior R&D, catalogued in
  `../references.md`; the work is to organize it into an effective standard.

## Execution philosophy

- Emergent, not decreed. Standardize what gets proven by building. Governance, catalogs, and
  conformance machinery are outputs of the work, not preconditions.
- Lean first versions, refined through use. Ship deliberate v0s and harden them by using them.
- Plan only what's next. Hold the immediate priorities; scope further efforts once these are
  addressed.

## Longer-term objectives

Beyond the roadmap's goal tree, the ecosystem grows toward:

- Governance — which these organizational repositories can eventually serve themselves; the
  blueprint governing the organizations it graduates.
- A baseline catalog and a conformance suite.
- The full source-control topology.
- Documentation, diagram, and voice standards, and the Pages sites that serve the architecture
  repository's and each repository's documentation (`sites`, `sites.md`).
- Later service layers beyond the cohesive reference: portable IaC and a web-platform-native
  client, each with its provider-swap class declared.
- Further application SDKs and their templates — worker and others — as a consumer earns them.
  go-cli-sdk and go-cli-sdk-template serve the command-line application type.
- Focused reference architectures — the home for provider and engine variants, style variants,
  and references on other application SDKs. A variant is never a switch inside the cohesive
  reference; it is a separate focused reference, created when a consumer demands it and named
  when the first exists.

## Capability map

- **Baseline profile** (built): the public org landing, the vision and the organization contents
  index, authored in [the baseline](https://github.com/standards-lab/.github/blob/main/profile/README.md); [`../CLAUDE.md`](../CLAUDE.md)
  states how the extended profile mirrors it.
- **Extended profile** (built): what members see, the baseline mirrored verbatim plus a
  member-only orientation, in `../private/profile/README.md` (member checkout);
  [`../CLAUDE.md`](../CLAUDE.md) states the mirror convention.
- **The workspace roadmap** (built): the goal tree and its tasks, each goal `active`, `planned`,
  or `backlog`, in [the index](../roadmap/index.toml) and the goal files beside it in the root
  `roadmap/`.
- **Leadership briefs** (built): orientation briefs for technical leadership, starting with
  [the orientation](../briefs/orientation.md) and its agent-interview prompt `../interview.md`;
  both move to sites.status-reports at its `layout` task.
- **References catalog** (built): portable identity for every repository in the effort, with a
  per-machine local map and a private annex in `../private/`; [the catalog](../references.md)
  states the mechanics.
- **Experiments** (built): running spikes are goals under `experiment` in the roadmap, archived
  spikes are in [the catalog](../references.toml), and `[workspace.experiments]` in
  `.claude/marathon.toml` holds the hosting convention. Every spike carries go-core's Apache-2.0
  `LICENSE` and a `main` ruleset, which `plan`'s spike setup applies, and takes its secrets as
  architecture's `principles/development-secrets.md` states.
  Capstones are `capstone.<slug>` goals at `~/capstones/<repo>`, each repository named
  `capstone-<name>`, listed in `[workspace.paths]` (`naming.md`).
- **Cross-repo coordination** (built): the catalog is the repository list, and the workspace
  order in [the configuration](../.claude/marathon.toml) is its dependency graph; `../CLAUDE.md`
  states the order's tiers, the exclusive group, and why the merge waits on no checks.

## Notes

- `auth-strategy.md` — authentication and authorization across the reference architecture.
- `service-organization.md` — the anticipated services and providers, runtime composition
  across services, and how the tiers co-evolve.
- `naming.md` — how the organization and its standards are named in prose.
- `blobfs-composition.md`, with `migration-sets.md` and `sqlate-library-support.md` — the
  blobfs protocols and operating constraints a consumer sequences, what a shipped migration set
  requires of its shipper and consumers, and what sqlate needs to host a library.
- `messaging.md` — the event and reactor layer: its decisions, the outbox sequencing, which
  commands emit events, where the primitives live, lifecycle registration, and what is open; the
  evidence and capability ledger are on its docs page.
- `ai-strategy.md`, with `ai-hosting.md`: the AI capability's four layers (hosting, client,
  processing, harness) and their homes, the pivot from tau to external harnesses, the decided
  design of the client and processing layers (go-ai, and docproc with its system tools), the
  questions of experiment.ocr and experiment.harness, and the self-hosted stack v1.ai.hosting
  built, with experiment.admin's question.
- `data-intelligence.md` — the first capstone, capstone.data-intelligence: what it is, its
  Cerebro and Athena architecture and repositories, what it asks of the goals beneath it, its
  gates, the evidence it carries from spike-ai-pipeline, and its design tasks.
- `docs-corpus.md` — the real workloads' corpus: what it holds, how it changes, how it is shared,
  the verified checkout on gb10-2, and the baseline.
- `system-tools.md` — the system-tools principle: Go builds only what it must own, and runs
  mature Linux tools through adapters it owns; spike-ai-pipeline is its first case.
- `web-client.md` — the web client stack: its repositories and what each owns, its conventions,
  packaging and release, `NewClient`, browser support, the color profiles, Web Elemental, and the
  reference client, for v1.client.
- `sites.md` — the Pages sites: where HTML is used, the topology, serving and build, the layers
  the sites share, the docs/ guides' conventions, and live Go demos, for `sites` and
  v1.repository-docs.
- `cli-applications.md` — the CLI application type: why it leaves cobra, where the cobra-era tools
  differ, and the role interfaces across the stack.
- `waves.md` — the plan to the capstone.
- `middleware.md` — v1.middleware's remaining adoptions: CORS's fired trigger and its
  findings, and what real client IP and compression wait for.
