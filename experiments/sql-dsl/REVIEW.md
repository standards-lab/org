# sql-dsl — the review

The comprehensive review of the `v1.data.sql.prototype` experiment, written at stage 13
(2026-09-03) and finalized after stage 14 as the input `close` promotes from. It evaluates
each layer of the architecture the spike built and then the whole; reviews the layout of
types, methods, and functions across `sqlate`, `go-database`, `go-web-sdk`,
`go-web-sdk-template`, and `go-web-service`;
states the promotion plan for every component as the adopted strategy for the DSL service
integration; lists the workspace context the experiment changes; and drafts the roadmap
refinement under `v1.data.sql.integration`. The running record is `NOTES.md`; the vocabulary
is its Ontology section. Everything here is a recommendation until `close` applies it.

## 1. Verdict

The experiment proved the strategy and changed its shape. The strategy's claim holds: every
line of SQL lives in a `.sql` file, the library owns only what SQL cannot express, and a
domain's database client is a page of wiring over typed handles. Three of the five questions
came out differently from the plan's expectation, each with evidence:

- **Catalog composition (Q1):** on-demand stitching over authored patterns, not build-time
  generation, and the library's own SQL is patterns too. Patterns are sourced from any
  `fs.FS` under a namespace; the catalog is built once at the composition root.
- **The file grammar (Q2):** `{{name}}` and `{{name:type}}` parameters with the engine as the
  value validator, `--|` declarations, and the body alone sent to the engine.
- **The split (Q5):** a DSL library, `sqlate`, standalone and below go-database rather
  than `query` and `migrate` inside it. `lib/` builds with go-database absent from its
  import graph.

The two others confirmed the plan: the exports by extraction (Q3) match the sketch with a
short deviations ledger, and migrate's protocol (Q4) passed its acceptance proofs live. The
spike is 6,900 lines of code and 4,900 of tests across sixteen packages, lint-clean, hermetic
under a strict scripted driver, and proven against PostgreSQL 18.4. Stage 14 built the four
preparations the review asked for, so what follows describes the spike as it closes.

## 2. The layers, evaluated

The library's five layers from the Ontology, then the service's, each with what it proved,
what it costs, and what the review changes.

### 2.1 Text

Authored files with `--|` declarations, `{{ }}` parameters and slots, `{{> ns.name}}`
includes. Proven: two domains and the seed on seventeen statements, twenty-three patterns in
two namespaces, and `sqlint` holding the conventions. The grammar has no lexer: one regular
expression scans the body, the delimiter is reserved even inside literals and comments, and
the lint reports a stray `{{`. Cost: a `{{` in prose is a load error, which the lint catches
first. The tier declaration turned out to be the best documentation the port list ever had;
a `grep` of `--| tier: native` is the port work list.

The field declarations stay, and deliberately. They are the whitelist of §2.5, the only
names a request may filter or sort by, which a base's SELECT list cannot state because a
base may output columns it does not expose; the type is what a request value is cast to at
compose time, before any database exists, so hermetic tests and the lint see the contract;
and the declaration is checked against the engine by `Verify`, so a typo fails startup.
Inference from the engine's column metadata is the fallback if six lines per base ever
proves tedious.

List expansion is built (stage 14): `{{ids...}}` and `{{ids...:type}}` expand at bind to
one placeholder per element of a non-empty slice, the text rendered by arity and cached by
it, `Verify` preparing the arity-one form. It is the one authored statement composed at
bind, and the grammar's rule that a name has one arity while a type belongs to the
occurrence keeps it readable. The person domain's `find_by_ids` behind
`GET /api/people?id=a,b,c` is the consumer: a batch fetch by key, unpaged, bounded by the
size limit, a lookup rather than a collection read.

### 2.2 Catalog

`Publish`, `Patterns()`, `As`, `Overlay`, `NewCatalog`. Proven: the application's namespace
beside the library's, a MySQL-shaped overlay of `paging`, validation that refuses a native
pattern in a standard statement and an overlay with the wrong slots. Cost: every catalog is
built at the root and handed down, which is the point; a domain cannot register a pattern,
which is the ontology's rule (patterns are protocol, the application's).

