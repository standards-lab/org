# Web client

How the reference architecture reaches the browser: the client stack, its conventions and
releases, and the reference client on it. v1.client.stack builds the stack before the capstone,
and v1.client.reference builds the reference client after it. The Pages sites are in `sites.md`.
The spikes that settled what follows are on [the reference page](../docs/web-client.md).

## The stack

Six standards-lab repositories, five of them `@standards-lab` packages, and a handler in
go-web-sdk. Each repository's README documents its interface.

- **web-tooling**: the browser-test and screenshot tooling the other repositories take by tag;
  its helper runs a browser suite in the pinned DejaVu set.
- **web-foundation**: native CSS and ES modules with no dependencies: the baseline sheets with
  five color role hooks, the profiles and their conformance check, the tool sheets with utility
  classes, the opt-in glyph-button, dialog, layout, and motion sheets, the DOM helpers, the keys
  family (`bindings()`), the theme preferences, and the Intl and JSON formatters, leaving the
  theme menu and keys help to the components' Settings and Help. spike-html-design v0.7.0 stays
  the frozen style reference, except its color profiles' values, which follow their upstream
  themes from web-foundation v0.2.0.
- **web-icons**: the icons layer, from spike-html-hosting-icons: Octicons as one string ES module
  per icon, and `iconFill()`, a Vite plugin that fills icon markers ("Iconography").
- **web-components**: Lit elements with a shadow root, from the chat thread and composer to
  `sl-drawer`, `sl-settings`, and `sl-shortcuts`, and `markdown()`, on marked; web-icons is a
  peer. A component takes properties and attributes, sends events, and may hold its own UI state.
  It holds no services, fetches no data, and holds no application state. An application's
  internal element becomes a component once a second application needs it.
- **web-app-sdk**: the application SDK, beside the components on no layer, peering only the
  optional @nats-io packages: the router, the API client, whose failure is go-web-sdk's RFC 9457
  problem, the token provider, the domain-service pattern, and the event sequence.
- **webappgen**: a Go CLI, a binary release rather than an @standards-lab package, that writes a
  client project for each kind of app. `new standalone <dir>` writes a static app that reads a
  `config.json` placed beside its build, with `--install` to install it. `new service-app` writes
  a service from go-web-sdk-template with a client served through `NewClient`, and
  `inject service-app` adds one to a template-shaped service. A written project's check reports
  the whole build's size and enforces no budget; the CLI's check writes a project and runs that
  project's own check. The app's page uses Settings on S and Help on ?. It rejected its own
  template copy, a go.mod `tool` directive for gonew, and dave/dst or astutil for the wiring.
- **`NewClient` in go-web-sdk** ("NewClient", below).

A layer imports only from the layers below it. The foundation is at the bottom and web-icons above
it; the components sit on the foundation and web-icons, and the SDK imports no layer of the stack;
an app may import all four. No app imports another, and a component never imports the SDK, which is why the
formatters are the foundation's. An app is one of two kinds:

- a static app: an entry and its own Lit elements;
- an application: an entry and a route table over four tiers, views, modules, app-internal
  elements, and domains.

## NewClient

