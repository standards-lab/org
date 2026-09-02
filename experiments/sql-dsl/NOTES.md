# sql-dsl — experiment notes

The record of the `v1.data.sql.prototype` experiment. Direction, the API sketch, and the five
questions: `go-database/context/concepts/sql-architecture.md`. This file carries what the
sessions learn; the reset file at `../../context/reset.md` carries the handoff.

## Position

- **Stage:** 1 approved (2026-09-02), after the review correction that introduced the admin
  layer and collapsed the composition root; see the decisions log. Stages 2 and 3 are
  committed and await their reviews: stage 2 is `lib/sqldb`, `lib/pgdialect`, `lib/drivertest`;
  stage 3 is `lib/migrate` plus the migration set and the schema stage, whose verbs now live
  on the admin mount.
- **Next move:** stage 4 — the live-engine acceptance proofs as `//go:build compose` tests in
  `lib/migrate/live_test.go` (non-transactional DDL, dirty state and force, concurrent
  starters in-process and process-level, the unlocked negative, the cancelled-context run),
  transcribed under Q4.
- **Compose state:** `sql-dsl-postgres` up on 127.0.0.1:5433; database `app` at schema
  version 3, clean, no data.

## Running it

See `README.md`. Schema operations and diagnostics are the admin mount's endpoints.

Stage 1 proof (2026-09-02, PostgreSQL 18.4, `postgres:18-alpine`):