Nothing changes in the mechanism. The layout question is where `Source`, `Catalog`, and
`Pattern` live once `query` is a `sqlate` package; §3 keeps them in `query`, since the
catalog is what statements compile against and nothing else uses it.

### 2.3 Statements

`Compile` against the catalog and the dialect; `Statements` as a domain's inventory;
`Verify` at startup. Proven: a renamed column fails startup at lifecycle stage 2 with the
statement named; includes resolve from two namespaces; a native pattern spliced into a
standard statement is refused at compile. Cost: none found. The inventory is per domain and
not yet walkable across domains, which §3.5 resolves with a registry on `data.Database`.

### 2.4 Handles

`Statement.Exec`, `Rows[T]` with `One`/`All`/`Each`, `Projection[T]` with `List`/`One`/
`Verify`, `Guard.Run`, and the mapper (`Scanner[T]`, `ArgsOf`, `Args.With`). Proven: three
consumers, `database.go` at 104 to 120 lines per domain with wiring and operations only.
`Rows.All` (now used by the batch fetch), `Rows.Each`, and the operators beyond `eq` and
`in` stay in the vocabulary whether or not a consumer has reached them yet: the sufficiency
rule is about mechanisms, and these are the vocabulary the SDK's parser grows into (the
request-syntax decision is go-web-sdk's, §3.3).

This is the layer Go 1.27 changed, and stage 14 made the change. The constructors were
package functions because Go had no generic methods, and the plan's decision 3 said so.
They are now methods, and a handle reads as what it is: a statement bound to a scan.

```go
// before                                                  // now
query.Scan(stmts.Statement("create"), query.Scanner[Identity]())   stmts.Statement("create").Scan(query.Scanner[Identity]())
query.Project(stmts.Statement("person_view"), query.Scanner[Person]())  stmts.Statement("person_view").Project(query.Scanner[Person]())
query.Guarded(stmts.Statement("edit"), check, "version")   stmts.Statement("edit").Guarded(check, "version")
sqldb.Transact(ctx, db, func(tx *sqldb.Tx) (T, error) {…})  db.Transact(ctx, func(tx *sqldb.Tx) (T, error) {…})
```

`Scanner[T]()` and `Scalar[T]` stay functions: they take no receiver. `Guarded` needed no
type parameter and moved with the others so every handle is constructed the same way. The
`T` of `Scan` and `Project` is inferred from the scan function. Both domains, the seeder,
and every test are the consumers; `database.go` reads as a table of handles.

### 2.5 Execution

Directives to a signature to a composed statement for the collection read; arguments bound
by name; execution through a session; errors mapped through the dialect at the runner
boundary; `ErrorMapper` for what the seam cannot see. Proven: filter values parsed by the
engine with class 22 as the request's 400, all three guard outcomes, the transaction
requirement checked at bind. Cost: the collection read composes text on every request. The
text is deterministic per signature and the driver caches prepared statements by text, so
the cost is string work, not round trips, and the verdict (the architect, stage 13) is that
the string work is well worth the dynamic SQL it buys. The signature cache is dropped.

### 2.6 The service: data, domains, admin, the root

- **`internal/data`** (290 lines): the session-and-catalog grouping, the schema, the
  application's patterns, the seeds behind a `Seeder`, and now the statements registry
  (stage 14): each domain and the seeder register their compiled inventory at wiring, and
  `GET /admin/database/statements` walks it beside `/patterns`. Settled at the stage 12
  review as the service's whole database infrastructure, with domains and the admin
  service as peers over it. It is the type the template scaffolds and the service grows: a
  second database, a read replica.
- **The domains** (541 and 559 lines each, tests included in neither): `entities.go` with
  `Validate` methods and the tag conventions, `database.go` as the SQL client, `service.go`
  and `handler.go` from the reference. Two findings the reference service does not yet have:
  a command's validation is the entity's, and existence and uniqueness are the store's as
  constraint violations; an action is a named transition with its own protocol, never an
  `edit`. Both are domain-architecture principles (§5).
