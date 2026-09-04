# sql-dsl: the review

The review of the `v1.data.sql.prototype` experiment, written at stage 13 (2026-09-03) and
finalized after stage 14. It evaluates each layer the spike built, then the whole; places every
type, method, and function across `sqlate`, go-database, go-web-sdk, go-web-sdk-template, and
go-web-service; and states the promotion plan. The running record is `NOTES.md`, and the
library's vocabulary is its Ontology section. The experiment's `close` applied this review on
2026-09-03: the current position lives in `context/concepts/sqlate.md`,
`context/design/dsl-driven-services.md`, and the roadmap, and this file is the record of how it
was reached.

## 1. Verdict

The experiment proved the strategy and changed its shape. The strategy's claim holds: every
line of SQL lives in a `.sql` file, the library owns only what SQL cannot express, and a
domain's database client is a page of wiring over typed values bound to statements. Three of
the five questions came out differently from the plan's expectation, each with evidence:

- **Catalog composition (Q1).** Patterns are authored `.sql` files that the library splices at
  compile time, not code generated at build time, and the library's own SQL is patterns too.
  Any package publishes patterns from an `fs.FS` under a namespace; the composition root builds
  the catalog once.
- **The file grammar (Q2).** Parameters are `{{name}}` and `{{name:type}}`, the engine
  validates values, the header is `--|` declarations, and the engine receives the body alone.
- **The split (Q5).** The library, `sqlate`, is standalone and sits below go-database, rather
  than `query` and `migrate` living inside go-database. The spike's `lib/` builds with
  go-database absent from its import graph.

The other two confirmed the plan. The exported API (Q3) matches the sketch with a short list of
deviations, and the migrate protocol (Q4) passed its acceptance tests against a live engine.
The spike is 6,900 lines of code and 4,900 of tests across sixteen packages, lint-clean, with
every unit test running over a strict scripted driver, and proven against PostgreSQL 18.4.
Stage 14 built the four preparations the review asked for, so what follows describes the spike
as it closed.

## 2. The layers, evaluated

The library's five layers from the Ontology, then the service's. Each entry states what the
layer proved, what it costs, and what the review changed.

### 2.1 Files

Authored files with `--|` declarations, `{{ }}` parameters and placeholders, and `{{> ns.name}}`
includes. Proven: two domains and the seeder on seventeen statements and twenty-three patterns
in two namespaces, with `sqlint` enforcing the conventions. The grammar has no lexer. One
regular expression scans the body, the delimiter is reserved even inside literals and comments,
and the lint reports a stray `{{`. Cost: a `{{` in prose is a load error, which the lint catches
first. The tier declaration turned out to be the best documentation the port list ever had: a
`grep` for `--| tier: native` is the port work list.

The `field` declarations stay, deliberately. They are the allow-list of the strategy's §2.5,
the only names a request may filter or sort by, and a base's `SELECT` list cannot state them
because a base may output columns it does not expose. Each field's type is what a request value
is cast to when the read is composed, before any database is involved, so unit tests and the
lint see the contract; and `Verify` checks the declaration against the engine, so a typo fails
startup. If six lines per base ever proves tedious, the fallback is inference from the engine's
column metadata.

List expansion was built at stage 14. `{{ids...}}` and `{{ids...:type}}` expand when values
are bound, one placeholder per element of a non-empty slice; the text is rendered per element
count and cached by it, and `Verify` prepares the one-element form. It is the one authored
statement whose text is composed at bind time. The grammar's rule that a name has one
expansion form while a type belongs to each occurrence keeps it readable. The consumer is the
person domain's `find_by_ids` behind `GET /api/people?id=a,b,c`: a batch fetch by key, unpaged,
bounded by the size limit, a lookup rather than a collection read.

### 2.2 Catalog

`Publish`, `Patterns()`, `As`, `Overlay`, `NewCatalog`. Proven: the application's namespace
beside the library's; a MySQL-shaped overlay of `paging`; validation that refuses a native
pattern inside a standard statement and an overlay with the wrong placeholders. Cost: every
catalog is built at the composition root and passed down, which is the point. A domain cannot
register a pattern, by the Ontology's rule that patterns are shared SQL, the application's or
the library's.

