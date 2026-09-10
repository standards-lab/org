# reset · model-routing-pass

- **Status:** closeout
- **Session:** start
- **Project:** claude-plugins, architecture, standards-lab (org), go-core, sqlate, go-database, go-web-sdk, go-web-sdk-template, go-web-service
- **Branch:** model-routing-pass

## Disposition

- **Integrated:** a personal, global Sonnet/Fable model-routing convention, outside this
  workspace's own git history. `~/.claude/agents/fable.md` and `~/.claude/agents/opus.md`
  define the two delegates (Fable for technical design and implementation, Opus for a rare
  high-stakes planning decision); `~/.claude/behavior/model-routing.md` states when to delegate,
  how to stay current on what a delegate produced, and that written artifacts are always
  finished in the orchestrator's own voice. Both agents are committed and pushed in the
  architect's `claude-settings` repository, not this workspace.
- **Integrated:** `harness/tool-based-skills.md` in the architecture repository. Its "When work
  is offloaded to another model" section conflated two claims; the rewrite states both
  separately — what a full tooling layer buys a model executing a skill, and the narrower case
  of moving structured, schema-validated generation to another model. `goals.v1.harness.tasks.hardening`
  in this repository's roadmap now names the institutional-layer model-routing work this
  principle grounds, alongside its existing scope.
- **Integrated:** a readability sweep across every written surface in all nine workspace repos —
  `context/*.md`, `CLAUDE.md`, the architecture repository's documentation pages, the docs guide
  in sqlate and the template, and godoc comments across the six Go modules. The dominant,
  repeated defects were opening lines with no subject or verb, series of four or more items left
  as running prose, sentences carrying more than one semicolon, and an em-dash used as a
  recurring parenthetical device rather than an occasional aside. The coined term "seam" is
  replaced throughout with the concrete mechanism each instance named (an interposed connection,
  an interface, a gap, a hook, a production surface). No documented behavior changed anywhere in
  the sweep; every touched Go module still builds, vets, and gofmts clean. Each repository landed
  its own commit on this branch and has an open pull request.
- **Retained:** `go-web-service/data/locks.go`'s `Lock` method doc comment. Its text is garbled
  ("for the rest of the transaction s is:") and needs the architect to confirm the intended
  wording before it's rewritten — flagged rather than guessed, per the architect's standing
  instruction not to guess at technical intent from unclear source.
- **Retained:** the standards-lab `context/design/dsl-driven-services.md` §10 History log's
  dense, semicolon-heavy dated entries. Flagged as needing a dedicated pass rather than a spot
  fix; out of scope here as a deliberate scope decision, since it is archival record-keeping
  content rather than prose read for understanding.

## Next-focus

`v1.harness.hardening` is still next, a `start` in claude-plugins, unchanged by this session — a
parallel readability effort, not an advance through the roadmap's `next` sequence. Two loose
ends precede it: confirm the intended wording for `data/locks.go`'s `Lock` comment and land that
as a follow-up commit on `go-web-service`'s open pull request, and merge (or otherwise resolve)
the nine open `model-routing-pass` pull requests this session opened. `harness/tool-based-skills.md`
in the architecture repository is the governing principle for the hardening task. After
hardening, `next` continues with `v1.auth.strategy` and the web service's layers.
