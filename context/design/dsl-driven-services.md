# SQL as a DSL-driven service

The strategy for SQL in the reference architecture: the `sqlate` library, go-database v0.4,
and go-web-service on authored SQL. It was settled on 2026-08-29 across two sessions that
reviewed go-database v0.3.0 (`ast`, `operation`, `exec`) against the organization domain in
go-web-service, and it was adjusted three times since; §11 lists each adjustment by date.
Every section below states the current position.

This is a strategy record. It contains the principles, the reasoning that produced them, and
the shape of the result. Implementation detail lives elsewhere: the sqlate repository's own
guide (`github.com/standards-lab/sqlate`, its README and `docs/`) for the library's packages
and grammar, the prototype's review
(`experiments/sql-dsl/REVIEW.md`) for the placement of every type, and the roadmap
(`goals.v1.data.sql`) for the task breakdown.

## 1. The ambition

The architecture exists to give an engineer layers of tooling for building precise, capable
infrastructure with as few layers and as little overhead as the work allows. Each layer must
find the structure that serves both using its features and expressing an implementation
against them. It must stay portable and configurable, so a change in the surrounding
environment has little effect on what has been built. And it must limit exposure to
technical-debt and supply-chain risk.

SQL tested this ambition hardest, because SQL is the one infrastructure integration that
brings its own language. The v0.3.0 answer, a Go statement vocabulary that could express any
query, was capable and well built, and wrong for the ambition: every statement was longer and
stranger than the SQL it rendered, and the library grew to carry expressive content that SQL
already carried. Working out why produced the principles below, which apply beyond SQL.

## 2. Principles

### 2.1 DSL-driven services are a distinct category

A protocol-driven infrastructure service (auth, blob storage, messaging) keeps its expressive
content in the host language. The consumer calls operations with typed arguments, and the
provider boundary is an interface over those operations. This is the pattern the standard
already establishes for infrastructure services, and it stands for them.

A DSL-driven service keeps its expressive content in a language the host cannot type-check:
SQL, and equally Cypher or Gremlin, a search engine's query DSL, KQL, PromQL, or Rego and
Cedar for policy. Its operations are trivial (execute, query) and all the meaning is in the
text. Forcing such a service into the protocol-driven pattern produces the v0.3.0 failure: a
large host-language interface standing in for a language that was already the right interface.

The DSL text is therefore the primary artifact. It is written in its own language, versioned
in the repository, and reviewed as itself. The host-language layer exists only for what the
DSL cannot do on its own:

1. bind parameters safely;
2. compose against runtime input through a declared list of fields;
3. map results back into host types;
4. verify the text against its target;
5. own the session or transaction boundary.

SQL is the reference implementation of the category. Its shape (native text, a thin library, a
verification step, a tier declaration) is what a later search or policy integration reproduces
with no shared code.

### 2.2 SQL is the DSL; Go is the mechanism

The library does only what SQL cannot express. Every pattern the domain layer needs (a
collection with dynamic filters, sort, and paging; a single record by unique constraint; a
mutation with side effects across tables in one transaction) is a SQL pattern with a small
amount of Go around it. The library supplies that Go and stays out of the expressive content.

A new pattern is a convention with two halves. The first is a SQL shape (a file layout, a
naming rule, a header) that a reviewer or a lint can check. The second, only where needed, is a
Go function that takes statements the consumer wrote. Most new patterns need no new function:
upsert, soft delete, and batch insert are SQL shapes over the existing runners. A pattern earns
a function only when it carries a protocol the SQL alone cannot guarantee; the
optimistic-concurrency guard is the worked example. The library takes the consumer's SQL as
input and never generates it.

### 2.3 The portable artifact is the language, not the library

Every protocol-driven service has one portability axis, the library. SQL has two, the library
and the dialect. A runtime query builder promises to solve the dialect axis and in exchange
worsens the library axis: it is a permanent dependency with its own release cadence and
vulnerability history, which no other infrastructure service carries.

This strategy makes the language the portable artifact. Authored SQL ports by editing SQL. The
library axis collapses to `database/sql` and one driver, the same shape auth or storage has.
The dialect axis is handled by discipline: a per-file tier declaration (§6.4) and a lint that
enforces it, rather than a dependency or a type system.

