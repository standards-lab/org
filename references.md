# Repository references

Descriptive companion to `references.toml`. Each heading maps to one or more manifest keys; the remote
is in `references.toml` and the local checkout path is in `references.local.toml`.

Two kinds of entry: the **effort repositories** that make up the Standards Lab organization, and the
**prior R&D** that informs the standard. The prior R&D is input, not infrastructure to inherit: carry
its strengths, correct its debts, and re-derive a clean standard.

An entry's key joins the two manifests: to clone a missing repository, look up its remote in
`references.toml` and its directory in `references.local.toml`. Remotes live only in
`references.toml` and local paths only in the gitignored `references.local.toml`. An access caveat,
such as an account switch required before cloning, is recorded beside the affected entries here and
in the `references.toml` header.

A private annex extends this catalog: prior R&D from private engagements is catalogued in the
private repository, whose annex `references.toml` and `references.md` join this catalog when the
member checkout is present at `private/`. The two share one key namespace, and the annex only adds
entries, never redefining a public one, so a key resolves to the same entry in every checkout that
has it.

This catalog records the relationships between repositories and the prior R&D. A repository's stable
documentation describes itself and the repositories it depends on, and never cites prior R&D or a
sibling; it justifies a convention by what the convention does.

## Standards

A standard is a named, technology-specific implementation of an architecture that a set of
repositories declares alignment to. Definitions live in the
[architecture repository](https://github.com/standards-lab/architecture). Each standard has a
`[standards.<key>]` entry in `references.toml` with a `definition` URL and the `architecture` it
implements. A member repository's entry declares `standard = "<key>"`, so membership is declared
where the repository is cataloged and never listed a second time, and a derived standard
declares `derives = "<key>"`. A repository also declares its standard in its README's Standard
section: a link to the definition, and its own principles stated as enhancements.

A repository belongs to exactly one standard, the one whose author answers for it. Another
standard that finds its modules sufficient adopts them as ordinary dependencies at pinned
releases, never as members, per the
[downward-dependencies](https://github.com/standards-lab/architecture/blob/main/principles/downward-dependencies.md)
principle.

### go-elemental

The Go implementation of the Elemental Architecture, the organization's first standard, built
on the standard library. Defined at
[standards/go-elemental](https://github.com/standards-lab/architecture/blob/main/standards/go-elemental/README.md).
Members: `go-core`, `go-database`, `go-observability`, `go-storage`, `go-web-sdk`, and
`go-web-sdk-template` (released); and `go-web-service`, the reference web service (versionless
until its 1.0).
`dotnet-elemental` is anticipated as its derived standard
(`derives = "go-elemental"`).

## Effort repositories — Standards Lab

### claude-plugins
The plugin host for the organization, mirroring the structure of `tau-marketplace`. Ships the `marathon`
workflow plugin and its `marathon-roadmap` and `marathon-architecture` extensions. The harness level of the reference architecture.

### architecture
The organization's architecture layer: the canonical home for its architectures, standards,
principles, and a catalog of the repositories that implement them, published as plain markdown
with YAML front matter. It is the workspace's architecture repository, and its content is what the
planned organization documentation site will serve.

### org
This repository: the workspace context that coordinates the organization — `context/`, the
references catalog, the marathon anchor. The home of the organization's context about running itself, as
`architecture` is for the architecture's. Marathon is anchored here.

### github-private
The extended organizational profile (`profile/README.md`): the baseline profile's body plus the
member-only details, alongside the private references annex. Checked out nested inside this repo
at `private/`.

### github-public
The baseline organizational profile (`.github`), public facing and the authored source of the
profile body. Holds only `profile/README.md`. Checked out nested inside this repo at `public/`.

### go-core
The Core SDK of the `go-elemental` standard: the process-level primitives every program of the
standard builds on, and the process half of the integration toolkit. Its README lists the
packages. Released; depends on the standard library alone.

### go-database
The SQL infrastructure library of the `go-elemental` standard: the infrastructure service over
a pool and the database admin service over `sqlate`, with the PostgreSQL provider as the
`postgres` sub-module. Its README lists the packages. Released, base and provider tagged
independently; the base depends on the standard library, `go-core`, and `sqlate`.

### go-observability
The observability infrastructure library of the `go-elemental` standard: the OpenTelemetry
configuration and process lifecycle, the trace-correlating log handler, the HTTP server
middleware, and the request-ID source function, with the OTLP exporters as the `otlp`
sub-module. Its README lists the packages. Released, base and `otlp` sub-module tagged
independently; the base depends on the standard library, `go-core`, and the stable v1
OpenTelemetry API and SDK, plus `otelhttp` as a stated v0 exception.

### go-storage
The object storage infrastructure library of the `go-elemental` standard: the standard-tier
`Client` interface over the operations Azure Blob and S3 share, the `Store` lifecycle wrapper,
the provider key constraints, the error sentinels, and the `storagetest` conformance suite,
with each provider a sub-module of its own. Its README lists the packages. Released; the base
depends on the standard library and `go-core` alone.

### go-web-sdk
The Application SDK for web services of the `go-elemental` standard: the HTTP surface a service
is built on, its middleware, and the HTTP half of the integration toolkit. Its README lists the
packages. Released; depends on the standard library and `go-core`.

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

### sqlate
The SQL templating library: authored `.sql` files made dynamic and composable, with the
PostgreSQL dialect as the `postgres` sub-module and the conventions linter as the `sqlint`
sub-module. A standalone library any Go project can adopt, adjacent to the `go-elemental`
standard rather than a member of it: the standard's libraries consume it, and it is the
blueprint for how a domain-specific language gains host-language support. The base module
depends on the standard library alone; the base and each sub-module are tagged independently.

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
registration (no `init()` side effects); constructor DI; the hand-rolled Observer/Event bus, whose
severity levels and event shape are mapped onto OpenTelemetry's model without taking an
OpenTelemetry dependency; the `taiki-e/create-gh-release-action` + CHANGELOG release pattern
inherited everywhere.

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
