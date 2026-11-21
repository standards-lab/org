# Web client

How the reference architecture reaches the browser: the client stack, its conventions and
releases, and the reference client on it. v1.client.stack builds the stack before the capstone,
and v1.client.reference builds the reference client after it. The Pages sites are in `sites.md`.
The spikes that settled what follows are on [the reference page](../docs/web-client.md).

## The stack

Six standards-lab repositories, five of them `@standards-lab` packages, and a handler in
go-web-sdk. Each repository's README documents its interface.

- **web-tooling**: the browser-test and screenshot tooling the other repositories take by tag,
  including a pinned font set for browser suites.
- **web-foundation**: native CSS and ES modules with no dependencies: the baseline sheets, the
  profiles and their conformance check, the tool sheets, the DOM helpers, the keys family, the
  theme preferences, and the formatters, leaving the theme menu and keys help to the components'
  Settings and Help. spike-html-design v0.7.0 stays a frozen reference.
- **web-icons**: Octicons as one string ES module per icon, and a Vite plugin that fills icon
  markers ("Iconography").
- **web-components**: Lit elements with a shadow root, from the chat thread and composer to
  `sl-drawer`; v0.2.0 (planned) adds `sl-settings` and `sl-shortcuts`. A component takes
  properties and attributes, sends events, and may hold its own UI state. It holds no services,
  fetches no data, and holds no application state. An application's internal element becomes a
  component once a second application needs it.
- **web-app-sdk**: the application SDK: the router, the API client, whose failure is go-web-sdk's
  RFC 9457 problem, the token provider, the domain-service pattern, and the event sequence.
- **webappgen** (planned): a Go CLI that writes a client project, one `new` leaf per kind of app:
  a standalone static app, or a client embedded in a new or existing go-web-sdk service and served
  through `NewClient`.
- **`NewClient` in go-web-sdk** ("NewClient", below).

A layer imports only from the layers below it. The foundation is at the bottom and web-icons above
it; the components sit on the foundation and web-icons, and the SDK on the foundation; an app may
import all four. No app imports another, and a component never imports the SDK, which is why the
formatters are the foundation's. An app is one of two kinds:

- a static app: an entry and its own Lit elements;
- an application: an entry and a route table over four tiers, views, modules, app-internal
  elements, and domains.

## NewClient