That is a deliberate trade, recorded as one. v0.3.0 enforced portability by construction, with
a typed `UnsupportedFeatureError` at render time. This strategy enforces it by declaration and
lint. The declared stack selects one engine, and the portability promise exists to bound a
future port, not to make today's code engine-neutral. Discipline is the right side of that
trade, and it stays written down so no later session tries to buy the type guarantee back with
a builder.

### 2.4 Sufficiency, not only capability

The v0.3.0 failure was not a missing capability. It was an extra layer that was fully capable.
The ambition's "as few layers as the work allows" therefore needs an active rule, applied
before a layer is built: a layer must justify itself against what the language, the standard
library, and the tools already inside the dependency line express. The question "does the
industry already solve this inside the line?" is asked at plan time, not discovered at review.
The marathon harness can ask it at `plan`; the SQL layer is its first worked case.

### 2.5 Injection safety is structural

The vulnerability in any SQL integration is a path where request input becomes SQL text. The
library's job is to make that path not exist:

- Statement text is fixed at build time: `.sql` files embedded with `//go:embed`.
- Request values enter only as bound arguments.
- The only text a request can influence is the choice of filter and sort fields, and those
  pass through the list of fields a statement's header declares. An unknown name is a typed
  error, never interpolation.
- The parameter delimiter is reserved. Text that looks like a parameter and is not one fails
  the load, so nothing is silently interpolated.

## 3. Alternatives considered

Recorded so the reasoning does not have to be reconstructed.

**ORMs (gorm, bun, ent).** Outside the standard's dependency line by definition. They are
frameworks, and the expressive content moves into their vocabulary.

**Runtime builders (goqu, squirrel, bob, jet).** Each is a framework-sized runtime dependency
with its own dialect model. squirrel is in maintenance mode; goqu's breadth is its own
liability; bob and jet generate against a live schema and still keep the builder at runtime.
None has the standard-versus-native tier split. Adopting one would replace the `ast` layer
with something larger, less aligned, and permanently on the dependency graph, the wrong side of
§2.3.

**SQL-to-Go generators (sqlc).** The strongest alternative: a build-time generator with no
runtime footprint, native SQL files, and schema verification at generate time. It lost on the
DSL principle. It does not generalize (there is no sqlc for Cypher or a search DSL, so it sets
no pattern for the category). It locks the engine at the tooling level (PostgreSQL, MySQL, and
SQLite only, which matters when future ports are unknown). It is a build-time dependency with a
C parser behind it. And it forces two authoring conventions, one for generated queries and one
for the runtime library. Its one irreplaceable contribution, schema verification, is provided
by the library itself (§4.2, verify).

**Keeping the `ast` vocabulary.** About 6,000 lines, 89% covered, aligned with the portability
principle, and a good answer to a protocol-driven framing of a DSL-driven problem. Its
statement vocabulary retires. What survived into the library is about 250 lines: the typed
request errors and the `Directives` vocabulary, the key tie-breaker and duplicate-field rules,
the generic scan and query functions, and the guard's outcome logic. The renderer, the
recursive-path helper, and the compound machinery were not ported.

**Runtime templating over `.sql` files (`text/template` or similar).** Considered at the
retrospective for dynamic composition and a shared catalog of shapes, and rejected. It reopens
the §2.5 injection path as discipline instead of structure, because text interpolation into SQL
becomes a first-class operation with no SQL-aware escaping. It breaks the properties native
files were chosen for: editor and lint support, an enumerable statement inventory, and a
verification step that prepares the final text. And it does not shrink the library, because
argument ordering across stitched fragments still needs Go. What the prototype built instead is
the pattern catalog (§7): includes resolve at compile time against SQL the library or the
application published, and request-time composition assembles library patterns only. No
request text is ever interpolated.

**PRQL.** Considered at the prototype and ruled out for this layer: analytical-only by its own
statement, no Go binding, and a second language above SQL. It is a reference point for the SQL
meta-language concept, not a mechanism.

## 4. sqlate

### 4.1 A standalone library below the infrastructure service

The library is its own repository and module, `github.com/standards-lab/sqlate`, released at
v0.1.0 on 2026-09-04. Its base module imports only the standard library; a sourced dependency
enters only through a sub-module's `go.mod`. It owns everything from the `.sql` file to the
scanned row. go-database v0.4 is the infrastructure service above it (§5). The original plan
kept `query` and `migrate` inside go-database; the prototype's split rehearsal showed the
library needs nothing of the service, so a CLI, a worker, or a test harness can use authored
SQL with no lifecycle machinery. The library is adjacent to the standard rather than a layer of
it: the standard's libraries consume it, it is the blueprint for how a DSL gains host-language
support, and its own text names nothing of the architecture. The packages, in brief; the
repository's guide documents them:

