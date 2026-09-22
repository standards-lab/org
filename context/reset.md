# reset · workflow-refinement

- **Status:** closeout
- **Session:** start
- **Project:** claude-plugins, standards-lab
- **Branch:** workflow-refinement

## Disposition

- **Integrated:** `claude-plugins/context/concepts/` `agent-profiles.md`, `checkpoint-execution.md`,
  `flat-context.md`, `isolated-experiments.md`, and `roadmap-waves.md` are deleted. marathon
  0.13.0, marathon-architecture 0.2.0, and marathon-roadmap 0.2.0 express them. The
  implementation departs from the notes in five settled ways:
  - Experiments have one form, a standalone project hosted where its SETTLE decides, and in-tree
    `experiments/` is gone.
  - Waves are lanes run in concurrent sessions, with per-lane records folded in one commit, and
    the roadmap has no `active` list.
  - The skill carries no legacy-layout rule.
  - The reviewer returns its report as text, because Claude Code refuses report files written by
    subagents.
  - The skill and extension markdown is trimmed from 1,806 lines to 1,200.
- **Add or sharpen:**
  - `claude-plugins/context/` is flat: `harness-testing.md`, `marathon-references-extension.md`,
    and `marathon-sitrep.md` move up with settledness lines.
  - The capability map in `README.md` covers the 0.13 workflow.
  - `roadmap.toml` deletes `v1.harness.workflow-refinement` and widens `context-migration` to the
    whole workspace's alignment with 0.13. That includes moving the experiments to
    `github.com/JaimeStill/<slug>` with `experiments.md` and a recorded hosting convention.
  - The roadmap also adds `v1.messaging.experiment` and `v1.ai.experiment`, rewrites `next` with
    the storage, messaging, and ai wave, and repoints the three `claude-plugins` note citations.
- **Cross-repo:** `standards-lab/.claude/marathon.toml` drops `[workspace.agents]`, and
  `.gitignore` lists `.claude/report.md`.
- **Validated:**
  - Checkpoint 1, the execution model: a walkthrough of a cross-repo `start` through the
    planner, stage commits, a checkpoint, a handoff, and `close`. Adjusted so `.gitignore` is
    confirmed before the report is written.
  - Checkpoint 2, flat context: a walkthrough of a `review` migrating go-web-service. Adjusted to
    drop the legacy-layout clause.
  - Checkpoint 3, experiments: a walkthrough of `v1.messaging` from setup to graduation. Adjusted
    to settle the host and archive at graduation, then to make the standalone form the only one.
  - Checkpoint 4, waves: a walkthrough of two lanes, a handoff, and the fold. Adjusted to drop
    `active` and add per-lane records.
  - Checkpoint 5, validation: `scripts/check.sh` passes, the TOML examples and `roadmap.toml`
    parse, and no removed concept is referenced. Adjusted to add lanes.
  - The branch review's 17 findings were fixed and confirmed.
- **Cleanup, after publish:** with the architect's go-ahead, delete `~/claude-settings` and its
  GitHub repository, and remove the `~/.claude/CLAUDE.md`, `behavior`, and `tools` symlinks.
  `context-migration`'s prose is the test of whether the voice standard is missed.

## Next-focus

`v1.harness.context-migration`, a cross-repo session run alone under marathon 0.13. It starts at
the coordinator and aligns standards-lab, go-web-service, architecture, go-web-sdk, and
go-web-sdk-template with the Migrating section of 0.13's CHANGELOG, per the task's roadmap
summary. It is likely more than one sitting. The wave `[[blobfs.build, blobfs.admin,
v1.storage.service, v1.storage.suite], v1.messaging.experiment, v1.ai.experiment]` follows it.
