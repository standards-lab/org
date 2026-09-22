# reset · harness-realignment

- **Status:** closeout
- **Session:** plan
- **Project:** standards-lab, claude-plugins
- **Branch:** harness-realignment

## Disposition

- **Add or sharpen:** `claude-plugins/context/concepts/agent-profiles.md` is new. It moves
  delegation from model-named agents to three purpose profiles marathon ships: the planner
  designs the session's initial stage list, the executor runs one technical stage, and the
  reviewer reviews the branch after the final technical stage. No profile pins a model. The
  session chooses the model for each engagement and states the profile, the model, and the reason
  before engaging it. The `[agents]` and `[workspace.agents]` tables leave the configuration
  schema. `claude-plugins/context/concepts/checkpoint-execution.md` now names the executor and
  reviewer profiles instead of `opus` and `fable`, and its open question about a review-role key
  is gone, since the profiles answer it.
- **Roadmap:** `goals.v1.harness.tasks.workflow-refinement` gains the agent-profiles change in
  its name, summary, proof, and context. The header comment records that `blobfs.sources` ran
  ahead of the harness wave and that the wave resumes at the head. The previous reset set
  Next-focus to `blobfs.build`, which contradicted `next`; this reset corrects that. Everything
  that reset carried for `blobfs.build` already lives in the notes: its scope in
  `goals.blobfs.tasks.build`, the sqlate backlog entries in `concepts/sqlate-library-support.md`,
  and the architecture-layer triggers in `concepts/blobfs.md`. That task's summary drops its
  fallback for a slipped `blobfs.sources`, since sqlate v0.2.0 is released.
- **Cleanup:** `~/claude-settings` drops its `agents/` directory (`fable.md`, `opus.md`) and
  `behavior/model-routing.md`, with the matching lines in `CLAUDE.md`, `install.sh`, and
  `README.md`, and the `~/.claude/agents` symlink is removed. This is a direct change to the
  architect's user-level configuration, not a marathon session, the same way the
  workflow-refinement planning session uninstalled unused plugins.
- **Cross-repo:** none this session. Dropping `[workspace.agents.fable]` and
  `[workspace.agents.opus]` from `standards-lab/.claude/marathon.toml` belongs to
  `workflow-refinement`, as a Cross-repo edit that lands when marathon's schema drops `[agents]`.
  Until then, marathon 0.12 still reads those tables, and they name agents that no longer exist,
  so a session does the work itself or uses a built-in agent.

## Next-focus

`v1.harness.workflow-refinement`, in the `claude-plugins` repository: one `start` session that
implements five settled changes. Start from the five concept notes in `claude-plugins/context/concepts/`,
in this order:

1. `checkpoint-execution.md`: checkpoint execution and the closing branch review.
2. `agent-profiles.md`: the planner, executor, and reviewer profiles. It shares
   `behavior/delegation.md` and `commands/close.md` with the first note, so stage them together.
3. `flat-context.md`: the flat `context/` tier.
4. `isolated-experiments.md`: isolated experiments.
5. `roadmap-waves.md`: roadmap waves.

Each note names the files it touches and the open questions left for its stages. The session
also removes `[workspace.agents]` from `standards-lab/.claude/marathon.toml` as a Cross-repo edit.
It ends with `scripts/check.sh` passing, and with marathon 0.13.0, marathon-architecture 0.2.0,
and marathon-roadmap 0.2.0 released and reinstalled. The new pipeline only runs once the plugins
are reinstalled (`claude-plugins/CLAUDE.md`: "changes here take effect once reinstalled").
`v1.harness.context-migration`, one cross-repo session covering `standards-lab` and
`go-web-service`, waits behind it. So do the first isolated experiments (`v1.messaging`,
`v1.ai`), per the roadmap's own ordering.
