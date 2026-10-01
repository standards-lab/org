# reset · software-factory

- **Status:** closeout
- **Session:** plan
- **Project:** standards-lab, claude-plugins, architecture
- **Branch:** software-factory

## Disposition

The architect's retrospective on marathon 0.15 became the design for reshaping the workflow into
a software factory. The roadmap now follows that design: goals and tasks, with each goal
`active`, `planned`, or `backlog`, and no wave, lane, fold, or `next`.

- **Add or sharpen:**
  - claude-plugins:
    - `context/marathon-factory.md`: the pipeline and its two architect touchpoints, the five
      profiles, checks first, plugin evals, and the retro.
    - `context/marathon-goals.md`: goals as the unit of parallel work, the repository lock,
      goal records, and sync.
    - `context/marathon-briefs.md`: every format the architect reads.
    - `context/marathon-commands.md`: the command set.
    - `harness-testing.md`: rewritten to the evals convention.
    - `README.md`: the capability map, updated.
  - standards-lab:
    - `roadmap.toml`: restructured into three states.
      - Active: `factory`, `v1.messaging`, `v1.ai.experiment`.
      - New goals: `factory`, `quality`, `cli`, and backlog entries for the sweeper, the pins
        the libraries owe, a total past the last page, and `organization_image.active`.
      - `v1.harness.testing` is folded into `factory.evals`.
    - `cli-applications.md`: brought in from the `cli-application-type` worktree, with the
      cobra question added.
    - `sqlate-library-support.md`: gains the mid-read connection-loss entry.
    - `blobfs-composition.md`: trimmed to the protocols and operating constraints that nothing
      else states yet.
  - architecture:
    - `context/standards-audit.md`: principles the code contradicts, premature and duplicated
      pages, and the v1-storage promotion candidates. This is input for
      `quality.architecture-diet`.
- **Culled:**
  - `elemental-runtime-layers.md`, `graduation.md`, `docs-site.md`, and
    `staged-query-aggregation.md` become backlog goals.
  - `admin-listener.md`: the summary of goal `v1.admin-listener` keeps what the note said.
- **Synced:**
  - `v1.storage` and `blobfs`, both complete, are deleted from the roadmap.
  - The v1-storage lane record's pending edits are applied:
    - blobfs is added to `order` and to the references catalog.
    - The spike-blobfs promotion is recorded in `experiments.md`.
    - The intake items for v1.messaging, v1.client, and the backlog are added to the roadmap.
    - The promotion candidates go to the architecture audit.
  - The lane record is deleted.
- **Applied from open records:**
  - The catalog rows, references, and roadmap edits that `reset/messaging-experiment.md` and
    `reset/ai-experiment.md` held back for a fold are applied, including `tau-platform`.
  - Both records stay as their active goals' handoff state until `factory.goals` migrates them.
  - The `cli-application-type` worktree and branch are removed. Its additions are merged into
    `reset/ai-experiment.md` and `cli-applications.md`.
- **Validated:**
  - `roadmap.toml` parses. Every dotted path in `active`, `planned`, and `backlog` resolves, and
    so does every `context` path.
  - claude-plugins `scripts/check.sh` passes.
  - Nothing outside history and records cites a culled note.
  - `git worktree list` shows only the main checkout.

## Next-focus

The active goals run side by side, one session per task, and each locks its own repositories:

- **`factory.pipeline`**, in claude-plugins: marathon's next minor release, built from
  `claude-plugins/context/marathon-factory.md`, `marathon-briefs.md`, and
  `marathon-commands.md`. It runs under marathon 0.15 because the new pipeline is the thing
  being built. Then `factory.goals`, then `factory.evals`.
- **`v1.messaging`**: the intake session for the spike-messaging result
  (`reset/messaging-experiment.md`).
- **`v1.ai.experiment`**: spike-harness-driver's path continues in its own repository
  (`reset/ai-experiment.md`).
