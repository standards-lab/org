# Organization documentation site

A documentation site for the organization, serving the
[architecture repository](https://github.com/standards-lab/architecture)'s content as a website. The
repository exists and is the sole authoring home; what remains open is the site build.

The settled hosting direction: a thin `standards-lab.github.io` repository whose Actions
workflow checks out `architecture`, builds, and deploys with `actions/deploy-pages`, triggered by
`repository_dispatch` from `architecture` — so the apex `https://standards-lab.github.io/` serves the
content while `architecture` stays the sole authoring home. Theme and toolchain are open; the
repository's pages are kept toolchain-neutral (plain markdown, YAML front matter, relative links) so
the choice stays free.

Deferred until the initial reference architecture is in place. The diagram and voice standards
the charter lists as later objectives are the same body of work.
