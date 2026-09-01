# Organization documentation site

A documentation site for the organization, serving the
[docs landing zone](https://github.com/standards-lab/docs)'s content as a website. The landing
zone exists and is the sole authoring home; what remains open is the site build.

The settled hosting direction: a thin `standards-lab.github.io` repository whose Actions
workflow checks out `docs`, builds, and deploys with `actions/deploy-pages`, triggered by
`repository_dispatch` from `docs` — so the apex `https://standards-lab.github.io/` serves the
content while `docs` stays the sole authoring home. Theme and toolchain are open; the landing
zone's pages are kept toolchain-neutral (plain markdown, YAML front matter, relative links) so
the choice stays free.

Deferred until the initial reference architecture is in place. The diagram and voice standards
the charter lists as later objectives are the same body of work.