- **`admin/database`** (508 lines): after the consolidation, operations and policy only.
  Its `Service` is generic over a migrator, a seeder, a catalog, and the pool; §3.2 places
  it.
- **The composition root** (298 lines in eight files): one file per layer, each
  internalizing its mount, `routes.go` at two lines. Evaluated against a single
  `services.go` in §3.4.
- **`internal/sdk`** (76 lines): `IfMatch`/`PreconditionError` bound for go-web-sdk;
  `Directives(web.Query)` the lowering onto the query vocabulary, which is service-side by
  dependency direction (§3.3).
- **`cmd/sqlint`** (700 lines): configured by `sqlint.toml`, the whole linter in `main.go`
  and `config.go`; decomposed at promotion (§3.1). Its stripper empties single-quoted
  literals and double-quoted identifiers and drops line and block comments before a native
  form is matched. An escape pragma was considered and culled: a suppression is a patch
  over the parser, and a false positive is fixed in the stripper.

### 2.7 The tests

Sixteen packages, every one hermetic under `drivertest`'s strict scripted driver, and the
compose suite (`//go:build compose`) as the engine-only tier of the testing hierarchy: the
migrator's acceptance proofs, engine-side value parsing, the live catalog. `drivertest`
records arguments unconverted and fails on an unscripted call, a mismatched argument count,
or a response that does not fit its call; it found two real defects during the stages. Its
home is §3.1.

## 3. The layout review

Which types, methods, and functions belong to which project, judged against the standard's
package layering: downward dependencies, one concern per package, a library that imports
nothing of ours below it. The stage 9 review set the direction (`sqlate` below go-database,
engines as sub-modules, the seam a plain `*sql.DB` and a dialect); stage 12 proved it
builds. This section finishes the placement.

### 3.1 sqlate

**Naming (the architect, stage 13).** The library is `sqlate`, at
`github.com/standards-lab/sqlate`, a portmanteau of SQL and template: it makes plain `.sql`
files dynamic and compositional through templating rather than abandoning them. Read aloud,
the name is also "escalate": the library escalates what a `.sql` file is capable of. The
motivating problem is that web services traditionally avoid real SQL; queries are either
embedded as string literals in the host language or generated by an ORM, and both make the
actual query invisible and cut it off from SQL's own tooling (highlighting, linting, LSPs,
direct `EXPLAIN ANALYZE`). `sqlate` keeps queries in first-class `.sql` files and addresses
their one legitimate weakness, that they are static and repetitive. It deliberately drops
the organization's `go-` prefix: `go-database` and `go-web-service` are interdependent
layers of an architecture, while `sqlate` is a standalone, independently adoptable library.

**What it is a blueprint for.** SQL is the first DSL in the architecture to need this degree
of host-language support, because SQL was not designed for the composition and
portability the architecture requires. Not every DSL will: one designed to compose on its
own operates independently of any host-language manipulation. For the ones that do need it,
`sqlate` is the pattern: the grammar the files carry, the host library that compiles and
composes them, the engine sub-modules that own an engine's spellings, and the lint that
holds the conventions.

Its own repository, in the workspace order beside go-core, since it imports the standard
library alone. Package by package, from the spike's `lib/`:

