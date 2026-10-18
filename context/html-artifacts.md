# HTML artifacts

Whether the organization's human-facing artifacts are written as HTML beside Markdown, and how
they are styled, served, and reviewed. The prompt is "Using Claude Code: the unreasonable
effectiveness of HTML" (claude.dev/blog, 2026-05-20). It argues that HTML wins on diagrams,
interactivity, navigating long documents, and sharing by link. It concedes the token cost, and
it doesn't address diffability, hosting, or access control.

experiment.html answers the question in six spikes, run one at a time. Planning on 2026-10-13
widened it from artifacts to the whole client stack, and `web-client.md` holds that strategy.
Its intake decides what these goals adopt:

- v1.client
- v1.repository-docs
- v1.harness.sitrep

## Scope

HTML is for persistent artifacts only: reports, stats updates, explainers, and demos, hosted in
the coordinator or a repository's Pages site. Workflow outputs (task briefs, plan rounds,
session briefs) stay Markdown, as the architect ruled on 2026-10-13. So do the `context/`
notes, which agents load every session, where Markdown is cheaper in tokens, diffs cleanly, and already
renders on github.com.

## Settled for the spikes

- **Serving.** GitHub Pages, deployed by `actions/deploy-pages`. Every repository involved is
  public, and Pages is free for public repositories on the org's free plan.
  - github.com blob views show HTML as source.
  - raw.githubusercontent.com, jsDelivr `/gh/`, and statically all serve HTML as `text/plain`
    with `nosniff`, so none of them render it.
  - Cloudflare Pages is revisited only if per-PR preview URLs prove necessary.
