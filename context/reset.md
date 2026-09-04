# reset · create-sqlate

- **Status:** closeout
- **Session:** start
- **Project:** standards-lab, sqlate, go-database
- **Branch:** create-sqlate

## Disposition

- **Integrated:** `v1.data.sql.integration.sqlate`. The `sqlate` repository exists at
  `github.com/standards-lab/sqlate`, created from `experiments/sql-dsl/lib` and `cmd/sqlint`,
  and is released: `v0.1.0`, `postgres/v0.1.0`, and `sqlint/v0.1.0` on 2026-09-04, each served
  by the proxy. The moved suites are green (93 tests), the seven engine proofs passed against
  PostgreSQL 18, and the linter runs as a package and installs by tag. The task is deleted from
  the roadmap and `next` advanced.
- **Integrated:** `design/dsl-driven-services.md` §4.1 and §8 now state what the build settled:
  the base module imports only the standard library and a sourced dependency enters through a
  sub-module (`postgres` over pgx, `sqlint` over the TOML parser); `header` is public because
  the linter is a separate module; one `sqlint.toml` per module at its root, sources and the
  engine named by module path and resolved through `go list -m`; `postgres` adds no migration
  catalog; the library is adjacent to the standard rather than a layer of it, and its own text
  names nothing of the architecture. `context/README.md` and `design/testing-hierarchy.md`
  point at the repository's guide where they pointed at the concept.
- **Integrated:** the workspace's task name for the integration tier is `integration`, beside
  `test` for the unit tier and `acceptance` for the one-shot engine proof; `design/testing-
  hierarchy.md` and the roadmap's `v1.data.sql.tasks.suite` say so.
- **Promoted:** `design/workspace-docs.md` distinguishes the architecture's documentation,
  which centralizes in the landing zone, from a standalone library's user guide, which lives
  with the library in its own `docs/`; sqlate is the first case. `backlog.marathon-docs-
  extension` notes that the marathon `docs` command's workspace rule needs the same distinction.
- **Culled:** `concepts/sqlate.md`. Its contents live in the repository: the name and packages
  in the README, the grammar in `docs/concepts.md`, the `sqlint.toml` schema in
  `docs/features.md`, the vocabulary in `docs/glossary.md`.
- **Retained:** `experiments/sql-dsl` as the record; the docs landing zone's pages for the docs
  pass (`v1.data.sql.tasks.docs`), which now has two items ready, the sqlate page and the
  topology-and-naming amendment for a prefix-less adjacent library, held until the pass.
- **Cross-repo:** the sqlate founding on its own `main` (the import commit, then PR #1 merged
  squashed, then the release commits: the base changelog dated, and the sub-modules pinned to
  v0.1.0 with their transient replace directives dropped); the repository description set;
  branches deleted on merge. In go-database, the three notes that cited the concept
  (`context/README.md`, `design/layers.md`, `concepts/sql-architecture.md`) cite the repository,
  on a `create-sqlate` branch. Outside the workspace: `~/claude-settings/behavior/voice.md`
  holds every piece of owned prose to the standard wherever it is met, and a verb names the
  relationship between subject and object in the mechanism's words (one commit on `main`,
  unpushed).

## Next-focus

`v1.data.sql.integration.database`, a `start` session at go-database: v0.4.0 over sqlate
v0.1.0. The removals (`ast`, `operation`, `exec`, `seed`, `Session`, `Tx`, `Dialect`, the
constraint classes), the `admin` package over sqlate, `postgres` without a dialect, and the
three context notes rewritten. The docs pass waits until after it; the sqlate page and the
naming amendment are its first two items.
