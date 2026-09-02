# reset · sql-plan

- **Status:** closeout
- **Session:** plan
- **Project:** go-database, standards-lab (org)
- **Branch:** sql-plan

## Disposition

- **Added:** go-database `concepts/sql-architecture.md` — the v0.4 direction as a concept:
  the thirteen decisions the session took (named `:name` parameters resolved at load; typed
  handles bound once at wiring; `Session` as the stdlib method set with mapping inside the
  seam; `Transact[T]` with options; writes by convention plus a `-- transaction: required`
  header; the field contract declared in the SQL header; one request sentinel; `seed`
  retired; migrate's history table, per-file transactions, dirty/force, and the `Locker`
  capability that fails typed when absent; explicit `Verify` composition; scan-function row
  mapping; pattern templates admitted as protocol-only frames), the API sketch, the
  organization worked example with `database.go` as the domain's sole SQL client, the five
  questions the prototype settles with the decision each changes, and the prototype's shape.
  A concept rather than design because the experiment supplies the evidence.
- **Retained:** go-database `design/layers.md` (superseded banner; the query session rewrites
  it) and `concepts/v0.4-findings.md` (cited by prototype, query, and migrate).
- **Integrated:** go-database `context/README.md` — the capability map carries the v0.4
  retirements (`ast`, `operation`, `exec`, `seed`) and the planned `query`, `migrate`, and
  `internal/drivertest`. Roadmap — `v1.data.sql.plan` deleted; `v1.data.sql.prototype` added
  ahead of `query` with the concept as its context; `query`, `migrate`, and `organization`
  reframed as extraction from the prototype; `migrate` drops the seed reconciliation and
  `hardening` gains the seed pattern; `next` advanced.
- **Cross-repo:** standards-lab `design/dsl-driven-services.md` gained §11, the plan session's
  adjustments to the strategy record: seed retired, §5.1 confirmed, pattern templates admitted
  under the protocol/expressive split, writes by convention, named parameters in place of
  ordinal `?`.

## Next-focus

`v1.data.sql.prototype` — runs as `experiment` in go-database, under `experiments/sql-dsl`.
Settle scope from the concept's "What the prototype settles" and "The prototype's shape"
(`go-database/context/concepts/sql-architecture.md`): the five questions with the decision
each changes; a nested module generated from `go-web-sdk-template`, depending on go-core,
go-web-sdk, and go-database v0.3.0 only for what stays; two domains (organization, people);
both composition shapes tried against the convention-plus-lint baseline; the migrate
acceptance proofs against the compose stack copied from go-web-service. At close, promote
what proved out into the concept and reframe `query` and `migrate` from it.
