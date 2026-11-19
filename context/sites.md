# Pages sites

The workspace's GitHub Pages sites: where each is served, how it builds, the layers the sites
share, and the live Go demos a site may embed. The `sites` goal builds them, last in the roadmap.
Its sub-goals sites.status-reports and sites.training publish earlier, before the capstone, as
standalone project sites. The client stack they build on is in `web-client.md`.

[spike-html-hosting](https://github.com/JaimeStill/spike-html-hosting/blob/main/context/README.md#the-answer)
and [spike-html-go-wasm](https://github.com/JaimeStill/spike-html-go-wasm/blob/main/context/README.md#the-answer)
settled what follows, and their READMEs hold the evidence.

## Where HTML is used

HTML is for persistent artifacts only: reports, stats updates, explainers, and demos, hosted on a
Pages site. Workflow outputs (task briefs, plan rounds, session briefs) stay Markdown, and so do
the `context/` notes, which agents load every session: Markdown costs fewer tokens, diffs
cleanly, and renders on github.com. Hand-written HTML cost 3.6 and 1.9 times the drafting tokens
of the same brief in Markdown.

## Topology

- **The org site** is a `standards-lab.github.io` repository at the origin root. It holds the
  charter, the index of sites, and the site list every site's nav fetches.
- **Project sites** sit under `/<repo>/` on the same origin: architecture, each code repository's
  docs/ guide, status-reports, and training.

Each site's sources stay Markdown, rendered at build, since agents load them and github.com
renders them. Every repository involved is public, and Pages is free for public repositories on
the org's free plan.

## Serving and build

- `actions/deploy-pages` deploys each site. github.com's blob views, raw.githubusercontent.com,
  jsDelivr `/gh/`, and statically all serve HTML as source or as `text/plain` with `nosniff`, so
  none of them render a page. Cloudflare Pages comes back only if per-pull-request preview URLs
  prove necessary.
- Each Pages workflow runs a Vite build with vite-plugin-standard-css-modules and uploads the
  build output, never the source tree, because a page that loads components unbundled fails in
  Safari. No site uses Jekyll or a site generator.
- Pages serves gzip, not brotli, with a max-age of 600 s. GitHub's deployment queue dominates
  the time from push to live, and a CDN node can serve the previous deploy for minutes after a
  new one.

## The shared layers

Two layers, each released by tag, sit between the client stack and the sites. The icons layer
is the stack's web-icons, not a sites layer: a site fills its glyph markers through web-icons' Vite
plugin and ships only the glyphs it uses, 7 or 8 of 986. Settings (`sl-settings`), Help
(`sl-shortcuts`), and the glyph button come from the stack too, from web-components and
web-foundation (`web-client.md`, "Conventions").

- **The documentation components layer** holds the sites' components, separate from
  web-components, which serves applications: the site nav, the index, and the section tree.
  sites.status-reports and sites.training each build components of their own on web-foundation,
  and only the overlap they prove moves into this layer. Its `sl-section-tree` follows claude.dev's
  article pages: J and K move between sections, H and L to the previous and next page through the
  SDK's pager links, and the arrow keys stay the browser's scrolling. A key hint is a `kbd` alone,
  and the tree marks the current section by an accent rule and bold text, with no reading
  progress.
- **The site SDK** builds the sites: a shell, a crawl that checks every in-site link and each
  page's head, a currency bin, and a template inside its release. It highlights code fences at
  build.

The `sites` goal's `sdk` task settles two conventions the spike left split: the spike's root kept
one provenance line per SDK tag it adopted while its other sites kept one naming the latest, and
its 14 web repositories each kept their own `scripts/currency.sh` while the sites ran the SDK's
currency bin. It also closes these gaps in the SDK:

- a site has no nav for its own pages;
- a Markdown fence can't name its file;
- the crawl counts a page missing from `content.json` as a hand page;
- `markdownPages` has no hook for marked extensions;
- its drawer-focus check helpers are private, and the template repeats its drawer wiring;
- it has no page mode for tools, and `holdsReadingWidth` assumes a reading page;
- it draws no focus outline on a shortcut dialog's script-focused heading;
- below 40rem the site nav is one row that doesn't wrap, so it needs a menu at narrow widths and
  sub-navs, since a real organization's ten or so sites won't fit.

Shiki's http grammar imports the JavaScript, TypeScript, and Java grammars, which matters if the
SDK highlights http fences in the client. sl-code-editor's paste adds a trailing newline in an
empty file and doubles a final newline pasted over the content.

## The docs/ guides

v1.repository-docs writes each repository's docs/ guide in Markdown, and the repository sites
render it.

- **Front matter** becomes the page's head meta, not visible text.
- **A CLI's output** is shown as text. Recorded terminal demos (VHS) cost more than they give
  for a guide whose text shows the output.
- **sqlate's quick start** takes the revision in spike-html-hosting-sqlate's
  `content/docs/quick-start.md`, applied as its diff against blob 0c54507 at sqlate f2b2117:
  - Step 1 adds `go get github.com/jackc/pgx/v5/stdlib`, without which step 5 doesn't build.
  - The task that runs the program is `program` throughout.
  - Step 5 builds the catalog on `postgres.Patterns()`, with a paragraph on overlays, and closes
    the pool when the catalog fails (`errors.Join(err, pool.Close())`).
  - Each package's `sql_test.go` ends with a `layout` helper that puts each SQL clause on its own
    line, and `go test -C <package> -run TestSQL[/<name>]` prints what PostgreSQL receives, in
    place of a `sql` task.
  - Step 9 adds a test printing the composed list read.
  - Step 10 plants a statement with `guard_set` and no `guard_where`, and a RETURNING in a
    standard file, shows sqlint's two findings and exit 1, then removes them. `guard_where` gives
    a guarded command optimistic concurrency: the write applies only if the row's version is
    still the one the caller read.
  - Step 11's `print` helper becomes `printJSON`, since `print` shadows Go's predeclared print.
    Each output line names its request and its outcome, a `refused` helper names the error each
    refused request matches, and the prose explains each line and that `Replace` deletes and
    creates in one transaction.
  - A new step 12 scopes projects by organization: `app.tenant` and `app.live` layered with
    `sql.guard_set` and `sql.guard_where` in a guarded returning command, and cursor paging on
    PostgreSQL's keyset overlay. The intro gains a bullet for it.
  - The clean-up step becomes 13.

## Live Go demos

A page can run workspace Go through `GOOS=js` WebAssembly in two kinds: a prebuilt module, and
source compiled and linked in the page by Go's own compiler and linker in a worker. Both are
options on v1.repository-docs' guides and every Pages site; each goal chooses its demos and their
kinds as it plans. The playground, arbitrary Go compiled in the page, works and is recommended to
no site. [The live demo](https://jaimestill.github.io/spike-html-go-wasm/) shows each kind.

- **What runs.** sqlate statements compiled live, and run in-process over sqltest's scripted
  driver; go-core's graph built from source; go-web-sdk's handlers called in-process through its
  Router. A browser can't listen on a socket, though Go's js port fakes the network in-process,
  so `Service.Run` binds and serves under node; that is unproven in a browser.
- **Cost, gzip as Pages serves it.** Prebuilt modules: hello 0.74 MB, sqlate 2.24 MB, websdk
  2.22 MB. The compiler is 10.76 MB and the linker 3.33 MB; a program's archives add 5 to 20 MB,
  so a cold run of a built-in-page demo transfers 19 to 35 MB. Pages' soft 100 GB a month covers
  about 2,900 to 5,200 such cold runs, or 45,000 to 76,000 of a prebuilt demo. A site deploys about
  240 MB of toolchain and archives against Pages' 1 GB limit.
- **Time.** A page is readable in 192 to 364 ms cold, before any module loads. A first result
  takes 151 to 199 ms cold for a prebuilt demo and 1.9 to 3.3 s for a built-in-page one; a rerun
  takes 15 to 17 ms.

For the `sites` goal:

- Warm the worker on intent (editor focus, a pointer over Run), never on page load and never
  under a data-saver signal: calc's first Run is mostly the compiler's warm-up, and a preload
  spends 19 to 35 MB a view.
- Deploy the toolchain once and share it across one owner's project sites, which share an origin
  and so its HTTP cache (unproven), and ship only the closures of a site's demos.
- Build with `-buildvcs=false`, so a live module matches a local build.
- Go's `wasm_exec.js` can resume an exited program from a pending timeout; the worker guards it.
- The sqlate page's 36 s cold Run on an iPhone didn't reproduce, and its cause is unknown.

A browser running a wasm build is another runtime layer, which bears on the backlog goal
`elemental-runtime-layers`.
