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
- status-reports and training, which each build components of their own on the foundation the
  intake promotes; only overlap they prove moves into a shared documentation components layer
  (plan, 2026-11-09)
- capstone.data-intelligence, whose UI builds on the stack it promotes; the intake runs before it

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
  - For the spike-html-apps repositories, the review script is spike-html-apps-tooling's
    screenshot bin. It shows captures with `kitten icat --transfer-mode=stream`. Inside a Neovim
    `:terminal` it shows none, since the buffer reports no pixel size and libvterm doesn't pass
    the graphics through, so `--output browser` opens a gallery instead.
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
      tree with reading progress, and arrow-key scrolling. spike-html-hosting-components builds
      the section tree as `sl-section-tree`, and departs from the reference where the keys
      module forced it or the architect chose:
      - J and K move between sections, through the keys module's `register()`. H and L move to
        the previous and the next page, through the SDK's pager links.
      - The arrow keys stay the browser's scrolling. The keys module cancels every key a binding
        matches, and its off switch covers character keys only, so bound arrows took over
        sideways scrolling and ignored the switch.
      - By the architect's direction, a key hint is a kbd alone, without brackets, and the tree
        marks the current section by an accent rule and bold text, with no reading progress.

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
  packages included, and takes only scoped packages. JSR emits JS and `.d.ts` only from TS
  entrypoints: tsc's `.js` without `@ts-self-types` gets no types in its npm-compatibility package.
  JSR refuses `.css` exports and CSS module script imports, so the components can't go on JSR as
  built. It installs natively in pnpm 10.9+ and Yarn 4.9+, while npm and Bun install it through
  npm.jsr.io as `@jsr/<scope>__<name>`, with `.npmrc` setup (jsr.io/docs/npm-compatibility).
  Verdaccio is a server you run. All 13 surveyed web-component libraries publish to the npm
  registry, which stays ruled out unless no workable alternative exists, so a registry release means
  GitHub Packages, JSR, or a self-hosted registry.
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
guide. Three JaimeStill project sites stand in for org, architecture, and sqlate, carrying
pinned copies with provenance, so the spike writes to no repository it reads. They measure the
`/<repo>/` base path every project site of one owner shares. The spike recommends an org site at
the origin root, a `standards-lab.github.io` repository. It tests navigation shared across sites
and one baseline pin, spike-html-apps-foundation by tag. The Markdown sources stay Markdown and
are rendered at build, since agents load them and github.com renders them. The sites'
components are a documentation layer of their own, spike-html-hosting-components, not
spike-html-apps-components, whose components serve the pipeline app. The sites share their
build through a site SDK released by tag, spike-html-hosting-sdk, after go-web-sdk-template,
with a template inside its release, and highlight code fences at build. An icons layer,
spike-html-hosting-icons, sits beneath the components and the SDK and ships Octicons. The spike
draws the line between HTML and Markdown, including whether the coordinator's leadership
briefs/ become HTML, and hosts a persistent report in the org stand-in. It reviews with
spike-html-apps-tooling's screenshot command and shows that no capture files remain after a
review. The review task built VHS demos and rejected them; the guide shows a CLI's output as
text. Each Pages workflow runs a Vite build with vite-plugin-standard-css-modules, the
ecosystem plugin spike-html-apps adopted in place of spike-html-conventions' local
`scripts/css-sheets.js`. It uploads the build output, not the source tree, because a page that
loads components unbundled fails in Safari (spike-html-conventions' Results 8). Decides the
Pages topology that v1.client and v1.repository-docs build, and how status-reports, a project
site in it, publishes.

## Experiment: spike-html-go-wasm

Can a page run workspace Go live in the browser through `GOOS=js` WebAssembly: library demos,
service handlers called in-process, and a Go playground that compiles arbitrary Go in the page?
It starts with four Go programs of its own beneath sqlate's base module, which imports only the
standard library, go-core's graph, and go-web-sdk's handlers. Pages serves gzip, not brotli: a
minimal build is about 2.4 MB, 0.7 MB gzip, and the sqlate demo 1.2 MB gzip. Sizes are measured,
not gated. A browser can't listen on a socket, so a service can't run there, though its handlers
can be called in-process. The playground runs Go's own compiler and linker in a worker, about
19 MB gzip on demand. The site takes the baseline through spike-html-hosting-sdk's foundation tag.
Its review fixed the defects it found in the reference layers at their root, releasing
spike-html-apps-foundation v0.3.1, spike-html-hosting-sdk v0.6.3 and v0.6.4,
spike-html-hosting-components v0.5.1, and spike-html-apps-components v0.10.3.
Decides whether v1.repository-docs and the Pages sites embed live demos.

## Experiment: spike-html-apps

What does a client application look like, and do the layers hold when they compose the UI
primitives spike-ai-pipeline's capstone needs, live over a backend? It runs after
spike-html-packages, on the conventions and packaging those two spikes settle, in six
repositories: the root (the apps, a Go pipeline emulator, and the serving half); three layer
repositories, spike-html-apps-foundation, -components, and -sdk, each releasing
`@jaimestill/spike-html-apps-<layer>` by tag from v0.1.0, copied from spike-html-packages' tags;
the extension repository spike-html-apps-noctis; and spike-html-apps-tooling, the browser-test
and screenshot tooling the others take by tag.
It builds the application SDK that spike-html-conventions specifies, the foundation's formatters
and spike-html-design's theming, eleven components (a chat thread, composer, upload dropzone,
stage timeline, event log, search results, artifact viewer, and document list, beside the
status badge, entry, and pagination), a pipeline app composing them, and an external client over
two services. Quality is held at every layer: each component is finished in every state and
profile, and the app's full sequence (upload, stages live, chat, search, artifact) reads as one
product; the architect's acceptance of each UI task's screenshots is the design review. herald
(`~/code/herald/app`) is a frame of reference only, not a compared configuration. GitHub Pages
stays with spike-html-hosting. Decides v1.client's app architecture and whether an
external-client goal follows 1.0, and the capstone's UI builds on its releases.