```
$ curl :8081/admin/database/diagnostics     (originally `server -schema diag`; re-homed at the stage 1 review)
{"dialect":"postgres","ping":…,"server_version":"PostgreSQL 18.4 on x86_64-pc-linux-musl, …","pool":{"open":1,…,"max_open":25,…}}

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
- **`ExecTx` dropped.** A write is never void: `database/sql`'s `Exec` returns a `Result`, every
  unit this spike plans returns its identity, version, or affected count, and a void runner
  invites the caller to skip the zero-rows signal. `Transact[T]` is the one runner; a void unit
  that surfaces in stages 5–12 is the evidence to reintroduce a variant. Stage 2 review.

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

### Stage 1 review: the same protocol through the admin mount, plus the first dirty state

Fresh database, `mise run serve`:

```
schema pending; applying versions=[1 2 3]  ·  schema current version=3  ·  server ready
GET  /readyz                     → {"status":"ready","checks":[…,{"name":"schema","ready":true}]}
GET  /admin/database/diagnostics → {"dialect":"postgres","ping":625430,"server_version":"PostgreSQL 18.4 …","pool":{"open":1,…}}
GET  /admin/database/schema      → {"version":3,"dirty":false,"pending":[],"ready":true,"migrations":[…]}
POST /admin/database/schema/down → {"version":2,"dirty":false,"pending":[3],"ready":false,…}
GET  /readyz                     → 503 … {"name":"schema","ready":false}
POST /admin/database/schema/verify → 409 {"detail":"migrate: migrations pending: [3]"}
POST /admin/database/schema/up   → {"version":3,…,"ready":true}
POST /admin/database/schema/force {"version":9} → 400 {"detail":"migrate: version not in the migration set: 9"}
```

Dirty state, produced by operator misuse rather than a failing file: `force 2` deletes row 3
but leaves the index in the schema; `steps 1` then re-runs migration 3 outside a transaction,
the engine refuses, and the row stays dirty:

```
POST /schema/force {"version":2} → {"version":2,"dirty":false,"pending":[3],…}
POST /schema/steps {"steps":1}   → 409 {"detail":"migrate: schema is dirty: version 3 failed: ERROR: relation \"ix_person_unit_id\" already exists (SQLSTATE 42P07)"}
GET  /schema                     → {"version":3,"dirty":true,"pending":[],"ready":false,…}
$ server (fresh process)         → exit 1: startup: schema: migrate: schema is dirty: version 3
POST /schema/force {"version":3} → {"version":3,"dirty":false,"pending":[],"ready":true,…}
POST /schema/verify              → 200
```

Two findings for the record: go-web-sdk's `ErrorWriter` sends the error text only on a 400,
which is right for the public API and wrong for an operator surface, so the admin handler
writes schema-state conflicts with detail itself (a go-web-sdk item for the startup task);
and `Force` is correctly an operator override with no schema effect, which is exactly why it
can manufacture a dirty row — the docs for the endpoint must say so.

## Q5 — The split

_Directory → destination table: `lib/sqldb`, `lib/pgdialect`, `lib/query`, `lib/migrate`,
`lib/drivertest`, `internal/sdk`, `admin/database`, `cmd/sqlgen`, `cmd/sqllint`; and what
stays service-side, with the reason._

The destinations are wider than go-database. Recorded at the stage 1 review: the experiment
drives the template (the `admin/` layer, `admin/database` as the schema's home, the collapsed
composition root, the config package kept separate with `configtest` beside it), go-web-service
(`design/domain-architecture.md` gains the admin layer; `cmd/db` retires into it), and the
architecture standard in the docs landing zone, where the admin mount is a new layer and the
anchor for the runtime-administration concept the context already carries.

## Promotion recommendation

_The shape to promote for `query` and for `migrate`; the rewritten task-breakdown input for
`v1.data.sql.query`, `.migrate`, `.organization`._

## Not proven / open

_Anything deferred, with the decision it would change._

## Decisions log

- 2026-09-02 · **Stage 1 review.** Schema operations and diagnostics are not server
  sub-commands. They are an administrative layer: the **admin mount** (`/admin`, beside `/api`)
  serves the route groups of **admin domains** (`internal/admin/<service>`, one per
  infrastructure service that needs administering), each an **admin service** whose operations
  are triggers over library functions and which owns its service's administrative
  infrastructure (for the database: the migrator, the embedded migration set, the seeds). The
  database admin service also runs the startup correction: verify, apply if pending, fail
  startup only on a state the mechanism cannot correct (dirty, unknown history), and gate
  readiness on the result. `SchemaConfig`, `internal/schema`, and the `-schema` mode were
  removed. `Infrastructure` exposes one database object, `SQL`; the v0.3.0 lifecycle is
  reached through it. Layout, on the architect's second pass: `admin/` is a root package
  like go-web-service's `domain/`, holding the admin domains; the composition root's wiring
  packages (`internal/domain`, `internal/admin`, `internal/reactors`) collapse into files of
  `internal/app` (`domain.go`, `admin.go`, `reactors.go`), each internalizing its mount, so
  `routes.go` is two lines. Reactors had to move with them: `reactors.New` takes the `Domain`
  type and would otherwise import `app`, a cycle. `internal/infrastructure` followed on the
  third pass: with the second binary gone, `app` is its only consumer and the layers take its
  fields, so it is wiring too (`internal/app/infrastructure.go`). `internal/config` stays a
  package, `configtest` beside it in the stdlib `<pkg>test` convention. Template finding.
  Promotion targets:
  go-web-service `design/domain-architecture.md`, the template's build points.
- 2026-09-02 · go-web-sdk finding: `ErrorWriter` sends error text only on a 400, right for
  the public API, wrong for an operator surface. Suggested: an option such as
  `web.WithDetail(statuses ...int)` so a handler opts statuses into carrying the text. The
  admin handler's local `reject` stands until then.

- 2026-09-02 · Home is `standards-lab/experiments/` (every experiment lives at the
  coordinator); module path `github.com/standards-lab/org/experiments/sql-dsl`; generated from
  `go-web-sdk-template/template@v0.5.0` (tag `template/v0.5.0`).
- 2026-09-02 · **Stage 2 review.** `drivertest` is strict where a real driver is: an
  unscripted exec or query fails (`ErrUnscripted`), an argument count that does not match the
  `$N` placeholders fails (`ErrArguments`), and a response that does not fit its call fails
  (`ErrScript`: rows for an exec, an affected count for a query, a row of the wrong width or
  holding a non-`driver.Value`). The one leniency kept is the argument set, which is recorded
  unconverted. Stages 1 and 3 passed unchanged under the strict driver. `ErrorMapper`'s
  interface has no consumer yet; decided once the `query` runners exist.
- 2026-09-02 · PRQL considered and ruled out for this layer: analytical-only by its own
  statement, no Go binding, a second language above SQL. A reference point for the meta-language
  concept.
