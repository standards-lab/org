# reset · workflow-refinement

- **Status:** closeout
- **Session:** plan
- **Project:** standards-lab, claude-plugins
- **Branch:** workflow-refinement

## Disposition

- **Add or sharpen:** four new concept notes at `claude-plugins/context/concepts/`:
  `checkpoint-execution.md` (autonomous stage commits gated by behavioral checkpoints, replacing
  per-stage architect review; the delegated review-and-report pattern formalized as `close`'s
  default, with the review's alignment check defaulting to ecosystem idiom rather than an assumed
  architecture), `flat-context.md` (collapsing `context/design` and `context/concepts` into a
  single flat `context/` tier, a note's own prose carrying its settledness instead of its
  directory), `isolated-experiments.md` (a standalone marathon project outside the workspace tree,
  `[experiment] workspace = "org"`, connected back through the references convention, graduated by
  a coordinator `plan` session reading the closed experiment), and `roadmap-waves.md` (`next` as a
  list of waves in the shape `[workspace] order` already uses, plus a root `active` list). Each
  states what it touches and the open questions left for its implementing session.
- **Roadmap:** two new tasks under `goals.v1.harness.tasks` (`workflow-refinement`,
  `context-migration`), moved to the head of `next` ahead of `blobfs.sources` — the third sequence
  jump the header comment narrates. `workflow-refinement` consolidates what was drafted as four
  separate `claude-plugins` tasks (checkpoint execution, flat context, isolated experiments,
  roadmap waves) into one session; `context-migration` consolidates what was drafted as two
  separate repository reviews (`standards-lab`, `go-web-service`) into one cross-repo session. Both
  consolidations are the architect's direction — each combined change is delicate enough to want
  one continuous session's attention. `blobfs.sources` and everything after it is unaffected in
  substance, only in when it starts.
- **Cleanup:** four unused plugins uninstalled directly (`claude plugin uninstall`), not through a
  marathon session, per the architect's confirmation they're unused on this workspace or any
  active project: `dev-workflow`, `iterative-dev`, `project-management`, `go-patterns` (all
  `tau-marketplace`). `~/claude-settings` was reviewed and needs no cleanup — it's lean and
  current, and the "companion `behavior/voice.md`" cross-reference in its git history is a
  resolved duplication (marathon core carries no such file; it was deliberately merged out of the
  skill into `claude-settings` in an earlier commit there).
- **Cross-repo:** none — the four concept notes are new files at `claude-plugins`, not edits to
  anything that repository's own context previously asserted.

## Next-focus

`v1.harness.workflow-refinement`, in the `claude-plugins` repository: one `start` session
implementing all four settled changes — checkpoint execution and the closing branch-review
pattern, the flat `context/` tier, isolated experiments, and roadmap waves. Start from the four
concept notes in dependency order: `context/concepts/checkpoint-execution.md`,
`flat-context.md`, `isolated-experiments.md`, `roadmap-waves.md` — each carries its own settled
design and the open questions left for that portion's stages (report file location/lifecycle,
whether `[agents]` needs a review-role key, whether Interrupt needs a hook, and so on). Stage the
session's own list by the same order; each note's "Files this touches" section names where the
stages land. The session ends with `scripts/check.sh` passing and marathon 0.13.0,
marathon-architecture 0.2.0, and marathon-roadmap 0.2.0 all reinstalled — reinstalling is required
before the new pipeline is the one actually running (`claude-plugins/CLAUDE.md`: "changes here
take effect once reinstalled"). `v1.harness.context-migration` — one cross-repo session covering
both `standards-lab` and `go-web-service` — and the first two isolated experiments (`v1.messaging`,
`v1.ai`) wait behind it, per the roadmap's own dependency ordering.
