# Web client

The evidence and detail behind the web client stack's conventions. The decisions are in the
coordinator's [web client note](../context/web-client.md); each stack repository's README holds its
interface, and go-web-sdk's package documentation holds `NewClient`'s.

## Spike evidence

Six spikes settled the stack. Each spike's README holds its answer and the evidence behind it:

- [spike-html-design](https://github.com/JaimeStill/spike-html-design/blob/main/context/README.md#the-answer)
- [spike-html-conventions](https://github.com/JaimeStill/spike-html-conventions/blob/main/context/README.md#the-answer)
- [spike-html-packages](https://github.com/JaimeStill/spike-html-packages/blob/main/context/README.md#the-answer)
- [spike-html-apps](https://github.com/JaimeStill/spike-html-apps/blob/main/context/README.md#the-answer)
- [spike-html-hosting](https://github.com/JaimeStill/spike-html-hosting/blob/main/context/README.md#the-answer)
- [spike-html-go-wasm](https://github.com/JaimeStill/spike-html-go-wasm/blob/main/context/README.md#the-answer)

The patterns the components keep locally, which web-foundation takes and web-components then
drops, are listed in
[spike-html-apps-components' foundation candidates](https://github.com/JaimeStill/spike-html-apps-components/blob/main/context/foundation-candidates.md).

## Toolchain

pnpm installs a consumer's git tag with no setting, refuses phantom imports, and applies a minimum
release age by default. A registry release would reopen npm against pnpm. Lit's runtime is 5,031 B
brotli, shared by every element on a page.

## TypeScript

Every module is `.ts` with `.ts` relative specifiers, which tsc rewrites to `.js`
(`rewriteRelativeImportExtensions`). The emitted `.d.ts` keep `.ts` specifiers.

## Browser tier

The browser tier drives Chrome for Testing's chrome-headless-shell, pinned in `mise.toml` through
mise's `http:` backend with a hand-taken sha256. It launches with `--no-sandbox`, since Ubuntu
23.10+ AppArmor refuses a downloaded Chrome's sandbox, and loads only the repository's own pages
on loopback. Playwright took 1.25 times as long.

## Browser gaps

No inline script works around Firefox's missing `blocking="render"`: one would copy the
preference logic out of its module and need a CSP hash on every page. For Safari's missing CSS
module scripts, Vite with vite-plugin-standard-css-modules works everywhere, and every consumer
the roadmap names builds with Vite.

## Settings and Help

The pattern is spike-html-go-wasm's shortcut dialog. Settings holds the theme and the keys
settings. Each dialog opens from a glyph button in the header's `.header-actions`, Help then
Settings, and each dialog's header row holds its heading and a Close glyph, with Esc to close.

## Color profiles

Seoulbones, Tomorrow, Ayu, and Nightfox are the four profiles' backups, shown only on
web-foundation's specimen page.