| Package | From | Holds |
|---|---|---|
| `sqlate`, the root | `lib/sqldb` | `Session`, `DB`, `Wrap`, `Tx`, `Begin`, `Transact`, `TxOption`, `Dialect`, `Locker`, `ErrorMapper`, and the error taxonomy: `ErrConnectionFailed`, `ErrInvalidValue`, and the constraint classes with `ConstraintError`, moved in from go-database because the engine sub-modules produce them |
| `query` | `lib/query` | the catalog (`Source`, `Publish`, `Patterns`, `Catalog`, `Pattern`), `Compile`, `Statements`, `Statement` with its handle methods, list expansion, the mapper, `Directives`, `Verify`, `ErrDirectives` and its errors, `ErrVersionMismatch` |
| `migrate` | `lib/migrate` | unchanged: `Migration`, `Files`, `Migrator`, `Options`, `Catalog`, the error types |
| `internal/header` | `lib/sqlheader` | the declaration grammar, shared by `query`, `migrate`, and `sqlint`; internal because no consumer parses headers |
| `sqltest` | `lib/drivertest` | the scripted driver, `Open`, `Recorder`, `Response`, the stub `Dialect`; public because consumers' domain tests use it (the testing hierarchy's prepare-capable fake) |
| `sqlint` | `cmd/sqlint` | `Config`, `Load`, a `Resolver` over `go list`, `Lint(fs, cfg, resolver) []Finding` with `Finding{Path, Line, Message}`, one file per role's checks, the glob matcher; `cmd/sqlint` reduces to arguments, root, output, exit code, so the harness calls the package |
| `postgres` (sub-module) | `lib/pgdialect` | the dialect entire: `Name`, `Placeholder`, `MapError` with the constraint classes and class 22, `Locker` over advisory locks, migrate's `Catalog`; its `sqlint.toml` with the native forms; live tests importing the driver directly. It ships no overlay: PostgreSQL is the standard spelling |

Two placements that were open are now closed by the rehearsal. The dialect interface is
sqlate's; go-database's postgres dialect satisfied it structurally, which shows the
interface was always the library's. The error classes follow `MapError`: whoever classifies
owns the sentinels.

### 3.2 go-database

Version 0.4.0, breaking. What it keeps is the infrastructure service: `Config` with the
pool settings and the environment layer, `New(pool, cfg)` over the provider-constructed
`*sql.DB`, `Start`, `Shutdown`, `Ready`, `Ping`, `ErrNotReady`. What it drops: `ast`,
`operation`, `exec`, `seed` as the strategy planned, and also `Session`, `Tx`, `ExecTx`,
`Dialect`, and the constraint classes, all of which sqlate now owns. The `postgres`
sub-module keeps the driver, the DSN, and pool construction, and no longer supplies a
dialect; the composition root takes the dialect from `sqlate/postgres`.

What it gains is the admin service, resolving the stage 9 question of go-database versus
go-web-sdk. The service half of `admin/database` is generic over a migrator, a seeder's
verify and run, a catalog, and the pool, and depends on sqlate, go-core's lifecycle, and
go-database's own `DB`. go-web-sdk cannot host it without importing go-database, an upward
dependency for a web SDK; go-database can, because consuming sqlate is the new direction the
split establishes. So `go-database/admin` holds `Service`, `Options`, `Register`, `Start`,
`Ready`, `Status`, the verbs, `Seed` with its policy, `Diagnose`, `Catalog`, and the entity
types those return. The HTTP half, the route group and handler, is go-web-sdk-shaped and
small; it stays application code that the template scaffolds, as does the choice to mount it
under `/admin`. The stage 1 rule holds: an admin service's operations are triggers over
library functions.

### 3.3 go-web-sdk

Three items, none of them SQL:

- `IfMatch` and `PreconditionError`, staged in `internal/sdk` since stage 8, promote as
  they are.
- `ErrorWriter` gains an option to carry the error text on chosen statuses
  (`web.WithDetail(statuses ...int)`), the stage 1 finding; the admin handler's local
  `reject` and the domains' matchers then retire it.
- The strict body decode (unknown fields rejected, size bounded, a validation rejection with
  the reason) and the `respond` shape repeat in every handler and are go-web-sdk plumbing.

One decision, not an item: whether the query parser gains an operator syntax. Every operator
but `eq` and `OpIn` are unused after three consumers because `web.Query` yields exact
matches only. The `Directives(web.Query)` lowering is service-side by dependency direction
(go-web-sdk must not know sqlate), the template scaffolds it, and it grows when the parser
does.

### 3.4 go-web-sdk-template

The template scaffolds what the experiment found every service needs, and this is where the
five incubated principles land as code:

- **`internal/data`**: `Database` and `New`; `migrations/`, `seeds/`, `patterns/`,
  `statements/` with `Migrations()`, `Patterns()`, and a `Seeder` skeleton; the `app`
  namespace registered at the root.
