# Organization documentation site

The organization documentation site is planned to serve the
[architecture repository](https://github.com/standards-lab/architecture)'s content as a website. The
repository exists and is the sole authoring home; what remains open is the site build.

The planned hosting is a thin `standards-lab.github.io` repository whose Actions workflow checks out
`architecture`, builds it, and deploys with `actions/deploy-pages` when `architecture` sends a
`repository_dispatch` event. The apex `https://standards-lab.github.io/` then serves the content
while `architecture` stays the sole authoring home. Theme and toolchain are open; the repository's
pages are kept toolchain-neutral (plain markdown, YAML front matter, relative links) so the choice
stays free.

The site build waits until the initial reference architecture is in place. The diagram and voice
standards, which the charter lists beside the site as later objectives, are planned with it.
