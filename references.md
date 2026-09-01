# Repository references

Descriptive companion to `references.toml`. Each heading maps to one or more manifest keys; the remote
is in `references.toml` and the local checkout path is in `references.local.toml`.

Two kinds of entry: the **effort repositories** that make up the Standards Lab organization, and the
**prior R&D** that informs the standard. The prior R&D is input, not infrastructure to inherit: carry
its strengths, correct its debts, and re-derive a clean standard.

A private annex extends this catalog: prior R&D from private engagements is catalogued in the
private repository, whose annex `references.toml` and `references.md` join this catalog under the
same key namespace when the member checkout is present at `private/`. The layering rule is
`context/design/repo-references.md`.

## Standards

A standard is a named, technology-specific implementation of an architecture that a set of
repositories declares alignment to. Definitions live in the
[docs landing zone](https://github.com/standards-lab/docs); the declaration mechanics are in
`context/design/standards.md`. Each standard has a `[standards.<key>]` entry in
`references.toml` with a `definition` URL and the `architecture` it implements, and a member
repository's entry declares `standard = "<key>"`.

### go-elemental

The Go implementation of the Elemental Architecture, the organization's first standard, built
on the standard library. Defined at
[standards/go-elemental](https://github.com/standards-lab/docs/blob/main/standards/go-elemental/index.md).
Members: `go-core`, `go-database`, `go-web-sdk`, and `go-web-sdk-template` (released), and
`go-web-service`, the reference web service (versionless until its 1.0). `dotnet-elemental` is
anticipated as its derived standard (`derives = "go-elemental"`).

## Effort repositories — Standards Lab

### claude-plugins
The plugin host for the organization, mirroring the structure of `tau-marketplace`. Ships the `marathon`
workflow plugin and its `marathon-roadmap` extension. The harness level of the reference architecture.

### docs
The organization's documentation landing zone: the canonical home for its architectures,
standards, principles, and the documented details of every repository, published as plain
markdown with YAML front matter. The workspace's centralized docs tier
(`context/design/workspace-docs.md`); anticipated host content for the organization
documentation site.

### org
This repository: the workspace context that coordinates the organization — `context/`, the
references catalog, the marathon anchor. The landing zone for the organization's context about
running itself, as `docs` is for the architecture's. Marathon is anchored here.

### github-private
The extended organizational profile (`profile/README.md`): the baseline profile's body plus the
member-only details, alongside the private references annex. Checked out nested inside this repo
at `private/`.

### github-public
The baseline organizational profile (`.github`), public facing and the authored source of the
profile body. Holds only `profile/README.md`. Checked out nested inside this repo at `public/`.

### go-core
The Core SDK of the `go-elemental` standard: layered configuration, the process lifecycle, and
the logger — the process-level packages the standard's programs build on. Released; depends on
the standard library alone.

### go-database
The SQL infrastructure library of the `go-elemental` standard: the data layer's standard tier
(`database`) and reference-data seeding (`seed`) in the base module, with the PostgreSQL provider
as the `postgres` sub-module. Released, base and provider tagged independently; the base depends
on the standard library and `go-core`.

### go-web-sdk
The Application SDK for web services of the `go-elemental` standard: the HTTP server and its
configuration, routing, RFC 9457 problem responses, the liveness and readiness probes, the
paginated read contract, the error-to-problem mapping, and the middleware primitives (`web`),
with the middleware implementations in the `middleware` package. Released; depends on the
standard library and `go-core`.

### go-web-sdk-template
The web service template of the `go-elemental` standard: scaffolds an initial Go Elemental web
service application with `gonew` from the module rooted at `template/`, built on `go-core` and
`go-web-sdk` at pinned releases. Released under the `template/` tag prefix.

### go-web-service
The reference web service of the `go-elemental` standard: a single cohesive service that composes
the SDKs and infrastructure libraries at pinned releases and demonstrates each capability in
place on one declared stack (Postgres for SQL), grown in documented layers on the template —
exercising both tiers, with native use contained and a documented port list. Provider and engine
variants are never switches inside it; a variant is a separate focused reference. This is where
composition patterns are proven before they promote outward into the SDKs, the libraries, and
the template. Versionless until its 1.0.

## External references

Repositories outside the effort, cited as illustrations.

### claude-settings
The maintainer's user-scoped Claude Code configuration, kept under source control on a personal
account and symlinked into `~/.claude`: identity-level behavior (`behavior/`) loaded with every
session, per-tool notes (`tools/`) consulted on demand. Illustrates the user-scope harness
programming responsibility layer the docs harness pages describe.

## Prior R&D — Go web service architecture

### herald
Document classification web service, client, and CLI; the mature precedent for modern Go web architecture
and the source of truth for the extracted libraries. Builds on the TAU ecosystem. See also its
`.claude/`, `.github/`, and `deploy/` infrastructure.
Draw from: the Layered Composition Architecture lifecycle (cold start → hot start → graceful shutdown);
`pkg/` (contracts) vs `internal/` (implementations) with downward-only dependencies; three-phase config
finalize; the CLI-as-primitive philosophy; Go conventions and naming discipline.

## Prior R&D — TAU ecosystem

### tau-protocol, tau-format, tau-provider, tau-agent, tau-orchestrate, tau-examples
Tailored-Agentic-Units: appropriately layered Go libraries establishing reusable infrastructure for
agentic functionality; the original multi-module release proving ground.
Draw from: the layered dependency hierarchy; interface-in-root + vendor-in-submodule with explicit
registration (no `init()` side effects); constructor DI; the OTel-aligned observability conventions; the
`taiki-e/create-gh-release-action` + CHANGELOG release pattern inherited everywhere.

### tau-marketplace
The Claude Code plugin marketplace (dev-workflow, iterative-dev, github-cli, go-patterns,
project-management); the structural template for the claude-plugins host.
Draw from: the plugin anatomy (`.claude-plugin/` manifest + `SKILL.md` + `commands/` or `references/` +
CHANGELOG); `dev-workflow` (concept → phase → objective → task → review → release; plan-files vs
context-documents) and `iterative-dev` (the lightweight issue → branch → PR loop; the role boundary).

### tau-diagrams
A diagramming toolchain and documentation conventions.
Draw from: the Typst + Fletcher + CeTZ stack; the color-anchored Primer palette with dual-theme
`<picture>` output; the 3-tier audience model and 4-voice guide; the read-only `technical-writer`
analysis agent. (Deferred to the later docs/diagram standards.)

### tau-blog
A Jekyll → GitHub Pages blog (`~/tau/jaime`) started during Herald, for weekly updates.
Draw from: the capture → draft → publish pipeline and the calibrated voice/style profile
(`.claude/context/style-profile.md`). (Deferred to the later socialization standards.)

## Prior R&D — agentic dev process

### curiosity
A game-engine side project whose agentic workflow (`.claude/`) is the most evolved context engineering in
the estate; the primary inspiration for marathon.
Draw from: layered on-demand behavior loading; the context inventory with a belongs-here test; the
validated (`design/`) vs unvalidated (`concepts/`) split; append-only decision/reset logs with compaction
passes; the documentation-decay rule (a design-doc section is a defect once code expresses it);
deliberate single-source-of-truth in-repo context.

## Prior R&D — event-driven architecture

### signal-lab
Progressive NATS research for event-driven architecture and distributed work (phases 1–4 shipped, 5–9
planned). Research-grade, not production-hardened.
Draw from: the uniform signal envelope; the dot-delimited subject namespace; per-domain contract packages
(subjects, enums, headers, payloads); the bus abstraction with lifecycle-coordinated draining; the LCA
lifecycle adapted from Herald. (Deferred to the later events/NATS layer.)