- **`admin/database`**: the handler and route group over `go-database/admin`'s service, the
  `/admin` mount, the `admin.seed` config block with `APP_ADMIN_SEED` off by default.
- **The composition root as files, not packages.** `internal/infrastructure`,
  `internal/domain`, and `internal/reactors` collapse into `internal/app/infrastructure.go`,
  `domain.go`, `admin.go`, `reactors.go`, each constructing its layer and internalizing its
  mount, with `routes.go` the list of mounts. The stage 1 review found the packages had one
  consumer each and that `reactors.New` taking the `Domain` type forced the collapse.
- **On a single `services.go`.** Evaluated and not recommended. The four files total 160
  lines; one file would hold four types and four constructors in one place, and the order
  of construction would still be `app.go`'s. The file-per-layer form makes the root's table
  of contents the architecture's layer list, which is the property worth keeping: a reader
  opening `internal/app` sees infrastructure, admin, domain, reactors as files. If a layer's
  file ever grows past a screen, that argues for keeping it separate, not merging it.
- **`routes.go`**: already the two-line list; each layer's file owns its mount function.
  The simplification is that it composes and does nothing else.
- **`internal/sdk`** seeded with `Directives(web.Query)`; **`sqlint.toml`** with the roles
  and sources; **`mise.toml`** with `lint` running `sqlint`, `test-compose`, and `db-up`.

The entity roles (validation methods on commands, `db` then `json` tags as the scan and
binding contract) are conventions the template documents rather than scaffolds, since it
ships no domain.

### 3.5 go-web-service

Adopts the whole shape: the organization domain rewritten onto sqlate (the spike's is the
draft), `internal/data` with the registry, the admin layer, the composition root as files,
`cmd/db` and golang-migrate deleted, the integration tier over the compose stack. The
spike's person domain does not promote: it was simplified to keep the experiment small, and
the people domain gets its own planning under `v1.data.people`. One addition the review
recommends the service build rather than the spike:

- **Soft delete** as the standard's convention, from the stage 8 review: `delete` moves a
  record to the recycle bin, `restore` returns it, `purge` removes it physically and is
  administrative; a `deleted_at` column, the base excluding it with a recycle view, partial
  unique indexes over live rows, and the delete pattern as an update, a pattern pair for the
  library's catalog once a domain needs it.

### 3.6 Cross-cutting: what waits, and why

- **Seed helpers.** The per-table loop with insert counting and code-to-id resolution is
  70 lines for two domains; a generic `seed.Table[T]` waits for the template's second
  service under the sufficiency rule.
- **The signature cache.** Considered and judged unnecessary (§2.5); recorded so it is not
  re-proposed without a measurement.
- **The lint's stripper.** With regular expressions over code, tokenizing is no remedy for
  anything; what the stripper does not yet strip is PostgreSQL's dollar quoting, added when
  a false positive appears.
- **The management listener's isolation.** The admin mount's own listener, authentication,
  and audit are the strategy's production constraint (A.3, A.5) and stay the
  `v1.data.sql.listener` task.

## 4. The promotion plan

The order is the dependency order. Each step is one session unless marked.

1. **Create `sqlate`** from `lib/` and `cmd/sqlint` per §3.1, `sqlate/postgres` from
   `pgdialect` with the constraint mapping folded in from go-database's dialect, the live
   tests over the driver. Insert `sqlate` into the workspace order beside go-core. Release
   v0.1.0.
2. **go-database v0.4.0** per §3.2: the removals, `admin` over sqlate, `postgres` without a
   dialect. Releases together with step 3's service change, as the goal's criterion says.
3. **go-web-sdk** per §3.3, v0.6.0.
4. **go-web-sdk-template** per §3.4.
5. **go-web-service**: the coordinated rewrite of §3.5, `cmd/db` and golang-migrate gone,
   the organization domain end to end, the release. Then the people domain from the spike's
   person domain under `v1.data.people`.
6. **The listener**, **the docs pass**, **the harness follow-through**, and **the integration
   tier** as the existing tasks, re-read against the split.

