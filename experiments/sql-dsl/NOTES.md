# sql-dsl — experiment notes

The record of the `v1.data.sql.prototype` experiment. Direction, the API sketch, and the five
questions: `go-database/context/concepts/sql-architecture.md`. This file carries what the
sessions learn; the reset file at `../../context/reset.md` carries the handoff.

## Position

- **Stage:** 3 done — `lib/migrate`, the three-migration set, the schema lifecycle stage
  under `Schema.Mode`, and the `-schema version|verify|up|down|steps|force` verbs.
- **Next move:** stage 4 — the live-engine acceptance proofs as `//go:build compose` tests in
  `lib/migrate/live_test.go` (non-transactional DDL, dirty state and force, concurrent
  starters in-process and process-level, the unlocked negative, the cancelled-context run),
  transcribed under Q4.
- **Compose state:** `sql-dsl-postgres` up on 127.0.0.1:5433; database `app` at schema
  version 3, clean, no data.

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
unused; symbols missing._

### Sketch-deviations ledger

- **Session wrapper is a package named `sqldb`, not `database`.** The spike imports
  go-database's root `database` for the pool, `Config`, and the sentinels in the same files;
  at promotion the symbols merge into `database` and the name question disappears.
- **`ErrorMapper` added beside `Session`.** Once `Dialect()` leaves the interface, errors that
  arise after a call returns (`rows.Err()`, `Scan`) have no mapping path; `*DB` and `*Tx` both
  expose `MapError` and a runner type-asserts it. Whether runners need it is Q3 evidence.
- **`Begin` does not gate on readiness.** v0.3.0's `Begin` returned `ErrNotReady` before
  `Start`; the wrapper drives `*sql.DB` directly. Readiness is the lifecycle wrapper's concern
  (`Check: db` at stage 0); a failed begin wraps `ErrConnectionFailed` as before.
- **`DB.Conn(ctx)` added.** A pinned `*sql.Conn` for protocols that need session scope
  (migrate's lock and non-transactional DDL). Not in the sketch.

## Q4 — Migrate's protocol

_Procedures, transcripts, engine output, and conclusions for: non-transactional DDL, dirty
state and force, concurrent starters (in-process, process-level, and the unlocked negative),
the cancelled-context run._

### The protocol as built (stage 3)

- History table `schema_version(version integer PK, name text, applied_at timestamp, dirty
  boolean)`, created with `CREATE TABLE IF NOT EXISTS` under the lock (every engine but SQL
  Server accepts the form; the port is a catalog-guarded create).
- Every mutating verb runs on one pinned `*sql.Conn`: lock (`pg_advisory_lock`, session
  scope), ensure table, read history, refuse any dirty row, require the applied rows to be
  the set's prefix by version and name, then apply or revert; unlock under
  `context.WithoutCancel` before the connection returns to the pool.
- Transactional migration: `BEGIN`, the file, the history insert, `COMMIT`; a failure rolls
  back and records nothing. Non-transactional (`-- transaction: none` in the header): insert
  the row dirty under autocommit, run the file, clear dirty; a failure leaves the row dirty
  and returns `*DirtyError`; only `Force` clears it.
- `Version` and `Verify` take no lock and never create the table (existence via
  `information_schema.tables`; a search-path caveat for a port with schemas of the same name).
- Default lock key: FNV-1a 64 of the table name, so a second migrator over another table
  never contends and never collides with small domain keys such as the organization tree
  lock (`1`).
- `Files` reads the header with a fifteen-line reader of its own rather than importing
  `query`; whether the two packages share a header package is a Q5 item.

### Stage 3 live transcript (PostgreSQL 18.4)

```
$ server -schema version            → version: 0 dirty: false
$ server -schema verify             → schema verify failed: migrate: migrations pending: [1 2 3]   (exit 1)
$ server -schema up
  migration applied version=1 name=organization
  migration applied version=2 name=person
  migration applied version=3 name=person_unit_index transactional=false
$ server -schema version            → version: 3 dirty: false
$ server -schema verify             → schema verify: ok
$ server -schema up                 → schema up: ok            (idempotent)
$ psql … 'SELECT version, name, dirty FROM schema_version'   → 1 organization f · 2 person f · 3 person_unit_index f
$ psql … '\di ix_person_unit_id'    → public | ix_person_unit_id | index | app | person
$ server -schema down 1             → migration reverted version=3 … transactional=false
$ server -schema steps 1            → migration applied version=3 …
$ mise run serve  (schema.mode=apply)  → "schema current" mode=apply version=3, then "server ready"
$ APP_SCHEMA_MODE=verify server, history forced to 2 → startup fails at the schema stage, exit 1, no ready record
```

Finding: `CREATE INDEX CONCURRENTLY` ran under pgx's default (extended-protocol) exec mode on
an autocommit connection without `simple_protocol`; the risk noted at planning did not
materialize on pgx v5.10 / PostgreSQL 18.

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