- **Build.** Static pages build as applications do, by spike-html-conventions' answer, pending
  the intake's decision. Both app branches build through Vite with the local CSS plugin
  (spike-html-conventions' `scripts/css-sheets.js`), to hashed filenames and a build manifest,
  and a Pages workflow uploads the build output. A page that loads components unbundled imports
  CSS module scripts, which no Safari release loads, so no page ships unbuilt. No Jekyll and no
  site generator. The baseline's own CSS works the same from `file://`, from a local server, and
  on Pages. Chromium blocks relative ES module scripts from `file://`, so a `file://` page loads
  its modules from jsDelivr, which allows CORS; an untagged working-tree page has no modules from
  `file://`. spike-html-apps builds applications on this toolchain.
- **In-session review.**
  - A headless chromium screenshot is shown inline with
    `kitty +kitten icat --transfer-mode=stream` in a herdr or tmux pane. tmux passes the image
    through with `allow-passthrough on`.
  - Interactive pages are served locally and opened in chromium.
  - The review script renders into a `mktemp -d` directory and removes it in an EXIT trap.
    Streaming sends the pixels inline, so deleting the file can't break the image. Screenshots
    are never committed.
- **Style baseline.** One native, dependency-free package of CSS and ES modules, released by
  git tag from its own repository. A consumer installs it by tag or links a pinned
  `cdn.jsdelivr.net/gh/<owner>/<repo>@vX.Y.Z/...` URL, which serves CSS as `text/css` and never
  changes for a tag. The interface is documented in
  [spike-html-design's README](https://github.com/JaimeStill/spike-html-design/blob/main/context/README.md).
  - **Layers.** One top-level layer, named for the package, holds tokens (with color, space, and
    type sub-layers), reset, elements, and tools. A consumer's later layer beats every one.
  - **Facets.** Color, space, and type are facets, token contracts that one profile each fills.
    - Color is sixteen tokens: three surfaces, two borders, three text tiers, and eight hues
      named for their color, with no role tokens. `--bg` is the base, and `--bg-1` and `--bg-2`
      step toward the text, lightest to darkest in light and darkest to lightest in dark.
      Dialogs sit on `--bg-1`.
    - Four color profiles ship, each light and dark: Flexoki (the default), Primer, Catppuccin,
      and Everforest. Space has regular and compact profiles, and type has mono and sans.
    - `data-palette`, `data-scheme`, `data-space`, and `data-type` apply a profile or scheme to
      any element's subtree, so a preview container renders as its profile does at the root.
      With no `data-scheme`, the page follows the system scheme live.
  - **Elements and tools.** The elements layer styles every native element without classes, at
    zero specificity, keeping chromium's default layout, and styles `kbd` as the only key hint.
    The tools layer adds opt-in classes for buttons, scroll, forms, and tables. A consumer
    extends a treatment by setting a hook: `--interaction-color`, `--code-color`, `--kbd-color`,
    `--button-color`, or `--table-stripe`.
  - **Entries.** The lean entry holds each facet's default profile, and the full entry every
    profile. The check fails when an entry or module exceeds its brotli budget.
  - **Modules.** A registry lists each facet's profiles, and the package and its extensions
    register into it. The preferences module applies the reader's stored choice before first
    paint and opens a theming menu. The `./conformance` export's `check(facet, name)` tests a
    color profile in a live page against `RULES.color`, seven rules that an extension package's
    color profile must pass.
  - **Keyboard.** Keys is a family of three modules: a grammar, a runtime, and a help module.
    - A page binds its own keys, chords, and combinations, written as pressed, through a
      `data-keys` attribute or `register()`. Keys writes `aria-keyshortcuts` from each binding.
      The package binds no shortcut itself.
    - The help dialog lists the bindings and the reader's settings: an off switch for
      character-key shortcuts, as WCAG 2.1.4 requires, and the sequence wait.
    - Escape in a field, outside an open modal, blurs the field so shortcuts work again.
    - claude.dev's article pages are the reference: bracketed key hints on navigation, a section
      tree with reading progress, and arrow-key scrolling. The section tree is
      spike-html-hosting's to build.

## Experiment: spike-html-design

Can one tagged, native baseline of swappable design facets be consumed by version, as a git-tag
package and through a pinned CDN URL, and extended by other packages? Color, space, and type are
facets, families of tokens that each take one of several profiles, over layers that style native
elements and add opt-in tool classes. The spike runs first, because the other spikes link to
it. It deploys the first Pages workflow, which the other spikes copy, and tags a minor
version per task, so a page pinned to an earlier tag can be shown unchanged. The answer
rewrote "Style baseline" to describe the baseline as shipped.

Decides whether the baseline graduates, and where: the architecture layer, beside the diagram
and voice standards, or its own standards-lab repository. Its packaging, a git-tag
package that other packages extend, is also an input to v1.client.

spike-html-conventions' finding for the intake (its Results 4): the baseline should ship each
layer (tokens with each facet and profile, reset, elements) as a self-layered, `@import`-free,
exported sheet.

- **The shape.** Each file wraps its rules in its own `@layer spike-html-design.<layer>` block,
  imports nothing, and is named in package.json's `exports`, so a component's shadow root can
  adopt the reset and elements layers by reference as CSS module scripts (`with { type: "css" }`).
  The one entry may keep the layer order statement and plain `@import`s for pages.
- **Why v0.7.0 can't be adopted.** Its entry builds the layers with `@import ... layer()`, which
  neither a constructed sheet nor a CSS module script loads (`replace` and `replaceSync` drop
  `@import`, WPT css/cssom/CSSStyleSheet-constructable-disallow-import), and it exports no layer
  file. spike-html-conventions therefore ported v0.7.0 (fe75220) into
  `layers/foundation/baseline/` as `foundation.*` layers with provenance headers. The port is the
  interim and the reference for the shape: its shadow components adopt `[reset, elements]`
  before a layout-only sheet, and no component styles a control's look.
- **The theme menu.** It builds its constructed sheet from a string
  (`modules/theme-menu.js:46-47` at v0.7.0), against the convention that CSS lives in `.css`
  files imported as CSS module scripts.
- **Support.** No Safari release ships CSS module scripts (WebKit has landed them), and bundlers
  need a plugin.

## Experiment: spike-html-conventions

Which conventions does the native web stack build on? The spike holds the web stack to the rigor
the Go standards hold Go to: code structure, module organization, the import direction between
layers, testing, lint, and few external dependencies, so that each layer stays small to maintain
and to load as context. It consumes spike-html-design v0.7.0 by tag. Its interim results wait on
the intake's decision. It argued each pick on industry standard and maintainability first,
footprint second, and speed only on an order-of-magnitude gap:

- **Toolchain.** Of Node with npm, Node with pnpm, and Bun, the runtime is Node, settled, and the
  package manager is pnpm, provisionally. The hypothesis's split by role (Node with pnpm for
  packages, Bun for bundling) is ruled out. tsc's emit builds libraries and Vite builds apps;
  esbuild doesn't earn its place. If spike-html-packages releases the layers through a registry,
  the choice between npm and pnpm reopens, and spike-html-packages weighs npm's standard by use
  and stewardship against pnpm's multi-package practice and npm's monorepo gaps, with its hosting
  and repository-layout decisions.
- **TypeScript form.** Every layer is written in `.ts` and released through a build that ships
  `.js` and `.d.ts`.
- **Layers and direction.** The layers are directories of `layers/` in one package:
  - the foundation, native only, with no package;
  - components (the element tier) and the application SDK, siblings on the foundation;
  - a static branch, which may import the foundation, components, and Lit;
  - an application branch, which may import all three plus the SDK.

  No app imports another. eslint-plugin-boundaries' `dependencies` rule, over the layer table in
  `eslint.config.js` with import-x's `no-unresolved` beside it, enforces that direction in the
  check by the file each import resolves to, so relative specifiers need no normalization
  convention.
- **Lint and format.** ESLint with Prettier, provisionally. It is the standard by a wide margin,
  its tools are the better stewarded, and its direction rule judges by resolved file; together
  these outweigh Biome's smaller surface (one binary and one config file against nine pins and
  four config files). ESLint Stylistic can't restore a disturbed tree, and oxlint with oxfmt
  isn't the standard and judges direction by text. Footprint and speed don't overturn the pick.
  It reopens if spike-html-packages makes the layers packages, or if ESLint's or Prettier's
  foundation or funding changes.
