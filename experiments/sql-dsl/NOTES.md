# sql-dsl — experiment notes

The record of the `v1.data.sql.prototype` experiment. Direction, the API sketch, and the five
questions: `go-database/context/concepts/sql-architecture.md`. This file carries what the
sessions learn; the reset file at `../../context/reset.md` carries the handoff.

## Position

- **Stage:** 1–5 approved (2026-09-02); stage 6, the `query` core, is built and awaits
  review. See the decisions log.
- **Next move:** stage 6b — the seed statements as authored files under
  `admin/database/sql/`, seed as `query`'s first consumer; then stage 7, projection and
  guard, then the frame catalog (stage 10) and external frame sourcing (stage 11, replacing
  the build-time shape).
- **Compose state:** `sql-dsl-postgres` up on 127.0.0.1:5433; database `app` at schema
  version 3, clean, no data.

## Running it

See `README.md`. Schema operations and diagnostics are the admin mount's endpoints.
`mise run test-compose` runs the live proofs against the compose stack (`SQLDSL_DSN`); each
proof owns `live_*` objects and drops them on exit. `curl :8081` resolves to IPv6 on this
machine and the listener is IPv4; use `127.0.0.1:8081`.

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

Narrowed at the stage 6 review (the architect, 2026-09-02): build-time generation is set
aside. A query shaped by request state can only be composed on demand, and build-time output
is static text; what shape B offered — editor and lint reach over the final text — is
recoverable by a lint or dump that renders every composed statement for inspection, without
making generation the runtime source. The injection rule holds as stated: request state
selects among build-time frames and fragments and never contributes text. Q1 therefore
judges on-demand stitching against the convention-plus-lint baseline, and stage 11 becomes
the proof that frames can be sourced from outside the library (an extension point, so a
consumer or a second library can contribute frames). Frame candidates beyond the sketch's
five, for stages 10–11 to pick from: keyset pagination, list expansion, batch insert,
insert-if-absent/`MERGE` with its port, guard variants owning the protocol columns,
existence and count probes, read-after-write. Content patterns (status filters, search,
hierarchy CTEs) are the domain's; documenting them is a docs-pass item, in the landing
zone's principle page rather than the reference service's schema.

## Q2 — The file grammar

_Scanner corner cases met in real files; header lines under editor tooling; what Verify caught
and could not; whether the field contract stays in the header._

Stage 6 review settled the grammar on two conventions, both the architect's:

- **Parameters are `{{name}}`**, with `{{name:type}}` binding through `CAST` to an SQL type
  written as the engine reads it — `uuid`, `numeric(12,2)`, `timestamp with time zone` —
  standard or native as the file's tier declares; the token reaches the engine verbatim and
  `Verify` catches a name the engine rejects. A Go-side kind registry (`Kind`, `Typer`) was
  built and removed in the same review: it constrained the feature to five names for no
  gain. This supersedes decision 2's `:name`: a surrounding delimiter with no meaning in SQL needs no
  lexer — the body is scanned by one regular expression, the delimiter is reserved even
  inside literals and comments, and a `{{` that is not a parameter is a load error. `::uuid`,
  `:=`, and `:name` are plain text. The syntax has room for list expansion (`{{ids...}}`),
  deferred to a consumer.
- **Directives are `--|` lines.** A `--| key: value` line is definitively a directive and a
  plain `--` line definitively prose, so a misspelled key is a hard error, not silent prose;
  a `--|` line that is not `key: value`, or one after the body has begun, fails the load.
  The header is the loader's and is never sent: the engine receives the body
  (`sqlheader.End`), for `query` statements and migrations alike — MySQL would reject `--|`
  as a comment, and the header was never the engine's business.