- `sqlate`, the root package: `Session`, `DB` over a plain `*sql.DB`, transactions, the
  `Dialect` interface, the lock capability, and the error types the engine sub-modules return.
- `sqlate/header`: the declaration grammar. Public, because the linter is a separate module and
  parses headers for every role.
- `sqlate/query`: the pattern catalog, compilation, statements, the typed values that run them,
  the struct-tag mapping, request directives, the guard, and verification.
- `sqlate/migrate`: schema versioning over embedded SQL files. It replaces golang-migrate in
  every consumer.
- `sqlate/sqltest`: the scripted `database/sql` driver every consumer's unit tests run over.
- `sqlate/sqlint`, a sub-module: the conventions linter as a package over the TOML parser;
  `sqlint/cmd/sqlint` is its command. One `sqlint.toml` per module, at the module root; a
  source or engine is a module path, resolved through `go list -m`.
- `sqlate/postgres`, a sub-module: placeholders, error classification, the lock, the
  server-version statement as a capability the admin service asserts (v0.1.1), its own
  `sqlint.toml`, and the integration tier's engine proofs. It adds no migration catalog, since
  the standard one serves PostgreSQL.

### 4.2 The query package

Described by responsibility; the concept and the review carry the signatures.

**Statements.** A statement is text plus ordered arguments. The text comes from a `.sql` file.
A parameter is written `{{name}}`; `{{name:type}}` casts through the engine's own type;
`{{ids...}}` expands a list into one placeholder per element. The compiler resolves parameters
to the dialect's positions once. A file's `--|` header declares its tier, the engine feature a
native file uses and its port, whether the statement requires a transaction, and, for a
projection base, its key and the fields a request may use. The engine receives the body alone.
A domain compiles its embedded directory against the pattern catalog into a `Statements` value,
which is also the inventory the verification step and the admin mount walk.

**Projections and directives.** A projection is an authored base query plus the fields its
header declares. Those fields are the allow-list of §2.5 and the vocabulary that request
directives (filter, sort, page) are resolved against. `Directives` and its typed
`UnknownFieldError` and `UnknownOperatorError` carried over from v0.3.0 unchanged; they were the
part that was already right. Each field carries a type, so a malformed filter value is a typed
400 rather than an engine cast error surfacing as a 500.

**Composition.** The collection read composes `SELECT … FROM (<base>) q WHERE <filters> ORDER BY
<sorts>, <key> OFFSET … FETCH …` and its count twin from library patterns at request time. The
derived-table wrap is standard SQL, and it lets the base be any authored query (a recursive CTE,
a join tree, a view) without the library inspecting it. Beyond the collection read, a list
parameter is the only statement whose text varies at bind time, and only in its placeholder
count.

**Execution.** Three generic types bind a statement to what runs it: `Rows[T]` to a scan
function, `Projection[T]` to a base with its fields, and `Guard` to a command and its version
check. Every runner takes a `Session` and none takes a dialect; the consumer never renders. The
engine's errors are mapped through the dialect inside the session, so constraint classification
is never opt-in.

**The guard.** The optimistic-concurrency protocol over two statements the consumer wrote: run
the command; on zero rows affected, run the version check; distinguish not-found from version
mismatch; return the new version without a second round trip. The SQL (the `SET` list, `WHERE
key = … AND version = …`, `version = version + 1`) is the consumer's, so the consumer names the
version column in its own statement.

**Verify.** Every statement in a domain's inventory is prepared against a live, migrated
schema. Engines validate column and table references at prepare time, so a column renamed by a
migration or a typo in a file fails at startup and in tests rather than at the first request,
on any engine. It runs at service startup (§6.2) and on demand from the admin mount (§6.3).

**Row mapping.** `Scanner[T]` matches a row's columns to struct fields by name and rejects a
column the struct has no field for; `ArgsOf` binds a command's fields to parameters by the same
names. The `json` tag is the usual source of a name and a `db` tag overrides it. The prototype
settled this in favor of the mapper: it is well under 150 lines of standard library, and the
alternative, one hand-written scan function per row shape, repeated across every domain.

### 4.3 The migrate package

