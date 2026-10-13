# Web client

How the reference architecture reaches the browser: the layers a web client is built from, what
each layer owns, who consumes them, and the order in which they're settled. It is the strategy
for v1.client, the goal that shows the stack beyond the terminal, and for the experiment.html
spikes that come before it. Everything below is direction set in planning (2026-10-13). The
spikes test it, and the experiment.html intake records what they proved in `html-artifacts.md`.

## The stack

Each layer is a cascade layer and a package of its own. A consumer stacks them in this order,
and a layer never reaches into one above it.

1. **Foundation.** The native style baseline, design facets (color, space, type) with swappable
   profiles over native elements and opt-in tools, released by git tag
   (experiment.html.spike-html-design). It owns tokens and element defaults and holds no layout
   or composition.
2. **Layouts.** Structural custom elements, a layout engine over CSS grid, and layout sheets:
   reusable templates for categories of page. This layer owns line length and aspect-ratio
   queries.
3. **Components.** Reusable custom elements. The spike decides whether they're packaged as one
   library, one package per component, or packages of related components.
4. **Apps.** A client application composed from the layers below and served to a reader.

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

1. spike-html-design: the foundation.
2. spike-html-components: how layouts and components are packaged, released, and layered over
   the foundation, consumed by tag.
3. spike-html-hosting: repository docs and artifact hosting. It covers the Pages topology,
   native-web publishing, the coordinator's persistent artifacts, and the line between HTML and
   Markdown.
4. spike-html-go-wasm: live Go demos.
5. spike-html-apps: herald's embedded app (`~/code/herald/app`: a Go package embedding a
   bun-built Lit client) compared with other app configurations and with external clients over
   several services. It runs last because it combines the other answers, and its answer is
   v1.client's most direct input.

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