The header grammar as `Load` enforces it: `tier` required; `native` required for native and
refused for standard; `transaction: required` is the only transaction value a query file may
carry (`none` is migrate's, refused here); `field` is `<name> <kind>` over the five kinds;
`key` must name a declared field; an unknown directive is a load error naming the file and
line. The field contract declares SQL types too (`--| field: created_at timestamp`), not
kinds: stage 7 tries binding a request's filter value as text through `CAST` and letting the
engine parse it, mapping a data exception (SQLSTATE class 22) to the 400 path the way class
23 maps today, so the engine is the one validator. If that proves awkward the fallback is a
Go parser per common type. Real files and editor tooling: stage 8.

Decisions for later stages, from the stage 6 API review: a domain's wiring test binds each
handle once with sample `Args` over the driver fake, so a key mismatch fails in CI (stage
8 sets the shape); the struct-tag row mapper reads `db` then falls back to `json`, so the
common case needs no second tag, and lands at stage 9 with the second domain rather than
waiting for a fourth; at promotion `MapError` joins `Session` itself, since both sessions
carry it and the type assertion in `mapErr` would otherwise lose mapping silently for a
foreign session.

## Q3 — The exports

_Per domain, the `query` and session-wrapper symbols `database.go` used; sketch symbols left
unused; symbols missing._

### Sketch-deviations ledger

- **Session wrapper is a package named `sqldb`, not `database`.** The spike imports
  go-database's root `database` for the pool, `Config`, and the sentinels in the same files;
  at promotion the symbols merge into `database` and the name question disappears.
- **`ErrorMapper` added beside `Session`.** Once `Dialect()` leaves the interface, errors that
  arise after a call returns (`rows.Err()`, `Scan`) have no mapping path; `*DB` and `*Tx` both
  expose `MapError` and a runner type-asserts it. Earned at stage 6: `query`'s runners map
  `rows.Err`, `Scan`, and `RowsAffected` failures through it (`mapErr`), the paths the seam
  cannot see; a session without it passes those errors through unmapped. Kept.
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
  boolean)`, created under the lock. The engine-specific half of the protocol is exactly two
  statements, the guarded create and the existence query, behind `migrate.Catalog`, a
  capability the dialect may implement (resolved by assertion like `Locker`;
  `StandardCatalog` otherwise). Every other statement is standard DML: booleans travel as
  bound parameters, never literals, and the head is a `MAX(version)` subquery rather than
  `FETCH FIRST` (stage 3 review).
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
- `Files` reads the `transaction` directive through `lib/sqlheader`, the header package
  `query`'s `Load` shares (stage 3 review); `required` or absence keeps the transaction,
  `none` opts out, any other value is a layout error.

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

### Stage 4 · the acceptance proofs (PostgreSQL 18.4, `lib/migrate/live_test.go`)

Six proofs, each a `//go:build compose` test over the compose database; all pass, the
cancelled run eight times in a row.

- **Non-transactional DDL.** `CREATE INDEX CONCURRENTLY` inside the transactional path fails
  with SQLSTATE 25001 and records nothing (head stays clean at 1); the same file under
  `-- transaction: none` builds a valid index (`pg_index.indisvalid`), the head is clean at 2,
  and `Down` drops it.
- **Dirty state, force, and the orphan.** A unique index built `CONCURRENTLY` over duplicate
  rows fails after its catalog entry exists: `Up` returns `DirtyError{3}` carrying
  `database.ErrUniqueViolation`, the index remains `INVALID`, `Verify` is `ErrDirty`, a second
  `Up` returns the discovered `DirtyError` with no cause. The repair is the operator sequence
  `DROP INDEX`, `Force(2)`, fix the data, `Up`; the index is then valid and `Verify` clean.
- **Concurrent starters in one process.** Four migrators over one pool, a set whose first
  migration sleeps 400 ms inside its transaction: every `Up` succeeds, the history holds each
  version once, clean.
- **The unlocked negative.** The same race with `Unlocked: true`: 3 of 4 starters fail with
  42P07 (duplicate table) or 23505 (duplicate history row). The collision is what the lock
  removes; the 400 ms window makes it reliable.
- **Concurrent starters across processes.** The test binary re-executes itself four times
  (`SQLDSL_HELPER=starter`), each child a process with its own pool: all exit 0, the history
  holds each version once. The session-scoped advisory lock serializes across sessions.
- **The cancelled-context run.** A 5 s `pg_sleep` migration under a 300 ms deadline: `Up`
  returns in ~330 ms with exactly `migrate: apply 1 slow: timeout: context deadline
  exceeded`, nothing is recorded, `pg_locks` shows no advisory lock, and a fresh run over the
  same history completes at once.