`NewClient(dist fs.FS, entry string, ClientOptions{Base, Title, RenderBlocking, Config})`, in
go-web-sdk v0.18.0, is the `http.Handler` that serves an app's Vite build embedded in a service's
binary (go-web-sdk's package documentation, "Serving a client"). Every path that isn't an asset
answers the HTML shell, so a deep link or a reload reaches the client's router; a missing asset
answers a 404 problem, never the shell. Bad input fails construction, and only the manifest
directory's files are immutable.

## Conventions

The stack follows
[spike-html-apps' conventions](https://github.com/JaimeStill/spike-html-apps/blob/main/context/conventions.md),
which the template and the web standard adopt. The choices underneath:

- **Toolchain.** Node 26 and pnpm ([the reference page](../docs/web-client.md#toolchain)). tsc's
  emit builds the libraries and Vite builds the apps.
- **TypeScript.** Every module is `.ts` ([the reference page](../docs/web-client.md#typescript)).
- **Lint and format.** ESLint with Prettier. In the layers, eslint-plugin-boundaries fails the
  check on any import against the layer direction; a written project lints with ESLint's
  recommended rules and typescript-eslint instead. Both enforce eslint-comments'
  `require-description`. Biome is the measured alternative, and comes back if ESLint's or
  Prettier's stewardship changes.
- **Components and CSS.** A shadow root can't see the page's styles; only tokens cross. So a
  component adopts the foundation's reset and elements sheets by reference, then a layout-only
  sheet of its own, and no component styles a control's look. A component may also adopt
  `utilities.css`, `glyph-button.css`, `dialog.css`, and `motion.css` after reset and elements; a
  page links the glyph-button, dialog, layout, and motion sheets after its entry, in that order.
  Each foundation sheet wraps its rules in its own `@layer` block and imports nothing, because
  neither a constructed sheet nor a CSS module script loads `@import`. A component's CSS is a
  `.css` file imported as a CSS module script, never written in `.ts`.
- **Testing.** The unit tier runs under `node --test` with no DOM shim, so logic that needs no DOM
  lives in DOM-free modules. The browser tier drives a pinned chrome-headless-shell over the raw
  DevTools pipe with no package (web-tooling's README).
- **Settings and Help.** In every client and page, Settings, on S, is the home of all settings,
  applied and stored at once (`.dialog-wide`); Help, on ?, lists the keyboard shortcuts as plain
  text (`.dialog-fit`). Neither has Save or Cancel
  ([the reference page](../docs/web-client.md#settings-and-help)).
- **Iconography.** web-icons is the convention for every web client and page. A page's glyph is a
  `<svg data-icon="set/name">` marker that web-icons' Vite plugin fills; a component imports the
  icon's module and renders it with Lit's `unsafeHTML`.
- **Color roles.** Five `--color-*` hooks name the roles (web-foundation's `docs/use.md`). No rule
  declares them and the hues stay role-free, so a page sets a role without touching a profile, and
  a reader writes `var(--color-danger, var(--red))`.
- **Page types.** `layout.css` has two: `body.page-app`, one screen, and `body.page-prose`, a
  reading column, with `.header-actions` at the header's inline end.
- **Keys.** Storage and `Symbol.for` keys are `web-foundation.*`.
- **In-session review.** web-tooling's screenshot command (its `docs/screenshots.md`). Captures
  are never committed.

## Packaging and release

Each repository is one package, released by Web Elemental's
[packages and releases](https://github.com/standards-lab/architecture/blob/main/standards/web-elemental/principles/packages-and-releases.md)
principle. Beyond it, the SDK builds on no layer, a pack test keeps pnpm's store in scratch, and
a README may carry light and dark captures under docs/.

## Browser support

The browser floor is Safari 26.2, Firefox 147, and Chromium, set by the Navigation API. Two gaps
are accepted:

- **Firefox has no `blocking="render"`** (bug 1751383), which `NewClient`'s shell and the sites
  put on the preferences module, so a Firefox reader with a non-default stored theme can see the
  default for one frame, with no inline script ([the reference page](../docs/web-client.md#browser-gaps)).
  Revisit when Firefox ships it.
- **No Safari release loads CSS module scripts**, which the components import, so a page using
  them requires a Vite build. The packages keep the standard syntax, with no pack-time rewrite.
  Revisit when Safari ships CSS module scripts.

The browser tier runs Chromium alone; Firefox and WebKit are untested.

## The color profiles

web-foundation ships four color profiles, each light and dark, chosen from
[terminalcolors.com](https://terminalcolors.com/) for variety of temperature and style, in order:
Zen (the default; zenbones' Zenwritten), GitHub, Catppuccin, and Everforest. Each comes from its
upstream theme at a pin, under a license checked before use, since the site states none, and keeps
the 16-token contract; backups appear only on the specimen page
([the reference page](../docs/web-client.md#color-profiles)). A profile ships when it passes the conformance check, including 4.5:1 WCAG contrast for
--fg on --bg, and the architect's review of its specimen captures.

## Web Elemental

[Web Elemental](https://github.com/standards-lab/architecture/tree/main/standards/web-elemental),
the draft web standard beside Go Elemental, holds the rules the stack's repositories share. A candidate principle for it, from spike-html-go-wasm: a component holds no
authoritative state. A DOM-free model below it, written by one owner (the composing element or
the page), holds the state, its rules, and its validation.

## The reference client

v1.client.reference (planned) builds go-web-service's client on spike-html-apps' app shape,
served by `NewClient` from the service's binary, in parity with the service as slab is on the
command line. It first shows the organizations, the document hierarchy, and a live view of
go-web-service's reactors over the WebSocket bridge (`docs/messaging.md`, "The capability ledger"),
against the open API. It
applies Web Elemental. Its own planning writes its tasks.

- **Embedded or external.** It evaluates the embedded client against an external client hosted
  apart from any one service.
- **Inputs.** The organization logo's fallback (go-web-service's storage domain), and NATS-native
  chat as a candidate transport (`docs/messaging.md`, "The capability ledger").
- **Auth.** v1.auth's client sweep puts the client behind authentication, and decides whether an
  image is fetched as an `<img>` with a cookie or with a bearer token.
- **CORS.** An embedded client is same-origin and sends no cross-origin requests. An external
  client or a Pages page that calls a service needs v1.middleware's CORS.