- **The emulator.** One Go binary generated from go-web-sdk-template template/v0.13.0, all state
  in memory: documents, runs (scripted stages on timers, one CloudEvent each), conversations (a
  scripted Open Responses stream over HTTP), and search (a text match). It embeds nats-server
  with WebSocket and JetStream, publishing through go-messaging's nats provider; go-messaging
  creates its stream with file storage, so the embedded server takes a temporary store
  directory. No Postgres, blob storage, outbox, or containers: the spike proves the client, not
  the capstone's backend.

What it builds from spike-html-conventions (Results 8):

- **The SDK.** It builds the router, the API client, the default token provider, and the
  foundation's formatters to spike-html-conventions' declarations (`layers/sdk/*.d.ts`,
  `layers/foundation/format.d.ts`). `api.d.ts` types `Problem.title` as always present, but
  go-web-sdk's `Write` omits it for a status outside net/http's table, so the client fills it.
  The browser floor is Safari 26.2, Firefox 147, and Chromium, set by the Navigation API
  (web-features 3.43.0).
- **The Go serving half.** It proves the half spike-html-conventions proposes in its own `sdk`
  package, shaped as go-web-service's `sdk/` stages it, before go-web-sdk takes it; a spike
  never writes to go-web-service. The proposal is one handler for an app's embedded
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
    content. A missing asset answers a 404 problem (go-web-sdk `Problem.WriteFor`, `problem.go`),
    never the shell.
  - The handler mounts as `router.Mount(web.NewHandlerModule("/app", client, mw...))` beside the
    API's module. go-web-sdk's `validatePrefix` (`group.go`) refuses a prefix that ends in `/`,
    so spike-html-apps settles how an app at the server's root mounts (`Router.SetNotFound` is
    the candidate), and makes Vite's `base`, the shell's `<base href>`, and `RouterOptions.base`
    agree.
- **Live transport.** The spec declares neither server-sent events nor the WebSocket bridge.
  spike-html-apps adds an event-sequence interface to the SDK, an ordered stream of CloudEvents
  resumable by an opaque cursor, with a NATS-over-WebSocket adapter (`@nats-io/nats-core` and
  `@nats-io/jetstream`, optional peers) reading a JetStream ordered consumer from the emulator,
  under one static token from the token provider. The sdk proves the adapter against a pinned
  nats-server, and the emulator proves it end to end. Its record, in spike-html-apps' "The
  answer" under "NATS over WebSocket, for spike-ai-pipeline", is evidence for
  capstone.data-intelligence's choice between NATS over WebSocket and a backend relay; real auth stays a
  gap.
