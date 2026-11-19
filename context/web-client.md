# Web client

How the reference architecture reaches the browser: the client stack's repositories and what each
owns, the conventions they are written in, how they are released, and the reference client built
on them. v1.client.stack builds the stack before the capstone, and v1.client.reference builds the
reference client after it. The Pages sites are in `sites.md`.

experiment.html's six spikes settled what follows. Each spike's README holds its answer and the
evidence behind it:
[spike-html-design](https://github.com/JaimeStill/spike-html-design/blob/main/context/README.md#the-answer),
[spike-html-conventions](https://github.com/JaimeStill/spike-html-conventions/blob/main/context/README.md#the-answer),
[spike-html-packages](https://github.com/JaimeStill/spike-html-packages/blob/main/context/README.md#the-answer),
[spike-html-apps](https://github.com/JaimeStill/spike-html-apps/blob/main/context/README.md#the-answer),
[spike-html-hosting](https://github.com/JaimeStill/spike-html-hosting/blob/main/context/README.md#the-answer),
and [spike-html-go-wasm](https://github.com/JaimeStill/spike-html-go-wasm/blob/main/context/README.md#the-answer).

## The stack

The stack is five planned standards-lab repositories, each a package under the `@standards-lab`
scope, and a handler in go-web-sdk. Each repository starts fresh at v0.1.0, seeded by copy from
its spike with a provenance line.

- **web-tooling**: the browser-test and screenshot tooling the other repositories take by tag,
  from spike-html-apps-tooling. It adds a helper that runs a browser suite in a pinned font set.
- **web-foundation**: native CSS and ES modules with no dependencies, from
  spike-html-apps-foundation v0.3.1: the baseline sheets (tokens, reset, and elements), the color,
  space, and type profiles, the tool sheets, the DOM helpers, the registry and keys family, the
  theme preferences and menu, the profile conformance check, and the Intl formatters.
  [Its README](https://github.com/JaimeStill/spike-html-apps-foundation/blob/v0.3.1/README.md)
  documents the interface. spike-html-design v0.7.0 stays a frozen reference.
- **web-components**: Lit elements with a shadow root, from spike-html-apps-components: the chat
  thread, composer, upload dropzone, stage timeline, event log, search results, artifact viewer,
  document list, status badge, entry, pagination, and `sl-drawer`. A component takes properties
  and attributes, sends events, and may hold its own UI state. It holds no services, fetches no
  data, and holds no application state. An application's internal element becomes a component
  once a second application needs it.
- **web-app-sdk**: the application SDK, from spike-html-apps-sdk. Its router rests on URLPattern
  and the Navigation API. Its API client reports upload progress, streams responses, and returns
  a `Result` whose failure is go-web-sdk's RFC 9457 problem. It also holds the token provider, the
  domain-service pattern, and an event sequence of CloudEvents resumable by an opaque cursor, with
  a NATS-over-WebSocket adapter whose `@nats-io` packages are optional peers.
- **webappgen** (renamed from web-app-sdk-template): a Go CLI on go-cli-sdk that writes a client
  project, one `new` leaf per kind of app, from files embedded in its binary. `new standalone`
  writes a static app that reads a `config.json` placed beside its build; the embedded kinds write
  a client into a new go-web-sdk-template service or an existing one, built by Vite, embedded with
  `//go:embed all:dist`, and served through `NewClient`. A written project's check reports the
  whole build's size and enforces no budget. Released as binaries for Linux and macOS.
- **`NewClient` in go-web-sdk** ("NewClient", below).

A layer imports only from the layers below it. The foundation is at the bottom; the components
and the SDK are siblings on it; an app may import all three. No app imports another, and a
component never imports the SDK, which is why the formatters are the foundation's. An app is one
of two kinds:

- a static app: an entry and its own Lit elements;
- an application: an entry and a route table over four tiers, views, modules, app-internal
  elements, and domains.

## NewClient

`NewClient(dist fs.FS, ClientOptions{Base, Title, Entry, RenderBlocking, Config})` returns the
`http.Handler` that serves an app's embedded Vite build. spike-html-apps' `sdk` package proves
it.

- It reads Vite's `.vite/manifest.json` from `dist` once, and fails if the entry isn't there.
- Every path that isn't an asset is a client route and answers the shell, so a deep link or a
  reload reaches the router. The shell writes `<base href>`, the title, the config value as JSON
  in a `<script type="application/json">`, and the entry as a module script with its CSS.
  `RenderBlocking` lists the manifest keys whose modules the shell loads with
  `blocking="render"` ahead of the entry.
- An asset answers its embedded file with `Cache-Control: public, max-age=31536000, immutable`.
  A path under the manifest's directories, a dotfile path, or a last segment with an extension,
  with no file behind it, answers a 404 problem, never the shell.
- At the root it is the router's not-found handler, so it answers only what no module or native
  route claims. Under a prefix it is a handler module, and the bare prefix redirects (301) to
  prefix/. Vite's `base`, the shell's `<base href>`, and the router's base agree.

## Conventions

[spike-html-apps' conventions](https://github.com/JaimeStill/spike-html-apps/blob/main/context/conventions.md)
hold the development, design, and process conventions the stack's repositories follow; the
template and the web standard adopt them. The choices underneath:

- **Toolchain.** Node 26 and pnpm. tsc's emit builds the libraries and Vite builds the apps.
  pnpm installs a consumer's git tag with no setting, refuses phantom imports, and applies a
  minimum release age by default. A registry release would reopen npm against pnpm.
- **TypeScript.** Every module is `.ts` with `.ts` relative specifiers, which tsc rewrites to
  `.js` (`rewriteRelativeImportExtensions`). The emitted `.d.ts` keep `.ts` specifiers.
- **Lint and format.** ESLint with Prettier. eslint-plugin-boundaries judges each import by the
  file it resolves to, and the check fails on any import against the layer direction. Biome is
  the measured alternative and comes back if ESLint's or Prettier's stewardship changes.
- **Components and CSS.** A shadow root can't see the page's styles; only tokens cross. So a
  component adopts the foundation's reset and elements sheets by reference, then a layout-only
  sheet of its own, and no component styles a control's look. Each foundation sheet wraps its
  rules in its own `@layer` block and imports nothing, because neither a constructed sheet nor a
  CSS module script loads `@import`. A component's CSS lives in a `.css` file imported as a CSS
  module script (`with { type: "css" }`), never written in `.ts`. Vite builds rewrite those
  imports through vite-plugin-standard-css-modules. Lit's runtime is 5,031 B brotli, shared by
  every element on a page.
- **Testing.** The unit tier runs DOM-free code under `node --test` with no DOM shim, so logic
  that needs no DOM lives in DOM-free modules. The browser tier drives Chrome for Testing's
  chrome-headless-shell, pinned in `mise.toml` through mise's `http:` backend with a hand-taken
  sha256, over the raw DevTools pipe with no package. It launches with `--no-sandbox`, since
  Ubuntu 23.10+ AppArmor refuses a downloaded Chrome's sandbox, and loads only the repository's
  own pages on loopback. Playwright took 1.25 times as long and is not used.
- **In-session review.** web-tooling's screenshot command captures pages in headless chromium
  into a `mktemp -d` directory, removed on exit, and shows them inline with
  `kitten icat --transfer-mode=stream`; tmux passes the image through with
  `allow-passthrough on`. Inside a Neovim `:terminal` no image shows, so `--output browser` opens
  a gallery. Captures are never committed.

## Packaging and release

- **One repository per layer**, each one package with its own check, CI, and release workflow,
  built on the layer below by its tag. The components take the foundation as a peer.
- **A plain `vX.Y.Z` tag** marks a pack commit, parented on the merge commit, whose tree is the
  package: tsc's `.js` and `.d.ts` with every `.css` copied beside them, all named by `exports`
  and `files`. The same pack is the release asset. A release is a version bump and a Keep a
  Changelog section, cut by hand on a green check.
- **Consumers install by tag** under npm, pnpm, and bun; npm needs `allow-git=root`. A page may
  also load the release from jsDelivr `/gh/` through an import map: the foundation's sheets and
  the SDK work that way in every browser, but the components and the theme menu only in Chromium
  and Firefox, since they import CSS module scripts ("Browser support"). GitHub Packages is the
  measured alternative: it resolves the peer range, at the price of a registry setting and a
  token for every install.

## Browser support

The browser floor is Safari 26.2, Firefox 147, and Chromium, set by the Navigation API.
v1.client.stack's plan (2026-11-18) settled its two gaps:

- **Firefox has no `blocking="render"`** outside its preview builds (bug 1751383), and
  `NewClient`'s shell and the sites put it on the preferences module. Accepted: on Firefox, a
  reader whose stored theme isn't the default can see the default for one frame before first
  paint. No inline script works around it, since one would copy the preference logic outside its
  module and need a CSP hash on every page. Revisit when Firefox ships `blocking="render"`.
- **No Safari release loads CSS module scripts**, which the components and the theme menu import.
  A page that uses them requires a build: Vite with vite-plugin-standard-css-modules works in
  every browser, and every consumer the roadmap names already builds with Vite. The released
  packages keep the standard syntax, with no pack-time rewrite. Revisit when Safari ships CSS
  module scripts.

The browser tier runs Chromium alone; Firefox and WebKit are untested.

## The color profiles

web-foundation ships four color profiles, each light and dark: Flexoki (the default), Primer,
Catppuccin, and Everforest. v1.client.stack's `palettes` task refines them from the palettes at
[terminalcolors.com](https://terminalcolors.com/). The site states no license, so each palette's
upstream license is checked before it is used. A profile ships when it passes the
conformance check, a new rule of 4.5:1 WCAG contrast, and the architect's review of its specimen
pages.

## Toward Web Elemental

v1.client.stack's `standard` task founds Web Elemental (`web-elemental`), a draft web standard
beside Go Elemental in the architecture layer, with Lit as its one runtime dependency. A candidate
principle for it, from spike-html-go-wasm: a component holds no authoritative state. A DOM-free
model below it, written by one owner (the composing element or the page), holds the state, its
rules, and its validation.

## Foundation candidates

The backlog goal `foundation-extension` is a web-foundation minor that takes these:

- From the sites: `sl-drawer` with its Close glyph passed through a slot and a modal-only mode;
  and an `outline-offset` token. Settings and Help as foundation changes (the theme menu's fixed
  heading and T to save, the keys help's built-in settings, so the components stop rebuilding them
  as `sl-settings` and `sl-shortcuts`), with a pattern page for the keyboard-shortcut dialog,
  `glyph-button` as a control, and a page-layout sheet (gutter, section spacing, and named content
  and full lines) moved to v1.client.stack's `foundation-settings` task, ahead of `template`
  (architect, 2026-11-19), so webappgen's starter builds on them. `preferences.js` and `keys.js` import
  `theme-menu.js` and `keys-help.js` dynamically, so every site builds both chunks without
  requesting them.
- From the components,
  [spike-html-apps-components' foundation candidates](https://github.com/JaimeStill/spike-html-apps-components/blob/main/context/foundation-candidates.md),
  each found in two or more components: tokens for a control's block size, the focus outline's
  reach, status roles, and a hook's tint; elements-sheet rules for a disabled text field and a
  dark-scheme `<progress>` track; utility classes for a bare list, keep-whole values, breaking
  only when longer than the line, one-line truncation, the scrolling frame, a slot as wide as its
  widest content, the head row, quiet label type, the status line, motion behind reduced motion,
  and visually hidden text; and `formatJson` with `formatBytes` below a kilobyte, which reads
  "0 byte".

## The reference client

v1.client.reference builds go-web-service's client on spike-html-apps' app shape, served by
`NewClient` from the service's binary, as slab is the service's command-line client. It first
shows the organizations, the document hierarchy, and a live view of go-web-service's reactors
over the WebSocket bridge (`messaging.md`), against the open API. It applies Web Elemental.

- **Embedded or external.** It evaluates the embedded client against an external client hosted
  apart from any one service. spike-html-apps' external client called two services cross-origin.
- **Inputs.** The organization logo's fallback, and NATS-native chat as a candidate transport
  (`messaging.md`).
- **Auth.** v1.auth's client sweep puts the client behind authentication, and decides whether an
  image is fetched as an `<img>` with a cookie or with a bearer token.
- **CORS.** An embedded client is same-origin and sends no cross-origin requests. An external
  client or a Pages page that calls a service needs v1.middleware's CORS.