Nothing changes in the mechanism. The placement question was where `Source`, `Catalog`, and
`Pattern` live once `query` is a `sqlate` package; §3 keeps them in `query`, since the catalog is
what statements compile against and nothing else uses it.

### 2.3 Statements

`Compile` against the catalog and the dialect; `Statements` as a domain's inventory; `Verify`
at startup. Proven: a renamed column fails startup at lifecycle stage 2 with the statement
named; includes resolve from two namespaces; a native pattern spliced into a standard statement
is refused at compile. Cost: none found. The inventory is per domain, and §3.5 makes it walkable
across domains with a registry on `data.Database`.

### 2.4 Typed values

`Statement.Exec`; `Rows[T]` with `One`, `All`, and `Each`; `Projection[T]` with `List`, `One`,
and `Verify`; `Guard.Run`; and the struct-tag mapping (`Scanner[T]`, `ArgsOf`, `Args.With`).
Proven: three consumers, with `database.go` at 104 to 120 lines per domain holding wiring and
operations only. `Rows.All` (used by the batch fetch), `Rows.Each`, and the operators beyond
`eq` and `in` stay in the vocabulary whether or not a consumer has used them yet. The
sufficiency rule is about mechanisms, and these are the vocabulary the SDK's query parser grows
into; the request-syntax decision is go-web-sdk's (§3.3).

This is the layer Go 1.27 changed, and stage 14 made the change. The constructors were package
functions because Go had no generic methods, and the plan's decision 3 said so. They are now
methods on `Statement` and `DB`, and each value reads as what it is: a statement bound to a
scan.

```go
// before                                                  // now
query.Scan(stmts.Statement("create"), query.Scanner[Identity]())   stmts.Statement("create").Scan(query.Scanner[Identity]())
query.Project(stmts.Statement("person_view"), query.Scanner[Person]())  stmts.Statement("person_view").Project(query.Scanner[Person]())
query.Guarded(stmts.Statement("edit"), check, "version")   stmts.Statement("edit").Guarded(check, "version")
sqldb.Transact(ctx, db, func(tx *sqldb.Tx) (T, error) {…})  db.Transact(ctx, func(tx *sqldb.Tx) (T, error) {…})
```

`Scanner[T]()` and `Scalar[T]` stay functions, since they take no receiver. `Guarded` needed
no type parameter and moved with the others so every value is constructed the same way. The
`T` of `Scan` and `Project` is inferred from the scan function. Both domains, the seeder, and
every test are the consumers; `database.go` reads as a table of bound statements.

### 2.5 Execution

For the collection read: directives to a signature to a composed statement. Arguments bind by
name; execution runs through a session; errors map through the dialect at the runner boundary,
with `ErrorMapper` for what the session cannot see. Proven: filter values parsed by the engine,
with SQLSTATE class 22 as the request's 400; all three guard outcomes; the transaction
requirement checked at bind. Cost: the collection read composes text on every request. The text
is deterministic per signature and the driver caches prepared statements by text, so the cost
is string work rather than round trips. The architect's verdict at stage 13 is that the string
work is well worth the dynamic SQL it buys; the signature cache is dropped.

### 2.6 The service: data, domains, admin, the root

- **`internal/data`** (290 lines): the session-and-catalog grouping, the schema, the
  application's patterns, the seeds behind a `Seeder`, and, from stage 14, the statement
  registry. Each domain and the seeder register their compiled inventory at wiring, and
  `GET /admin/database/statements` walks it beside `/patterns`. The stage 12 review settled
  `internal/data` as the service's whole database infrastructure, with the domains and the
  admin service as peers over it. It is the type the template scaffolds and the service grows:
  a second database, a read replica.
- **The domains** (541 and 559 lines each, tests excluded): `entities.go` with `Validate`
  methods and the tag conventions, `database.go` as the SQL client, `service.go` and
  `handler.go` from the reference. Two findings the reference service does not yet have: a
  command's validation is the entity's, and existence and uniqueness are the store's as
  constraint violations; and an action is a named transition with its own protocol, never an
  `edit`. Both are domain-architecture principles.
- **`admin/database`** (508 lines): after the consolidation, operations and policy only. Its
  `Service` is generic over a migrator, a seeder, a catalog, and the pool; §3.2 places it.
