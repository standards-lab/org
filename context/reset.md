# reset · context-migration

- **Status:** closeout
- **Session:** start
- **Project:** claude-plugins, go-core, sqlate, go-database, go-web-sdk, go-observability, go-storage, go-web-sdk-template, go-web-service, architecture, standards-lab (with .github and .github-private)
- **Branch:** context-migration

## Disposition

- **Integrated:**
  - standards-lab: `architecture-layer`, `context-architecture`, `testing-hierarchy`, and
    `reference-architecture-context` (the architecture repository's principles);
    `dsl-driven-services` (sqlate `docs/concepts.md` and the go-elemental DSL-driven principle);
    `observability-strategy` (the go-observability README's Design section); `storage-strategy`
    (go-storage `docs/design.md`); `standards` and `repo-references` (`references.md`);
    `workspace-structure` (`CLAUDE.md`); `blueprint-organization`'s roles (the profile and the
    architecture README); `tooling-principles` (the architecture's harness and principle pages).
  - go-web-service: `documented-layers` (`CLAUDE.md`), `stack` (the README's Stack section),
    `slab-conventions` (`tools/slab/README.md` and six new `doc.go` files).
  - go-web-sdk: the wiring-time-methods idiom (`doc.go`).
- **Culled:** go-web-service `identity-linking` and `organization-lineage` (the auth strategy
  holds both); standards-lab `marathon-extraction` (claude-plugins holds it) and
  `dependency-sourcing` (landed in architecture).
- **Add or sharpen:** every remaining note moved to a flat `context/` under the current-truth rule
  in go-web-sdk, go-web-sdk-template, go-web-service, architecture, and standards-lab; new
  `sqlate/context/dollar-quoting.md`, `go-storage/context/provider-assumptions.md`, and
  `standards-lab/context/graduation.md`; `admin-listener.md` carries its requirement and current
  findings.
- **Promoted:** `dependency-sourcing`, the harness rules from `testing-hierarchy`, and the
  promote-on-fit rule landed as notes in `architecture/context/` for pages a session there
  writes.
- **Cross-repo:**
  - architecture: go-elemental `dependencies.md` now separates a library with no HTTP concern,
    which supplies a collaborator, from an HTTP-shaped one, which exposes
    `func(http.Handler) http.Handler` over `net/http` alone; `configuration-boundaries.md` and
    the harness README drop the archived claude-settings example.
  - go-observability: the docs said `Telemetry` registers at stage 0; they now say startup and
    shutdown hooks, as the service wires it.
  - Spikes: `JaimeStill/spike-blobfs` and `JaimeStill/spike-sql-dsl`, split with history,
    public and archived; `experiments.md` catalogs them with the hosting convention.
  - Profiles: `.github` and `.github-private` list only the organization-level repositories.
  - `.claude/report.md` is gitignored in all 11 repositories; `~/architecture/voice.md` is
    deleted.
- **Roadmap:** deleted `v1.harness.context-migration`; `next` now opens on the wave. Added
  `backlog.entrypoint-composition-split`; `docs-clarity` dropped at the architect's call. Every
  context path resolves, and principle numbers are section names.
- **Validated:**
  - Checkpoint 1: a note written under the new rules, traced rule by rule.
  - Checkpoint 2: a cross-repo `start` traced through marathon 0.14, `check.sh`, and CI on
    claude-plugins PR #31.
  - Checkpoint 3: the lower-layer repositories' docs, with each module's build, vet, test, and
    lint.
  - Checkpoint 4: a real-client-IP walkthrough through the sourcing note, the dependency-sourcing
    note, and `dependencies.md`.
  - Checkpoint 5: a profile-to-standard-to-roadmap walkthrough; relative links, backticked
    paths, GitHub URLs, and roadmap paths all resolve.
  - Checkpoint 6: no old directories; citations resolve; 15 Go modules build (`GOWORK=off`),
    vet, test, and lint; `sqlint` and `check.sh` pass; no settledness lines; the editor pass on
    Opus; the branch review's 11 findings fixed as an Adjust and rechecked.

## Next-focus

The wave `next` opens on, from the coordinator. Three lanes, each with its record at
`context/reset/<lane>.md`:

- **blobfs-build**: `blobfs.build`, `blobfs.admin`, `v1.storage.service`, `v1.storage.suite`,
  in order.
- **messaging-experiment**: `v1.messaging.experiment`, a `plan` session settling the question
  and the decision it changes, then an `experiment` session setting up the spike.
- **ai-experiment**: `v1.ai.experiment`, the same two sessions for AI.

The lanes share no member repository. Edits a lane would make to shared coordinator files —
`roadmap.toml` and `experiments.md` — go in its Disposition and are applied when the wave folds.
The architect names each session's lane.
