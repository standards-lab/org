# sql-dsl — experiment notes

The record of the `v1.data.sql.prototype` experiment. Direction, the API sketch, and the five
questions: `go-database/context/concepts/sql-architecture.md`. This file carries what the
sessions learn; the reset file at `../../context/reset.md` carries the handoff.

## Position

- **Directory:** a domain's statements live in `statements/` (renamed from `sql/` at the
  handoff, so the directory never reads as the builtin pattern namespace `sql`).
- **Stage:** 1–13 approved (2026-09-03), each on the branch as its own commit; stage 14,
  the sqlate preparations, built and proven, awaiting review with the final `REVIEW.md`.
  See the decisions log, Q1, Q5, and the Ontology section.
- **Next move:** the stage 14 review; then `close`, which promotes what `REVIEW.md`
  recommends: the concept `standards-lab/context/concepts/sqlate.md`, the coordinator's
  design note and roadmap edits, the cross-repo corrections, and the reset.
- **Then stage 13 (the architect, 2026-09-03):** the comprehensive review, written to
  `REVIEW.md` with NOTES's promotion section pointing at it. Each layer of the architecture
  evaluated, then the whole; an optimization pass that is a **layout review** — which
  types, methods, and functions belong to `go-sql`, `go-database`, `go-web-sdk`, the
  template, and the reference service, judged against the standard's package layering,
  without sacrificing capability (performance only if the layout review finds a reason;
  the signature cache stays a concept); the promotion plan for every component as the
  adopted strategy for the DSL service integration; the workspace context adjustments the
  experiment implies, seeded by the principles it incubated — the admin layout with a
  consolidated database package (migrations, seeds, patterns, statements), the entity
  roles (validation methods, the tag conventions), `internal/data`, the file-per-layer
  composition root and a possible `services.go` collapse of `infrastructure.go`,
  `domain.go`, and `reactors.go`, the `routes.go` mount simplification — each with the
  note it lands in; and the roadmap refinement, a `v1.data.sql.integration` goal replacing
  the pre-split `query`/`migrate`/`organization`/`startup` breakdown. Stage 13 drafts;
  `close` applies the tending and the manifest. Then `close`.
- **Compose state:** `sql-dsl-postgres` up on 127.0.0.1:5433; database `app` at schema
  version 3, seeded (7 organizations, 6 people); the stage 11 live proof created and
  deleted its own rows.

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

## Ontology

The library's vocabulary as settled at the stage 11 review (the architect, 2026-09-03),
in the layers the words belong to. Two acts produce statements, compile and compose; one
consumes them, execute.

**Text** — SQL as written, never sent as written. A **pattern** is protocol SQL any
package publishes under a **namespace**: a header and a body holding **slots** only, never
an include. A slot is a `{{name}}` hole: composed, the library fills it with text it
composed; included, it passes through and compiles as a parameter of the including
statement. An authored file's **header** is its `--|` **declarations** — tier, native,
transaction, key, field — the loader's and never the engine's; the **body** is what the
engine receives. The **tier** is declared portability, standard or native, a native file
naming the reach used and the port. A **parameter** is a statement's named input,
`{{name}}` or `{{name:type}}`; an **include**, `{{> ns.name}}`, is a statement's reference
to a pattern. **Protocol** SQL, which every domain would write identically, may be a
pattern; **content** SQL is the domain's own and never is.

**Catalog** — built once at the composition root, read-only after. A **pattern source** is
one namespace's patterns as declared: a directory plus any **overlays**, an engine's
replacement of patterns by name with the same slots; an **alias** registers a source under
another namespace. The **catalog** is the registered sources, read and validated, the
context every statement compiles against.

**Statements** — a **statement** is compiled SQL text with parameters; it holds no values.
**Compile** turns an authored file, against the catalog and the dialect, into a statement:
includes spliced, parameters positional. A domain's **statements** (`query.Statements`) are
its compiled inventory, and **verify** prepares each against the live schema at startup.

**Handles** — a statement bound at wiring to what runs it: `Statement` for a command,
`Rows[T]` with a scan, `Projection[T]` over a **base** — the authored query the collection
read wraps as a derived table, declaring its **key** and **field contract** — and `Guard`
over a **command** and its **check**. A domain holds handles, never text; the **mapper**
(`Scanner[T]`, `ArgsOf`) makes the entities' tags the scan and binding contract.

**Execution** — a read's **directives** are page, sorts, filters; its **signature** is the
directives with the values abstracted: field and operator pairs, sort terms, the paging
flag. **Compose** turns a base, the catalog's patterns, and a signature into a statement,
once per signature when cached. **Arguments** are the values a request binds to
parameters by name; **execute** runs a statement with them through a **session** — the
pool or a transaction — every error mapped through the **dialect**, the engine's
spellings. The signature cache (a promotion concept) is composed statements by signature,
bounded per projection.

## Q1 — Catalog composition

_Baseline (stage 8–9), shape A load-time stitching (stage 10), shape B build-time generation
(stage 11), the judging table, the verdict._

