# sql-dsl — experiment notes

The record of the `v1.data.sql.prototype` experiment. Direction, the API sketch, and the five
questions: `go-database/context/concepts/sql-architecture.md`. This file carries what the
sessions learn; the reset file at `../../context/reset.md` carries the handoff.

## Position

- **Stage:** 1 done — scaffold, database up, diagnostics.
- **Next move:** stage 2 — `lib/sqldb` (session wrapper), `lib/pgdialect` (lock capability),
  `lib/drivertest` (prepare-capable driver fake), each with hermetic tests.
- **Compose state:** `sql-dsl-postgres` up on 127.0.0.1:5433, empty database `app`.

## Running it

See `README.md`. `mise run schema -- <verb>` is the one-shot mode; verbs land per stage.

Stage 1 proof (2026-09-02, PostgreSQL 18.4, `postgres:18-alpine`):

```
$ mise run schema -- diag
dialect:        postgres
ping:           265.34µs
server version: PostgreSQL 18.4 on x86_64-pc-linux-musl, compiled by gcc (Alpine 15.2.0) 15.2.0, 64-bit
pool:           open=1 in_use=0 idle=1 max_open=25 wait_count=0 wait=0s

$ mise run serve &  (config.local.json: 127.0.0.1:8081)
$ curl -s -o /dev/null -w '%{http_code}' :8081/healthz   → 200
$ curl -s :8081/readyz
{"status":"ready","checks":[{"name":"lifecycle","ready":true},{"name":"database","ready":true}]}
```

Notes: the `gonew` mise shim on this machine is broken (points at a missing go@1.27.0);
`go run golang.org/x/tools/cmd/gonew@latest …@v0.5.0` works, and the nested module's version
is written `@v0.5.0`, not `@template/v0.5.0`.

## Q1 — Catalog composition

_Baseline (stage 8–9), shape A load-time stitching (stage 10), shape B build-time generation
(stage 11), the judging table, the verdict._

## Q2 — The file grammar

_Scanner corner cases met in real files; header lines under editor tooling; what Verify caught
and could not; whether the field contract stays in the header._

## Q3 — The exports

_Per domain, the `query` and session-wrapper symbols `database.go` used; sketch symbols left
unused; symbols missing; the sketch-deviations ledger._

## Q4 — Migrate's protocol

_Procedures, transcripts, engine output, and conclusions for: non-transactional DDL, dirty
state and force, concurrent starters (in-process, process-level, and the unlocked negative),
the cancelled-context run._

## Q5 — The split

_Directory → destination table: `lib/sqldb`, `lib/pgdialect`, `lib/query`, `lib/migrate`,
`lib/drivertest`, `internal/sdk`, `internal/schema`, `cmd/sqlgen`, `cmd/sqllint`; and what
stays service-side, with the reason._

## Promotion recommendation

_The shape to promote for `query` and for `migrate`; the rewritten task-breakdown input for
`v1.data.sql.query`, `.migrate`, `.organization`._

## Not proven / open

_Anything deferred, with the decision it would change._

## Decisions log

- 2026-09-02 · Home is `standards-lab/experiments/` (every experiment lives at the
  coordinator); module path `github.com/standards-lab/org/experiments/sql-dsl`; generated from
  `go-web-sdk-template/template@v0.5.0` (tag `template/v0.5.0`).
- 2026-09-02 · PRQL considered and ruled out for this layer: analytical-only by its own
  statement, no Go binding, a second language above SQL. A reference point for the meta-language
  concept.