Finding from the cancelled run: pgx discards the cancelled connection, so the explicit
rollback and the deferred unlock on it fail (`ErrTxDone` or `driver: bad connection`,
racing database/sql's own rollback goroutine) while the session's end releases the lock
anyway. `locked` and `inTx` now report neither once `ctx.Err() != nil`; the cancellation
alone reaches the caller. The open item on unlock after cancellation is closed by this.

## Q5 — The split

_Directory → destination table: `lib/sqldb`, `lib/pgdialect`, `lib/sqlheader`, `lib/query`,
`lib/migrate`, `lib/drivertest`, `internal/sdk`, `admin/database`, `cmd/sqlgen`, `cmd/sqllint`;
and what stays service-side, with the reason._

`lib/sqlheader` is shared by `query` and `migrate` and knows no keys; in go-database it is
an internal package unless a consumer outside the module needs the grammar.

**Seed stays service-side** (stage 5), as the strategy's sufficiency rule predicted, with a
correction to the size claim: the loader in `admin/database/seed.go` is about 70 lines for
two domains over `sqldb.Transact` and `encoding/json`, not 25 — the parent-by-code and
unit-by-code resolution and the insert-or-find are the bulk, and neither is library-shaped.
The seed files are the admin domain's, one per domain, in the domain's API vocabulary; the
native tier is `ON CONFLICT` and `RETURNING`. Shape (stage 5 review): one seed function and
one insert per table, `Seed` composing them in dependency order inside the transaction.
Planned: once `query` exists, the seed statements become authored files under
`admin/database/sql/`, loaded through a `Source` and verified at startup, and seed is the
first `query` consumer ahead of the domains (Q3 evidence for `Exec`, `Rows.One`, `Args`).
Template finding: the `admin` config block with its tri-state `seed` switch and
`APP_ADMIN_SEED`, off by default.

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

- **Repairing orphaned dirty state.** Stage 4 proved the concrete case: a failed concurrent
  index stays `INVALID` in `pg_index`, and the repair is drop, `Force` to the previous
  version, fix the cause, `Up`. Detection is engine-specific and the fix depends on the
  cause, so the library gets no `repair` verb; the admin endpoint's docs carry the sequence.
  Whether the non-transactional convention becomes "one idempotent statement" is a docs-pass
  question (`v1.data.sql.tasks.docs`).

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
- 2026-09-02 · **Stage 3 review.** Once a binary migrates the database, the database is
  managed under that binary or newer: a history row past the set is `UnknownVersionError`
  and startup refuses, so rolling a binary back after a forward migration is refused by
  design. The header grammar is its own package, `lib/sqlheader`, shared by `migrate` and
  `query`; the migrator's engine-specific SQL is reduced to the catalog pair
  (`CreateHistory`, `HistoryExists`) behind the `Catalog` capability in `statements.go`, with
  booleans bound as parameters and the head as a `MAX` subquery so the rest is standard DML;
  a second engine wires in by implementing two methods on its dialect, and a consumer by
  wrapping the dialect. Proven live on PostgreSQL 18.4 after the change: startup `schema
  current version=3`, `down` → `up` → `force 3` through the admin mount, history clean at 3. `Files`' `NNNN_name.{up,down}.sql` layout is kept
  as the helper (golang-migrate's convention, already the service's); `Migration` values are
  the contract.
- 2026-09-02 · **Stage 4.** The six acceptance proofs are `//go:build compose` tests in
  `lib/migrate/live_test.go`; they promote with the package as the engine-only tier of the
  testing hierarchy, and the cross-process starter proof is the shape the service's
  integration suite takes up. Finding: once the context has ended, a rollback or unlock
  failure on the discarded connection is noise, and `migrate` no longer reports it. The
  orphan case has an operator repair, not a library verb.
- 2026-09-02 · **Stage 5.** Seeding is the database admin service's operation: `Seed` runs
  at every startup of an environment whose `admin.seed` is on, once the schema is current,
  and on demand as `POST /admin/database/seed`; off, the endpoint answers 403 with the
  reason. It is idempotent through the tables' unique constraints, one transaction for both
  files, and returns the rows it inserted per domain. Proven live (PostgreSQL 18.4): a fresh
  start logs `seeded organizations=7 people=6`; the next start, and the endpoint, report
  zeros; `APP_ADMIN_SEED=false` seeds nothing and the endpoint is `403 seeding is disabled`.
- 2026-09-02 · **Stage 6 review.** Parameter syntax is `{{name}}` / `{{name:kind}}`,
  superseding decision 2; directive lines are `--|`; the engine receives the body only. Q1
  drops build-time generation; stage 11 proves external frame sourcing instead. The
  content-patterns reference goes to the docs pass. Details under Q1 and Q2.
- 2026-09-02 · **Stage 6.** `lib/query` core: `Load`/`MustLoad` over `sqlheader`, `Source`
  as inventory with `Statement(name)` panicking on a miss, `Statement` with `Exec`, `Scan`
  → `Rows[T]` with `One` (`sql.ErrNoRows` unmapped), `All`, and `Each` (closes on break),
  `Scalar`, `Source.Verify` preparing every statement and joining every failure by name,
  `Verifier`/`Verify`. `ErrTransactionRequired` is checked at bind, before the driver; a
  missing argument is `*ArgumentError` naming statement and parameter, an extra is ignored.
  The engine receives the whole file, header comments included. Projection, guard, and
  directives are stage 7.
- 2026-09-02 · PRQL considered and ruled out for this layer: analytical-only by its own
  statement, no Go binding, a second language above SQL. A reference point for the meta-language
  concept.
