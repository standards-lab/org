# reset · context-migration

- **Status:** handoff
- **Session:** start
- **Project:** claude-plugins, standards-lab
- **Branch:** context-migration

## Disposition

- **Cross-repo:** claude-plugins PR #31, merged and tagged `marathon/v0.14.0`,
  `marathon-architecture/v0.2.1`, and `marathon-roadmap/v0.2.1`, with all three release
  workflows passing.
  - marathon 0.14.0 replaces settledness lines with the current-truth rule. A note or document
    states what exists, in the present tense, or what is planned, marked as planned. The roadmap
    and the reset file track status, and CHANGELOGs are exempt.
  - It adds the `editor` profile, which makes one pass over a branch's prose before the final
    checkpoint.
  - It rewrites the marathon, marathon-architecture, and marathon-roadmap prose for clarity. A
    rule-inventory check of old against new confirmed the rules are unchanged.
  - `claude-plugins/context/marathon-extraction.md` now holds the coordinator's
    `concepts/marathon-extraction.md`, rewritten. The coordinator copy is deleted in stage 17.
- **Validated:** checkpoint 1, a note written under the new rules and traced rule by rule.
  Checkpoint 2, a cross-repo `start` traced through the rewritten files, `scripts/check.sh`, and
  CI on PR #31.
- **Pending:** the architect's go-ahead on the `~/claude-settings` cleanup, carried from the
  previous closeout.

## Next-focus

`v1.harness.context-migration`, resumed under marathon 0.14.0 after the architect reloads the
plugins. Run it from the coordinator.

Stages: 6/19 · checkpoints 1 and 2 of 6 confirmed · stage 6 committed and published · the
claude-plugins branch is merged and deleted. The standards-lab branch holds only this record, and
no other repository has a branch yet.

Rules for every remaining stage:
- Notes follow the current-truth rule.
- Repository docs match sqlate's README and `docs/`: concise and current.
- The editor pass runs once per repository at final validation.
- Delegate to Opus only; there is no Sonnet delegation until Sonnet 5.5 releases.

The remaining stage list, in `order`:

- **Checkpoint 3: the lower-layer repositories.**
  - 7 · go-core and go-database: add `.claude/report.md` to `.gitignore`.
  - 8 · sqlate:
    - Add the rejected alternatives from `dsl-driven-services` §3 to `docs/concepts.md`.
    - Add a note on the §9 dollar-quote stripper to `context/`.
    - `.gitignore`.
  - 9 · go-web-sdk:
    - Add the wiring-time-methods idiom to `doc.go`.
    - Move `error-handling.md` and `middleware-sourcing.md` flat, trimmed.
    - The Placement bullet 3 contradiction goes to architecture in stage 14.
    - `.gitignore`.
  - 10 · go-observability:
    - Add the reasoning from `observability-strategy` to the README.
    - Resolve the stage-0 contradiction against the code.
    - Repoint `otlp/doc.go:7` and the `context/README.md` citations.
    - `.gitignore`.
  - 11 · go-storage:
    - Add the reasoning from `storage-strategy` to the README or `docs/`.
    - Add `context/provider-assumptions.md`.
    - `.gitignore`.
- **Checkpoint 4: the template, the service, and architecture.**
  - 12 · go-web-sdk-template:
    - Move `scaffolding-cli.md` flat and fix `infrastructure.New` to `newInfrastructure`.
    - `.gitignore`.
  - 13 · go-web-service:
    - Merge `documented-layers` into `CLAUDE.md`.
    - Move `stack` into the README.
    - Move `slab-conventions` into `tools/slab/README.md`, plus `doc.go` files for `demo`,
      `scenario`, `httpx`, `env`, `repo`, and `statement`.
    - Move `domain-architecture`, `data-layer`, and `integration-tier` flat.
    - Cull `identity-linking` and `organization-lineage`.
    - Fix the citations and add `.gitignore`.
  - 14 · architecture:
    - Move `entrypoint-composition-split` and `sql-meta-language` flat.
    - Land notes on dependency-sourcing, the testing harness rules, the "promote on fit" rule,
      and the middleware-placement contradiction.
    - Update `CLAUDE.md:19-22` and add `.gitignore`.
- **Checkpoint 5: standards-lab.**
  - 15 · Move the experiments out:
    - `git subtree split` each into `~/experiments/spike-<slug>`, and rewrite its module path to
      `github.com/JaimeStill/spike-<slug>`.
    - Create each remote with `gh repo create`, push it, and archive it.
    - Delete `experiments/`.
    - Add `experiments.md` with the hosting convention.
  - 16 · Design notes:
    - Integrate `architecture-layer`, `context-architecture`, `standards`, `repo-references`,
      `workspace-structure`, `dsl-driven-services`, `observability-strategy`, `storage-strategy`,
      `testing-hierarchy`, and `reference-architecture-context`.
    - Split `blueprint-organization`, with its open questions moving to `graduation.md`.
    - Delete `dependency-sourcing`, which stage 14 promotes.
    - Move `auth-strategy`, `service-organization`, and `naming` flat.
  - 17 · Concept notes:
    - Integrate `tooling-principles` and delete `marathon-extraction`.
    - Move the rest flat, trimmed.
  - 18 · The brief, the interview, and the profiles:
    - Single homes: the go-elemental README holds the module list, and the profile holds only
      the organization-level repositories.
    - The brief and the interview link only the fixed entry points: the profile, the architecture
      repository, the standards index, the harness, and the roadmap.
    - The v1 target links the live roadmap.
    - The architect confirms whether to cut the "holding pattern" paragraph.
    - Touches `.github` and `.github-private`.
  - 19 · Citations:
    - `CLAUDE.md`, `references.*` (including `private/`), and the capability map.
    - The `roadmap.toml` paths, and principle numbers renamed to section names.
    - Backlog items `entrypoint-composition-split` and `docs-clarity`.
    - Widen `repos`.
    - Delete `~/architecture/voice.md`.
- **Checkpoint 6: final validation.**
  - No `design/`, `concepts/`, or `experiments/` directories remain, and every citation resolves.
  - `.gitignore` in all 11 repositories.
  - Build and test pass, and `check.sh` passes.
  - No settledness lines remain.
  - The spikes are archived.
  - The editor pass, and the architect's sample read.

The per-note trimming detail comes from the three triage passes. Re-derive it at each stage from
the notes themselves, applying the current-truth rule.

Next move: after the reload, run `/marathon:marathon start`. It resumes at stage 7.
