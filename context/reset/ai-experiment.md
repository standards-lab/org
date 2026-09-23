# reset · ai-experiment

- **Status:** closeout
- **Session:** experiment
- **Project:** standards-lab
- **Branch:** spike-harness-driver

## Disposition

- **Add or sharpen:** `context/ai-strategy.md`, "Experiment: spike-harness-driver". At SETTLE,
  the architect reframed the question. It no longer reruns herald's workflow. It now asks whether
  Go can drive an external harness as the infrastructure for agentic work, in four parts:
  capabilities (skills, tool calls, vision, embeddings, and audio), sessions across turns,
  scoped exchanges bound to a session, and long-running workflows. herald moves from proof
  workload to prior art. The hypothesis now turns on whether the harness-session surface covers
  the native capabilities and the workflows. The section also names the experiment's home.
- **Cross-repo:** set up `~/experiments/spike-harness-driver` with `marathon init`, as a `code`
  project with `[experiment] serves = "standards-lab"`, and published it public at
  [JaimeStill/spike-harness-driver](https://github.com/JaimeStill/spike-harness-driver). Its
  `references.toml` lists tau-protocol, tau-format, tau-provider, tau-agent, tau-orchestrate,
  tau-platform, and herald as read-only references, and a gitignored `references.local.toml`
  maps them to local checkouts.
- **Catalog** (recorded for the wave fold, not applied): add this row to `experiments.md`:
  `| spike-harness-driver | Can Go drive an external agent harness (Pi, Claude Code, OpenCode) as the infrastructure for agentic work: capabilities, sessions, scoped exchanges, and long-running workflows? | [JaimeStill/spike-harness-driver](https://github.com/JaimeStill/spike-harness-driver) |`
- **References** (recorded for the wave fold, not applied): `ai-strategy.md` cites
  `tau-platform`, which `references.toml` doesn't catalog. At the fold, add
  `[repos.tau-platform]` with remote
  `https://github.com/tailored-agentic-units/tau-platform.git` (public), and
  `tau-platform = "~/tau/tau-platform"` to `references.local.toml`, with an entry in
  `references.md`'s TAU section.
- **Roadmap** (recorded for the wave fold, not applied). This carries forward the plan session's
  edits and amends them:
  - `goals.v1.ai`:
    - Rewrite the summary. The capability splits into four layers: hosting in `ai-hosting`, the
      harness in `v1.harness.local-models`, and go-ai and the service layer here. go-ai's
      two-surface shape and tau's retirement depend on spike-harness-driver.
    - Add `context = ["standards-lab/context/ai-strategy.md", "standards-lab/context/ai-hosting.md"]`.
  - `goals.v1.ai.tasks.experiment` stays in `next`. Its summary becomes: the three spikes' setup
    (spike-harness-driver, then spike-local-subagents, then personal-agents adopted as the hosting
    spike), each spike's run through its own path, and one combined intake session after all
    three close. The intake session's close records the task's deletion.
  - The `next` header comment's lane-as-goal rule is recorded once, in the messaging-experiment
    lane's record, and this lane follows it.
  - Add `goals.v1.ai.tasks.hosting`, the task `v1.ai` opens on:
    - name "The ai-hosting specification repository"
    - repos `["ai-hosting", "architecture", "standards-lab"]`
    - summary: build `ai-hosting` fresh from the hosting spike's result, add it to the
      workspace's `order` and the references catalog, land the portable-method pages as notes in
      architecture, and archive personal-agents with a forward link. Waits on the hosting spike's
      close.
    - context `["standards-lab/context/ai-hosting.md"]`
  - Add `goals.v1.harness.tasks.local-models`:
    - name "Local models as Claude Code subagents"
    - repos `["claude-plugins", "architecture"]`
    - summary: the convention for delegating subagent work to locally hosted models (profile
      format, routing mechanism, and which kinds of task may run locally), from
      spike-local-subagents' result. Waits on that spike's close.
    - context `["standards-lab/context/ai-strategy.md"]`
  - The go-ai library and service tasks stay unwritten until the spike-harness-driver intake.
- **Retained:**
  - personal-agents stays outside the workspace until `v1.ai.hosting`.
  - The architecture-layer candidates (model tiers, context sizing, the memory-footprint method,
    and the serving conventions) wait for the hosting spike. Nothing is promoted, because both
    AI notes are provisional.
- **Validated:**
  - The experiment's setup: `.claude/marathon.toml` parses with `[experiment] serves`, and each of
    the seven references resolves to a local checkout whose `origin` matches the committed remote.
  - Checkpoint: `gh repo view JaimeStill/spike-harness-driver` shows it public, and local `main`
    matches `origin/main`. The architect confirmed the publish.
  - Every path, section name, and roadmap path this record and `ai-strategy.md` cite resolves.
  - The editor pass ran on Opus over this record and the changed `ai-strategy.md` passages.
  - The architect's calls after the first publish: the lane runs until its spikes' results are
    taken in, through one combined intake session. spike-harness-driver's `main` now carries its
    six-step path, pushed.

## Next-focus

The lane stays open until all three spikes finish and their results are taken in. Its remaining
steps, in order:

1. `goals.v1.ai.tasks.experiment`, third session: an `experiment` session here that sets up
   spike-local-subagents.
   - The local directory is `~/experiments/spike-local-subagents`.
   - The remote is `github.com/JaimeStill/spike-local-subagents`.
   - The question and the decision come from `context/ai-strategy.md`, "Experiment:
     spike-local-subagents".
   - The spike's README gets a path whose last step answers the question.

   Its catalog entry goes in this lane's Disposition and is applied at the wave fold.
2. An `experiment` session here that adopts personal-agents as the hosting spike
   (`context/ai-hosting.md`). It gives the spike a path the same way.
3. The spikes' own `start` sessions, each run in its project from that project's own reset file
   and following the path in its `context/README.md`. spike-harness-driver's path has six steps,
   and step 1 is the session interface and a Pi adapter. The spikes never edit this repository,
   so this record doesn't change while they run.
4. After all three spikes' final closes, one `plan` session here on `v1.ai.experiment`. It reads
   each close Disposition (the question, the answer, and the evidence), and decides with the
   architect:
   - go-ai's shape, and whether a service's container carries a harness
   - the `v1.harness.local-models` convention
   - what `v1.ai.hosting` builds

   It records the decisions as notes, and as roadmap edits for the fold. It confirms each spike
   is pushed, archives the spikes' remotes (personal-agents stays until `v1.ai.hosting`), and
   writes `Lane finished.` here. If that finishes the wave's last lane, it folds the wave.
