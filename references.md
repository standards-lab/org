# Repository references

Descriptive companion to `references.toml`. Each repository heading names one or more manifest
keys, each linked to its remote, which `references.toml` holds; the local checkout path is in
`references.local.toml`.

Three kinds of entry: the **effort repositories** that make up the Standards Lab organization, the
**archived experiments** whose results the effort took in, and the **prior R&D** that informs the
standard. The prior R&D is input, not infrastructure to inherit: carry its strengths, correct its
debts, and re-derive a clean standard. The catalog also lists the **upstream sources** an effort
repository reads but never writes.

An entry's key joins the two manifests: to clone a missing repository, look up its remote in
`references.toml` and its directory in `references.local.toml`. Remotes live only in
`references.toml` and local paths only in the gitignored `references.local.toml`. An access caveat,
such as an account switch required before cloning, is recorded beside the affected entries here.

A private annex extends this catalog: prior R&D from private engagements is catalogued in the
private repository, whose annex `references.toml` and `references.md` join this catalog when the
member checkout is present at `private/`. The two share one key namespace, and the annex only adds
entries, never redefining a public one, so a key resolves to the same entry in every checkout that
has it.

This catalog records the relationships between repositories and the prior R&D. A repository's stable
documentation describes itself and the repositories it depends on, and justifies a convention by
what it does; third-party provenance lines and worked-case links excepted.

This catalog is also the workspace's one lineage record. An entry's **Lineage** line holds where
the repository's code came from in the workspace's own repositories and spikes, per repository at
a commit. A README or working context that needs the lineage links to the entry, and a new seed
adds its sources to its entry's line.

## Standards

