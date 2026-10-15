# Web client

How the reference architecture reaches the browser: the layers a web client is built from, what
each layer owns, who consumes them, and the order in which they're settled. It is the strategy
for v1.client, the goal that shows the stack beyond the terminal, and for the experiment.html
spikes that come before it. Everything below is direction set in planning (2026-10-13). The
spikes test it, and the experiment.html intake records what they proved in `html-artifacts.md`.

## The stack

The stack has three layers, a hypothesis that spike-html-conventions tests. A layer imports only
from the layers below it, and each works on its own.

1. **Foundation.** Native-only infrastructure, with no framework: the style baseline
   (experiment.html.spike-html-design) and JS/TS modules. The baseline's design facets (color,
   space, type) take swappable profiles over native elements, and it is released by git tag. Its
   tools split into three lateral concerns:
   - utilities, such as scroll;
   - structure, the grid and flex containers that pages lay out with, which replace a separate
     layouts layer;
   - meta components and variants, such as badges and hued button variants.
2. **Components.** The element tier, built with Lit. A component takes properties and attributes,
   emits events, and may hold its own UI state. It holds no services, fetches no data, and holds
   no application state. An application's internal element becomes a component once a second
   application needs it.
3. **Apps.** Two branches:
   - static content, such as the Pages sites, built vanilla on the foundation;
   - applications, with Lit, hosted standalone or embedded in a web service as its client, as
     herald's app is. An application keeps its own modules and views, and builds on an
     application SDK: the router, an API client, an auth hook, formatting, and the domain-service
     pattern. spike-html-conventions specifies the SDK and spike-html-apps builds it.

## Consumers

- **The reference client.** v1.client's application, embedded in go-web-service and served by
  its binary, just as slab is the service's command-line client. Its first demonstrations are
  the organizations, the document hierarchy, and a live view of v1.messaging's reactors over
  the WebSocket bridge (`messaging.md`). It is served from the same origin as the service.
- **External clients.** Clients hosted apart from any one service that call several services.
  They are post-1.0. spike-html-apps settles their strategy beside the embedded case, and the
  intake decides whether a goal follows.
- **Pages sites.** Native web, with no Jekyll and no site generator. The hypothesis that
  spike-html-hosting tests:
  - architecture is served as its own project site;
  - org, the coordinator, holds the organization's charter and its hosted artifacts, and links
    to every project site;
  - each code repository serves its own docs/ guide, which v1.repository-docs writes.
- **Persistent artifacts.** Reports, stats updates, and other artifacts kept over time, written
  as HTML and hosted in the coordinator.
- **Live Go demos.** Workspace Go packages running in a page through WebAssembly, if
  spike-html-go-wasm shows the size is acceptable.

## Where HTML is used

HTML is for persistent artifacts only. Workflow outputs, meaning task briefs, plan rounds,
and session briefs, stay Markdown, and so do the `context/` notes.

## Sequence

experiment.html runs its spikes one at a time, because each answer can change the spikes after
it. The spikes lock only their own repositories, so they run beside v1.messaging:

1. spike-html-design: the foundation's style baseline.
2. spike-html-conventions: the toolchain, TypeScript form, code structure, testing, lint, and
   component convention the stack builds on, and the application SDK's specification.
3. spike-html-packages: how the foundation and components are packaged and released by tag.
4. spike-html-apps: herald's embedded app (`~/code/herald/app`: a Go package embedding a
   bun-built Lit client) compared with other app configurations and with external clients over
   several services. It builds the application SDK, and its answer is v1.client's most direct
   input.
5. spike-html-hosting: repository docs and artifact hosting. It covers the Pages topology,
   native-web publishing, the coordinator's persistent artifacts, and the line between HTML and
   Markdown.
6. spike-html-go-wasm: live Go demos.

The intake then decides what v1.client builds, and v1.client's own planning writes its tasks.
v1.client starts once the intake has closed and v1.messaging has completed. It is the next goal
in the workspace's exclusive group, ahead of v1.ai, v1.data, v1.auth, and v1.admin-listener. If
v1.messaging completes before the intake closes, planning decides what holds the exclusive group
until it does.

## What lands later

- **Auth.** The reference client runs against the service's open API. v1.auth adds a client
  sweep task, which also decides whether an image is fetched as an `<img>` with a cookie or with
  a bearer token.
- **CORS.** A same-origin client sends no cross-origin requests. CORS's trigger in
  v1.middleware is the first client served cross-origin, such as an external client or a
  Pages-hosted page that calls a service.
- **The architecture repository's pointer.** `architecture/context/README.md` names docs-site as
  the owner of site hosting. v1.client's sites work repoints it to this layout.