The spike stays under `experiments/sql-dsl` as the record; nothing in stable context cites
it after close, and the concepts written at close carry what promotion reads.

## 5. Workspace context adjustments

What the experiment changes in each repository's context, seeded by the principles it
incubated. `close` applies the coordinator's and records the rest as **Cross-repo** or as
the integration tasks' first act; the docs landing zone pages are flagged for the docs pass.

**standards-lab** (the coordinator)

- `design/dsl-driven-services.md`: §3 gains PRQL, considered and ruled out (analytical-only,
  no Go binding, a second language above SQL); §4.1's planned shape becomes the sqlate and
  go-database split; §4.2's `Source` is `Statements`, `?` placeholders are `{{name}}`, the
  pattern catalog is sourced and namespaced; §6's table becomes the catalog as built with
  the include-versus-compose distinction; §7's artifact table gains sqlate; §8's sequence is
  §4 above.
- `context/README.md`'s capability map and `.claude/marathon.toml`'s `[workspace] order`
  gain sqlate, beside go-core.
- `context/concepts/sqlate.md`, new at close: the library's layout and API from §3.1, the
  naming rationale, and the Ontology, the concept the repository-creation task reads. It
  moves into sqlate's own context once the repository exists.
- `roadmap.toml` per §6.

**go-database**

- `concepts/sql-architecture.md`: the Home line, decision 2 (superseded by `{{name}}`),
  question 1's verdict, and a pointer to this review; the integration task culls it once
  sqlate exists.
- `context/README.md`'s capability map and `design/layers.md`: rewritten by the v0.4.0
  task around the infrastructure service and the admin package; the superseded banner
  already says so.

**go-web-service**

- `design/domain-architecture.md`: `entities.go`'s roles gain validation methods and the
  tag conventions; `database.go` is the SQL client over sqlate's handles with the naming
  the spike settled (the command's name is the file, the store method, the service method,
  and the route); the action as a named transition; the admin layer as a new section; the
  open tension on multi-entity layers closes by citing the amended definition.
- `design/composition-root.md`: the layout is files under `internal/app`, `internal/data` is
  the database's home, `cmd/db` retires into the admin mount.

**go-web-sdk**

- `concepts/error-handling.md`: the detail option; a concept for the precondition parse and
  the strict decode.

**go-web-sdk-template**

- `context/README.md`'s capability map: four layers as files, `internal/data`,
  `admin/database`, `sqlint.toml`; `concepts/scaffolding-cli.md` unchanged.

**docs** (flagged for the docs pass)

- `concepts/dsl-docs-pass.md`'s inventory gains the sqlate pages and loses the "query and
  migrate in go-database" framing; the content-patterns reference (status filters, search,
  hierarchy CTEs as the domain's, never the library's) is a page in the principle's section.
- `concepts/sql-meta-language.md`: this experiment is the meta language's first phase (the
  architect, stage 13). The grammar is language-independent — `--|` declarations, `{{ }}`
  parameters, slots, and expansion, `{{> ns.name}}` includes, namespaces, tiers, overlays,
  `sqlint.toml` with `[export]` — and sqlate is its first host, in Go; another host
  implements the grammar, not the Go API, so the grammar is recorded as the standard's own
  artifact in the docs tier, separate from the library's pages. What the concept still holds
  beyond this phase is schema typing (types from the migration set rather than declared) and
  portability by construction (compiled to a dialect rather than declared by tier);
  `backlog.sql-meta-language` narrows to those two.
- The architecture definition: a Domain Service anchors a **domain**, a composition of one
  or more Entities, the Entity a primitive; "anchors exactly one Entity" was the
  single-entity special case (the architect, stage 13). It resolves go-web-service's
  multi-entity tension and leaves the rejection of "feature" as an element standing, since a
  feature is not a composition of entities either. It is an element definition and lands at
  the coordinator's level, in the landing zone.

**claude-plugins**

- The hardening task: the checkable rules are `sqlint` as a package the harness calls, not
  rules the harness re-implements; the two marathon backlog items the experiment found stand
  as recorded.

## 6. The roadmap refinement

