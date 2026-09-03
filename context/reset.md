# reset · sql-dsl

- **Status:** handoff
- **Session:** experiment
- **Project:** standards-lab
- **Branch:** sql-dsl

## Disposition

- **Retained:** go-database `concepts/sql-architecture.md` — its "Home" line still says
  `go-database/experiments/sql-dsl`, its decision 2 (`:name` parameters) is superseded by
  `{{name}}` / `{{name:type}}`, and its question 1 has a verdict (on-demand stitching over
  authored patterns; build-time generation set aside). Corrected at close, with the task's
  `repos`.
- **Retained:** roadmap `v1.data.sql.prototype` — `repos = ["go-database"]` is stale (the
  experiment lives at the coordinator); the `v1.data.sql` breakdown (`query`, `migrate` into
  go-database v0.4.0) is superseded by the `go-sql` split recorded under NOTES Q5. A handoff
  does not advance the manifest; both land at close.
- **Retained:** `standards-lab/context/design/dsl-driven-services.md` §3 — PRQL, considered and
  ruled out, lands in §3 at close.
- **Retained:** the two marathon backlog items (`backlog.marathon-stage-review`,
  `backlog.marathon-workspace-experiments`) follow the prototype's close; `next` is unchanged.

Every decision of this session is in `experiments/sql-dsl/NOTES.md`: the decisions log
(dated entries per stage and per review), Q1–Q5, and the sketch-deviations ledger. The
notes under `context/` were not edited this session.

## Next-focus

`v1.data.sql.prototype`, resumed as `experiment` on branch `sql-dsl` of this repository; the
spike is `experiments/sql-dsl`, a nested Go module. Read `experiments/sql-dsl/NOTES.md`
Position and the decisions log first — the log's last four entries (stage 10 and its three
reviews) carry the design stage 11 executes.

**Review gate (binding until the skill carries it):** a stage is not complete until the
architect has reviewed its diff; report with the working tree uncommitted, iterate until
approved, commit only on approval, and the architect states whether a reset follows. Open
each stage with a summary of what is in scope for that review.

**Position:** stages 1–10 of 12 are approved and committed, one commit per stage plus review
corrections; HEAD is `03be5fd`. The library (`lib/`: sqlheader, sqldb, pgdialect, drivertest,
migrate, query with patterns, mapper, projection, guard), the admin domain (migrations, seeds,
its own statements), two domains (organization, person) on authored SQL, and `cmd/sqlint` are
built, hermetically tested, proven live, and lint-clean.

**Exact next move — stage 11, the pattern catalog API**, as decided at the stage 10 review
(NOTES decisions log, "patterns and the application"), in this order:

1. `lib/query`: `Patterns(namespace, fs, dir)`, `Builtin()` (namespace `sql`, aliasable with
   `As`), `PatternSource.Overlay(fs, dir)` for explicit same-name replacement,
   `NewCatalog(sources...)`/`MustCatalog` validating tier and slots and refusing a duplicate
   namespace, and `Catalog.CompileStatements`/`MustCompileStatements` replacing the package-level `Load`
   and the global `patterns` (the catalog is built at the composition root; a domain loads
   its statements against it); a `Statement` carries its catalog as it carries its dialect.
2. `internal/data.Database{*sqldb.DB; Catalog}` built in `newInfrastructure`; the domains'
   and the admin service's `New` take it (domains define no patterns).
3. `admin/database/patterns/` as the application's namespace, registered at the root.
4. A MySQL-shaped `Overlay` test supplying `sql.paging` — the port proof.
5. `sqlint.toml`: roles as directory globs, checks switchable, sources with namespaces so
   includes resolve as at runtime, the native-forms list from the configured engine; defaults
   equal to today's conventions.

Then stage 12: the split rehearsal (`sqldb.Wrap` over a plain `*sql.DB` and dialect; prove
`lib/` builds with go-database absent from its import graph, extending `split-check`), the Q5
promotion recommendation (`go-sql` in its own repository, `go-sql/postgres`; go-database keeps
config, pool, lifecycle, readiness, the admin service), the rewritten task breakdown for
`v1.data.sql`, and the concepts to capture at close: the content-patterns reference for the
docs pass, soft delete (`delete`/`restore`/`purge`), the reusable plumbing candidates, the
shape cache. Iterate until the architect judges the strategy definitive, then `close`.

**Running it:** in `experiments/sql-dsl`: `mise trust && mise install`, `mise run db-up`
(postgres:18 on 127.0.0.1:5433), `mise run serve` (127.0.0.1:8081; startup applies pending
migrations and, with `admin.seed` on, seeds), `mise run test`, `mise run test-compose`,
`mise run lint` (golangci-lint + sqlint), `mise run split-check`. Use `127.0.0.1:8081`, not
`:8081` (IPv6 resolution). Compose state at handoff: `sql-dsl-postgres` up, database `app` at
schema version 3, seeded (7 organizations, 6 people; Ada Lovelace at version 4 after the live
proofs).

**Context budget:** the architect set the reset threshold at ~800k tokens for this session;
this handoff was taken at the stage 10 boundary as agreed.