- **The composition root** (298 lines in eight files): one file per layer, each owning its
  mount, with `routes.go` at two lines. §3.4 evaluates it against a single `services.go`.
- **`internal/sdk`** (76 lines): `IfMatch` and `PreconditionError`, staged for go-web-sdk, and
  `Directives(web.Query)`, the translation from the SDK's query type to the library's
  directives. That translation is service-side by dependency direction (§3.3).
- **`cmd/sqlint`** (700 lines): configured by `sqlint.toml`, the whole linter in `main.go` and
  `config.go`, decomposed at promotion (§3.1). Its stripper empties single-quoted literals and
  double-quoted identifiers and drops line and block comments before a native form is matched.
  An escape pragma was considered and dropped: a suppression is a patch over the parser, and a
  false positive is fixed in the stripper.

### 2.7 The tests

Sixteen packages, every one a unit-test package over `drivertest`'s strict scripted driver,
plus the compose suite (`//go:build compose`) as the tier that needs a real engine: the
migrator's acceptance tests, engine-side value parsing, and the live catalog. `drivertest`
records arguments unconverted and fails on an unscripted call, a mismatched argument count, or
a response that does not fit its call; it found two real defects during the stages. Its home is
§3.1.

## 3. The layout review

Which types, methods, and functions belong to which repository, judged against the standard's
package layering: downward dependencies, one concern per package, and a library that imports
nothing of ours below it. The stage 9 review set the direction (`sqlate` below go-database,
engines as sub-modules, the library's boundary a plain `*sql.DB` and a dialect); stage 12
proved it builds. This section finishes the placement.

### 3.1 sqlate

**Naming (the architect, stage 13).** The library is `sqlate`, at
`github.com/standards-lab/sqlate`. The name combines SQL and template: the library makes plain
`.sql` files dynamic and composable through templating instead of replacing them. Read aloud,
it is also "escalate": the library extends what a `.sql` file can do. The problem it addresses
is that web services usually avoid real SQL. Queries are embedded as string literals in the host
language or generated by an ORM, and both make the actual query invisible and cut it off from
SQL's own tooling (highlighting, linting, language servers, `EXPLAIN ANALYZE` run directly).
`sqlate` keeps queries in first-class `.sql` files and fixes their one real weakness, that they
are static and repetitive. The name drops the organization's `go-` prefix on purpose:
go-database and go-web-service are interdependent layers of one architecture, while `sqlate` is
a standalone library any Go project can adopt.

**What it is a pattern for.** SQL is the first DSL in the architecture that needs this much
support from the host language, because SQL was not designed for the composition and
portability the architecture requires. Not every DSL will need it; a language designed to
compose on its own needs no host-language manipulation. For the ones that do, `sqlate` is the
pattern: a grammar the `.sql` files are written in, a host library that compiles and composes
them, a catalog that sources patterns from multiple locations under namespaces, engine
sub-modules that own an engine's spellings, and a lint that enforces the conventions.

Its own repository sits in the workspace order beside go-core, since it imports only the
standard library. Package by package, from the spike's `lib/`:

| Package | From | Contents |
|---|---|---|
| `sqlate`, the root | `lib/sqldb` | `Session`; `DB`, which wraps a plain `*sql.DB` through `Wrap`; `Tx`, `Begin`, `Transact`, `TxOption`; the `Dialect` interface; `Locker`; `ErrorMapper`; and the error types `ErrConnectionFailed`, `ErrInvalidValue`, and the constraint classes with `ConstraintError`, moved in from go-database because the engine sub-modules return them |
| `query` | `lib/query` | the catalog (`Source`, `Publish`, `Patterns`, `Catalog`, `Pattern`); `Compile` and `Statements`; `Statement` with the methods that produce the typed values; list expansion; the struct-tag mapping; `Directives`; `Verify`; `ErrDirectives` and its errors; `ErrVersionMismatch` |
| `migrate` | `lib/migrate` | unchanged: `Migration`, `Files`, `Migrator`, `Options`, `Catalog`, and the error types |
| `internal/header` | `lib/sqlheader` | the declaration grammar, shared by `query`, `migrate`, and `sqlint`; internal because no consumer parses headers |
| `sqltest` | `lib/drivertest` | the scripted driver: `Open`, `Recorder`, `Response`, and a stub `Dialect`. Public, because consumers' unit tests use it (the testing hierarchy's prepare-capable driver) |
| `sqlint` | `cmd/sqlint` | `Config`, `Load`, a `Resolver` over `go list`, `Lint(fs, cfg, resolver) []Finding` with `Finding{Path, Line, Message}`, one file per role's checks, and the glob matcher. `cmd/sqlint` reduces to arguments, the root directory, output, and the exit code, so the harness calls the package |
| `postgres` (sub-module) | `lib/pgdialect` | the whole dialect: `Name`, `Placeholder`, `MapError` with the constraint classes and class 22, `Locker` over advisory locks, and migrate's `Catalog`; its `sqlint.toml` with the native forms; live tests that import the driver directly. It ships no overlay, because PostgreSQL is the standard spelling |

