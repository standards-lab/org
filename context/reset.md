# reset · dsl-docs-pass

- **Status:** closeout
- **Session:** start
- **Project:** claude-plugins, architecture, standards-lab (org), go-core, sqlate, go-database, go-web-sdk, go-web-sdk-template, go-web-service
- **Branch:** dsl-docs-pass

## Disposition

- **Integrated:** the architecture layer replaces the docs landing zone. The architect ruled
  that the architecture repository holds only what generalizes past one repository (the
  architecture, the principles, the standards' principles, the harness principles, and a
  catalog of repositories) and that a repository documents its own implementation in its
  README, its package documentation, its source, and an optional README-indexed `docs/`. The
  `docs` repository is renamed `architecture` on GitHub and in the checkout; a README is every
  directory's index; the four module directories under `standards/go-elemental/` are removed
  after their principle content was promoted (peers compose in the application and a provider
  is selected by construction into `dependencies.md`, the wiring rule into
  `lifecycle-and-context.md`, no policy numbers in a library into `baseline-standards.md`, the
  composition root as one file per layer into `topology-and-naming.md`); the standard's README
  catalogs its members by repository link with sqlate adjacent and `dotnet-elemental`
  anticipated; the root README carries the what-belongs-here rule and the schema without the
  `module` and `page` types. The rule is codified in marathon (unreleased): the architecture
  layer is conventional, `[workspace] architecture` names the repository, `init` scaffolds
  `architecture/` in a standalone project, `review` checks promotion candidates, `close` lands
  a generalized note in the architecture repository, the `docs` command is removed, and a
  stage's unit follows what it produces. The coordinator's `design/workspace-docs.md` is
  rewritten as `design/architecture-layer.md`; the architecture repository's
  `concepts/architecture-layer.md` (the session's work list) and `concepts/dsl-docs-pass.md`
  (the module-page inventory, moot once the pages went) are deleted.
- **Integrated:** the principle pages the pass landed. In `principles/`: context architecture,
  validation-first layering, rolling currency (from `concepts/rolling-currency.md`, deleted), a
  standalone tool beside the library, and providers as adapters as a section of
  `service-tiers.md`. In `standards/go-elemental/principles/`: DSL-driven services with the
  authored-SQL conventions (the tier declaration as the port list, a statement named for its
  operation, validation as the entity's and existence as the store's, a host function only for
  a protocol SQL cannot guarantee, the three validation moments), and `tests-and-docs.md` and
  `release-and-ci.md` amended with the integration tier, the toolkit convention, and the
  green-integration-licenses-release rule. In `harness/`: tool-based skills, consolidating
  tooling principles 1, 2, 3, 4, 6, 8, 9, 10, and 11; `concepts/tooling-principles.md` reduces
  to the adoption map. `architecture.md`: a Domain Service serves a domain, a composition of
  one or more Entities around a root Entity, and the Principles section lists all seven.
- **Culled:** the module-page rewrites of stages 5 through 9 (six SQL pages, the go-database,
  go-core, go-web-sdk, and template rewrites), reverted as restatements of package
  documentation; the module directories themselves; `backlog.marathon-docs-extension` and
  claude-plugins' `concepts/marathon-docs-extension.md`, whose premise dissolved with the
  command; the grammar's-page open question in `design/dsl-driven-services.md`, resolved.
- **Retained:** `design/testing-hierarchy.md` (the harness rules stay design content; the
  toolkit packages' documentation states their APIs); the architecture repository's
  `concepts/entrypoint-composition-split.md` (its own session); `concepts/sql-meta-language.md`
  there, already reframed on 2026-09-03; the harness README's two conventions not yet paged.
  Debt on record, unchanged: go-database's `go.mod` pins go-core v0.3.0 and sqlate v0.1.0 and
  `postgres/go.mod` pins the base at v0.4.0; `experiments/sql-dsl/` keeps the retired slugs
  and the landing-zone vocabulary as an archive.
- **Cross-repo:** the six members' README Standard sections link the standard's README and
  state that the README and the package documentation document the repository; their
  `CLAUDE.md` and context READMEs say the same, and four notes (go-web-sdk's
  `error-handling.md` and `middleware-sourcing.md`, go-web-service's `domain-architecture.md`
  and `documented-layers.md`) drop the landing-zone claims. The coordinator: the order map and
  `[workspace] architecture`, the references catalog and local map, the brief, the interview,
  `CLAUDE.md`, and every context note adopt the architecture vocabulary and the README paths;
  `design/context-architecture.md` records the principle page; the roadmap's `goals.v1`
  criteria close a layer with its repository documentation, `goals.v1.tasks.repository-docs`
  joins `next` last, and `goals.v1.alignment` is deleted with its docs task, its criteria
  holding, so `next` opens at `v1.harness.hardening`; `v1.harness.sitrep` moves to the
  backlog as `backlog.marathon-sitrep` at the architect's direction, held until called for, so
  the web service's tasks follow hardening. claude-plugins: the marathon changes above and the
  changelog's Unreleased entry.

## Next-focus

`v1.harness.hardening`, a `start` in claude-plugins. Begin by cutting marathon's next release
from the Unreleased entry, so the architecture layer, the removed `docs` command, and the
generalized stage rule reach the workspace's sessions; then the task's own work: the
sufficiency question at the plan stage and the SQL conventions as `sqlint` called as a
package. `harness/tool-based-skills.md` in the architecture repository is the principle the
task applies. After hardening, `next` continues with `v1.auth.strategy` and the web service's
layers.
