# Workspace context

The contextual index of the Standards Lab organization. This repository holds the
organization's context about itself — how it presents, the direction records, and the catalog
of repositories the effort is built from — and coordinates the workspace as marathon's declared
coordinator. Authoritative context lives at its single home and is linked from here:

- **The vision** — the opening of the profiles: `../public/profile/README.md`, the authored
  baseline, mirrored into the extended profile at `../private/profile/README.md` (the mirror
  convention is `design/workspace-structure.md`).
- **The definitions** — the three-level hierarchy (architecture → standard → module) and the
  narrowing rule that binds it: the [docs landing zone](https://github.com/standards-lab/docs),
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
  serves the landing zone's content (`concepts/docs-site.md`).
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
  orientation. The mirror convention is `design/workspace-structure.md`.
- **The workspace roadmap** (`context/roadmap.toml`) — the goal tree, tasks, and backlog for
  the buildout, ephemeral and citable by slug path; the only sequence it asserts is `next`.
- **References catalog** (`../references.toml`, `../references.local.toml`, `../references.md`)
  — portable identity for every repository in the effort, with a per-machine local-directory
  map and the standard grouping. A private annex in `../private/` extends it with the prior
  R&D that cannot be catalogued publicly. See `design/repo-references.md`.
- **Standards declaration** — how standards, architectures, and principles are declared in the
  catalog and the repositories, with definitions in the docs landing zone. See
  `design/standards.md`.
- **Blueprint organization** — the blueprint / boundary / catalog roles, the graduation model,
  and the agentic thesis. See `design/blueprint-organization.md`.
- **The workspace docs tier** — the centralized documentation convention. See
  `design/workspace-docs.md`.
- **Service organization** — the anticipated services and providers, and how the tiers
  co-evolve. See `design/service-organization.md`.
- **DSL-driven services** — the strategy distinguishing DSL-driven infrastructure (SQL first)
  from protocol-driven, and the native-SQL direction for go-database v0.4 and the reference
  service. See `design/dsl-driven-services.md`.
- **Context architecture** — the single-source-of-truth principle for every layer of written
  context. See `design/context-architecture.md`.
- **Dependency sourcing** — when the organization hand-rolls a capability and when it sources
  an industry-standard one, and how a sourced dependency stays inside the dependency-line
  principle. See `design/dependency-sourcing.md`.
- **Workspace structure** — the two-repository layout that lets one tree coordinate the public
  and member profiles, and the profile mirror convention. See `design/workspace-structure.md`.
- **Naming** — how the organization and its standards are named in prose. See
  `design/naming.md`.
- **Cross-repo coordination** — coordinating the Standards Lab repositories as a group; the
  workspace order in `.claude/marathon.toml` is the dependency graph in machine-readable form.
  Kept minimal; it grows only as concrete needs appear.