Drafted for `close` to apply through the roadmap extension. The finished task `prototype`
is deleted at close. The pre-split tasks `query`, `migrate`, `organization`, and `startup`
are superseded and replaced by an `integration` goal under `v1.data.sql`; `docs`,
`hardening`, and `suite` stay where they are with their summaries re-read against the split.

```toml
[goals.v1.data.sql.integration]
name = "The DSL service integration"
summary = '''
The prototype's shape adopted across the workspace in dependency order: sqlate created
from the spike's lib/ and sqlint, go-database reduced to the infrastructure service and
given the admin package, go-web-sdk's three items, the template scaffolding the data and
admin layers and the composition root as files, and the reference service rewritten onto
sqlate with cmd/db and golang-migrate gone. The review is the linked record.
'''
criteria = [
  "sqlate v0.1.0 with its postgres sub-module released; go-database v0.4.0 and the service change release together; the organization domain runs on authored SQL end to end.",
  "Every service-side principle the experiment incubated is scaffolded by the template or documented in the domain architecture.",
]
context = ["standards-lab/experiments/sql-dsl/REVIEW.md", "standards-lab/context/concepts/sqlate.md"]

[goals.v1.data.sql.integration.tasks.sqlate]
name = "Create sqlate"
repos = ["standards-lab"]
summary = "The repository and module from lib/ and cmd/sqlint per the review's layout: the seam and taxonomy at the root, query, migrate, sqltest, sqlint, the postgres sub-module with the folded dialect; the workspace order gains the layer beside go-core."
proof = "sqlate v0.1.0 and sqlate/postgres tagged; the spike's suites green against the module; sqlint runs as a package."

[goals.v1.data.sql.integration.tasks.database]
name = "go-database v0.4.0"
repos = ["go-database"]
summary = "ast, operation, exec, seed, Session, Tx, Dialect, and the constraint classes removed; Config, the pool, lifecycle, and readiness kept; the admin package over sqlate; postgres without a dialect; layers.md and the capability map rewritten."
proof = "v0.4.0 tagged with the service change; the admin service drives verify, apply, and seed at startup in the reference service."

[goals.v1.data.sql.integration.tasks.websdk]
name = "go-web-sdk's three items"
repos = ["go-web-sdk"]
summary = "IfMatch and PreconditionError promoted; ErrorWriter's detail option; the strict decode and respond plumbing. The operator-syntax decision for the query parser is taken, either way."

[goals.v1.data.sql.integration.tasks.template]
name = "The template scaffolds the shape"
repos = ["go-web-sdk-template"]
summary = "internal/data with its directories and Seeder skeleton, admin/database over go-database/admin, the composition root as files with routes.go the list of mounts, internal/sdk seeded with the directives lowering, sqlint.toml, the mise tasks; the entity conventions documented."

[goals.v1.data.sql.integration.tasks.service]
name = "The reference service on sqlate"
repos = ["go-web-service", "go-database", "go-web-sdk"]
summary = "The coordinated rewrite: the organization domain on authored SQL as the spike built it, internal/data with the statements registry, the admin layer, the composition root as files, cmd/db and golang-migrate deleted; the release with go-database v0.4.0."
context = ["go-web-service/context/concepts/retrospective-findings.md"]

[goals.v1.data.sql.integration.tasks.listener]
name = "The management listener"
repos = ["go-web-service", "go-web-sdk", "go-web-sdk-template"]
summary = "The startup task's surviving half: the admin mount on its own listener, disabled unless configured, auth gating the mutating half, the posture decisions on record (strategy A.3, A.5); go-web-sdk's config env segment made per-block."
```

`next` after close: the two marathon backlog items already queued, then
`v1.data.sql.integration.sqlate`.

## 7. Open, and deliberately so

- Where in the landing zone the grammar's page sits and what it is called, the docs pass's
  call.
- Dollar quoting in the lint's stripper, and any further blind spot, each fixed in the
  stripper when a false positive appears.
- Whether the query parser gains an operator syntax, go-web-sdk's decision under
  `v1.data.sql.integration.websdk`.