`NewClient` is the `http.Handler` that serves an app's Vite build embedded in a service's binary
(go-web-sdk's package documentation, "Serving a client"). Every path that isn't an asset answers
the HTML shell, so a deep link or a reload reaches the client's router; a missing asset answers a
404 problem, never the shell.

## Conventions

The stack follows
[spike-html-apps' conventions](https://github.com/JaimeStill/spike-html-apps/blob/main/context/conventions.md),
which the template and the web standard adopt. The choices underneath:

- **Toolchain.** Node 26 and pnpm. tsc's emit builds the libraries and Vite builds the apps. pnpm
  installs a consumer's git tag with no setting, refuses phantom imports, and applies a minimum
  release age by default. A registry release would reopen npm against pnpm.
- **TypeScript.** Every module is `.ts` ([the reference page](../docs/web-client.md#typescript)).
- **Lint and format.** ESLint with Prettier; eslint-plugin-boundaries fails the check on any import
  against the layer direction. Biome is the measured alternative, and comes back if ESLint's or
  Prettier's stewardship changes.
- **Components and CSS.** A shadow root can't see the page's styles; only tokens cross. So a
  component adopts the foundation's reset and elements sheets by reference, then a layout-only
  sheet of its own, and no component styles a control's look. Each foundation sheet wraps its
  rules in its own `@layer` block and imports nothing, because neither a constructed sheet nor a
  CSS module script loads `@import`. A component's CSS is a `.css` file imported as a CSS module
  script, never written in `.ts`. Lit's runtime is 5,031 B brotli, shared by every element on a
  page.
- **Testing.** The unit tier runs under `node --test` with no DOM shim, so logic that needs no DOM
  lives in DOM-free modules. The browser tier drives a pinned chrome-headless-shell over the raw
  DevTools pipe with no package (web-tooling's README). Playwright took 1.25 times as long.
- **Settings and Help.** In every client and page, Settings, on S, is the home of all settings,
  applied and stored at once; Help, on ?, lists the keyboard shortcuts as plain text. Neither has
  Save or Cancel ([the reference page](../docs/web-client.md#settings-and-help)).
- **Iconography.** web-icons is the convention for every web client and page. A page's glyph is a
  `<svg data-icon="set/name">` marker that web-icons' Vite plugin fills; a component imports the
  icon's module and renders it with Lit's `unsafeHTML`.
- **Color roles.** Five `--color-*` hooks name the roles (web-foundation's `docs/use.md`). No profile
  declares them and the hues stay role-free, so a page sets a role without touching a profile.
- **Page types.** The foundation's opt-in page-layout sheet has two: an application page, one
  screen, and a prose page, a reading column.
- **In-session review.** web-tooling's screenshot command (its README, "Screenshots"). Captures
  are never committed.

## Packaging and release

- **One repository per layer**, each one package with its own check, CI, and release workflow,
  built on the layer below by its tag. The components take the foundation as a peer.
- **A plain `vX.Y.Z` tag** marks a pack commit, parented on the merge commit, whose tree is the
  package and the release asset. Releases are cut by hand on a green check (README, "Release").
- **Consumers install by tag** under npm, pnpm, and bun. A page may also load a release from
  jsDelivr `/gh/` through an import map, though the components load that way only where CSS
  module scripts do ("Browser support"). GitHub Packages is the measured alternative: it resolves
  the peer range, at the price of a registry setting and a token for every install.

## Browser support

The browser floor is Safari 26.2, Firefox 147, and Chromium, set by the Navigation API. Two gaps
are accepted:

- **Firefox has no `blocking="render"`** (bug 1751383), which `NewClient`'s shell and the sites
  put on the preferences module, so a Firefox reader with a non-default stored theme can see the
  default for one frame. No inline script works around it: one would copy the preference logic
  out of its module and need a CSP hash on every page. Revisit when Firefox ships it.
- **No Safari release loads CSS module scripts**, which the components import, so a page using
  them requires a build: Vite with vite-plugin-standard-css-modules works everywhere, and every
  consumer the roadmap names builds with Vite. The packages keep the standard syntax, with no
  pack-time rewrite. Revisit when Safari ships CSS module scripts.

The browser tier runs Chromium alone; Firefox and WebKit are untested.

## The color profiles

web-foundation ships four color profiles, each light and dark, refined from the palettes at
[terminalcolors.com](https://terminalcolors.com/); its README lists them. The site states no
license, so each palette's upstream license is checked before it is used. A profile ships when it
passes the conformance check, including a rule of 4.5:1 WCAG contrast, and the architect's review
of its specimen pages.

## Toward Web Elemental

v1.client.stack's `standard` task (planned) founds Web Elemental (`web-elemental`), a draft web
standard beside Go Elemental in the architecture layer, with Lit as its one runtime dependency. A
candidate principle for it, from spike-html-go-wasm: a component holds no authoritative state. A
DOM-free model below it, written by one owner (the composing element or the page), holds the
state, its rules, and its validation.

## The reference client

v1.client.reference (planned) builds go-web-service's client on spike-html-apps' app shape,
served by `NewClient` from the service's binary, in parity with the service as slab is on the
command line. It first shows the organizations, the document hierarchy, and a live view of
go-web-service's reactors over the WebSocket bridge (`messaging.md`), against the open API. It
applies Web Elemental. Its own planning writes its tasks.

- **Embedded or external.** It evaluates the embedded client against an external client hosted
  apart from any one service.
- **Inputs.** The organization logo's fallback (v1.storage), and NATS-native chat as a candidate
  transport (`messaging.md`).
- **Auth.** v1.auth's client sweep puts the client behind authentication, and decides whether an
  image is fetched as an `<img>` with a cookie or with a bearer token.
- **CORS.** An embedded client is same-origin and sends no cross-origin requests. An external
  client or a Pages page that calls a service needs v1.middleware's CORS.