Under §2.2 a migration is SQL text plus a thin mechanism: a version table, an ordered set of
embedded `.up.sql` and `.down.sql` files, each applied in a transaction under a named advisory
lock, and the version recorded. It replaces golang-migrate, which in go-web-service was a
runtime dependency carrying thirty drivers for the sake of one, the weakest link in the module
graph under the dependency line, present only because the CLI used it.

The package was built to the cases a mature library has already met, and proved them live
against PostgreSQL 18: dirty state after a failed migration (recorded, reported, cleared only by
an explicit force), engines without transactional DDL (each statement recorded individually),
concurrent starters across processes (the lock), and cancellation. These were the acceptance
criteria of the session that built it.

## 5. go-database v0.4.0

Released 2026-09-04 as v0.4.0, with postgres/v0.3.0, breaking. go-database keeps the
infrastructure service: `Config` with the pool settings and the environment layer, the pool over
the provider, lifecycle, and readiness. It dropped `ast`, `operation`, `exec`, `seed`, `Session`,
`Tx`, `Dialect`, and the constraint classes, all of which sqlate owns. The `postgres` sub-module
keeps the driver, the DSN, and pool construction, and supplies no dialect; a composition root
takes the dialect from `sqlate/postgres`.

It gained the `admin` package: the database admin service, generic over a migrator the consumer
builds, a `Seeder` whose `Seed` returns a count map, a `Registry` of compiled statements, the
pattern catalog, and the pool. The service verifies, migrates, and seeds at startup and on
demand, and every operation it exposes is a trigger over a library function. It lives in
go-database rather than go-web-sdk because it depends on the pool, and a web SDK importing
go-database would be an upward dependency. The HTTP half (the route group and handler) is small
and stays application code that the template scaffolds. The server-version read is a dialect
capability the `admin` package declares and `sqlate/postgres` implements, so engine-native text
stays with the engine. The composition a root writes is `go-database/context/design/
infrastructure-service.md`.

## 6. go-web-service

### 6.1 A domain on authored SQL

```
domain/organization/
  doc.go
  entities.go        # the entity, the commands with their Validate methods, the tag conventions
  database.go        # the store: compiles statements/, binds the typed values, exposes operations as methods
  service.go
  handler.go
  statements/
    organization_view.sql   # the read model: columns plus the computed path, a recursive CTE
    in_subtree.sql
    lock_tree.sql
    create.sql              # native: RETURNING
    edit.sql                # guarded
    transfer.sql            # guarded
    delete.sql              # guarded
    version.sql             # the guard's check
```

Every statement is SQL in a file an editor can check, with a header that declares its tier.
The recursive lineage CTE that v0.3.0 expressed as `RecursivePath`, with `||` built by Go string
concatenation, is nine lines of SQL with `||` written as `||`. `database.go` is the only file in
the domain that imports the library. It compiles the directory against the catalog, registers
the inventory, binds each statement to its typed value, and exposes each operation as a method
that reads as what it does. A command's validation belongs to the entity's `Validate` method;
existence and uniqueness belong to the store, as constraint violations. A statement is named
for its operation, never its SQL verb.

### 6.2 Startup

`cmd/db` is gone. Schema versioning, verification, and seeding are library mechanisms the
composition root triggers, in three concerns kept separate because their risks differ:

- **Verify** is always on, cheap, and safe. The migration history must be the embedded set's
  clean head, and every statement must prepare against the live schema. Failing fast here is
  pure upside.
- **Apply** runs when verification finds pending migrations: the admin service applies them
  under the lock and verifies again. A state the mechanism cannot correct (a dirty row, a
  history the set does not carry) fails startup, and an operator resolves it through the admin
  endpoints. Whether a process that serves traffic should ever hold DDL privileges, and whether
  the standard should mandate a separate migration role and a one-shot invocation of the same
  binary, are open posture questions (§10).
- **Seed** is development and test tooling, off unless the environment enables it. Reference
  data that production needs is a migration.

The admin service owns the first lifecycle stage. The domains verify their statements at the
second, against the migrated schema.

### 6.3 The admin mount and the management listener

The admin service is mounted under `/admin` with the former CLI verbs (verify, up, down, steps,
force, seed), the pattern catalog and the statement inventory for inspection, and diagnostics
the CLI never had: pool statistics, ping latency, the server version and dialect, and native
views such as active sessions. Every endpoint calls the same function startup calls.

