# reset · ai-experiment

- **Status:** closeout
- **Session:** plan
- **Project:** standards-lab
- **Branch:** ai-decomposition

## Disposition

- **Add or sharpen:**
  - New `context/ai-strategy.md`. It holds:
    - the AI capability's four layers and their homes: hosting, harness, library, and service
    - the pivot from tau to external harnesses. The note maps tau's responsibilities, names
      herald as the proof workload, and cites `claude-classify-docs` as the prior art that
      showed a harness can run the workflow without Go infrastructure.
    - go-ai's two-surface hypothesis: a harness-session surface and a model-client surface
    - the questions and decisions for spike-harness-driver and spike-local-subagents
  - New `context/ai-hosting.md`. It holds:
    - the hosting layer's planned home, `ai-hosting` (working name): one public,
      specification-level repository, built fresh
    - personal-agents adopted as the hosting spike, with its question and decision
    - what moves into `ai-hosting`, what moves into the architecture layer, and what goes
      nowhere
    - the visibility rule
    - the naming and admin-tooling (`outpost`) questions, left for `v1.ai.hosting`'s SETTLE
  - `context/README.md` lists both notes.
- **Retained:**
  - personal-agents stays outside the workspace until `v1.ai.hosting`. That task builds
    `ai-hosting` fresh, then archives personal-agents with a forward link.
  - For the architecture layer, after the hosting spike validates them: pages for model tiers,
    context sizing, the memory-footprint method, and the serving conventions. None is promoted
    yet, because both notes are provisional.
- **Roadmap** (recorded for the wave fold, not applied):
  - `goals.v1.ai`:
    - Rewrite the summary. The capability splits into four layers: hosting in `ai-hosting`, the
      harness in `v1.harness.local-models`, and go-ai and the service layer here. go-ai's
      two-surface shape and tau's retirement depend on spike-harness-driver.
    - Add `context = ["standards-lab/context/ai-strategy.md", "standards-lab/context/ai-hosting.md"]`.
  - `goals.v1.ai.tasks.experiment`: the plan half is done. The summary now reads as three
    `experiment` sessions, run in order:
    - setting up spike-harness-driver
    - setting up spike-local-subagents
    - adopting personal-agents as the hosting spike

    Each session catalogs its experiment. The task stays in `next`.
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
- **Validated:**
  - Checkpoint 1 was a walkthrough, confirmed by the architect. It traced two results through
    the notes:
    - spike-local-subagents recommending a gateway lands in `v1.harness.local-models` and the
      `ai-hosting` specification.
    - spike-harness-driver concluding "harness-only" gives go-ai one surface and retires tau.
      tau also retires under "both surfaces".
  - Every cited path, section name, and roadmap path resolves.
  - The editor pass ran on Opus. The session neutralized one heading that asserted the spike's
    outcome before it was known.
  - The architect's call: llama.cpp's `/v1/messages` compatibility with Claude Code is
    discovered in spike-local-subagents, and alternatives are explored there if it fails.

## Next-focus

`goals.v1.ai.tasks.experiment`, second session. This is an `experiment` session in standards-lab
that sets up spike-harness-driver:

- the local directory is `~/experiments/spike-harness-driver`
- the remote is `github.com/JaimeStill/spike-harness-driver`
- the question, the decision, and the evidence list come from `context/ai-strategy.md`
- it reads herald and tau as reference repositories

The catalog entry for `experiments.md` goes in this lane's Disposition and is applied at the wave
fold. Two more sessions follow: setting up spike-local-subagents, then adopting personal-agents
as the hosting spike (`context/ai-hosting.md`). After those, the lane is finished.