- **Components.** Components are Lit with a shadow root by default, the architect's judgement
  after reading all 8 final versions: two elements (a pagination container on go-web-sdk's Page
  envelope, and the entry it holds), each in Lit and vanilla, shadow and light DOM.
  - Light DOM lets the page's baseline style the controls directly, but its hand-placed
    composition costs more than it gives: MutationObserver placement in place of slots, a
    cascade layer ordered by adoption, and page styles leaking in.
  - Vanilla matches Lit's observable contract only by hand-writing attributes, conversion,
    reflection, accessors, and pre-upgrade properties, and it forgoes batched and binding-level
    updates. No build removes Lit's runtime (5,031 B brotli in production, shared by every Lit
    element on a page), and Svelte, Stencil, and solid-element each ship their own runtime.
  - A shadow root can't see the page's baseline: its element selectors stop at the boundary, and
    only tokens cross. No shipped mechanism lets a shadow root inherit page styles
    (csswg-drafts#10176 and whatwg/html#11687 are open). So a shadow-root component adopts the
    foundation's reset and elements sheets by reference, then a layout-only sheet of its own, and
    no component styles a control's look.
  - The foundation holds a port of spike-html-design v0.7.0 (its tokens with the Flexoki,
    regular, and mono profiles, reset, and elements) as `.css` files. Each wraps its rules in a
    `@layer foundation.<layer>` block and imports nothing, because neither a constructed sheet
    nor a CSS module script loads `@import`. A page entry states the layer order and imports
    them, and every page links it, so a shadow page loads the reset and elements layers twice,
    linked and adopted.
  - A component's CSS lives in its own `.css` file, imported as a standard CSS module script and
    never written in `.ts`. CSS module scripts run in Chrome and Edge 123+ and Firefox 147+, and
    are not Baseline; WebKit has landed them, but no Safari release ships them yet. Apps build
    through Vite with a ~30-line local plugin, so their production pages don't depend on Safari;
    a page that loads the modules unbundled fails there until a Safari release ships them.
  - Native CSS doesn't reopen the toolchain pick: Bun loads the form as `{}`, and herald's native
    CSS was a bundler plugin.
- **Testing.** The check has two tiers (Results 5, m 2026-10-18T20:47:44Z).
  - The unit tier runs DOM-free code under node --test, with no DOM shim, since happy-dom and
    jsdom load no CSS module script and compute no layout or style. Logic that needs no DOM
    moves into DOM-free modules.
  - The browser tier covers anything that touches the DOM. It serves the components through
    Vite's dev server with the local CSS plugin, and runs the components' 27 contract cases in
    each of four versions and 4 restyle checks under node --test. It doesn't serve from
    `file://`, which needs a file-access flag, an import map into the package store, and a copied
    `.css` per module, and hides stylesheet rules.
  - The browser tier's driver is the raw Chromium DevTools pipe, with no package, on Chrome for
    Testing's chrome-headless-shell 155.0.8059.84. `mise.toml` pins it through mise's `http:`
    backend with a sha256 and size taken by hand, since Chrome for Testing publishes no checksums
    (chrome-for-testing#70). It launches with `--no-sandbox`, because Ubuntu 23.10+ AppArmor
    refuses a downloaded Chrome's sandbox; the browser loads only the repository's own pages on
    loopback. It runs Chromium alone, 0.9 s of a 12 s check. A CI job runs the same
    `mise install`.
  - The pin replaces Playwright (playwright-core), the architect's earlier pick, first on the
    system Chromium and Firefox and then on Playwright's own browsers so CI would match, because
    Microsoft supports Playwright on Linux only on Ubuntu and Debian. Google names only Ubuntu,
    Debian, openSUSE, and Fedora for Chrome, but the headless shell is one generic linux64 zip
    that runs on Arch and on CI's runners. No repository pins the runners' own browsers
    (ubuntu-24.04 image 20261012.334.1: Chrome 155.0.8059.39, a Chromium 155.0.8059.0 snapshot,
    Firefox 157), and Playwright's Chromium is itself a Chrome for Testing build.
  - Playwright stays the measured comparison: on the pin it takes 1.25 times the raw pipe's time
    cold and 1.26 warm, with 59 harness lines against 143, 13.7 MB, and 0 failures in 20 runs
    each. @playwright/test, Web Test Runner, and Vitest's browser mode are sourced facts, not
    measured.
  - Firefox and WebKit are gaps. Firefox removed CDP in 141, so keeping it means WebDriver BiDi
    through a package (puppeteer-core), which the spike rejected. Playwright's WebKit r2370 is an
    Ubuntu 24.04 build that doesn't launch on the spike's Arch host, and no Safari release ships
    CSS module scripts, so a page that loads the components unbundled fails in Safari until one
    does.

It also specifies the application SDK that spike-html-apps builds: the router, an API client, an
auth hook, the domain-service pattern, the modules and views tiers, the build configuration, and
a Go serving half proposed for go-web-sdk. The Intl formatters are a foundation spec, shared by
components and apps, because components format too and a component may not import the SDK. Its
standards stay in the spike's STANDARDS.md and `context/` until the intake. Decides what
spike-html-packages, spike-html-apps, and spike-html-hosting build on, and whether the intake
founds a web standard.

## Experiment: spike-html-packages

How are the foundation's lateral concerns, its JS/TS modules, and the components packaged and
released by tag over spike-html-design v0.7.0? It settles tag naming, the mechanism that ships
release builds beside the sources, and whether the layers ship as one library or several
packages. It runs after spike-html-conventions, on its toolchain. Decides how v1.client packages
its layers.

It also decides two questions spike-html-conventions hands it:

- **Hosting and repository layout.** spike-html-conventions' package manager pick depends on
  them: a repository release keeps pnpm, and a registry release reopens the choice between npm
  and pnpm, which spike-html-packages weighs with these decisions (npm's standard by use and
  stewardship against pnpm's multi-package practice and npm's monorepo gaps). The options it
  weighs are an npm registry publish, release-asset tarballs installed by URL, and plain URL
  imports through an import map, on spike-html-conventions' evidence (Results 1, 2, and 7, and
  the Ecosystem survey's Release hosting).
- **Packages or directories.** It decides whether the layers become packages, and the import
  extension the releases use: spike-html-conventions' layers import `.js` paths, which bundler
  resolution maps to `.ts`. As packages, each layer's declared dependencies would enforce the
  import direction at install as well as in the lint, because pnpm refuses an import of an
  undeclared package by default, and a lint rule can refuse one too (import-x's
  `no-extraneous-dependencies`, not measured; Results 3). Packages would also reopen the
  provisional ESLint with Prettier pick (Results 6): the direction plugin's role shrinks to the
  files inside a package, so direction weighs less against Biome's smaller surface. The lint
  tooling stays the root package's devDependencies, under spike-html-conventions' STANDARDS.md
  rule "Lit is the only runtime dependency; lint tooling is the only devDependencies".

spike-html-conventions' facts for these decisions:

- **Prefixed tags.** pnpm installs a subdirectory with `#fixture/v0.1.0&path:/fixture`, and bun
  can't resolve the spec. npm ignores `&path:` as an unknown key, and its own `::path:` resolves
  the tag to its commit, then fails with ENOENT on the clone's package.json.
- **Git-tag dependencies.** npm 12 defaults to allow-git=none and refuses any. allow-git=root
  admits a consumer's own but refuses a git-tag package's own git-tag dependency, which
  allow-git=all installs. pnpm 12's `blockExoticSubdeps` default refuses only that nested one,
  and bun has no guard.
- **A git dependency's `prepare`.** npm runs it at pack time while warning that it was blocked,
  and only `--ignore-scripts` skips it. pnpm refuses the install until `allowBuilds` names the
  resolved spec with its commit. bun installs without running it, and reports it until
  `trustedDependencies` names it.
- **The release build.** Each `.ts` module ships as an unbundled, unminified `.js` ES module with
  its `.d.ts`, which `tsc -p tsconfig.build.json` emits. The built files sit in the tag or a
  release asset, `exports` points at the built `.js` with its types, and `files` names the built
  output (bun ignores `files`).
- **CSS in a release.** A release of the components ships each component's `.css` beside its
  emitted `.js`, and the foundation's sheets beside its own: the port in
  `layers/foundation/baseline/`, which the shadow components import as CSS module scripts, and
  the page entry `layers/foundation/baseline.css`, which pages link. `exports` and `files` name
  the `.css` files too, because tsc's emit copies no `.css`; spike-html-conventions' measure
  copies every `.css` in `layers/` beside tsc's emit by hand, in `scripts/browser-cases.js`.
  - A CSS module script (`with { type: "css" }`) loads only when the file is served as
    `text/css`, and jsDelivr serves `.css` as `text/css`.
  - Consumers that load the modules unbundled in Safari fail until a Safari release ships
    WebKit's CSS module scripts (bug 227967, landed as 322887@main).
  - Bundled consumers go through Vite with a local plugin (spike-html-conventions'
    `scripts/css-sheets.js`).
  - Neither Node 26 (`ERR_UNKNOWN_FILE_EXTENSION`) nor Bun 1.4.3 (`{}`) loads the form, so a
    test run outside a browser needs the same treatment. Source: Results 4.
- **Hosting.** jsDelivr `/gh/` caches an exact tag for good, serves `.ts` as `text/plain` with
  `nosniff`, and returns 404 for `/+esm`. GitHub Packages needs a token for every install, public
  packages included, and takes only scoped packages. JSR emits JS and `.d.ts` from `.ts` and
  installs natively in pnpm 10.9+ and Yarn 4.9+, while npm needs `.npmrc` setup and Bun's support
  isn't documented. Verdaccio is a server you run. All 13 surveyed web-component libraries
  publish to the npm registry, which stays ruled out unless no workable alternative exists, so a
  registry release means GitHub Packages, JSR, or a self-hosted registry.
- **npm's monorepo gaps.** Workspace runs follow the `workspaces` array's order, not the
  dependency graph (npm/rfcs#707). npm has no `workspace:` protocol (npm/cli#8845) and no
  catalogs (npm/rfcs#872), and five linked-layout bugs are open, two on workspaces
  (npm/cli#10082, #9982). npm's linked install strategy (`--install-strategy=linked`) refuses
  phantom dependencies, the strictest layout measured, but it is opt-in, stable only since
  11.18.0 and 12.0.0, and still copies each package into a per-project `node_modules/.store`.

## Experiment: spike-html-hosting

What do the repositories' Pages sites and the hosting of persistent artifacts look like? It
starts from a hypothesis: architecture is its own project site; org holds the charter and the
hosted artifacts and links to every project site; each code repository serves its own docs/
guide. It publishes native web with no generator, and tests the org site's URL (as-is the `org`
repository serves at `standards-lab.github.io/org/`), navigation shared across sites, and one
baseline pin. It draws the line between HTML and Markdown, including whether the coordinator's
leadership briefs/ become HTML, and hosts a persistent report in the coordinator. It builds
the review script, covers recorded CLI demos (VHS), and shows that no screenshot files remain
after a review. Its Pages workflow runs a Vite build step with spike-html-conventions' local CSS
plugin (`scripts/css-sheets.js`) and uploads the build output, not the source tree, because a
page that loads components unbundled fails in Safari (spike-html-conventions' Results 8).
Decides the Pages topology that v1.client and v1.repository-docs build, and whether
v1.harness.sitrep publishes HTML.

## Experiment: spike-html-go-wasm

Can a page run a workspace Go package live in the browser through `GOOS=js` WebAssembly, at an
acceptable size? It starts with sqlate's base module, which imports only the standard library.
A minimal Go WASM build is about 2 MB, around 500 KB with Brotli compression. A browser can't
listen on a socket, so a service can't run there, though its handlers can be called in-process.
The page links the baseline by its pinned URL. Decides whether v1.repository-docs and the
Pages sites embed live demos.

## Experiment: spike-html-apps

What does a client application look like? It compares herald's embedded app
(`~/code/herald/app`, a Go package embedding a bun-built Lit client) with other configurations,
and with an external client that calls several services. It runs after spike-html-packages, on
the conventions and packaging those two spikes settle. It builds the application SDK that
spike-html-conventions specifies, and names the SDK's go-web-sdk serving half. Decides
v1.client's app architecture and whether an external-client goal follows 1.0.

What it builds from spike-html-conventions (Results 8):

- **The SDK.** It builds the router, the API client, the default token provider, and the
  foundation's formatters to spike-html-conventions' declarations (`layers/sdk/*.d.ts`,
  `layers/foundation/format.d.ts`). `api.d.ts` types `Problem.title` as always present, but
  go-web-sdk's `Write` omits it for a status outside net/http's table, so the client fills it.
  The browser floor is Safari 26.2, Firefox 147, and Chromium, set by the Navigation API
  (web-features 3.43.0).
- **The Go serving half.** It proves the half spike-html-conventions proposes in go-web-service's
  `sdk/` package before go-web-sdk takes it. The proposal is one handler for an app's embedded
  Vite build output, `func NewClient(dist fs.FS, opts ClientOptions) (http.Handler, error)`.
  - `ClientOptions` holds the base path, the page title, the manifest key of the app's entry,
    and a config value. `NewClient` reads `.vite/manifest.json` from `dist` once, and fails if
    the entry isn't there.
  - Every path that isn't an asset is a client route and answers the shell, so a deep link or a
    reload reaches the router. The shell is an HTML page that writes `<base href>`, the title,
    the config value as JSON in a `<script type="application/json">`, and the manifest's entry
    as a module script with its CSS.
  - A request under the build's asset directory answers the embedded file with
    `Cache-Control: public, max-age=31536000, immutable`, since its hashed name changes with its
    content. A missing asset answers a 404 problem (go-web-sdk `WriteProblem`, `problem.go`),
    never the shell.
  - The handler mounts as `router.Mount(web.NewHandlerModule("/app", client, mw...))` beside the
    API's module. go-web-sdk's `validatePrefix` (`group.go`) refuses a prefix that ends in `/`,
    so spike-html-apps settles how an app at the server's root mounts (`Router.SetNotFound` is
    the candidate), and makes Vite's `base`, the shell's `<base href>`, and `RouterOptions.base`
    agree.
- **Live transport.** It is a named gap: the spec declares neither server-sent events nor the
  WebSocket bridge. spike-html-apps names it with v1.client, to which `messaging.md` gives the
  bridge.
- **herald's elements.** Four of herald's element files in `app/client/ui/elements` import from
  `@domains` (herald 6fc22b3): `classify-progress.ts` the `WORKFLOW_STAGES` constant and the
  `WorkflowStage` type, `document-card.ts` the `WorkflowStage` and `Document` types,
  `prompt-card.ts` the `Prompt` type, and `blob-viewer.ts` the `findFormat` lookup. Under
  spike-html-conventions' layout a component may not import domain code, so the types become the
  element's own properties, and the constant and the lookup move up to the app or down into the
  element.
- **Pagination.** herald's pagination takes `total_pages` (`pkg/pagination` `PageResult`, the
  element's `total-pages` attribute, 6fc22b3). go-web-sdk's Page envelope carries `items`,
  `page`, `size`, `total`, `more`, and `next`, and omits `page` and `total` under a cursor
  (`total` also when the read didn't count). spike-html-conventions' pagination derives the page
  count from total and size, follows `more` for Next when total is absent, disables the page
  input when page is absent, and sends `page-change {page: null}` on cursor Next so the host
  follows `next`. The real app wires these fields and treats a null page as a cursor follow.

## Answers · experiment.html

### Answer · experiment.html.spike-html-design

**Question:** Can one tagged, native baseline of design facets (color, space, and type, each
with swappable profiles, over elements and tools layers) be consumed by version, as a
git-tag package and through a pinned CDN URL, and extended by other packages?

**Answer:** Yes: each tag, v0.1.0 to v0.7.0, is consumed unchanged by pinned jsDelivr URL, and
v0.6.0 and v0.7.0 also by bun's git-tag install; an extension package adds a color profile that
passes the exported conformance check; the open cost is size, most of it comments, left to the
intake.

1. Layers, no build: one dependency-free package whose layers nest under one top-level
   layer, each entry one @import hop deep, beaten by a consumer's later layer. Proven by
   tests alone.
2. Facet contracts: color, space, and type declare only their own tokens, a profile swap
   changes only its facet, and the fluid scales hold their rem bounds. Proven by
   test/baseline.test.js ("tokens", "facet independence", "fluid space and type", "radius",
   "space and type profiles"); the scales were set at the facets review stop.
3. Color profiles: Flexoki, Primer, Catppuccin, and Everforest, light and dark, following
   the system scheme live, contrast recorded per profile and scheme. Proven by
   test/baseline.test.js ("color interface", "color profiles", the contrast records); the
   set was settled at profiles review stop 1.
4. Native behavior: under the reset and elements layers every element keeps chromium's
   default layout; only the opt-in tools layer styles a class. Proven by
   test/baseline.test.js ("elements behavior", "tools"); accepted at the facets review
   stop.
5. Preview containers: a data-palette or data-scheme scope renders as its profile does at
   the root. Proven by test/baseline.test.js ("color interface", the preview cards); the
   grid was accepted at the profiles review stop.
6. Preferences: a theming menu over a registry that extensions add to; the choice persists
   and, render-blocking, applies before first paint (a 0 ms flash; about 366 ms without).
   Proven by tests alone.
7. Keys: grammar, runtime, and help modules that bind nothing themselves, with
   aria-keyshortcuts, an off switch, and a help dialog. Proven by test/keys.test.js and
   test/keys-grammar.test.js; one convention, keys as written, was kept at the keys review
   stops.
8. Consumption by version: pinned pages render the same from file://, a local server, and
   Pages, and the git-tag install yields the tag's files byte-equal, rendering as the pinned
   page. Proven by mise run pin, which passed v0.1.0 to v0.7.0 on 2026-10-15 with no failure
   and installed v0.6.0 and v0.7.0 from GitHub, and hermetically by test/pinned.test.js and
   test/extension-package.test.js. A consumer's first request after a tag can briefly get a
   503 from jsDelivr while it fetches the new path cold.
9. Extension: a fixture package adds the Harbor profile into the color layer, registers it,
   shows it in the menu, applies it, and passes ./conformance; the four built-in profiles
   conform and each broken fixture fails by rule. Proven by tests alone.
10. Stability: each pinned page is tested against its own tag's files, and mise run pin
    passed every published tag on one day, after six later tags. Proven by a test and mise
    run pin.
11. Size and cost: every entry within its brotli budget, one hop of about 65 ms from
    jsDelivr, and comments about 80% of the CSS's brotli size as served. Proven by mise run
    size (enforced in the check by test/size.test.js) and mise run chain v0.6.0, whose file
    and hop counts test/chain.test.js holds. A release build (one file per entry, comments
    stripped, jsDelivr's .min, or a build in a later spike) is left to the intake.

v0.5.0's <theme-menu> binds T with no way to turn it off, failing WCAG 2.1.4 (Level A); the
tag is immutable, and v0.6.0 moves T to keys, whose off switch covers it.

[The answer](https://github.com/JaimeStill/spike-html-design/blob/main/context/README.md#the-answer) · [spike-html-design](https://github.com/JaimeStill/spike-html-design)

### Answer · experiment.html.spike-html-conventions

**Question:** Which conventions does the native web stack build on?

**Answer:** Node 26 with pnpm, `.ts` shipped through a release build, the layers of `layers/`
with their import direction enforced by resolved file, Lit with a shadow root for components,
a DOM-free unit tier and a browser tier on the raw DevTools pipe, ESLint + Prettier, and one
Vite build for both app branches over an SDK declared on the platform.

1. Toolchain: Node over Bun clearly, pnpm over npm narrowly, never Bun beside it; tsc emit
   builds libraries, Vite builds apps, and esbuild doesn't earn its place. pnpm is
   provisional until spike-html-packages decides how the layers release; a registry release
   reopens npm against pnpm. Proven by measure run m 2026-10-16T14:03:02Z and the survey's
   sourced facts.
2. TypeScript form: `.ts` with a release build, over JSDoc with a type check and plain JS,
   the architect's decision on the measured Forms rows (m 2026-10-16). What a release must
   ship is handed to spike-html-packages.
3. Layout and direction: the layers are directories of `layers/` (the foundation,
   components, the SDK, and the static and application app branches), and
   eslint-plugin-boundaries judges each import by the file it resolves to. An application
   app adds four tiers: views, modules, app-internal elements, and domains. Proven by the
   check: `test/layers.test.js` passes only on an exact match over 256 violation files and
   48 allowed files. The rejected mechanisms are measured in m 2026-10-16T21:11:09Z.
4. Components: Lit with a shadow root, adopting the foundation's reset and elements sheets
   before a layout-only `.css` of its own, the architect's choice after reading all 8
   versions (Lit and vanilla, shadow and light DOM). Proven by the check's browser tier
   (`scripts/browser.test.js`), with served bytes in m 2026-10-17T23:45:28Z and a running
   demo: `mise run preview`.
5. Testing tiers: a unit tier under node --test and bun test with no DOM shim, and a browser
   tier over the raw DevTools pipe on the pinned chrome-headless-shell, Chromium alone.
   Proven by the check, and compared with Playwright in m 2026-10-18T20:47:44Z. Firefox and
   WebKit are gaps.
6. Lint and format: ESLint + Prettier, over Biome, oxlint + oxfmt, and ESLint Stylistic,
   provisionally: the pick reopens if spike-html-packages makes the layers packages, or if
   ESLint's or Prettier's foundation or funding changes. Run by the check; compared in
   m 2026-10-16T21:11:09Z.
7. Consumption: spike-html-design v0.7.0 and this package's v0.1.0 install by tag under
   npm, pnpm, and bun, and load from jsDelivr; v0.1.0 ships source only. Proven by the
   check's offline fixtures and by m 2026-10-16 and m 2026-10-16T17:15:39Z.
8. Branches and the SDK: a static app is an entry plus its own Lit elements; an application
   app is an entry and a route table over the four tiers; both build through Vite with the
   local CSS plugin, to hashed filenames and a build manifest. The SDK's router rests on
   URLPattern and the Navigation API, its client returns a `Result` whose failure is
   go-web-sdk's RFC 9457 problem, its auth hook is a token provider, and its services follow
   one domain-service pattern; the Intl formatters are the foundation's. The browser floor
   is Safari 26.2, Firefox 147, and Chromium. Specified, not built: tsc checks the type-only
   declarations, `test/declarations.test.js` holds them free of runtime code, and
   `test/layers.test.js` checks the tier direction, but no router, client, formatter, or Go
   serving half runs anywhere. spike-html-apps builds them.

[The answer](https://github.com/JaimeStill/spike-html-conventions/blob/main/context/README.md#the-answer) · [spike-html-conventions](https://github.com/JaimeStill/spike-html-conventions)