- **herald's elements.** Four of herald's element files in `app/client/ui/elements` import from
  `@domains` (herald 6fc22b3): `classify-progress.ts` the `WORKFLOW_STAGES` constant and the
  `WorkflowStage` type, `document-card.ts` the `WorkflowStage` and `Document` types,
  `prompt-card.ts` the `Prompt` type, and `blob-viewer.ts` the `findFormat` lookup. Under
  spike-html-conventions' layout a component may not import domain code, so the spike's own
  components take their data as their own properties, and lookups live in the app.
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

### Answer · experiment.html.spike-html-packages

**Question:** How are the foundation's lateral concerns, its JS/TS modules, and the components
packaged and released by tag over spike-html-design v0.7.0: tag naming, release builds beside
the sources, and one library or several packages? It also decides the hosting and repository
layout, on which spike-html-conventions' pnpm pick depends, and the import extension the
releases use.

**Answer:** One repository per layer, each released by a plain `vX.Y.Z` git tag on a commit
whose tree is its pack, tsc's `.js` and `.d.ts` with every `.css` beside them, from `.ts`
sources with `.ts` specifiers; the components take the foundation as a peer; a release is cut
by hand on a green check; pnpm, and ESLint + Prettier.

1. Hosting: the git tag is the channel, installed by its tag under npm, pnpm, and bun, and
   loaded from jsDelivr /gh/ through an import map. Proven offline by the check's
   `test:consumers` tier, and from the network by `mise run live` ("Live, 2026-10-23" and
   "Live, 2026-10-26, components v0.2.0"), which adds the release assets' URLs. GitHub
   Packages is the measured alternative, published for real at the tags' versions: its one
   gain is that the peer range resolves there, so the components alone install their
   foundation, at the price of a registry setting and a token under every manager. Proven by
   `mise run live` ("Live, 2026-10-26, GitHub Packages at 0.0.1" and "Live, 2026-10-26,
   components v0.2.0"). JSR is recorded from deno 2.9.8's dry runs only, with no publish.
2. Layout: one repository per layer, each one package, built on the layer below by its tag,
   each with its own check, CI, and release workflow. The architect's decision in the goal
   record; one package against workspace packages was measured in the README's Measures tables
   of 2026-10-22, now retired.
3. Tag naming: one plain `vX.Y.Z` per repository. Proven by the check's `test:consumers` tier
   under npm, pnpm, and bun, and by `mise run live`'s tables, which add jsDelivr /gh/.
4. The release build: tsc's `.js` and `.d.ts` with every `.css` copied beside them, named by
   `exports` and `files`, carried in the tag's tree on a pack commit parented on the merge
   commit, with the same pack as the release asset. Proven by the check's `test:consumers` tier
   and `mise run live`'s tables, every install its tag's tree file for file, and by `mise run
   release-flow`'s table of 2026-10-26, where the pack commit equals the tarball.
5. The import extension: `.ts` relative specifiers, rewritten to `.js` by tsc
   (`rewriteRelativeImportExtensions`), by the goal record's decision on the Measures table of
   2026-10-22, now retired; the emit loads unbundled and through Vite in the check's
   `test:consumers` tier. The emitted `.d.ts` keep `.ts` specifiers, which typescript-go 7.0.2
   and typescript 6.0.3 accept; other `.d.ts` readers are unmeasured.
6. Consumers: a page loading each install unbundled by an import map, and a Vite app with the
   local CSS module script plugin, built and on its dev server, each rendering the components on
   one foundation, a peer at `^0.1.0`, styled by its page.css. Proven offline by the check's
   `test:consumers` tier over the tags, and from the network by `mise run live` ("Live,
   2026-10-26, components v0.2.0") over the release assets and GitHub Packages. Only npm needs
   a guard, `allow-git=root` at the tags.
7. The release flow: the hand flow, a version bump and a Keep a Changelog section, released by
   each layer's `release.yml` on a green check. pnpm's change-intent flow is rejected: its tree
   can't be published, its notes carry the changelog's preamble, and it bumps no dependent
   repository. Proven by `mise run release-flow`'s table of 2026-10-26 and the goal record's
   decision on it.
8. The manager and the lint: pnpm, no longer provisional, on a consumer's git tag with no
   setting, phantom imports refused, and a minimum release age by default, against npm's
   standard by use and stewardship. ESLint + Prettier, settled, with Biome the measured
   alternative: Biome 2.6.3 matched every case with far less surface, but judges a declaration
   import by a naming convention. The lint is proven by `mise run biome`'s table of 2026-10-26;
   both picks are the goal record's decisions; the download and funding figures are sourced
   facts only, read 2026-10-26.

A change in ESLint's or Prettier's stewardship reopens the lint, with Biome the alternative. A
registry release reopens npm against pnpm; GitHub Packages stays published as the measured
alternative.

[The answer](https://github.com/JaimeStill/spike-html-packages/blob/main/context/README.md#the-answer) · [spike-html-packages](https://github.com/JaimeStill/spike-html-packages)

### Answer · experiment.html.spike-html-apps

**Question:** What does a client application on experiment.html's stack look like, and do its
layers hold up when they compose the capstone's UI primitives, live over a backend?

**Answer:** Three layers released by tag, a foundation, Lit shadow components that take
properties and send events, and an SDK of router, API client, token provider, and a resumable
event sequence over NATS WebSocket, composed by apps of their own files and served by a Go
`NewClient`; they hold, live over the emulator in one app and over two services in an external
client.

1. The SDK runtime: the router, the API client with upload progress and streamed responses,
   the token provider, and the domain service. Proven only by tests: the sdk's unit and browser
   tiers and the root's browser tier.
2. The foundation's Intl formatters, against foundation v0.1.0's `format.d.ts`. Proven only by
   tests: the foundation's `test/unit/format.test.ts`.
3. An event-sequence interface in the SDK, resumable by an opaque cursor, with a
   NATS-over-WebSocket adapter. Proven only by tests: the sdk's `test/unit/nats.test.ts`
   against a pinned nats-server, and the root's `test/browser/live.test.ts` against the
   emulator. The README records it for spike-ai-pipeline's capstone.
4. The Go serving half, `NewClient`, at `/app` and at the server's root. Proven by
   `sdk/web_test.go`, the integration tier, and `test/browser/base.test.ts`, and by a running
   demo: `mise run serve`.
5. Eleven primitives as Lit shadow components. Proven by the components' browser tier, every
   specimen page in every profile, and by the specimen pages under the components'
   `mise run demo`.
6. A pipeline app composing them live over the emulator. Proven by the root's browser tier,
   and by `mise run serve` and `mise run screenshots`.
7. The layers clean at scale: zero direction violations, proven by tests, each tier's ESLint
   table with `test/unit/apps.test.ts` and each layer's `layers.test.ts`. Size at ship, all
   12 components with Lit, marked, and their foundation modules: 151.0 kB min, 39.2 kB gzip,
   34.5 kB brotli, beside Material Web's 72.1 kB gzip. The budget is left to the experiment's
   final review before intake.
8. An external client hosted apart, over two services under the spike's CORS. Proven by
   `test/browser/external.test.ts` and `instances.test.ts`, and by a running demo:
   `mise run demo`.
9. Consumption by tag from outside the spike, under npm, pnpm, and bun, and the gaps, each
   naming who decides it. Proven by `mise run consumers` on 2026-11-03; npm needs
   `allow-git=root`.
10. Quality at every layer, judged as a finished product. Proven by the architect's acceptance
    of the screenshots at each UI task's design review.

The spike's conventions are its `context/conventions.md`, for the intake to adopt.

[The answer](https://github.com/JaimeStill/spike-html-apps/blob/main/context/README.md#the-answer) · [spike-html-apps](https://github.com/JaimeStill/spike-html-apps)

### Answer · experiment.html.spike-html-hosting

**Question:** What do the repositories' Pages sites and the hosting of persistent artifacts look
like, published as native web?

**Answer:** The hypothesis holds with the org site moved to the origin root: architecture and
each code repository's docs/ guide are project sites under `/<repo>/`, built by Vite from
Markdown through one site SDK and one documentation components layer, each released by tag, and
the org site is a standards-lab.github.io repository holding the charter, the index of sites,
and the site list every nav fetches.

1. Three project sites. Proven by the live sites, each answering 200 on 2026-11-10, and by each
   site's `mise run check`, which runs its Vite build.
2. The org stand-in. Proven by the root's browser tier: the SDK's shell checks on its five
   pages, its index held to sites.json, and the SDK's crawl; live from the org task, PR #7.
3. The architecture stand-in. Proven by the architecture site's crawl, each page's head held to
   its front matter's meta and every in-site link followed to its page and fragment.
4. The sqlate stand-in. Proven by the components' `sl-section-tree` tests for J, K, and the
   arrow keys, and by the sqlate site's crawl, with H and L pressed at 390px.
5. Shared navigation when one site deploys ahead. Proven only by tests: the components' fixture
   tests on one local origin, where the built list skews and the fetched list converges. Live,
   only the root fetches its list, served with `max-age=600`.
6. One baseline pin and a theme across sites. Proven by the components' two-base test, and live
   on the three deployed sites, the theme set in storage rather than through the menu.
7. No CSS module script in any build. Proven only by the check: the SDK's ESLint leftover check
   over each build. No page was loaded in Safari.
8. The HTML/Markdown line. Proven by the root's orientation brief both ways and its sitrep
   entry, PR #7. Hand HTML cost 3.6 and 1.9 times the drafting tokens, counted with Gemma 4's
   tokenizer.
9. The review across every site. Proven by `mise run screenshots` and the browser test that it
   leaves no capture file, and by the architect's review of the captures at the design task's
   four checkpoints.
10. The CLI demo: built and rejected. The review task recorded five VHS demos and removed them
    at its second checkpoint.
11. Cost per site. Proven by `mise run cost`, its run of 2026-11-10 over the live sites and
    their Pages runs: Pages serves gzip, not brotli; GitHub's deployment queue dominates
    push-to-live; a CDN node can serve the previous deploy minutes after a new one.
12. One icon toolkit. Proven by the icons layer's unit and browser tiers and each site's
    `showsGlyphs` shell check. Counted on the builds at SDK v0.6.1 (2026-11-10), the architecture,
    sqlate, and root builds ship 7, 7, and 8 of 986 Octicons: arrow-left, arrow-right, gear,
    list-ordered, list-unordered, question, and x, and the root also ships report.

For the intake:

- A pattern page for the keyboard-shortcut dialog, with Settings and Help its worked instances.
  - An upload under U is an extension of the pattern.
  - Close sits in the dialog's header row with the h2.
  - The page is inert under a modal and keys fire only inside it, so no second modal opens over
    Help; `sl-shortcuts` reads the bindings against the topmost modal.
  - In a modal, Escape with a glyph hint showing closes the dialog and the hint; on a plain page
    it closes the hint alone.
- Settings and Help as foundation changes: the theme menu's fixed heading and T save, and the
  keys help's built-in settings.
  - The components layer builds both again, as sl-settings and sl-shortcuts, because the
    foundation's theme menu and keys help lack them.
- The foundation's `preferences.js` and `keys.js` dynamically import `theme-menu.js` and
  `keys-help.js`, so every site builds both chunks without requesting them.
- glyph-button as a foundation control.
- Front matter as head meta, not visible, as v1.repository-docs' convention.
- The sqlate quick-start revision against sqlate f2b2117, from spike-html-hosting-sqlate's
  revised copy, `content/docs/quick-start.md`.
  - Apply the copy's diff against blob 0c54507 at sqlate f2b2117.
  - Step 1 adds `go get github.com/jackc/pgx/v5/stdlib`, without which step 5 doesn't build as
    published.
  - The task that runs the program is `program` throughout.
  - Step 5 builds the catalog on `postgres.Patterns()`, with a paragraph on overlays, and closes
    the pool when the catalog fails (`errors.Join(err, pool.Close())`).
  - Each package's `sql_test.go` ends with a `layout` helper that puts each SQL clause on its own
    line, changing only whitespace, and `go test -C <package> -run TestSQL[/<name>]` prints what
    PostgreSQL receives, in place of a `sql` task.
  - Step 9 adds a test printing the composed list read.
  - Step 10 plants a statement with `guard_set` and no `guard_where`, and a RETURNING in a
    standard file, shows sqlint's two findings and exit 1, then removes it. `guard_where` gives a
    guarded command optimistic concurrency: the write applies only if the row's version is still
    the one the caller read, and a stale write fails.
  - Step 11's `print` helper becomes `printJSON`, since `print` shadowed Go's predeclared print.
    Each output line names its request and its outcome, and a `refused` helper names the error
    each refused request matches. Prose explains each line, and that `Replace` deletes and
    creates in one transaction.
  - A new step 12 scopes projects by organization: `app.tenant` and `app.live` layered with
    `sql.guard_set` and `sql.guard_where` in a guarded returning command, and cursor paging on
    PostgreSQL's keyset overlay. The intro gains a bullet for it.
  - The clean-up step becomes 13.
- A site-nav menu at narrow widths, and sub-navs: below 40rem the nav is one row and doesn't
  wrap, and a real org's ten or so sites won't fit.
- The VHS verdict: recorded terminal demos aren't worth the overhead for a guide whose text
  shows the output. v1.repository-docs shows a CLI's output as text.
- One `sl-drawer` in the foundation, its Close glyph passed through a slot.
- 14 web repositories each keep their own `scripts/currency.sh`, where the sites run the SDK's
  currency bin.
- The SDK provenance convention differs: the root keeps one provenance line per SDK tag it
  adopted, and the architecture and sqlate sites one line naming the latest tag whose content
  the file carries.

[The answer](https://github.com/JaimeStill/spike-html-hosting/blob/main/context/README.md#the-answer) · [spike-html-hosting](https://github.com/JaimeStill/spike-html-hosting)

### Answer · experiment.html.spike-html-go-wasm

**Question:** Can a page run workspace Go live in the browser through GOOS=js WebAssembly
(library demos, service handlers called in-process, and a Go playground that compiles arbitrary
Go in the page with Go's own compiler and linker) with acceptable capability and performance?
Sizes are measured, not gated.

**Answer:** Yes: live Go demos are viable on v1.repository-docs' guides and on every Pages site,
in both proven kinds, a prebuilt handler module and source built in the page by Go's own
compiler and linker; both stay options, each goal chooses its demos and their kinds as it plans,
and no site is pre-selected.

The live demo is <https://jaimestill.github.io/spike-html-go-wasm/>. Every figure is the
README's
[Measurements](https://github.com/JaimeStill/spike-html-go-wasm/blob/main/README.md#measurements),
taken on 2026-11-13 against the deployed commit 13d8348, unless it says otherwise.

1. Built from published tags, served as wasm: go.mod requires each module by its tag, with no
   replace directive or local path; no `.wasm` or `.a` is tracked; `wasm_exec.js` is the pinned
   Go's copy. Proven by `mise run check` (`test/unit/go-module.test.ts`,
   `scripts/check-go.sh`), the Pages workflow's deploy, and `mise run live`, which holds
   `go/hello.wasm` and the compiler as `application/wasm`, gzip-encoded, on the live site.
2. sqlate compiled live: edit a statement, see its `$n` SQL or sqlate's error, from the prebuilt
   `go/sqlate.wasm` and from source built in the page. Proven by
   `test/browser/sqlate-compile.test.ts`, `test/browser/sqlate.test.ts`, and `internal/sqlate`'s
   and `playground/sqlate`'s Go tests, and by running demos:
   [sqlate-compile.html](https://jaimestill.github.io/spike-html-go-wasm/sqlate-compile.html)
   and [sqlate.html](https://jaimestill.github.io/spike-html-go-wasm/sqlate.html).
3. A statement run in-process over sqltest's scripted driver: the sqlate page's guarded edit.
   Proven by `playground/sqlate`'s Go tests, run as wasm under node in the check, and
   `test/browser/sqlate.test.ts`, and live on the sqlate page; that it opens no socket is proven
   only by tests.
4. Sizes and cold-load cost: each module's raw, gzip-as-served, and brotli-11 bytes; each page
   readable before any module, no `.wasm` requested before Run, and the first result cold,
   cached, and in place. Proven by `mise run measure` on 2026-11-13 and on the iPhone by hand;
   "no Go before a Run" also by `test/browser/go-demo.test.ts` and each demo page's browser
   test. The figures are under Cost, below.
5. The review: every page in chrome-headless-shell at 390, 900, 1120, and 1376px, the first pass
   across every palette and scheme, the editor included; Firefox 158.0.1 and the iPhone's Safari
   by hand, with no findings. Proven by the architect's review over `mise run screenshots`, with
   `test/browser/screenshots.test.ts` holding that it leaves no capture file. Desktop Safari and
   WebKitGTK are gaps.
6. The verdict: this section, the architect's ruling of 2026-11-17.
7. The playground: arbitrary Go edited, compiled, linked, and run in the page, with load,
   compile, link, and run timings. Proven by `test/browser/playground.test.ts`,
   `test/browser/go-build.test.ts`, and `test/unit/toolchain.test.ts`, which holds each
   program's closure to Go's own `go list -deps` and runs the compiler and linker on the
   in-memory fs, and by a running demo:
   [playground.html](https://jaimestill.github.io/spike-html-go-wasm/playground.html).
8. go-core's graph built from source in the page, and go-web-sdk's handlers called in-process
   through its Router in the prebuilt `go/websdk.wasm`. Proven by
   `test/browser/go-core-graph.test.ts`, `test/browser/go-web-sdk-handlers.test.ts`, and
   `playground/graph`'s and `internal/websdk`'s Go tests, and by running demos:
   [go-core-graph.html](https://jaimestill.github.io/spike-html-go-wasm/go-core-graph.html) and
   [go-web-sdk-handlers.html](https://jaimestill.github.io/spike-html-go-wasm/go-web-sdk-handlers.html).
9. One bridge, runtime, and sl-go-demo for every scenario, on three rendered Markdown pages and
   three hand pages. That they are used unchanged is proven only by tests:
   `test/browser/go-demo.test.ts`, `test/browser/go-runs.test.ts`, each demo page's browser
   test, the layer direction (`eslint.config.js`, `test/unit/go-module.test.ts`), and
   `test/unit/go-demo-fences.test.ts`, which holds each page's fences to its program's files.
10. Four programs, basics, generics, concurrency, and the standard library, edited and run in
    the page. Proven by `test/browser/playground-programs.test.ts` and each program's Go tests,
    which hold its exact output, and by a running demo:
    [playground.html#basics](https://jaimestill.github.io/spike-html-go-wasm/playground.html#basics)
    and its siblings; `mise run report:programs` reports each one's cost.

**Cost,** as measured facts and Pages arithmetic, with no budget; the intake sets any:

- Prebuilt modules, gzip as Pages serves them (brotli 11): hello 0.74 MB (0.55), probe 1.31 MB
  (0.94), sqlate 2.24 MB (1.59), websdk 2.22 MB (1.58).
- The toolchain: the compiler 10.76 MB (6.90), the linker 3.33 MB (2.31). A program's closure of
  archives: calc 5.14 MB (3.81) in 58 files, stdlib 8.26 MB (6.12) in 85, graph 6.99 MB (5.19)
  in 77, sqlate 20.38 MB (14.89) in 219.
- Pages serves gzip alone, with a max-age of 600 s; after it, a building page's worker
  revalidates the manifest, some 100 to 200 B a build, and every module and archive comes from
  the HTTP cache.
- Desktop pages are readable (first-contentful-paint) in 192 to 364 ms cold, medians.
- Desktop, first result after Run's click, median ms, cold / cached / in place: the components
  page 151 / 52 / 68; sqlate-compile 199 / 84 / 15; go-web-sdk-handlers 197 / 67 / 17; calc
  1,893 / 1,461 / 433; stdlib 2,565 / 2,029 / 733; graph 2,137 / 1,688 / 573; the sqlate page
  3,338 / 2,592 / 1,158. A rerun with no rebuild is 15 to 17 ms.
- iPhone 16 Pro Max, one reading each, total ms: the sqlate page 3,301 cold; the graph page's
  first run 1,429; the components page's 183; calc's 1,357; a rerun 5 to 21.
- Deployed weight, read live on 2026-11-17: about 240 MB raw for the toolchain and its 371
  standard-library and 35 module archives, and 22.7 MB for the four prebuilt modules, against
  Pages' 1 GB site limit. Every archive deploys, though a program fetches only its closure.
- Pages' soft 100 GB a month, divided by one cold run's transfer: calc 19.3 MB, about 5,190;
  graph 21.1 MB, about 4,730; stdlib 22.4 MB, about 4,470; the sqlate page 34.6 MB, about
  2,890; prebuilt, sqlate-compile 2.24 MB, about 44,700, go-web-sdk-handlers 2.22 MB, about
  45,100, and the components page 1.31 MB, about 76,500.

**The playground** is proven: arbitrary Go against the standard library and the site's pinned
modules, compiled, linked, and run in the page. It is recommended to no site.

**Two corrections to the framing** under "Experiment: spike-html-go-wasm", which stays as posed:

- The sqlate prebuilt module serves at 2.24 MB gzip, not 1.2 MB. The minimal build (0.74 MB
  gzip) and the playground's 19 MB on demand (calc's 19.3 MB cold) hold.
- "A service can't run there" holds only for sockets. Go's js port fakes the network
  in-process, so go-web-sdk's Service.Run binds and serves under node in the library's own wasm
  tests; that is unproven in a browser. The spike's demos call handlers through the Router.

**A candidate principle** for the web standard experiment.html's intake may found: a component
holds no authoritative state; a DOM-free model below it, written by one owner (the composing
element or the page), holds the state, its rules, and its validation. Evidence: this spike's
FileSet and EditorSettings, written by sl-go-demo, and its Arrangement, a reader's stored
preference the page owns and gives to every fixed demo.

For the intake:

- Warm the worker on intent, the architect's lead optimization: calc's first Run is mostly the
  compiler's warm-up inside Go compile (854 / 875 / 198 ms cold / cached / in place) and Link
  (331 / 334 / 185). A page-load preload spends 19 to 35 MB a view against the 100 GB month,
  so preload on intent (editor focus, pointer over Run), never under a data-saver signal.
- Chrome compiles the 52 MB compiler lazily: its cold WebAssembly.compile reads about 74 to
  123 ms locally and 33 to 38 ms live, so its real cost may sit inside Go compile.
- Build drift: the live module differs from a local build only in Go's `vcs.*` stamps;
  `-buildvcs=false` would remove it.
- The sqlate page's 36 s cold Run on the iPhone (2026-11-12) didn't reproduce; its cause is
  unknown.
- A `timings.test.ts` stall seen once under the parallel browser tier's load; the per-test
  bound now fails it by name.
- spike-html-design v0.7.0's bare `::backdrop` veils every top-layer backdrop, a popover's
  included; spike-html-apps-foundation v0.3.1 fixes it there.
- spike-html-hosting-sdk gaps:
  - no nav for a site's own pages; the spike's Site nav stands in;
  - a Markdown fence can't name its file; sl-go-demo's `file` attribute stands in;
  - the crawl counts a page missing from `content.json` as a hand page, so a site's own
    Markdown tree needs a scratch-root crawl;
  - markdownPages has no hook for marked extensions;
  - its drawer-focus check helpers are private, and the template's drawer wiring is repeated;
  - no page mode for tools; holdsReadingWidth assumes a reading page.
- `sl-drawer` has no modal-only mode; the Site drawer's unreachable dock stands in.
- A page-layout sheet (gutter, section spacing, content and full named lines) in the SDK or the
  foundation, so no page sets its own section spacing.
- The foundation has no token for `outline-offset`, and `@custom-media` isn't shipped, so
  complementary container queries repeat their breakpoint.
- A browser-test helper that runs a suite in a pinned font set, in the architecture or the SDK.
- Go's `wasm_exec.js` exit race, a pending timeout resuming an exited program, is upstream Go's;
  the worker guards it.
- Shiki's http grammar statically imports the JavaScript, TypeScript, and Java grammars, which
  matters if the SDK's client-side fences highlight http.
- A browser running a wasm build is another runtime layer; this answer bears on the
  `elemental-runtime-layers` backlog goal.
- Deploy the toolchain once and share it across one owner's project sites, which share an
  origin and so its HTTP cache (unproven), and ship only the closures of a site's demos.
- SDK v0.6.3 draws no focus outline on a shortcut dialog's script-focused heading, which isn't
  a control; spike-html-hosting-sqlate's STANDARDS.md says "Focus is always visible".
- sl-code-editor's paste stores an extra trailing newline in an empty file, and doubles a final
  newline pasted over the content; likely an editor bug.
- The SDK's Release workflow tags a release itself, above the merge, so a hand-pushed tag is
  rejected as existing; marathon's release steps assume a hand-pushed tag.
- For the goal that next locks sqlate, today v1.ai.processing's `providers` task:
  `sqltest.Open` requires a `testing.TB`, where a constructor that takes none would serve a
  demo; and sqlate's own currency trails pgx 5.12.0, golangci-lint 2.15.0, and go 1.27.4.

[The answer](https://github.com/JaimeStill/spike-html-go-wasm/blob/main/context/README.md#the-answer) · [spike-html-go-wasm](https://github.com/JaimeStill/spike-html-go-wasm)