In production the mount lives on its own listener: its own port or socket, authenticated,
unreachable from the public API's network path, with audit logging on anything that mutates.
That isolation is a design constraint, not a deployment detail. `down` and `force` are
destructive and require an explicit confirmation token. The listener, its authentication, and
the token are the `v1.data.sql.integration.listener` task; config rendering on it waits on a
redaction contract in go-core.

### 6.4 The tier declaration

Each `.sql` file opens with a header:

```sql
--| tier: standard
```

or

```sql
--| tier: native
--| native: postgres — RETURNING. Ports: OUTPUT INSERTED (SQL Server), RETURNING INTO (Oracle).
```

A standard file uses ISO/IEC 9075 forms only. A native file names the engine feature it uses
and how another engine expresses it, so the native files in a repository are the complete work
list for a port. The compiler reads the header, the admin mount reports it, and `sqlint` checks
it in CI: a standard file that matches one of the engine's declared native forms fails the
lint. This is the render-time capability check of v0.3.0 translated into authored SQL, and it
is the first phase of the meta-language concept's compiler policy: a unit that uses a native
feature declares it.

## 7. The pattern catalog

A pattern is SQL that implements a shared rule, published as a `.sql` file with a tier and
placeholders under a namespace. The library's own SQL is patterns under the namespace `sql`; an
application registers its own namespace beside it (`app`, from `internal/data`); an engine
sub-module overlays the one pattern it spells differently. The composition root builds the
catalog once, and every domain compiles its statements against it. A statement includes a
pattern at compile time with `{{> sql.guard_where}}`; the collection read composes the
request-time patterns (`collection`, `count`, `one`, `where`, the operators, `order`, `paging`)
from a base and the request's signature. `sqlint.toml` names the same sources, so the linter
resolves includes exactly as the runtime does.

The conventions the standard names. Each is a SQL shape; a Go function is listed only where one
exists.

| Pattern | SQL shape | Go |
|---|---|---|
| Collection | a base query naming the read model; the header's fields map to its columns | `Projection[T]`: page and count |
| Single by key | `WHERE <unique> = {{key}}` over the same base | `Projection[T].One`, or a named statement |
| Guarded mutation | a fixed `SET` list with the `guard_set` and `guard_where` includes; a version-check statement | `Guard` |
| Identity-returning insert | `INSERT … RETURNING` (native) or insert plus lookup (standard) | `Rows[T]` |
| Transactional side effects | several named statements in one `Transact` | `Transact` |
| Lineage or hierarchy | a recursive CTE as the base of a projection | none |

Admission rule for a new pattern: at least one domain has needed it, its SQL shape is stated,
and it earns a Go function only if it carries a protocol the SQL cannot guarantee alone.

## 8. What changes, by artifact

