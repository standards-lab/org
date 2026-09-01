# reset · public-workspace

- **Status:** closeout
- **Session:** start
- **Project:** standards-lab (org, .github-private)
- **Branch:** public-workspace

## Disposition

- **Integrated:** the coordinator split. `~/architecture/standards-lab/` is now the working tree
  of the new public `standards-lab/org` repository (fresh history; the prior history stays in
  `.github-private`): `context/`, the public references catalog, `CLAUDE.md`, the marathon
  anchor. `.github-private` thinned to the nested `private/` working tree — the extended
  organizational profile plus the private references annex — mirroring how `public/` holds
  `.github`. The profile vocabulary settled as baseline (public, authored) and extended (member,
  mirror plus member section). `design/workspace-structure.md` and `CLAUDE.md` rewritten to the
  three-repository layout; the marathon exception shrank to the profile nesting.
- **Integrated:** the sensitivity split. The engagement prior-R&D entries moved from the public
  catalog to the annex (`private/references.toml`, `private/references.md`, untracked
  `private/references.local.toml`), which extends the public catalog under one shared key
  namespace when the member checkout is present; the layering rule is in
  `design/repo-references.md`. Public personal prior R&D stayed public. Deployment-posture
  wording in `design/dsl-driven-services.md` generalized; the catalog files declare the annex
  without naming its entries.
- **Culled:** `concepts/public-workspace.md` — executed; its open items resolved this session:
  the repository is named `org` (it houses the organizational landing zones and names the
  organization's own context, pairing with `docs`), fresh history over filtered migration,
  annex scope limited to the engagement entries, annex homed in `.github-private`.
- **Retained:** `concepts/leadership-brief.md`, its visibility open item resolved — the
  coordination context is now publicly fetchable, so the interview prompt has public reach.
- **Cross-repo:** roadmap — `backlog.public-workspace` deleted, its wait-on phrase dropped from
  `backlog.leadership-brief`, `next` advanced.

## Next-focus

`backlog.leadership-brief` — the standardized brief for the CTO and technical leaders: vision
overview led by the thesis, a roadmap summary from `goals.v1`, the Organization Contents link
tree, and the pasteable agent-interview prompt with its hosted `interview.md`. Honest
early-prototype tone. Contents, tone, and open items: `context/concepts/leadership-brief.md`.
Runs in standards-lab (org). `v1.testing` follows.
