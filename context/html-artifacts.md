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
- **Build.** None for the foundation and static pages, pending spike-html-conventions. Pages are
  hand-written HTML, CSS, and ES modules, with custom elements and no framework. A page's CSS
  works the same from `file://`, from a local server, and on Pages, and the Pages workflow
  uploads a directory. Chromium blocks relative ES module scripts from `file://`, so a `file://`
  page loads its modules from jsDelivr, which allows CORS; an untagged working-tree page has no
  modules from `file://`. No Jekyll and no site generator. spike-html-conventions picks the
  toolchain for components and applications, and spike-html-apps builds applications on it.
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

## Experiment: spike-html-conventions

Which conventions does the native web stack build on? The spike holds the web stack to the rigor
the Go standards hold Go to: code structure, module organization, the import direction between
layers, testing, lint, and few external dependencies, so that each layer stays small to maintain
and to load as context. It consumes spike-html-design v0.7.0 by tag. Its hypotheses:

- **Toolchain.** Node 26 LTS with pnpm is the toolchain for packages, and Bun only bundles
  applications, serves them in development, and processes their CSS. The spike measures Node
  with pnpm against Bun 1.4 on git-tag install, TypeScript, build, watch, test, lint, and
  jsDelivr, and picks one.
- **TypeScript form.** The spike compares `.ts` with type stripping, JavaScript checked through
  JSDoc, and plain JavaScript on the same element and module, and reaches its own conclusion.
  Turbo 8's move away from TypeScript and Svelte's move to JSDoc are positions it weighs.
- **Component convention.** The foundation and static pages are written vanilla, and components
  and the application branch are written with Lit.
- **The element tier.** A component takes properties and attributes, emits events, and may hold
  its own UI state. It holds no services, fetches no data, and holds no application state. An
  application's internal element becomes a component once a second application needs it.

It also specifies the application SDK that spike-html-apps builds: the router, an API client, an
auth hook, formatting, the domain-service pattern, the modules and views tiers, the build
configuration, and a Go serving half proposed for go-web-sdk. Its standards stay in the spike's
STANDARDS.md and `context/` until the intake. Decides what spike-html-packages, spike-html-apps,
and spike-html-hosting build on, and whether the intake founds a web standard.

## Experiment: spike-html-packages

How are the foundation's lateral concerns, its JS/TS modules, and the components packaged and
released by tag over spike-html-design v0.7.0? It settles tag naming, the mechanism that ships
release builds beside the sources, and whether the layers ship as one library or several
packages. It runs after spike-html-conventions, on its toolchain. Decides how v1.client packages
its layers.

## Experiment: spike-html-hosting

What do the repositories' Pages sites and the hosting of persistent artifacts look like? It
starts from a hypothesis: architecture is its own project site; org holds the charter and the
hosted artifacts and links to every project site; each code repository serves its own docs/
guide. It publishes native web with no generator, and tests the org site's URL (as-is the `org`
repository serves at `standards-lab.github.io/org/`), navigation shared across sites, and one
baseline pin. It draws the line between HTML and Markdown, including whether the coordinator's
leadership briefs/ become HTML, and hosts a persistent report in the coordinator. It builds
the review script, covers recorded CLI demos (VHS), and shows that no screenshot files remain
after a review. Decides the Pages topology that v1.client and
v1.repository-docs build, and whether v1.harness.sitrep publishes HTML.

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
spike-html-conventions specifies, and names the SDK's go-web-sdk serving half. Decides v1.client's app architecture and whether an external-client goal follows 1.0.

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