| Artifact | Change |
|---|---|
| sqlate | done: the repository and module from the prototype's `lib/` and `cmd/sqlint` (§4.1), released as v0.1.0 with `postgres/v0.1.0` and `sqlint/v0.1.0` on 2026-09-04 |
| go-database | v0.4.0 (§5): the infrastructure service plus `admin`; `ast`, `operation`, `exec`, `seed`, the session types, the dialect, and the constraint classes removed; `layers.md` rewritten |
| go-web-sdk | done: v0.6.0 (2026-09-05): `IfMatch` and `DecodeJSON`, `ErrorWriter.Detail`, the error-returning handler adapter pulled forward from `v1.web.adapter` in place of a respond helper, and the bracket operator grammar in `ParseQuery` |
| go-web-sdk-template | scaffolds `internal/data` (the session-and-catalog grouping, migrations, the application's patterns, the seeds behind a seeder, the statement registry), `admin/database` over go-database's admin service, the composition root as one file per layer, `sqlint.toml`, and the mise tasks |
| go-web-service | `cmd/db` and golang-migrate removed; a `statements/` directory per domain; `database.go` rewritten over sqlate; the admin mount; the entity roles (validation methods, the tag conventions); the management listener |
| docs | the DSL-driven-services principle page (§2); the sqlate pages; the go-database pages rewritten; the grammar recorded as the standard's own artifact, sqlate its first host; the SQL meta-language concept reframed with this work as its first phase; the architecture definition amended so a Domain Service anchors a domain, a composition of one or more Entities |
| claude-plugins | the sufficiency question (§2.4) enters the `plan` stage; the checkable conventions are `sqlint` called as a package, not rules the harness re-implements |

## 9. Sequence

`goals.v1.data.sql.integration` carries the tasks in dependency order: `sqlate`, `database`,
`websdk`, `template`, `service`, `listener`. Each library releases when its task closes, since
it depends on nothing above it, and the tasks above pin the release; the `service` task is a
coordinated session that pins them all. Then `docs`, `hardening`, and `suite` under
`goals.v1.data.sql`.

## 10. Open questions

- **DDL in the serving role.** Whether a process that serves traffic should ever hold DDL
  privileges, and whether the standard should mandate a separate migration role and a one-shot
  invocation of the same binary. The prototype applies at startup; the listener task decides.
- **The management listener's default.** Whether network isolation is sufficient, or the
  standard should require the listener to be off by default and enabled per environment. The
  listener task decides.
- **The confirmation token** for `down` and `force`: its mechanism, with the listener's
  authentication.
- **Guarded-statement conventions in the lint.** The library no longer guarantees the guard's
  SQL shape, so a consumer can write a guarded update that forgets the increment or the version
  predicate. A check that a file named or annotated as guarded contains both clauses belongs to
  `sqlint`; the hardening task carries it.
- **The lint's stripper** does not yet strip PostgreSQL's dollar quoting. Added when a false
  positive appears.
- **A generic seed helper.** The per-table loop is 70 lines for two domains; a generic
  `seed.Table[T]` waits for the template's second service, under §2.4.
- **The grammar's page** in the docs landing zone: where it sits and what it is called, the docs
  pass's call.

## 11. History

- **2026-08-29, settled.** Two sessions reviewing go-database v0.3.0 against the organization
  domain produced §1 to §3 and the plan for `query` and `migrate` inside go-database, with the
  organization domain, startup, and the management listener as the service-side tasks.
- **2026-08-31, retrospective.** The layer evaluation widened the v0.4 breaking window: error
  mapping moved inside the session, so constraint classification is never opt-in; one
  transaction-runner shape across every runner, with panic recovery, joined rollback errors,
  and `sql.TxOptions` reachable; the `postgres` sub-module recognized as provider-visible blast
  radius, with per-module `GOWORK=off` builds entering CI first because the committed `go.work`
  had masked a broken pin at tag v0.3.0; the salvage from `ast` sized at about 250 lines; the
  field list given types; and the scripted driver promoted to a shared test package for a
  prepare-capable verify. Runtime templating was considered and rejected (§3).
- **2026-09-01, plan session.** The deferred design questions were worked through to a reviewed
  API and then routed to evidence: the `v1.data.sql.prototype` experiment, building the whole
  SQL-to-Go layer in one template-generated service before v0.4 broke the library, the provider,
  and the service in lockstep. `seed` retired from a package to a documented pattern under
  §2.4. Pattern templates were admitted under one split: a template carries protocol only, the
  domain all expressive content. Named `:name` parameters replaced ordinal `?`.
- **2026-09-03, prototype close.** The experiment (`experiments/sql-dsl` at the coordinator; its
  record `NOTES.md`, its verdict `REVIEW.md`) settled what the plan had left to evidence: the
  split into `sqlate` and go-database (§4, §5); the `{{name}}` parameter grammar with list
  expansion and `--|` declarations, superseding `:name`; the pattern catalog as authored,
  namespaced files with includes at compile time and composition at request time (§7); the
  struct-tag mapper; the guard as two statements; PRQL ruled out; and the service-side shape
  the template scaffolds (`internal/data`, the admin mount, the composition root as files, the
  entity roles). The library's vocabulary is the Ontology section of `NOTES.md`.
- **2026-09-04, go-database v0.4.0.** The `database` task released the infrastructure service
  and the `admin` package (§5), with postgres/v0.3.0 and the standalone build step in CI. The
  server-version read became a capability of `sqlate/postgres` (v0.1.1) rather than a member of
  `sqlate.Dialect`. The sequence (§9) no longer holds a library release for the service change:
  each library releases as its task closes, and the tasks above pin it.
- **2026-09-05, go-web-sdk v0.6.0.** The `websdk` task promoted the If-Match parse and the strict
  body decode (413 on overflow), gave the error writer its detail set, and, in place of a
  respond helper, pulled the error-returning handler adapter forward from `v1.web.adapter` so
  the service rewrite writes its handlers once. The operator-syntax question closed with the
  bracket grammar, `field[op]=value`, the operator lexical for sqlate's `query` package to
  validate, taken now because `Query.Filters` had no consumer past the service rewrite. The
  SDK's own errors map themselves through a sealed interface; consumer policy stays the
  matchers.