**Verdict (stage 10):** on-demand stitching over authored patterns, and it beats the baseline
on the baseline's own terms. The library's SQL is twenty-two pattern files under
`lib/query/patterns/`, each with a tier header, slots in the `{{ }}` syntax; `projection.go`
holds no SQL text, only the whitelist check, list arity, and parameter positions. Two
composition times: request time, where the collection read renders `count`, `collection`,
`one`, `verify` from `where`/`filter_<op>`/`value`/`order`/`order_term`/`paging`; and load
time, where a domain file includes a pattern with `{{> name}}` and `Load` splices its text
before rewriting parameters — `guard_where` and `guard_set` are the first, and all eight
guarded commands across both domains now write their protocol once. Against the baseline:
the composed SQL is byte-identical to the Go-composed text (every projection test passed
unchanged), lint reach is preserved because `sqlint` lints the patterns directory like any
`statements/` directory and `Load` expands includes before `Verify` prepares, and editor reach is
better, since the pattern is a `.sql` file. What Go still owns is exactly what cannot be
text. The `Pager` capability is gone: a port overrides `paging.sql` by name, which is stage
11's mechanism. Patterns hold slots only and never include other patterns (the catalog
refuses one that does), so a pattern is readable on its own.

**Stage 11, external sourcing proven.** Patterns are sourced from any `fs.FS` under a
namespace, and the catalog is built once at the composition root: `query.Patterns()` (the
library's, namespace `sql`, aliasable with `As`), `database.Patterns()` (the application's,
namespace `app`, from `admin/database/patterns/`), and for a port `Patterns().Overlay(fs,
dir)`. `NewCatalog` validates every source — tier declared, a native pattern names its port,
slots only, an overlay respells only a pattern its source defines and with the same slots,
no namespace twice — and joins every failure. `Catalog.Compile` replaces the
package-level `Load`; a `Statement` carries its catalog as it carries its dialect, so a
projection composes from the catalog its base compiled against. Two checks the catalog
adds: a native pattern spliced into a standard-tier statement is a load error (it would hide
the port), and the request-time patterns resolve under whatever namespace `Builtin`
registered as. The port proof is a MySQL-shaped overlay of `sql.paging` (`LIMIT {{fetch}}
OFFSET {{offset}}`): the collection read composes with the port's spelling and binds offset
then fetch as before, whatever order the port's text names them in. The first application
pattern is `app.identity` (`RETURNING id, version`), ending both domains' `create`; the
domains define no patterns.

Narrowed at the stage 6 review (the architect, 2026-09-02): build-time generation is set
aside. A query shaped by request state can only be composed on demand, and build-time output
is static text; what shape B offered — editor and lint reach over the final text — is
recoverable by a lint or dump that renders every composed statement for inspection, without
making generation the runtime source. The injection rule holds as stated: request state
selects among build-time patterns and fragments and never contributes text. Q1 therefore
judges on-demand stitching against the convention-plus-lint baseline, and stage 11 becomes
the proof that patterns can be sourced from outside the library (an extension point, so a
consumer or a second library can contribute patterns). Pattern candidates beyond the sketch's
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

