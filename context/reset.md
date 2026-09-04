# reset · stage-review-gate

- **Status:** closeout
- **Session:** start
- **Project:** claude-plugins, standards-lab, go-database, go-web-sdk, go-web-sdk-template, go-web-service, docs
- **Branch:** stage-review-gate

## Disposition

- **Integrated:** `backlog.marathon-stage-review` and `backlog.marathon-workspace-experiments`
  into the marathon skill, released as marathon v0.10.0 with marathon-roadmap v0.1.4. Every
  working session now reports a stage with the working tree uncommitted and commits only on
  the architect's approval; in a workspace, every experiment lives at the coordinator's
  `experiments/`. Both tasks deleted from the roadmap; `next` advanced.
- **Integrated:** `design/dsl-driven-services.md` consolidated so every section states the
  current position, the dated adjustment sections and the review appendix folded in, a
  History section keeping each adjustment by date, the open questions in one section.
  `concepts/sqlate.md` rewritten for clarity with every claim kept.
- **Integrated:** the workspace's written context read whole under the refactored voice
  standard (`~/claude-settings/behavior/voice.md`, which now names the compressed register
  the sqlate page exhibited: coined project words, noun piles without verbs, dropped kinds,
  metaphor for mechanism). Stale claims the prototype's close left behind are corrected in
  every repository; retired task names are replaced by the integration tasks; the experiment's
  `REVIEW.md` §1 to §4 and §7 and `NOTES.md`'s Position and Ontology are rewritten, §5 and §6
  of the review recorded as applied.
- **Promoted:** `design/workspace-structure.md` gains the Experiments section: the coordinator
  keeps the workspace's only `experiments/`, the spike depends on member modules at published
  versions, and a change it implies reaches a member only by promotion at close.
- **Culled:** the sqlint finding "holds one statement" in `experiments/sql-dsl`, now "contains
  exactly one statement", with its doc comment and tests.
- **Retained:** go-database `concepts/sql-architecture.md` with its dated outcome banner, for
  the `database` task to cull; the docs landing-zone pages, for the docs pass; the go-core
  context, unchanged.
- **Cross-repo:** context corrections on a `stage-review-gate` branch in each of go-database
  (the capability map and the v0.4 notes record the sqlate split), go-web-service (the
  integration tasks cited; dated banners on the composition root and the stack; the
  multi-entity tension resolved), docs (dated banners on the DSL docs pass and the SQL meta
  language), go-web-sdk (the `v1.web` goal and the listener task cited), go-web-sdk-template
  (the template task named as next), and claude-plugins (the session record lives at the
  coordinator). Outside the workspace: the voice standard refactored in `~/claude-settings`
  (three commits on `main`, unpushed), and the sqlate page republished at
  `https://claude.ai/code/artifact/bb8d3625-e7b9-4e8c-980c-fce374f69c99`.

## Next-focus

`v1.data.sql.integration.sqlate`, a `start` session at standards-lab: create the `sqlate`
repository and module from `experiments/sql-dsl/lib` and `cmd/sqlint` per `concepts/sqlate.md`
and `experiments/sql-dsl/REVIEW.md` §3.1, insert it into the workspace order beside go-core,
and move the concept into the repository's own context.