Two placements that were open closed with the split rehearsal. The dialect interface is
sqlate's: go-database's postgres dialect satisfied it structurally, which shows the interface
was always the library's. The error types follow `MapError`: whoever classifies owns the error
values.

### 3.2 go-database

Version 0.4.0, breaking. It keeps the infrastructure service: `Config` with the pool settings
and the environment layer, `New(pool, cfg)` over the provider-constructed `*sql.DB`, `Start`,
`Shutdown`, `Ready`, `Ping`, and `ErrNotReady`. It drops `ast`, `operation`, `exec`, and `seed`
as the strategy planned, and also `Session`, `Tx`, `ExecTx`, `Dialect`, and the constraint
classes, all of which sqlate now owns. The `postgres` sub-module keeps the driver, the DSN, and
pool construction, and no longer supplies a dialect; the composition root takes the dialect
from `sqlate/postgres`.

It gains the admin service, which settles the stage 9 question of go-database versus
go-web-sdk. The service half of `admin/database` is generic over a migrator, a seeder's verify
and run, a catalog, and the pool, and it depends on sqlate, go-core's lifecycle, and
go-database's own `DB`. go-web-sdk cannot host it without importing go-database, an upward
dependency for a web SDK. go-database can, because consuming sqlate is the direction the split
establishes. So `go-database/admin` contains `Service`, `Options`, `Register`, `Start`, `Ready`,
`Status`, the verbs, `Seed` with its policy, `Diagnose`, `Catalog`, and the entity types those
return. The HTTP half, the route group and handler, is small and go-web-sdk-shaped; it stays
application code that the template scaffolds, as does the choice to mount it under `/admin`.
The stage 1 rule holds: an admin service's operations are triggers over library functions.

### 3.3 go-web-sdk

Three items, none of them SQL:

- `IfMatch` and `PreconditionError`, staged in `internal/sdk` since stage 8, promote as they
  are.
- `ErrorWriter` gains an option to carry the error text on chosen statuses
  (`web.WithDetail(statuses ...int)`), the stage 1 finding. The admin handler's local `reject`
  and the domains' matchers then retire.
- The strict body decode (unknown fields rejected, size bounded, a validation rejection with
  the reason) and the `respond` shape repeat in every handler and belong in go-web-sdk.

One decision rather than an item: whether the query parser gains an operator syntax. Every
operator but `eq` and `in` is unused after three consumers, because `web.Query` yields exact
matches only. The `Directives(web.Query)` translation is service-side by dependency direction
(go-web-sdk must not know sqlate), the template scaffolds it, and it grows when the parser
does.

### 3.4 go-web-sdk-template

The template scaffolds what the experiment found every service needs; this is where the five
principles the experiment incubated land as code:

- **`internal/data`**: `Database` and `New`; the `migrations/`, `seeds/`, `patterns/`, and
  `statements/` directories with `Migrations()`, `Patterns()`, and a `Seeder` skeleton; the
  `app` namespace registered at the root.
- **`admin/database`**: the handler and route group over `go-database/admin`'s service, the
  `/admin` mount, and the `admin.seed` config block with `APP_ADMIN_SEED` off by default.
- **The composition root as files, not packages.** `internal/infrastructure`,
  `internal/domain`, and `internal/reactors` collapse into `internal/app/infrastructure.go`,
  `domain.go`, `admin.go`, and `reactors.go`, each constructing its layer and owning its mount,
  with `routes.go` the list of mounts. The stage 1 review found the packages had one consumer
  each, and that `reactors.New` taking the `Domain` type forced the collapse.