A standard is a named, technology-specific implementation of an architecture that a set of
repositories declares alignment to. Definitions live in the
[architecture repository](https://github.com/standards-lab/architecture). Each standard has a
`[standards.<key>]` entry in `references.toml` with a `definition` URL and the `architecture` it
implements, and `status = "draft"` while its definition is a draft. A member repository's entry declares `standard = "<key>"`, so membership is declared
where the repository is cataloged and never listed a second time. A repository also declares its
standard in its README: a link to the definition, and its own principles stated as enhancements.
A repository adjacent to a standard (sqlate, blobfs, docproc, docinfer) and the infrastructure
outside the tiers (ai-hosting) declare none.

A repository belongs to exactly one standard, the one whose author answers for it. Another
standard that finds its modules sufficient adopts them as ordinary dependencies at pinned
releases, never as members.

### go-elemental

The Go implementation of the Elemental Architecture, the organization's first standard, built
on the standard library. Defined at
[standards/go-elemental](https://github.com/standards-lab/architecture/blob/main/standards/go-elemental/README.md).
Members: `go-core`, `go-database`, `go-observability`, `go-storage`, `go-web-sdk`,
`go-web-sdk-template`, `go-cli-sdk`, `go-cli-sdk-template`, `go-messaging`, and `go-ai`, the
model client (released); and `go-web-service`, the reference web service (versionless until its
1.0).

### web-elemental

The web implementation of the Elemental Architecture, a draft beside Go Elemental, on the browser
platform with Lit as its one runtime dependency and marked for Markdown alone. Defined at
[standards/web-elemental](https://github.com/standards-lab/architecture/blob/main/standards/web-elemental/README.md).
Members, all released: `web-tooling`, `web-foundation`, `web-icons`, `web-components`,
`web-app-sdk`, and `webappgen`. Workflows, configs, and scripts copied between members come from
web-tooling 1ff49e4; web-foundation 72e759f, ceaab61, and 309e6b9; web-components a659353,
3f32a8f, 49f819b, and 6052866; and web-app-sdk fbaba50 and 35d1ab5.

## Effort repositories — Standards Lab

### [claude-plugins](https://github.com/standards-lab/claude-plugins)
The plugin host for the organization, mirroring the structure of `tau-marketplace`. Ships the `marathon`
workflow plugin and its `marathon-architecture` extension.

### [architecture](https://github.com/standards-lab/architecture)
The organization's architecture layer: the canonical home for its architectures, standards,
principles, and a catalog of the repositories that implement them, published as plain markdown
with YAML front matter. It is the workspace's architecture repository, and its content is what the
planned organization documentation site will serve.

### [org](https://github.com/standards-lab/org)
This repository: the workspace context that coordinates the organization — `context/`, the
references catalog, the marathon anchor. The home of the organization's context about running itself, as
`architecture` is for the architecture's. Marathon is anchored here.

### [github-private](https://github.com/standards-lab/.github-private)
The extended organizational profile (`profile/README.md`): the baseline profile's body plus the
member-only details, alongside the private references annex. Checked out nested inside this repo
at `private/`.

### [github-public](https://github.com/standards-lab/.github)
The baseline organizational profile (`.github`), public facing and the authored source of the
profile body. Holds only `profile/README.md`. Checked out nested inside this repo at `public/`.

### [go-core](https://github.com/standards-lab/go-core)
The Core SDK of the `go-elemental` standard: the process-level primitives every program of the
standard builds on, and the process half of the integration toolkit. Its README lists the
packages. Released; depends on the standard library alone.

### [go-database](https://github.com/standards-lab/go-database)
The SQL infrastructure library of the `go-elemental` standard: the infrastructure service over
a pool, the database admin service over `sqlate`, and the general sweeper (`sweep`), which runs a
service's reclaimers as one reactor, with the PostgreSQL provider as the `postgres` sub-module
and pgvector's metric resolution and integration tier as the `postgres/pgvector` sub-module.
Its README lists the packages. Released, base and provider tagged independently; the base depends
on the standard library, `go-core`, and `sqlate`.

### [go-observability](https://github.com/standards-lab/go-observability)
The observability infrastructure library of the `go-elemental` standard: the OpenTelemetry
configuration and process lifecycle, the trace-correlating log handler, the HTTP server
middleware, and the request-ID source function, with the OTLP exporters as the `otlp`
sub-module. Its README lists the packages. Released, base and `otlp` sub-module tagged
independently; the base depends on the standard library, `go-core`, and the stable v1
OpenTelemetry API and SDK, plus `otelhttp` as a stated v0 exception.

### [go-storage](https://github.com/standards-lab/go-storage)
The object storage infrastructure library of the `go-elemental` standard: the standard-tier
`Client` interface over the operations Azure Blob and S3 share, the `Store` lifecycle wrapper,
the provider key constraints, the error sentinels, and the `storagetest` conformance suite,
with each provider a sub-module of its own. Its README lists the packages. Released; the base
depends on the standard library and `go-core` alone.

### [go-ai](https://github.com/standards-lab/go-ai)
The model client infrastructure library of the `go-elemental` standard: one client over the
standard wire formats the model engines serve natively (Chat Completions, Anthropic Messages, and
Open Responses), with generation, streaming, embedding, transcription, and the model listing, and
its convenience CLI, `infer`, in the nested module `tools/infer`. The base depends on the standard
library alone. Released; built from spike-ai-pipeline-client.

### [go-messaging](https://github.com/standards-lab/go-messaging)
The messaging infrastructure library of the `go-elemental` standard: the broker contract and
subscriptions, the runtime with its error hook, outbox emission and inbox idempotency, the memory
provider, and the `messagingtest` conformance suite, with the postgres engine, which also holds
the outbox and inbox retentions, and the NATS JetStream provider as sub-modules. Its events are
go-core's CloudEvents `event` type and its consumers go-core `reactor`s. Released, base and
sub-modules tagged independently. Lineage: spike-messaging 46a9f5a.

### [go-web-sdk](https://github.com/standards-lab/go-web-sdk)
The Application SDK for web services of the `go-elemental` standard: the HTTP surface a service
is built on, its middleware, and the HTTP half of the integration toolkit. Its README lists the
packages. Released; depends on the standard library and `go-core`. Lineage: spike-html-apps
2883d78, whose `sdk` package became `web.NewClient` (v0.18.0).

### [go-web-sdk-template](https://github.com/standards-lab/go-web-sdk-template)
The web service template of the `go-elemental` standard: scaffolds an initial Go Elemental web
service application with `gonew` from the module rooted at `template/`, built on `go-core` and
`go-web-sdk` at pinned releases. Released under the `template/` tag prefix.

### [go-cli-sdk](https://github.com/standards-lab/go-cli-sdk)
The Application SDK for command-line tools of the `go-elemental` standard: package `cli`, a
dispatcher on the standard library's `flag` whose commands build only the `go-core` graph nodes
they declare, and package `scenario`, which runs a tool's narrated scenarios as `cli` commands.
Released; depends on the standard library and `go-core`.

### [go-cli-sdk-template](https://github.com/standards-lab/go-cli-sdk-template)
The CLI template of the `go-elemental` standard: scaffolds an initial Go Elemental command-line
application with `gonew` from the module rooted at `template/`, built on `go-core` and
`go-cli-sdk` at pinned releases. Released under the `template/` tag prefix.

### [go-web-service](https://github.com/standards-lab/go-web-service)
The reference web service of the `go-elemental` standard: a single cohesive service that composes
the SDKs and infrastructure libraries at pinned releases and demonstrates each capability in
place on one declared stack (Postgres for SQL), grown in documented layers on the template —
exercising both tiers, with native use contained and a documented port list. Provider and engine
variants are never switches inside it; a variant is a separate focused reference. This is where
composition patterns are proven before they promote outward into the SDKs, the libraries, and
the template. Versionless until its 1.0.

### [sqlate](https://github.com/standards-lab/sqlate)
The SQL templating library: authored `.sql` files made dynamic and composable, with the
PostgreSQL dialect as the `postgres` sub-module, pgvector's SQL layer as the `postgres/pgvector`
sub-module, and the conventions linter as the `sqlint` sub-module. A standalone library any Go project can adopt, adjacent to the `go-elemental`
standard rather than a member of it: the standard's libraries consume it, and it is the
blueprint for how a domain-specific language gains host-language support. The base module
depends on the standard library alone; the base and each sub-module are tagged independently.

### [blobfs](https://github.com/standards-lab/blobfs)
The virtual-directory metadata library: a SQL-backed directory and file-metadata layer over any
object store, for any Go project on an engine it ships DDL for, with the PostgreSQL engine as the
`postgres` sub-module owning its migration set. Adjacent to the `go-elemental` standard, as
`sqlate` is: go-web-service composes it, and go-storage's keys stay unaware of it. Promoted from
spike-blobfs. Its `docs/` guide describes the library; the base and each sub-module are tagged
independently.

### [docproc](https://github.com/standards-lab/docproc)
The processing library, released (v1.ai.processing): it prepares messages, documents, and streams
for native model capabilities, running mature system tools through adapters it owns. Its own
system tools, docproc-ooxml, docproc-mail, and docproc-onenote, are nested under `tools/<format>`,
each with its own toolchain and tagged `tools/<format>/v<version>`. Adjacent to the `go-elemental`
standard, as `sqlate` is. Its references are spike-ai-pipeline-processing and the spike's three
CLIs.

### [docinfer](https://github.com/standards-lab/docinfer)
The inference library, released (v1.ai.inference): above go-ai's tier, it runs vision,
transcription, extraction, chunking, and embedding over docproc's dispatched parts through go-ai,
with a result cache behind an interface. Its PostgreSQL cache, `docinfer/postgres`, and its
command, `cmd/docinfer`, are nested modules, each tagged on its own. Adjacent to the `go-elemental`
standard, as `sqlate` is. Its reference is spike-ai-pipeline's docinfer. Lineage:
spike-ai-pipeline 6a631f1; spike-ai-pipeline-processing 4402f7c (v0.9.0); and, for
`cmd/docinfer`, go-cli-sdk-template template/v0.3.1, docproc ddb1c07 and 6f955e3, and go-ai
72f1b98 (`tools/infer`).

### [ai-hosting](https://github.com/standards-lab/ai-hosting)
The hosting layer of the AI capability (v1.ai.hosting): the specification the organization's
local model hosts follow. It holds the `hosting` command suite, host-class profiles, each host
class's vLLM rows with their install, update, and uninstall, agentgateway's compose stack with
overflow to Azure AI Foundry, a runbook per host class, and the upgrade-evaluation convention and
its skill; admin tooling joins after experiment.admin's intake. Low-level infrastructure outside
the five tiers, beside the harness repositories, and the one hosting repository on each
workstation. Its references are spike-model-engines, spike-model-gateway, spike-model-hosting,
spike-hosting-upgrades, and spike-model-layout.

### [docs-corpus](https://github.com/standards-lab/docs-corpus)
The real workloads' corpus (corpus.docs): the redistributable samples docproc's and docinfer's
acceptance and capstone.data-intelligence run over, with each file's source, license, truth, and
scenarios in its manifest. Git holds the manifest, truth, and check; the files are release assets
on each version's tag. It is built from scratch, with the local pool at `~/docs` as its reference,
and becomes public when v1 is published.

### [web-tooling](https://github.com/standards-lab/web-tooling)
The browser-test and screenshot tooling the web repositories take by tag: the browser driver, the
test server, the process helper, the `web-screenshots` bin, and a helper that runs a browser suite
in a pinned DejaVu font set, spike-html-go-wasm's. Released (v1.client.stack). A member of the
`web-elemental` standard (above). Lineage: spike-html-apps-tooling d77877b;
spike-html-apps-foundation 9c15b8d, 7e5d6c2, and 6e9669f; spike-html-apps-components 2fded6d and
8ba2be0; spike-html-apps-noctis 7d7351a; spike-html-apps-sdk ca3089e; spike-html-apps a6ff18d;
and spike-html-go-wasm f026de3.

### [web-foundation](https://github.com/standards-lab/web-foundation)
The native foundation: CSS and ES modules with no dependencies. Released at v0.4.0
(v1.client.stack): baseline sheets with five color role hooks and control, focus, and tint
tokens; color, space, and type profiles; tool sheets with utility classes; opt-in
`glyph-button.css`, `dialog.css`, `layout.css`, and `motion.css`; DOM helpers, the registry and
keys family (`bindings()`), theme preferences (no menu or keys help), the conformance check, and
Intl and JSON formatters (`formatJson`, `tryFormatJson`). A member of the `web-elemental` standard
(above). Lineage: spike-html-apps-foundation 07ba39a (v0.3.1) and 9c15b8d;
spike-html-packages-foundation 5dd8a58; spike-html-design fe75220 (v0.7.0);
spike-html-apps-tooling d77877b; spike-html-apps-components 2fded6d; spike-html-go-wasm f026de3;
spike-html-hosting-sdk f49087e; and spike-html-hosting-components 9fa5850.

### [web-icons](https://github.com/standards-lab/web-icons)
The icons layer, `@standards-lab/web-icons`, the iconography convention for every web client and
page. Released at v0.1.0 (v1.client.stack): Octicons 19.42.0's 780 icons and 23 aliases, a string
ES module each at `@standards-lab/web-icons/octicon/<name>`, under Octicons' MIT LICENSE; the
`iconFill()` Vite plugin at `@standards-lab/web-icons/vite`, which fills
`<svg data-icon="set/name">` markers in dev and build; no runtime dependency, and vite ^8.3.8 an
optional peer. A member of the `web-elemental` standard (above). Lineage:
spike-html-hosting-icons 8b8d955; spike-html-hosting-sdk f49087e, its fill.

### [web-components](https://github.com/standards-lab/web-components)
The Lit components with a shadow root, on web-foundation as a peer: the chat thread, composer,
upload dropzone, stage timeline, and the rest of the application set. Released at v0.2.0
(v1.client.stack): fourteen Lit elements (new: `sl-settings`, `sl-shortcuts`) and `markdown()`,
peering web-foundation `^0.4.0` and web-icons `^0.1.0`, on lit and marked (Markdown only); no
`json.js`. A member of the `web-elemental` standard (above). Lineage: spike-html-apps-components
10dde51; spike-html-hosting-components 9fa5850; spike-html-apps-foundation 618cb83 and 07ba39a;
spike-html-design fe75220; and spike-html-go-wasm f026de3.

### [web-app-sdk](https://github.com/standards-lab/web-app-sdk)
The web application SDK: the router on URLPattern and the Navigation API, the API client, the
token provider, the domain-service pattern, and the resumable event sequence with its
NATS-over-WebSocket adapter. Released at v0.1.0 (v1.client.stack): six subpaths (`api`, `auth`,
`router`, `service`, `events`, `events/nats`), no runtime dependency, and optional
`@nats-io/nats-core` and `@nats-io/jetstream` `^3.5.0` peers, imported only by `events/nats`. A
member of the `web-elemental` standard (above). Lineage: spike-html-apps-sdk df65376 (v0.4.0).

### [webappgen](https://github.com/standards-lab/webappgen)
The web client CLI: it writes a project for each kind of app on the stack, from files embedded in
its Go binary. Released (v1.client.stack) as linux and darwin binaries on amd64 and arm64, with
SHA256SUMS. v0.1.0: `new standalone`, a static app on the layers by exact tag, with Settings,
Help, web-icons' glyphs, and its own README. v0.2.0: `new service-app` and `inject service-app`,
served by go-web-sdk v0.18.0's `NewClient`, at gonew x/tools v0.53.0 and go-web-sdk-template
template/v0.16.0. v0.2.1: written projects and its own sources carry no workspace provenance
lines. Renamed from web-app-sdk-template. A member of the `web-elemental` standard (above).
Lineage: spike-html-apps 2883d78, its starter app; go-cli-sdk-template a4d314c; go-web-sdk
c0c8267; herald c697566; and, for its fixtures, go-web-sdk-template 7e5f330 and go-web-service
31b92a4.

## Experiments — archived spikes

A running spike is a sub-goal of its experiment, `experiment.<topic>.<spike>`, in
`roadmap/`, whose goal file records its remote and local path; the experiment's intake records
each spike here and archives its remote. A new spike follows the hosting convention in
`.claude/marathon.toml`'s `[workspace.experiments]`. A spike whose sync has landed while its
experiment awaits intake, or whose archive a goal's sync owns, is listed here unarchived.

### [spike-sql-dsl](https://github.com/JaimeStill/spike-sql-dsl)
Asked whether the whole SQL-to-Go layer can run on authored SQL files instead of a Go statement
vocabulary. Its library became sqlate.

### [spike-blobfs](https://github.com/JaimeStill/spike-blobfs)
Asked whether the blobfs design, a SQL-backed virtual directory and file-metadata library over any
object store, holds up when built. Promoted to blobfs.

### [spike-messaging](https://github.com/JaimeStill/spike-messaging)
Asked whether one broker-agnostic event and reactor contract, on CloudEvents with outbox emission,
runs on JetStream and in memory. Promoted to go-messaging and go-core's reactor and event packages.

### [spike-cli-architecture](https://github.com/JaimeStill/spike-cli-architecture)
Asked whether a CLI can drop cobra for a dispatcher on the standard library's flag and bring up
only the dependencies each command declares. Its graph and lifecycle went into go-core, its cli
package into go-cli-sdk.

### [spike-s3-storage](https://github.com/JaimeStill/spike-s3-storage)
Asked whether go-storage's `Client` interface holds over S3, with an aws-sdk-go-v2 provider
validated against SeaweedFS's S3 gateway, and whether the blobfs CLI runs unchanged on it. Its
`s3` module became go-storage's `s3` provider.

### [spike-harness-driver](https://github.com/JaimeStill/spike-harness-driver)
Asked whether Go can drive an external agent harness (Pi, Claude Code, OpenCode) as the
infrastructure for agentic work. Answered yes, with a harness-session and a model-client
surface; experiment.harness's intake archives it. Its `model` client stands in for go-ai in
spike-model-hosting.

### [spike-html-design](https://github.com/JaimeStill/spike-html-design)
Asked whether one tagged, native baseline of design facets (color, space, and type, each with
swappable profiles) can be consumed by version, by git tag and by pinned CDN URL, and extended by
other packages. Answered yes. Its v0.7.0 is the frozen reference for web-foundation's style
baseline.

### [spike-html-conventions](https://github.com/JaimeStill/spike-html-conventions)
Asked which conventions the native web stack builds on. Answered with Node and pnpm, `.ts`
released through a build, layer direction enforced by resolved file, Lit shadow components, a
DOM-free unit tier and a browser tier on the raw DevTools pipe, and ESLint with Prettier. They are
v1.client.stack's conventions (`context/web-client.md`).

### [spike-html-packages](https://github.com/JaimeStill/spike-html-packages)
Asked how the stack's layers are packaged and released by tag. Answered with one repository per
layer, each released by a plain `vX.Y.Z` tag on a pack commit, cut by hand. v1.client.stack's
repositories release this way.

### [spike-html-packages-foundation](https://github.com/JaimeStill/spike-html-packages-foundation), [spike-html-packages-sdk](https://github.com/JaimeStill/spike-html-packages-sdk), [spike-html-packages-components](https://github.com/JaimeStill/spike-html-packages-components)
spike-html-packages' layer repositories: the foundation, the application SDK, and the
components, released by tag. spike-html-apps' layer repositories were copied from their tags.

### [spike-html-apps](https://github.com/JaimeStill/spike-html-apps)
Asked what a client application on the stack looks like, and whether its layers hold when they
compose the capstone's UI primitives live over a backend. Answered yes, in a pipeline app over a
Go emulator and an external client over two services, served by a Go `NewClient`. Its layers
seeded the web client stack, its `sdk` package go-web-sdk's `NewClient`, and its app shape is
v1.client.reference's.

### [spike-html-apps-foundation](https://github.com/JaimeStill/spike-html-apps-foundation), [spike-html-apps-components](https://github.com/JaimeStill/spike-html-apps-components), [spike-html-apps-sdk](https://github.com/JaimeStill/spike-html-apps-sdk), [spike-html-apps-noctis](https://github.com/JaimeStill/spike-html-apps-noctis), [spike-html-apps-tooling](https://github.com/JaimeStill/spike-html-apps-tooling)
spike-html-apps' layer repositories: the foundation, the components, the application SDK, an
extension color profile, and the browser-test and screenshot tooling. They are the references
for web-foundation (from foundation v0.3.1), web-components, web-app-sdk, and web-tooling.

### [spike-html-hosting](https://github.com/JaimeStill/spike-html-hosting)
Asked what the repositories' Pages sites and the hosting of persistent artifacts look like as
native web. Answered with an org site at the origin root and project sites under `/<repo>/`,
built by Vite from Markdown through one site SDK and one documentation components layer. It is
the reference for the `sites` goal (`context/sites.md`).

### [spike-html-hosting-components](https://github.com/JaimeStill/spike-html-hosting-components), [spike-html-hosting-architecture](https://github.com/JaimeStill/spike-html-hosting-architecture), [spike-html-hosting-sqlate](https://github.com/JaimeStill/spike-html-hosting-sqlate), [spike-html-hosting-sdk](https://github.com/JaimeStill/spike-html-hosting-sdk), [spike-html-hosting-icons](https://github.com/JaimeStill/spike-html-hosting-icons)
spike-html-hosting's layer and site repositories: the documentation components, the architecture
and sqlate stand-in sites, the site SDK, and the icons layer. They are the references for the
`sites` goal's tasks. web-icons is seeded from spike-html-hosting-icons (8b8d955), its marker fill
from spike-html-hosting-sdk's `src/icons.ts`; web-components' sl-settings and sl-shortcuts come
from spike-html-hosting-components v0.5.1.

### [spike-html-go-wasm](https://github.com/JaimeStill/spike-html-go-wasm)
Asked whether a page can run workspace Go live in the browser through `GOOS=js` WebAssembly:
library demos, handlers called in-process, and a Go playground. Answered yes, as prebuilt modules
or as source compiled in the page; each site chooses its demos (`context/sites.md`). Its pinned
DejaVu font set is web-tooling's.

### [spike-model-hosting](https://github.com/JaimeStill/spike-model-hosting)
Asked what llama.cpp's Vulkan router on the Framework establishes for the self-hosted stack.
Answered with a host-class profile, three consumers at once, tool-call reliability, a memory
budget on unified memory, and a measurement method.

### [spike-model-engines](https://github.com/JaimeStill/spike-model-engines)
Asked which engine, llama.cpp or vLLM, serves each workstation, in a container or on bare metal.
Answered llama.cpp in a container on both host classes, in ggml's CUDA image on the GB10s and its
Vulkan image on the Framework; vLLM superseded that answer on every workstation. Its `host/`
suite, the hosts' vLLM rows and scripts, is the source v1.ai.hosting's design reviewed.

### [spike-hosting-upgrades](https://github.com/JaimeStill/spike-hosting-upgrades)
Asked what convention evaluates an upgrade to the hosting stack (hardware configuration, model
host, or models) from what others publish: a project-local Claude Code skill writes dated, cited
reports of ranked upgrades, and anchors A and B verify each on our host. Answered with the
survey-and-measure convention and its cadence, which ai-hosting rebuilt on vLLM.

### [spike-model-layout](https://github.com/JaimeStill/spike-model-layout)
Asked how models are placed across the memory pools of gb10-1, gb10-2 and the Framework, and what
context and how many concurrent sessions each placed model holds. Answered with one model to a
vLLM instance and one heavy purpose to a GPU: gb10-1 serves the agent, gb10-2 vision, and the
Framework audio beside embedding.

### [spike-model-gateway](https://github.com/JaimeStill/spike-model-gateway)
Asked which gateway fronts the vLLM layout and how access to the model APIs is restricted without
the tailnet. Answered with agentgateway v1.7.0 on gb10-2: purpose aliases on per-purpose base
URLs, gateway-issued keys with priority, buckets, and budgets, and overflow to Azure AI Foundry for
agent and vision; ai-hosting's gateway is built from it.

### [spike-ai-pipeline](https://github.com/JaimeStill/spike-ai-pipeline)
Asked whether a Go model client and a message, document, and stream processing layer can carry a
transactional, evented pipeline that a browser follows live. Answered yes for the client and
processing layers, which v1.ai builds as go-ai and docproc; the pipeline and
browser moved to capstone.data-intelligence. It served set A on gb10-2 from its own preset, which
spike-model-layout reads, never writes, while moving gb10-2 onto spike-model-engines' preset.

### [spike-ai-pipeline-client](https://github.com/JaimeStill/spike-ai-pipeline-client), [spike-ai-pipeline-processing](https://github.com/JaimeStill/spike-ai-pipeline-processing), [spike-ai-pipeline-office](https://github.com/JaimeStill/spike-ai-pipeline-office), [spike-ai-pipeline-mail](https://github.com/JaimeStill/spike-ai-pipeline-mail), [spike-ai-pipeline-onenote](https://github.com/JaimeStill/spike-ai-pipeline-onenote)
spike-ai-pipeline's layer repositories: the model client, the processing layer, and the Open XML,
mail, and OneNote CLIs. They are the references for go-ai, docproc, and docproc's system tools
docproc-ooxml, docproc-mail, and docproc-onenote.

### [spike-graph](https://github.com/JaimeStill/spike-graph)
Asked whether go-core's candidate `graph` and `lifecycle` API makes go-web-service's composition
root and slab's CLI shorter and their layer placement legible, on real infrastructure. Answered
yes, without arity adapters, with every node's layer stated at its Define line: `mise run measure`
took go-web-service's root from 460 to 411 code lines and 21 closures to none, and slab's from 207
to 202 lines and 11 closures to 7. The layer tests, the name check, and the runner's "service
failed" and "server stopped" logs are proven only by tests; the rest ran against real
infrastructure. go-core's breaking release ships its API, whose reasoning is go-core's
`docs/graph-design.md`.

## Upstream sources

### [octicons](https://github.com/primer/octicons)
GitHub's Primer icon set. web-icons reads it at 9a274e2 (tag `@primer/octicons@19.42.0`), as
spike-html-hosting-icons did before it, and copies its LICENSE; never written.

### [pgvector](https://github.com/pgvector/pgvector)
The PostgreSQL vector extension. sqlate's and go-database's compose images build it from source,
pinned by `ARG PGVECTOR_VERSION`, and sqlate's `postgres/pgvector` names its native forms and
metrics; never written.

### [pgvector-go](https://github.com/pgvector/pgvector-go)
pgvector's Go types. Applications use it directly; go-database's `postgres/pgvector` requires it
for its integration tier only; never written.

## Prior R&D — Go web service architecture

### [herald](https://github.com/JaimeStill/herald)
Document classification web service, client, and CLI; the mature precedent for modern Go web architecture
and the source of truth for the extracted libraries. Builds on the TAU ecosystem. See also its
`.claude/`, `.github/`, and `deploy/` infrastructure.
Draw from: the Layered Composition Architecture lifecycle (cold start → hot start → graceful shutdown);
`pkg/` (contracts) vs `internal/` (implementations) with downward-only dependencies; three-phase config
finalize; the CLI-as-primitive philosophy; Go conventions and naming discipline.

## Prior R&D — TAU ecosystem

### [tau-protocol](https://github.com/tailored-agentic-units/protocol), [tau-format](https://github.com/tailored-agentic-units/format), [tau-provider](https://github.com/tailored-agentic-units/provider), [tau-agent](https://github.com/tailored-agentic-units/agent), [tau-orchestrate](https://github.com/tailored-agentic-units/orchestrate), [tau-examples](https://github.com/tailored-agentic-units/examples)
Tailored-Agentic-Units: appropriately layered Go libraries establishing reusable infrastructure for
agentic functionality; the original multi-module release proving ground.
Draw from: the layered dependency hierarchy; interface-in-root + vendor-in-submodule with explicit
registration (no `init()` side effects); constructor DI; the hand-rolled Observer/Event bus, whose
severity levels and event shape are mapped onto OpenTelemetry's model without taking an
OpenTelemetry dependency; the `taiki-e/create-gh-release-action` + CHANGELOG release pattern
inherited everywhere.

### [tau-platform](https://github.com/tailored-agentic-units/tau-platform)
The TAU ecosystem's coordination repository: standards drafts, planning artifacts, and an
`archive/` of retired work. Draw from: `archive/tau-runtime/claude-classify-docs/`, the prior art
`context/ai-strategy.md` cites for driving Claude Code as a harness.

### [tau-marketplace](https://github.com/tailored-agentic-units/tau-marketplace)
The Claude Code plugin marketplace (dev-workflow, iterative-dev, github-cli, go-patterns,
project-management); the structural template for the claude-plugins host.
Draw from: the plugin anatomy (`.claude-plugin/` manifest + `SKILL.md` + `commands/` or `references/` +
CHANGELOG); `dev-workflow` (concept → phase → objective → task → review → release; plan-files vs
context-documents) and `iterative-dev` (the lightweight issue → branch → PR loop; the role boundary).

### [tau-diagrams](https://github.com/tailored-agentic-units/diagrams)
A diagramming toolchain and documentation conventions.
Draw from: the Typst + Fletcher + CeTZ stack; the color-anchored Primer palette with dual-theme
`<picture>` output; the 3-tier audience model and 4-voice guide; the read-only `technical-writer`
analysis agent. (Deferred to the later docs/diagram standards.)

### [tau-blog](https://github.com/tailored-agentic-units/jaime)
A Jekyll → GitHub Pages blog (`~/tau/jaime`) started during Herald, for weekly updates.
Draw from: the capture → draft → publish pipeline and the calibrated voice/style profile
(`.claude/context/style-profile.md`). (Deferred to the later socialization standards.)

## Prior R&D — agentic dev process

### [curiosity](https://github.com/JaimeStill/curiosity)
A game-engine side project whose agentic workflow (`.claude/`) is the most evolved context engineering in
the estate; the primary inspiration for marathon.
Draw from: layered on-demand behavior loading; the context inventory with a belongs-here test; the
validated (`design/`) vs unvalidated (`concepts/`) split; append-only decision/reset logs with compaction
passes; the documentation-decay rule (a design-doc section is a defect once code expresses it);
deliberate single-source-of-truth in-repo context.

## Prior R&D — event-driven architecture

### [signal-lab](https://github.com/JaimeStill/signal-lab)
Progressive NATS research for event-driven architecture and distributed work (phases 1–4 shipped, 5–9
planned). Research-grade, not production-hardened.
Draw from: the uniform signal envelope; the dot-delimited subject namespace; per-domain contract packages
(subjects, enums, headers, payloads); the bus abstraction with lifecycle-coordinated draining; the LCA
lifecycle adapted from Herald. (Deferred to the later events/NATS layer.)

## Prior R&D — local model hosting

### [personal-agents](https://github.com/JaimeStill/personal-agents)
The architect's self-hosted model setup: a llama.cpp router on the Framework desktop (Strix Halo,
Vulkan) driven by Pi over Tailscale, the `outpost` admin dispatcher, and a measured reference on
model tiers, context sizing, and memory footprint. Archived; its README links forward to
`ai-hosting`.
Draw from: presets keyed by capability tier (a recipe per model, instantiated per tier); the
context-sizing and memory-footprint methods; the deferral test in `capabilities/gateway-proxy.md`;
outpost's verb set.

### [dotfiles](https://github.com/JaimeStill/dotfiles)
The architect's workstation configuration, including the two GB10s' setup: DGX OS host
preparation as named, idempotent steps (`platforms/dgx/system-setup.sh`) and the QSFP link between
them, with its measured bandwidth. Read-only.
Draw from: the scripted-step pattern; the QSFP link record.
