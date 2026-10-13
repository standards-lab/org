# HTML artifacts

Whether the organization's human-facing artifacts are written as HTML beside Markdown, and how
they are styled, served, and reviewed. The prompt is "Using Claude Code: the unreasonable
effectiveness of HTML" (claude.dev/blog, 2026-05-20). It argues that HTML wins on diagrams,
interactivity, navigating long documents, and sharing by link. It concedes the token cost, and
it doesn't address diffability, hosting, or access control.

experiment.html answers the question in five spikes, run one at a time. Planning on 2026-10-13
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
- **Build.** None. Pages are hand-written HTML, CSS, and ES modules, with custom elements and no
  framework. A page works the same from `file://`, from a local server, and on Pages, and the
  Pages workflow uploads a directory. No Jekyll and no site generator. The stack for
  applications, herald's bun + Lit or something else, is spike-html-apps' to settle.
- **In-session review.**
  - A headless chromium screenshot is shown inline with
    `kitty +kitten icat --transfer-mode=stream` in a herdr or tmux pane. tmux passes the image
    through with `allow-passthrough on`.
  - Interactive pages are served locally and opened in chromium.
  - The review script renders into a `mktemp -d` directory and removes it in an EXIT trap.
    Streaming sends the pixels inline, so deleting the file can't break the image. Screenshots
    are never committed.
- **Style baseline.** One native stylesheet and two small ES modules, released by git tag from
  their own repository. Pages link a pinned `cdn.jsdelivr.net/gh/<owner>/<repo>@vX.Y.Z/...`
  URL, which serves CSS as `text/css` and never changes for a tag.
  - **Structure.** It follows herald's client design (`~/code/herald/app/client/design`):
    cascade layers `tokens, reset, theme, components`, herald's spacing, text, and radius
    scales, and its semantic token names. Each artifact adds its own `app` layer last. Component
    styles are plain classes, with no CSS modules, which need a bundler. Fonts use system stacks
    only: mono for chrome, labels, and code, and sans for prose.
  - **Palette.** It is terminal-style and flat, following the herald presentation diagrams
    (`~/code/herald/_project/presentation/assets/design/README.md`).
    - Each theme supplies background and foreground tiers, `muted`, `accent`, and 18 hue
      tokens: the ANSI 16 (black, red, green, yellow, blue, magenta, cyan, white, and each one's
      bright) plus orange and bright orange. Every hue is named for its color, and every bright
      is distinct from its normal.
    - There are no tinted fill tokens, because `color-mix()` derives a tint where it is used.
    - There are no shadows; elevation is an outline.
    - Primer is not used, since its layered palette pays off only inside github.com.
  - **Themes.** One dark and one light theme, both from Flexoki (stephango.com/flexoki, MIT),
    which publishes both modes from one scale per hue, orange included. Each sets
    `color-scheme`, and the baseline ships Flexoki's license notice. omarchy's 22 themes were
    ruled out: they add a generator and vendored files to every artifact, and their hue slots
    aren't semantic (hackerman's `red` is green, and no theme has a bright orange).
  - **Toggle.** A native `<theme-toggle>` cycles system, light, and dark, and keeps the reader's
    choice in localStorage. With nothing stored, the theme follows the system setting through
    `prefers-color-scheme`, including when it changes while the page is open.
  - **Keyboard.** `keys.js` holds one registry of single-key bindings.
    - Bindings never fire while typing in a field or with a modifier held.
    - Each binding is declared with `aria-keyshortcuts` and listed in a `?` help dialog.
    - It ships T (theme), ? (help), j and k (next and previous section), and g (top). Artifacts
      register their own keys.
    - claude.dev's article pages are the reference: bracketed key hints on navigation, a section
      tree with reading progress, and arrow-key scrolling. The baseline ships the key-hint
      styling; the section tree is spike-html-hosting's to build.

## Experiment: spike-html-design

Can one tagged, native baseline of swappable design facets be consumed by version, as a git-tag
package and through a pinned CDN URL, and extended by other packages? Color, space, and type are
facets, families of tokens that each take one of several profiles, over layers that style native
elements and add opt-in tool classes. The spike runs first, because the other spikes link to
it. It deploys the first Pages workflow, which the other spikes copy, and tags a minor
version per task, so a page pinned to an earlier tag can be shown unchanged. "Style baseline"
describes the design the spike started from; the answer rewrites it.

Decides whether the baseline graduates, and where: the architecture layer, beside the diagram
and voice standards, or its own standards-lab repository. Its packaging, a git-tag
package that other packages extend, is also an input to v1.client.

## Experiment: spike-html-components

How are layouts and components packaged, released, and layered over the baseline? It consumes
the baseline by tag from its own repository, the baseline's first real downstream package. It
covers layouts and templates (structural custom elements, a layout engine over CSS grid, layout
sheets for categories of page) and components, and weighs one library, one package per
component, and packages of related components. Decides v1.client's layouts and components
layers and where they sit in the architecture.

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
and with an external client that calls several services. It runs last, so it builds on every
other answer: the components, hosting for an external client, and live WASM as an option.
Decides v1.client's app architecture and whether an external-client goal follows 1.0.

## Answers · experiment.html