Stage 7 tried it and it holds. A filter value binds as given — the request's text — under
`CAST(<placeholder> AS <declared type>)`; the engine parses it, and a value it cannot read
comes back as SQLSTATE class 22, which `pgdialect.MapError` classifies as
`sqldb.ErrInvalidValue` and the projection wraps as `*InvalidValueError` under
`ErrDirectives`, the engine's reason intact (`invalid input syntax for type uuid:
"not-a-uuid"`). No Go type registry exists. Two consequences to carry: the value grammar is
the engine's — PostgreSQL accepts `yesterday` and `now` as timestamps, and a port may accept
other literals — so the API's documented value syntax is "what the engine reads for the
declared type"; and the 400 surfaces at execution rather than pre-flight, one round trip
later than a Go parser would, which the request path does not notice.

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

Third consumer (stage 9, the person domain, nine files): the plain-table base proves the
common case of the collection pattern — `SELECT … FROM person` wrapped, filtered by
`status`, sorted by `family_name` with the key tie-breaker. The struct-tag mapper
(`query.Scanner[T]`, `query.ArgsOf`, `Args.With`) replaced every scan function and `Args`
literal in both domains; `database.go` is 104 lines for organization and 110 for person,
wiring and operations only. The action protocol — read state in the transaction, version
check, transition rule, guarded update — is `store.transition`, shared by the three actions;
`state` is an unexported entity scanned by tag. Still unused after three consumers:
`Rows.All`, `Rows.Each`, every operator but `eq`, `OpIn`; the SDK's query parser decides
whether they earn a request syntax (a go-web-sdk item).

Second consumer (stage 8, the organization domain, eight files): `Project` + `List` and
`One` over the lineage CTE base (path as an ordinary contract field, filterable and
sortable), `Scan`/`Rows.One` for the identity-returning insert and the subtree count,
`Statement.Exec` for the lock under `transaction: required`, three `Guard`s sharing one
check, `sqldb.Transact` for the transfer. `database.go` is 120 lines, the store's handles
bound once in `newStore`, the operations as methods; `service.go` and `handler.go` are the
reference service's with the matcher's two `errors.As` cases collapsed to one
`errors.Is(err, query.ErrDirectives)`, as the concept predicted. Symbols the sketch has that
this domain did not use: `Rows.All`, `Rows.Each`, `OpIn` and the other operators beyond
`eq` (the SDK's query parser yields exact matches only). Nothing missing. The `Args`
wiring test (`database_test.go`) binds every handle once over the strict driver and
asserts the composed SQL, the lock-first transfer, and the scan order; a key that does not
match its file fails there.

First consumer (stage 6b, the seed): `Rows[string].One` with `Scalar[string]` for the
identity-returning insert, `sql.ErrNoRows` as the no-row-on-conflict signal, `Exec` for the
count-returning insert, `Args` keyed by the file's names, and `{{parent:uuid}}` in the
lookup so a nil parent binds with a type. Nothing missing; nothing unused among the core
symbols. The admin domain holds its handles as service fields bound once in `New`, the
shape `database.go` takes at stage 8, and `Start` verifies its `Source` after the schema is
current, at lifecycle stage 1 ahead of the domains.

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
- **Projection deviations (stage 7).** `Project` panics on a base without a contract or
  one binding parameters of its own: a projection base takes no `Args` in the sketch, and
  a tenant filter is a directive until a consumer proves otherwise (open). Filter values
  are cast to the field's declared type rather than parsed in Go (Q2). `Pager` is the
  optional dialect capability for the paging fragment, the sketch's one library-generated
  text with a known divergence; the standard `OFFSET … FETCH` otherwise. `Projection.Verify`
  prepares `SELECT q.<every field> FROM (<base>) q`. `List` returns the total from the count
  twin first, then the page. A base's trailing semicolon is stripped at load so it composes
  as a derived table.
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
- Locks are named, never numbered (stage 8 review): `Locker` takes a name, the migrator's
  default is `migrate.<table>`, and a domain's transaction-scoped lock file binds its own
  name (`organization.tree`), each hashed into the engine's key space by the dialect or the
  file (`hashtext`). A registry of names cannot collide by accident the way small integers
  can, and a name is what the SQL Server, MySQL, and Oracle ports take directly.
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
`lib/migrate`, `lib/drivertest`, `internal/sdk`, `admin/database`, `cmd/sqlgen`, `cmd/sqlint`;
and what stays service-side, with the reason._

`lib/sqlheader` is shared by `query` and `migrate` and knows no keys; in go-database it is
an internal package unless a consumer outside the module needs the grammar.

**`cmd/sqlint`** (stage 9, configured at stage 11) is the checkable-rules half the
strategy assigns to the harness (`claude-plugins`, the hardening task): every statement
directory compiles against the pattern sources the runtime registers, every pattern
directory validates as a catalog source, a file is named for its operation, `{{` appears in
no body comment or literal, a standard-tier file uses none of the native forms the
configured engine declares, a non-transactional migration holds one statement. It runs in
`mise run lint`. Its `sqlint.toml` (the architect's design, stage 11): a table per role
(`statements`, `patterns`, `migrations`) holding the directory globs it covers and the
switches of its checks, with a directory set's exception written as an override table under
its own glob, quoted as a key; `[sources]` mapping each namespace to a path, and `engine` a
path, where a value whose first segment holds a dot is a Go package path resolved through
`go list` to the version `go.mod` pins and anything else a directory of the tree. A
**producer** — a package path, or a directory holding its own `sqlint.toml` — declares in
its `[export]` table what a consumer reads: the directory its namespace publishes, or the
overlay directory and the native forms an engine supplies; a bare directory is the pattern
files themselves, the service's own namespace declared entirely in the root file, which
exports nothing because nothing consumes it. A consumer inherits a producer's declarations
and never its checks. The lint reads one file, the root's; a producer's file is read only
through a reference, so the library's own role tables are inert until it is a module.
Native forms are a named table of regular expressions (RE2) in the engine's export —
`returning = '(?i)\bRETURNING\b'` — so the engine states the position and case in which a
spelling counts, and the finding carries the name; the lint strips string literals and the
comment tail before matching, since data versus syntax is SQL's property, not the engine's,
and compiles each expression when the engine resolves, so a malformed one is a
configuration finding. A test drives every entry of the real PostgreSQL declaration once
and a silent file of identifiers, literals, and comments; tokenizing is the promotion
refinement for what substring context still cannot see (a quoted identifier). The lint
imports no engine and holds no list of its own; at the split `sql = "lib/query"` becomes
`"github.com/standards-lab/go-sql"` with no other change. Promotion (the architect, stage
11 review): `cmd/sqlint/main.go` reduces to arguments, root, output, and the exit code; a
public `sqlint` package carries `Config` and `Load`, a `Resolver` over `go list`,
`Lint(fs, cfg, resolver) []Finding` with a typed `Finding{Path, Line, Message}`, one file
per role's checks, and the glob matcher — so the harness can call the lint as a package.
Each engine sub-module ships its `sqlint.toml`.

**`internal/sdk`** (stage 8) stages `IfMatch`/`PreconditionError` for go-web-sdk and
`Directives(web.Query)` for the service side — the latter cannot live in go-web-sdk (it
would import go-database) and is too small for go-database; the template scaffolds it.

**Seed stays service-side** (stage 5), as the strategy's sufficiency rule predicted, with a
correction to the size claim: the loader in `admin/database/seed.go` is about 70 lines for
two domains over `sqldb.Transact` and `encoding/json`, not 25 — the parent-by-code and
unit-by-code resolution and the insert-or-find are the bulk, and neither is library-shaped.
The seed files are the admin domain's, one per domain, in the domain's API vocabulary; the
native tier is `ON CONFLICT` and `RETURNING`. Shape (stage 5 review): one seed function and
one insert per table, `Seed` composing them in dependency order inside the transaction.
Done at stage 6b: the seed statements are authored files under `admin/database/statements/`
(`seed_organization`, `find_organization`, `seed_person`), loaded through a `Source`,
verified at startup and by the verify endpoint, and run through `query` handles; seed is the
first `query` consumer ahead of the domains (Q3).
Template finding: the `admin` config block with its tri-state `seed` switch and
`APP_ADMIN_SEED`, off by default.

The destinations are wider than go-database. Recorded at the stage 1 review: the experiment
drives the template (the `admin/` layer, `admin/database` as the schema's home, the collapsed
composition root, the config package kept separate with `configtest` beside it), go-web-service
(`design/domain-architecture.md` gains the admin layer; `cmd/db` retires into it), and the
architecture standard in the docs landing zone, where the admin mount is a new layer and the
anchor for the runtime-administration concept the context already carries.

### The DSL library as its own module (the architect, stage 9 review)

*Renamed at the stage 13 review (the architect):* the library is **sqlate**, at
`github.com/standards-lab/sqlate` — a portmanteau of SQL and template, read aloud as
"escalate": it makes plain `.sql` files dynamic and compositional through templating rather
than abandoning them, and escalates what a `.sql` file is capable of. It drops the `go-`
prefix deliberately: the prefixed repositories are interdependent layers of an
architecture, while sqlate is a standalone, independently adoptable library. The
`<prefix>-<language>` rule below is superseded: a DSL library is named for what it does to
the language. Root package `sqlate`; `sqlate/query`, `sqlate/migrate`, `sqlate/sqltest`,
`sqlate/sqlint`; `sqlate/postgres` the engine sub-module. In the workspace order it sits
beside go-core, importing nothing of ours. SQL is the first DSL to need this degree of
host-language support; a DSL designed to compose on its own will not, and sqlate is the
blueprint for the ones that do (`REVIEW.md` §3.1).

Decided by the architect: the library is `go-sql`, its own repository, and a DSL library is
named `<prefix>-<language>` — the language, never the service it serves — so the naming
distinguishes a DSL library (`go-sql`) from an infrastructure service library
(`go-database`) at a glance. Engine flavors are sub-modules (`go-sql/postgres`). The shape,
to confirm at stage 12 against the finished catalog: go-database keeps the infrastructure
service — configuration, provider construction over the driver,
the pool's lifecycle and readiness, the admin service that triggers verify, migrate, and
seed at startup. A separate DSL library owns everything from the file to the row: the
header, the parameter syntax, `query` with projection and guard, `migrate`, the mapper, the
pattern catalog and its sourcing, `sqlint`, and the execution seam itself — `Session`, `Tx`,
`Transact`, the dialect contract, and the error taxonomy (`ErrInvalidValue`,
`ErrVersionMismatch`, the constraint classes), which the runners depend on and the engine
sub-modules produce. Each engine sub-module holds what `pgdialect` holds now: placeholder
form, error classification, `Locker`, `Catalog`, `Pager`; nothing about connections. The DSL
library sits below go-database and imports nothing of ours, so a CLI, a worker, or a test
harness uses authored SQL with no lifecycle machinery. Universal for any infrastructure
service whose contract is a language rather than a protocol. Costs: a module and
repository, a workspace-order layer, and the `v1.data.sql` task breakdown rewritten (`query`
and `migrate` were to land in go-database v0.4.0). Settled: `go-sql` never sees
go-database — its entry point is a plain `*sql.DB` and a dialect, so anyone can use it over
`database/sql` and the driver of their choice, and go-database is one consumer among others.
The one item left for the split is whether the admin service is go-database's or
go-web-sdk's (where the lifecycle contract lives).

### Every line of SQL in a .sql file (the architect, stage 7 review)

The library's own patterns are no exception: the collection wrap, the operator fragments,
the paging fragment, the guard and check patterns, composed in Go today as the baseline, are
to be authored `.sql` files the library ships, slots in the `{{ }}` syntax, judged at stage
10 and sourced from any `fs.FS` at stage 11. Go keeps only what cannot be text — the
whitelist check against the header, list arity, parameter positions. The domain-facing
API (`Project`, `List`, `One`, `Guarded`, `Run`) is the seam and does not change, which is
why stages 8–9 build against it before the patterns move.

### Reusable plumbing (the architect, stage 6b review)

Candidates for promotion beyond `lib/`, so a service implementation connects its own
infrastructure and reuses the core plumbing; evaluated at stage 12 against the sufficiency
rule, each with the second consumer that would prove it (the template, then go-web-service):

- **The database admin service.** `Start` (verify, apply if pending, fail on dirty or
  unknown, verify the source, seed), `Verify`, `Status`, the verbs, `Ready`, `Register` are
  generic over a migration set, a `Source`, and a seed function; only those three are the
  service's. A `dbadmin`-shaped package (go-database or go-web-sdk; the lifecycle and
  readiness contract decides which) would leave a service with the wiring line.
- **Seed helpers.** The JSON read with strict decoding, the per-table loop with insert
  counting, and the code-to-id resolution repeat per table; a generic `seed.Table[T]` over a
  per-row insert function is the shape. The plan's "documented pattern" verdict predates
  the 70-line finding; the helper is justified when the template scaffolds a second
  service.
- **Handler plumbing.** The status matcher, `respond`, strict `decode`, and the
  detail-carrying `reject` are go-web-sdk shaped, alongside the `ErrorWriter` detail
  finding already on record.

### The split rehearsal (stage 12)

`lib/` builds with go-database absent from its import graph, and `split-check` now fails on
any go-database package, any service package, or the driver's name outside tests. What the
rehearsal had to move, and what that says about the split:

- **The dialect is go-sql's.** `sqldb.Dialect` (`Name`, `Placeholder`, `MapError`) is the
  library's own interface; go-database's postgres dialect satisfies it structurally, so the
  composition root passes `db.Dialect()` unchanged and `pgdialect.Wrap` takes it. At
  promotion `go-sql/postgres` owns the dialect entire — placeholders, error classes, the
  lock — and go-database's `Dialect` (a v0.3.0 addition made for `query`) retires or is
  reduced to what the pool itself needs; which is a stage 13 layout decision.
- **The error classes follow the dialect.** `sqldb.ErrConnectionFailed` and
  `sqldb.ErrInvalidValue` are the seam's; `query.ErrVersionMismatch` is the guard's own,
  the service mapping it to 412. The constraint classes (`ErrUniqueViolation` and kin) are
  still go-database's, produced by its mapping inside the wrapped dialect; once
  `go-sql/postgres` owns `MapError` they are go-sql's sentinels, and go-database's retire.
- **The seam is a plain pool.** `sqldb.Wrap(pool *sql.DB, dialect)`; lifecycle stays with
  whoever owns the pool. The root passes `db.Conn()`. `Base()` is gone: the admin service,
  which administers the provider's lifecycle object, takes it as its own argument
  (`database.New(pool, db, …)`), and `Infrastructure` exposes `Pool` beside `SQL` — the
  provider's object and the data layer's, from two libraries.
- **`drivertest` is go-sql's.** Its go-database constructor went; a test that needs the
  started lifecycle object builds it over `drivertest.Open`'s pool, as the admin tests do.
  The live tests keep go-database as their compose harness (a test-only import, outside the
  build graph): at promotion `go-sql/postgres`'s live tests take the driver directly, as
  go-database/postgres does today.
- **The `sqlint.toml` switch is made.** `sql` and `engine` are package paths of this
  module, resolved through `go list` on every `mise run lint`; the split changes the module
  name and nothing else, and `lib/query/patterns` leaves the `[patterns]` role.

The two questions, answered from the evidence for stage 13:

- **Is the session-with-catalog grouping library-shaped?** Both members are now go-sql
  types, so a `query`-level type is possible. Decided (the architect, stage 12 review):
  service-side, and `internal/data` is the service's whole database infrastructure. The
  rehearsal showed the service holds two database objects from two libraries — the
  provider's lifecycle object and go-sql's session — and the grouping is the service's
  composition of them, the seam the template scaffolds and the service grows (a second
  database, a read replica, the registry below). The stretch the review found was not two
  packages but two concerns in one: `admin/database` held the operations and also the
  content — migrations, seeds, the application's patterns, the seed statements — that the
  domains depend on at runtime, which is why domain tests imported an admin package to
  build a catalog. The content moved to `internal/data` (`Migrations()`, `Patterns()`, a
  `Seeder` compiled against the catalog) and `admin/database` keeps the operations and
  their policy. Layering: `data` at the bottom, the domains and the admin service peers
  over it, `app` composing the three; nothing under `domain/` imports `admin/`. The stage
  1 principle reads "administers the infrastructure the data layer owns".
- **Where does a statements inventory register?** On `data.Database`: each domain's
  `newStore` registers its compiled `Statements` under the domain's name
  (`db.Register("organization", stmts)`), the admin service reads the registry for
  `GET /admin/database/statements` beside `/patterns`, and verification stays each domain's
  own lifecycle stage. The grouping is the natural owner because it already holds the
  catalog, the patterns inventory. A promotion item, not built here.

## Promotion recommendation

`REVIEW.md` (stage 13): the layer evaluation, the layout review across go-sql, go-database,
go-web-sdk, the template, and go-web-service, the promotion plan in dependency order, the
workspace context adjustments, and the roadmap refinement under `v1.data.sql.integration`.
In one line: `go-sql` from `lib/` and `sqlint` on Go 1.27 generic methods, with the
constraint classes and the dialect folded in from go-database; go-database v0.4.0 reduced
to the infrastructure service plus an `admin` package over go-sql; the template scaffolding
`internal/data`, `admin/database`, and the composition root as files; the reference service
rewritten onto it.

## Not proven / open

_Anything deferred, with the decision it would change._

- **Repairing orphaned dirty state.** Stage 4 proved the concrete case: a failed concurrent
  index stays `INVALID` in `pg_index`, and the repair is drop, `Force` to the previous
  version, fix the cause, `Up`. Detection is engine-specific and the fix depends on the
  cause, so the library gets no `repair` verb; the admin endpoint's docs carry the sequence.
  Whether the non-transactional convention becomes "one idempotent statement" is a docs-pass
  question (`v1.data.sql.tasks.docs`).

## Decisions log

- 2026-09-03 · **Stage 14, the sqlate preparations** (the architect's call after the
  stage 13 review). Four adjustments, each proving a claim the review makes: the handle
  constructors as Go 1.27 generic methods — `Statement.Scan`, `Statement.Project`,
  `Statement.Guarded`, `DB.Transact` — with both domains, the seeder, and every test as
  consumers; list expansion — `{{ids...}}` and `{{ids...:type}}`, the argument a non-empty
  slice, one placeholder per element, the text rendered by arity and cached by it,
  `Verify` at arity one, a name with two arities a compile error while the type stays the
  occurrence's — proven by the person domain's `find_by_ids` behind `GET /api/people?id=a,b,c`,
  a batch fetch by key bounded by the size limit; the lint's stripper extended to
  double-quoted identifiers and block comments across lines, with the escape pragma
  considered and culled (a suppression patches the parser; the fix belongs in the
  stripper); the statements registry on `data.Database` — each domain and the seeder
  register at wiring, `GET /admin/database/statements` walks it beside `/patterns`,
  verification stays each domain's stage. `REVIEW.md` finalized with the review's
  decisions: sqlate, the vocabulary kept, the cache dropped, the meta language's first
  phase, the EA amendment for multi-entity domains, the person domain not promoted.
  Proven: hermetic and compose suites, lint, split-check; live — three people by ids
  through the expanded statement, the registry listing organization (8), person (10),
  seed (3).

- 2026-09-03 · **Stage 13.** The review, `REVIEW.md`, as the architect defined it after
  stage 11: each layer evaluated against what it proved and cost; the layout review placing
  every type, method, and function across the five projects — the admin service to a
  `go-database/admin` package (go-web-sdk cannot import go-database), its handler
  scaffolded; the error classes to go-sql with `MapError`; `drivertest` public as `sqltest`;
  `sqlheader` internal; the `Directives(web.Query)` lowering service-side by dependency
  direction; the composition root kept as one file per layer over a `services.go`; the
  handle constructors as Go 1.27 generic methods at promotion — the promotion plan in
  dependency order, the context adjustments per repository with the five incubated
  principles placed, and the roadmap draft: `prototype` deleted at close, `query`,
  `migrate`, `organization`, `startup` superseded by `v1.data.sql.integration` with six
  tasks, `next` continuing into `gosql` after the two marathon items.

- 2026-09-03 · **Stage 12.** The split rehearsal: `sqldb.Dialect`, `sqldb.ErrConnectionFailed`,
  `sqldb.Wrap(pool *sql.DB, dialect)` with `Base()` removed; `query.ErrVersionMismatch`;
  `pgdialect.Wrap(sqldb.Dialect)`; `drivertest.DB` removed; `Infrastructure.Pool` beside
  `SQL`, the admin service taking the pool as its own argument; `split-check` failing on
  go-database, the service packages, or the driver's name in `lib/`'s build graph;
  `sqlint.toml` naming the library and the engine by package path. Findings and the two
  answers under Q5, "The split rehearsal". Review (the architect): Go 1.27's promoted-field
  composite-literal keys apply nowhere here (the one struct embedding is a pointer); its
  generic methods are a stage 13 ergonomics item (`Scan`, `Project`, `Transact` as methods).
  The consolidation of the database concerns: content to `internal/data`, operations stay
  in `admin/database` (Q5, the grouping answer). Proven: hermetic and compose suites, lint with
  `go list` resolution, split-check; live on PostgreSQL 18.4 — ready, diagnostics through
  the pool, create, a stale edit 412 through `query.ErrVersionMismatch`, delete.

- 2026-09-03 · **Stage 11.** The pattern catalog API: `query.Publish(namespace, fs, dir)`,
  `Patterns()`, `As`, `Overlay`, `NewCatalog`/`MustCatalog`, `Catalog.Compile`/
  `MustCompile` replacing `Load`/`MustLoad` and the global `patterns`; a
  `Statement` carries its catalog. `internal/data.Database{*sqldb.DB; Catalog}`, built in
  `newInfrastructure` with `query.NewCatalog(query.Patterns(), database.Patterns())`, handed
  to every domain's and the admin service's `New`. `admin/database/patterns/` is the
  application's namespace `app`; its first pattern, `identity`, ends both `create`
  statements. The MySQL-shaped overlay test proves the port. `sqlint` rewritten over
  `sqlint.toml` as the architect designed it in this session (details under Q5): per-role
  tables with quoted-glob overrides — the list form kept over named directory sets because
  TOML tables are unordered and the override key duplicates only the glob — sources and the
  engine as paths, `[export]` in each producer (`lib/query`, `lib/pgdialect`,
  `admin/database`), native forms moved out of the lint. Decoding rules: an override key
  must equal a `dirs` entry, holds switches only, and the first matching `dirs` entry wins;
  absent the file, the roles are `**/statements`, `**/patterns`, `**/migrations` with every
  check on and no source, so an include is a load error. Dependency added:
  `github.com/BurntSushi/toml`. Review renames (the architect): `Catalog.CompileStatements`
  → `Catalog.Compile`; `query.Source` → `query.Statements`, so "source" means only a
  pattern source; `sqlheader.Directive` → `Declaration`, so "directive" means only a read's
  directives. The read's "shape" is its **signature**; the vocabulary is under the stage
  10 ontology entry and the Ontology section. Section 2 of the review: `PatternSource` →
  `query.Source`, `query.Patterns(ns, fs, dir)` → `query.Publish` (the ontology's verb),
  `query.Builtin()` → `query.Patterns()`, so every publisher exposes `Patterns()` and the
  root reads `query.NewCatalog(query.Patterns(), database.Patterns())`; `internal/data`
  stays, the goal's own word at the application level; `GET /admin/database/patterns`
  reads the catalog — the dump for inspection the stage 6 review named — and diagnostics
  lists the namespaces. A cross-domain statements inventory waits on domains registering
  their `Statements` with the admin service, a stage 12 or promotion question. Whether the
  session-with-catalog grouping moves into the library once `sqldb` wraps a plain `*sql.DB`
  is on the stage 12 list. Section 3: the producer-or-directory rule for sources and
  overlays (`admin/database/sqlint.toml` removed; `app = "admin/database/patterns"`), native
  forms as named regular expressions in the engine's export with literal and comment
  stripping, the every-form-once test over the real declaration, and the `cmd/sqlint`
  decomposition recorded under Q5 for promotion. Proven live (PostgreSQL 18.4): startup verified both domains
  and the admin statements with includes resolved from two namespaces; organization create
  → edit → delete and person create → delete through `app.identity` and the guards; `mise
  run lint` and `split-check` clean; the compose suite green.

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
  drops build-time generation; stage 11 proves external pattern sourcing instead. The
  content-patterns reference goes to the docs pass. Details under Q1 and Q2.
- 2026-09-03 · **Stage 10 review, patterns and the application** (the architect). Domains
  define no patterns: a pattern is protocol, a property of the application, so a domain
  that needs another's pattern has found application content. Application patterns live in
  `admin/database/patterns/` beside the migrations and seeds — the admin domain already
  owns the service's database infrastructure — registered at the composition root under
  the application's namespace alongside the library's and any external source. A
  service-side type groups what a domain needs from the database — the session and the
  catalog, the dialect through the session — because neither library may own it
  (`go-sql` must not know the pool, `go-database` must not know patterns) and `internal/app`
  cannot (the domains import it): `internal/data.Database`, built in `newInfrastructure`,
  handed to every domain's and the admin service's `New`; the template scaffolds it. The
  lint is `sqlint`. Stage 11's design, decided here: no package-level catalog or global —
  `query.Publish(namespace, fs, dir)`, `query.Patterns()` (namespace `sql`, aliasable with
  `As`), `Source.Overlay(fs, dir)` for explicit same-name replacement (an engine's
  `sql.paging`), `query.NewCatalog(sources...)` validating each pattern's tier and slots
  and refusing a duplicate namespace, and `Catalog.Compile`/`MustCompile` replacing `query.Load` — named for
  what it does: compile a domain's statements (includes resolved, parameters positional)
  against a catalog the composition root already built, after a reader took
  `Catalog.Load` for loading the catalog — so a `Statement`
  carries its catalog as it carries its dialect. Then `sqlint.toml`: roles as
  directory globs, checks switchable, sources with namespaces so includes resolve as at
  runtime, the native-forms list from the configured engine; defaults equal to today's
  conventions.
- 2026-09-03 · **Stage 10 review, the ontology** (the architect). A **pattern** is
  reusable protocol SQL any package may publish under its namespace (the strategy's own
  word: §6 "the pattern catalog", decision 13 "a pattern carries only protocol"); a
  **statement** is the authored file that includes patterns and compiles to executable
  text, the same word as its loaded form; the collection read is the library composing
  patterns into a statement on the caller's behalf. "Fragment" retires, and "frame" was
  the sketch's metaphor and did not describe an include. Includes are qualified —
  `{{> sql.guard_where}}`, `sql` the library's namespace — so origin is visible and
  collisions impossible; a bare include is a load error. The boundary is not library
  versus domain: a domain's `patterns/` registers like the library's. Stage 11 builds the
  registry (`query.Publish(namespace, fs.FS)`, `Load` sourcing from it), override
  precedence for the library's request-time patterns (an engine supplies `sql.paging`),
  namespace aliasing at registration as the last resort against a collision, and
  `sqlint.toml` — roles as directory globs, checks switchable, the native-forms list
  supplied by the configured engine — with defaults equal to today's conventions. At
  promotion: a bounded cache of the collection read's composed text keyed by directive
  shape, so the driver reuses prepared statements; includes are already compiled once at
  load, and only the collection read composes on demand.
  *Vocabulary settled at the stage 11 review (the architect, 2026-09-03):* "shape" above is
  the read's **signature** — its directives with the values abstracted: the predicates as
  field and operator pairs (and an `in` list's arity), the sort terms, the paging flag. A
  **statement** is compiled text with parameters and holds no values; values are the
  **arguments** a request binds at execution. Three verbs meet at that noun: **compile** an
  authored file against the catalog into a statement, once at load; **compose** a base
  statement, the catalog's patterns, and a signature into a statement, once per signature
  when cached; **execute** a statement with arguments, every request. "Compose" is the
  request-time verb, never "generate": build-time generation is the shape the stage 6
  review set aside. *Stage 13 (the architect):* the signature cache is not needed — the
  driver keys its statement cache on text and the composition is string work — and is
  dropped from the concepts. *Stage 14:* an **expanded parameter**, `{{ids...}}`, is the one
  authored statement composed at bind: its text depends on the list's length, is rendered
  by arity and cached by it, and `Text()` and `Verify` see the arity-one form.
- 2026-09-03 · **Stage 10.** The pattern catalog: twenty-two authored patterns under
  `lib/query/patterns/`, rendered at request time for the collection read and spliced at
  load time through `{{> name}}` includes; both domains' guarded commands on
  `guard_where`/`guard_set`; `Pager` removed in favor of overriding `paging.sql` (stage 11);
  `sqlint` lints pattern directories and its literal check tracks quote state (a false
  positive on `'active', {{> guard_set}}` found it). Q1's verdict is on-demand stitching.
  Proven live: list, deactivate, activate, edit, stale edit through the patterns.
- 2026-09-03 · **Stage 9.** The person domain (`domain/person/statements`: view on the plain table,
  create, edit, delete, version, state, activate, deactivate, transfer_unit), its three
  actions on the shared transition protocol with `ErrTransition` at 409; the struct-tag
  mapper in `lib/query` (`Scanner[T]`, `ArgsOf`, `Args.With`; `db` tag, `json` fallback,
  field name lowercased; an unmapped column is an error), both domains moved onto it;
  `cmd/sqlint` in the lint task. Decision: an action compares the version it read before
  applying its rule, so a stale client gets 412 rather than a rule it never saw; the guard
  then only catches a change between read and update. The cross-domain custody check that
  blocks deactivation waits for the inventory domain. Proven live: active people sorted by
  family name, create pending, duplicate email 409, deactivate-pending 409, activate → 2,
  activate-again 409, stale 412, transfer-unit → 3, missing unit 409, edit → 4, deactivate
  → 5, delete 204, the seed untouched.
- 2026-09-03 · **Stage 8 review, the command ontology** (the architect). Three direct
  commands: `create` a record; `edit` — replace the fields a client may set directly, the
  editable set being the contract; `delete` — remove it. An **action** is a named state
  transition with its own validation and protocol (`transfer` here; `activate`,
  `deactivate`, `transfer-unit` at stage 9), invoked as `POST /{id}/<action>`, returning
  `Identity` like every command; if a change needs a lock, a check, or a transition rule
  it is an action, never an `edit`. The command's name is the statement's file name, the
  store method, the service method, and the route — `create.sql`, not `insert.sql`: a file
  is named for its operation, never for its SQL verb (the seed's statements followed:
  `seed_organization`, `seed_person`). Reserved for the soft-delete
  convention, out of this experiment's scope: `delete` moves a record to the recycle bin,
  `restore` brings it back, `purge` removes it physically (administrative); the mechanics
  — a `deleted_at` column, the base excluding it with a recycle view, partial unique
  indexes over live rows, the delete pattern as an update — are a concept at close and a
  pattern-pair candidate for the catalog.
- 2026-09-03 · **Stage 8 review, separation of responsibilities** (the architect). Entity
  and command types own validation as methods (`Validate`, context-free rules only;
  existence and uniqueness stay the store's as constraint violations) and own binding and
  scanning through their tags — `db` first, `json` fallback — so `database.go` holds
  wiring and operations only and nothing outside it imports `query`. Validation lands
  here; the tag-driven `ArgsOf` and scanner land at stage 9 with the second domain, in
  place of hand-written scan functions and `Args` literals.
- 2026-09-03 · **Stage 8 review.** Every lock is named, `<owner>.<structure>`: the
  `Locker` capability, the migrator's default `migrate.<table>`, and the domain's
  `organization.tree`, hashed by `hashtext` in the dialect or the file. Numbers were a
  registry across domains waiting to collide.
- 2026-09-03 · **Stage 8.** The organization domain: `statements/` (organization_view, create,
  edit, transfer, delete, version, in_subtree, lock_tree; two native with ports declared,
  six standard), `database.go` as the SQL client, service and handler from the reference
  with the directive matcher collapsed, `Register` at lifecycle stage 2 running `Verify`
  (eight statements plus the contract probe) after the schema stage, `internal/sdk`, the
  `reads` config block. Proven live against the seeded database: list sorted by path with
  a filter, find by path, a bad timestamp filter value answered 400 with the engine's
  reason, unknown field 400, create, duplicate sibling 409, edit → version 2, stale edit
  412, cycle transfer 409, transfer to root, missing parent 409, delete with children 409,
  delete 204, find 404.
- 2026-09-02 · **Stage 7.** `Directives` (`Page`, `Sort`, `Op` × 10, `Filter`) carried from
  v0.3.0 minus the `ast.Predicate` escape; `Projection[T]` with `List` (count twin, page
  under the collection wrap, key tie-breaker), `One`, `Verify`; `Guard` with `Run` (row
  affected → version+1, miss → check → `sql.ErrNoRows` or `database.ErrVersionMismatch`
  with both versions), the caller's `Args` never written. The error family:
  `ErrDirectives` with `UnknownFieldError`, `UnknownOperatorError`, `InvalidValueError`
  unwrapping to it. `sqldb.ErrInvalidValue` and the class-22 mapping in `pgdialect` make the
  engine the value validator (Q2). Proven live: text values parsed by type, three bad
  values classified with the engine's reason, `One`, and all three guard outcomes.
- 2026-09-02 · **Stage 6b.** The seed's three statements are authored files under
  `admin/database/statements/`, native tier with their ports declared, bound as handles in
  `database.New`; `Start` and `Verify` prepare the admin domain's `Source` once the schema
  is clean, so a statement the schema no longer satisfies fails startup at stage 1. Proven
  live: fresh seed 7/6 through the authored statements, rerun zeros, verify 200.
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
