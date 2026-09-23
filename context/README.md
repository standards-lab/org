# Workspace context

The contextual index of the Standards Lab organization. This repository holds the
organization's context about itself — how it presents, the direction records, and the catalog
of repositories the effort is built from — and coordinates the workspace as marathon's declared
coordinator. Authoritative context lives at its single home and is linked from here:

- **The vision** — the opening of the profiles: `../public/profile/README.md`, the authored
  baseline, mirrored into the extended profile at `../private/profile/README.md` (the mirror
  convention is in `CLAUDE.md`).
- **The definitions** — the three-level hierarchy (architecture → standard → module) and the
  narrowing rule that binds it: the [architecture repository](https://github.com/standards-lab/architecture),
  home of the Elemental Architecture. `go-elemental` is the first standard, the Go
  implementation of the architecture.
- **The path** — `roadmap.toml`: the goal tree and backlog hold what remains; `next` is the
  only sequence.
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
- Documentation, diagram, and voice standards, and the organization documentation site that
  serves the architecture repository's content (`docs-site.md`).
- Later service layers beyond the cohesive reference: portable IaC and a web-platform-native
  client, each with its provider-swap class declared.
- Further application SDKs and their templates — command-line, worker, and others — as a
  consumer earns them.
- Focused reference architectures — the home for provider and engine variants, style variants,
  and references on other application SDKs. A variant is never a switch inside the cohesive
  reference; it is a separate focused reference, created when a consumer demands it and named
  when the first exists.

## Capabilities of this repository

- **Baseline profile** (`public/profile/README.md`) — the public org landing: the vision and
  the organization contents index; the authored source of the profile body.
- **Extended profile** (`private/profile/README.md`) — what members see in place of the
  baseline profile: the baseline body mirrored verbatim, plus the appended member-only
  orientation. `CLAUDE.md` states the mirror convention.
- **The workspace roadmap** (`context/roadmap.toml`) — the goal tree, tasks, and backlog for
  the buildout, citable by slug path; the only sequence it asserts is `next`.
- **Leadership briefs** (`../briefs/`) — orientation briefs for technical leadership;
  `../briefs/orientation.md` is the first, paired with the agent-interview prompt
  `../interview.md`.
- **References catalog** (`../references.toml`, `../references.local.toml`, `../references.md`)
  — portable identity for every repository in the effort, with a per-machine local-directory
  map, the standards declarations, and a private annex in `../private/`. `references.md` states
  the mechanics.
- **Experiments** (`../experiments.md`) — the catalog of spikes and their hosting convention.
- **Cross-repo coordination** — the references catalog is the repository list, and the
  workspace order in `.claude/marathon.toml` is its dependency graph in machine-readable form.

## Notes

- `auth-strategy.md` — authentication and authorization across the reference architecture.
- `service-organization.md` — the anticipated services and providers, runtime composition
  across services, and how the tiers co-evolve.
- `naming.md` — how the organization and its standards are named in prose.
- `graduation.md` — what graduating a standard to its own organization implies, and its open
  questions.
- `admin-listener.md` — the management listener's requirement and what an exploration of the
  build found.
- `blobfs.md`, with `blobfs-api.md`, `blobfs-composition.md`, `migration-sets.md`, and
  `sqlate-library-support.md` — the blobfs library, its proposed API, how a consumer composes it,
  shipped migration sets, and what sqlate needs to host a library.
- `staged-query-aggregation.md` — composing staged cross-service queries into one response.
- `elemental-runtime-layers.md` — the elemental layers beyond the app class.
- `docs-site.md` — the organization documentation site.