- **A single `services.go`**, evaluated and not recommended. The four files total 160 lines;
  one file would hold four types and four constructors in one place, and the order of
  construction would still be `app.go`'s. The file-per-layer form makes the root's table of
  contents the architecture's layer list, which is the property worth keeping: a reader opening
  `internal/app` sees infrastructure, admin, domain, and reactors as files. If a layer's file
  ever grows past a screen, that argues for keeping it separate, not merging it.
- **`routes.go`** is already the two-line list; each layer's file owns its mount function. The
  simplification is that it composes and does nothing else.
- **`internal/sdk`** seeded with `Directives(web.Query)`; **`sqlint.toml`** with the roles and
  sources; **`mise.toml`** with `lint` running `sqlint`, `test-compose`, and `db-up`.

The entity roles (validation methods on commands; `db` then `json` tags as the scan and
binding contract) are conventions the template documents rather than scaffolds, since it ships
no domain.

### 3.5 go-web-service

Adopts the whole shape: the organization domain rewritten onto sqlate (the spike's is the
draft), `internal/data` with the registry, the admin layer, the composition root as files,
`cmd/db` and golang-migrate deleted, and the integration tier over the compose stack. The
spike's person domain does not promote. It was simplified to keep the experiment small, and
the people domain gets its own planning under `v1.data.people`. One addition the review
recommends the service build rather than the spike:

- **Soft delete** as the standard's convention, from the stage 8 review: `delete` moves a record
  to the recycle bin, `restore` returns it, and `purge` removes it physically and is
  administrative. A `deleted_at` column, a base that excludes deleted rows with a recycle view
  beside it, partial unique indexes over live rows, and the delete pattern as an update: a
  pattern pair for the library's catalog once a domain needs it.

### 3.6 Cross-cutting: what waits, and why

- **Seed helpers.** The per-table loop with insert counting and code-to-id resolution is 70
  lines for two domains; a generic `seed.Table[T]` waits for the template's second service,
  under the sufficiency rule.
- **The signature cache.** Considered and judged unnecessary (§2.5); recorded so it is not
  re-proposed without a measurement.
- **The lint's stripper.** With regular expressions over code, tokenizing would remedy nothing.
  What the stripper does not yet strip is PostgreSQL's dollar quoting, added when a false
  positive appears.
- **The management listener's isolation.** The admin mount's own listener, authentication, and
  audit are the strategy's production constraint and stay the
  `v1.data.sql.integration.listener` task.

## 4. The promotion plan

The order is the dependency order. Each step is one session unless marked.

1. **Create `sqlate`** from `lib/` and `cmd/sqlint` per §3.1, with `sqlate/postgres` from
   `pgdialect`, the constraint mapping folded in from go-database's dialect, and the live tests
   over the driver. Insert `sqlate` into the workspace order beside go-core. Release v0.1.0.
2. **go-database v0.4.0** per §3.2: the removals, `admin` over sqlate, `postgres` without a
   dialect. It releases together with step 5's service change, as the goal's criterion says.
3. **go-web-sdk** per §3.3, v0.6.0.
4. **go-web-sdk-template** per §3.4.
5. **go-web-service**: the coordinated rewrite of §3.5, with `cmd/db` and golang-migrate gone,
   the organization domain end to end, and the release. Then the people domain from the spike's
   person domain under `v1.data.people`.
6. **The listener**, **the docs pass**, **the harness follow-through**, and **the integration
   tier** as the existing tasks, re-read against the split.

The spike stays under `experiments/sql-dsl` as the record.

## 5. Workspace context adjustments, and 6. The roadmap refinement

Applied at the experiment's `close` on 2026-09-03; the reset file's disposition of that session
(`context/reset.md` at commit `cef2527`) records each edit. The roadmap under
`goals.v1.data.sql.integration` carries the tasks the review drafted, and the coordinator's
concept and design note carry the positions.

## 7. Open, and deliberately so

- Where in the landing zone the grammar's page sits and what it is called: the docs pass's
  call.
- Dollar quoting in the lint's stripper, and any further blind spot, each fixed in the stripper
  when a false positive appears.
- Whether the query parser gains an operator syntax: go-web-sdk's decision under
  `v1.data.sql.integration.websdk`.
