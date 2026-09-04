# SQL as a DSL-driven service

Strategy for go-database v0.4 and go-web-service. Settled 2026-08-29 across two sessions
reviewing go-database v0.3.0 (`ast` / `operation` / `exec`) against the organization domain in
go-web-service, and adopted — with the adjustments in §10 — at the 2026-08-31 retrospective.
This is a strategy record: it captures the principles, the reasoning that produced them, and
the shape of the result. Implementation detail is deliberately left to the sessions that
execute it (§9); `goals.v1.data.sql` in the roadmap carries the task breakdown.

## 1. The ambition

The architecture exists to establish layers of tooling that let an engineer build precise,
capable infrastructure at an optimal level of expressiveness and velocity, with a minimal
number of layers and minimal overhead. Each layer must find the structure that is most
effective not only for using its features but for *expressing an implementation against* them.
It must stay portable and configurable, so that a change in the surrounding environment has
minimal impact on what has been built. And it must minimize exposure to external
vulnerabilities — technical-debt burden and supply-chain risk alike.

The SQL layer is where this ambition was tested hardest, because SQL is the one infrastructure
integration that carries its own language. The v0.3.0 answer — a Go statement vocabulary that
could express any query — was capable, well-built, and wrong for the ambition: it made every
statement longer and stranger than the SQL it rendered, and it grew the library to carry
expressive content that the language already carried. Working out why produced the principles
below, which apply beyond SQL.

## 2. Principles

### 2.1 DSL-driven services are a distinct category

A **protocol-driven** infrastructure service (auth, blob storage, messaging) has its expressive
content in the host language: the consumer calls operations with typed arguments, and the
provider seam is an interface over those operations. This is the pattern the standard already
establishes for infrastructure services, and it holds for them.

A **DSL-driven** service has its expressive content in a foreign language the host cannot
type-check: SQL, but equally Cypher or Gremlin, a search engine's query DSL, KQL, PromQL,
Rego or Cedar for policy. For these the operations are trivial (execute, query) and all the
meaning is in the text. Fitting them to the protocol-driven template produces exactly the
failure v0.3.0 exhibited: a large host-language interface standing in for a language that was
already the right interface.

The strategy for a DSL-driven service is therefore different in kind. The DSL text is the
primary artifact — authored natively, versioned in the repository, reviewed as itself. The
host-language layer exists only for what the DSL cannot do on its own:

1. bind parameters safely;
2. compose against runtime input through a declared vocabulary;
3. map results back into host types;
4. verify the text against its target;
5. own the session or transaction boundary.

SQL is the reference implementation of the category. The shape — native text, a thin mechanism,
a verification step, a tier declaration — is what a later search or policy integration should
reproduce with no shared code.

### 2.2 SQL is the DSL; Go is the mechanism

The library's job is to be the thinnest seam that does only what SQL cannot express. Every
pattern the domain layer needs — a collection with dynamic filters, sort, and paging; a single
record by unique constraint; a mutation with transactional side effects across tables — is a
SQL pattern with a small amount of mechanism around it. The library supplies the mechanism and
stays out of the expressive content entirely.

A corollary for extensibility: a new pattern is a **convention**, with two halves. The first is
a SQL shape — a file layout, a naming rule, a header — that a reviewer or a skill can check. The
second, only where needed, is a mechanism function that takes callbacks or statements the
consumer authored. Most new patterns need no new mechanism (upsert, soft delete, batch insert
are SQL shapes over the existing runners). A pattern earns a mechanism function only when it
carries a protocol the SQL alone cannot guarantee; the optimistic-concurrency guard is the
worked example. Mechanisms take the consumer's SQL as input; they never generate it.

### 2.3 The portable artifact is the language, not the library

Every protocol-driven service has one portability axis: the library. SQL has two: the library
and the dialect. A runtime query builder promises to solve the dialect axis and in exchange
worsens the library axis — a permanent dependency with its own release cadence and CVE surface,
which no other infrastructure service carries.

This strategy scores well on both axes by making the *language* the portable artifact. Authored
SQL ports by editing SQL. The library axis collapses to `database/sql` and one driver — the same
shape auth or storage would have. The dialect axis is handled by discipline: a per-file tier
declaration (§5.4) and a verification step, rather than by a dependency or by types.

That is a deliberate trade and must be recorded as one. v0.3.0 enforced portability by
construction (a typed `UnsupportedFeatureError` at render time). This strategy enforces it by
review and lint. The declared stack selects one engine; the portability promise exists to bound
a future port, not to make today's code engine-neutral. Given that, discipline is the right side
of the trade — and it should stay written down so no later session tries to buy the type
guarantee back with a builder.

### 2.4 Sufficiency, not only capability

The v0.3.0 failure was not a missing capability. It was an extra layer that was fully capable.
The ambition's "minimal number of layers" therefore needs an active rule, applied *before* a
layer is built: a layer must justify itself against what the language, the standard library,
and the tools already inside the dependency line already express. The question "does the
industry already solve this inside the line?" is asked structurally at plan time, not
discovered at review. This is a rule the marathon harness can enforce at `plan`, and the SQL
layer is its first worked case.

### 2.5 Injection safety is structural

The vulnerability in SQL integration is any path where request input becomes SQL text. The
mechanism's job is to make that path not exist:

- Statement text is build-time only: string literals and `//go:embed` files.
- Request values enter through bound arguments exclusively.
- The only runtime-shaped *text* — sort and filter fields — passes through a declared field
  contract that acts as a whitelist; unknown names are typed errors, never interpolation.

Go cannot enforce "constant only" at the type level, so the first rule is a convention a skill
checks: no statement constructor whose text argument is not a literal or an embedded variable.

## 3. What was considered and why it lost

Recorded so the reasoning does not have to be reconstructed.

**ORMs (gorm, bun, ent).** Outside the standard's dependency line by definition; frameworks,
not libraries, and the expressive content moves into their vocabulary.

**Runtime builders (goqu, squirrel, bob, jet).** Each is a framework-sized runtime dependency
with its own dialect model. Squirrel is in maintenance mode; goqu's breadth is its own liability;
bob and jet generate against a live schema and still keep the builder at runtime. None has the
standard-versus-native tier split. Adopting one replaces the `ast` layer with something larger,
less aligned, and permanently on the dependency graph — the wrong side of §2.3.

**SQL-to-Go generators (sqlc).** The strongest alternative: a build-time generator with no
runtime footprint, native SQL files, and schema verification at generate time. It lost on the
DSL principle: it does not generalize (there is no sqlc for Cypher or a search DSL, so it would
set no pattern for the category); it is an engine lock at the tooling level (PostgreSQL, MySQL,
SQLite — no SQL Server or Oracle, which matters when future ports are unknown); it is a
dev-time dependency with a C parser behind it; and it forces two authoring conventions, one for
generated queries and one for the runtime mechanism. Its one irreplaceable contribution —
schema verification — is provided by the mechanism itself (§4.2, Verify).

**Keeping the `ast` vocabulary.** ~6k lines, 89% covered, principle-aligned on portability, and
a good answer to a protocol-driven framing of a DSL-driven problem. Its statement vocabulary
retires; its predicate and directive machinery survives inside the mechanism (§4.2) — the
retrospective narrowed the survivor list; see §10.

**Runtime templating over `.sql` files (`text/template` or similar).** Considered at the
retrospective for dynamic composition and a shared shape catalog. Rejected: it reopens the
§2.5 injection path as discipline instead of structure (text interpolation into SQL becomes a
first-class operation with no SQL-aware escaping), it breaks the properties native files were
chosen for (editor and lint support, an enumerable statement inventory, prepare-based Verify
over final text), and it does not shrink the mechanism — argument ordering across stitched
fragments still needs Go. The SQL meta-language concept had already considered and set aside a
`text/template` layer (2026-08-28: fragment references stay SQL-legal). The catalog instinct
is served instead by the pattern catalog (§6) and, if text-level reuse proves out across
domains, by build-time fragment composition — stitching includes into complete, plain-SQL
statements before embed — which is recognizably phase one of the meta-language concept. The
`v1.data.sql.plan` session carries the investigation.

**PRQL.** Considered at the prototype (2026-09-02) and ruled out for this layer: analytical-only
by its own statement, no Go binding, and a second language above SQL. A reference point for
the meta-language concept, not a mechanism.

## 4. go-database

### 4.1 Shape (settled by the prototype, 2026-09-03)

Two modules, and the mechanism is a standalone library below the infrastructure service.
`sqlate` (`github.com/standards-lab/sqlate`; the concept is `concepts/sqlate.md`) owns
everything from the file to the row and imports the standard library alone:

```
sqlate           — the seam: Session, DB over a plain *sql.DB, Tx, Transact, Dialect, Locker,
                   ErrorMapper, and the error taxonomy the engine sub-modules produce
sqlate/query     — the mechanism: the pattern catalog, compile, statements, the handles, the
                   field contract, directives, the guard, the mapper, verify
sqlate/migrate   — schema versioning over embedded SQL (replaces golang-migrate in consumers)
sqlate/sqltest   — the scripted driver every consumer's hermetic tests run over
sqlate/sqlint    — the conventions lint as a package; cmd/sqlint is its entrypoint
sqlate/postgres  — the engine sub-module: placeholders, error classes, the lock, migrate's
                   catalog, its sqlint.toml
```

go-database v0.4.0 is the infrastructure service: `Config`, the pool over the provider,
lifecycle and readiness, and `admin`, the database admin service over sqlate that a
service's admin mount triggers. `ast`, `operation`, `exec`, `seed`, `Session`, `Tx`,
`Dialect`, and the constraint classes leave it; `postgres` keeps the driver and pool
construction and supplies no dialect. A breaking release. The original single-module plan
(`query` and `migrate` inside go-database) is superseded: the prototype's split rehearsal
showed the mechanism needs nothing of the service, so a CLI, a worker, or a test harness
uses authored SQL with no lifecycle machinery.

### 4.2 The `query` mechanism

Described at the level of responsibilities; the exact signatures belong to the session that
builds them.

**Statements.** The rendered unit is text plus ordered arguments. Text comes from authored
files; a parameter is written `{{name}}`, or `{{name:type}}` to cast through the engine's own
type, or `{{ids...}}` to expand a list into one placeholder per element, resolved to the
dialect's positions once at compile (the `?` and `:name` grammars this section and §11 stated
are superseded). A file's `--|` declarations state its tier, its native reach, whether it
needs a transaction, and a base's key and field contract; the engine receives the body alone.
`Statements` compiled from an `embed.FS` against the pattern catalog give a domain its
statements by name and are the inventory the verification step and the admin mount walk
through the service's statements registry.

**The field contract.** A projection is an authored base statement plus a map of contract names
to columns of that base. It is the whitelist of §2.5 and the vocabulary that request directives
(filter, sort, page) are lowered against. `Directives` and its typed `UnknownFieldError` /
`UnknownOperatorError` carry over from v0.3.0 unchanged; they were the part that was already
right.

**Composition.** The collection pattern renders `SELECT … FROM (<base>) q WHERE <filters>
ORDER BY <sorts>, <key> OFFSET/FETCH` and its count twin `SELECT COUNT(*) FROM (<base>) q WHERE
<filters>`. The derived-table wrap is standard SQL and is what lets the base be *any* authored
query — a recursive CTE, a join tree, a view — without the mechanism knowing its shape. Beyond
the projection, composition is limited to appending authored fragments with their own arguments;
the mechanism concatenates only text it was handed at build time.

**Runners.** Generic over a scan function: list (page and count), one, rows, scalar, exec. All
take a `Session`; none take a dialect. The consumer never renders. Errors are mapped through the
dialect at the runner boundary.

**The guard.** The optimistic-concurrency protocol as a function over two consumer-authored
statements: run the command; on zero rows affected, run the version check; distinguish
not-found from version mismatch; return the deterministic new version without a second round
trip. The SQL — SET list, `WHERE key = ? AND version = ?`, `version = version + 1` — is the
consumer's. `Guard.Column` disappears because the consumer names the column in its own
statement, which is baseline-standard ownership stated directly.

**Verify.** Every statement in a `Source` is prepared against a live, migrated schema. Engines
validate column and table references at prepare time, so a column renamed by a migration or a
typo in a file fails at startup and in tests rather than at first request, for any engine. It
runs at service startup (§5.2) and on demand from the management surface (§5.3).

**Row mapping.** One scan function per row shape, written by the consumer. If that boilerplate
grows across domains, a struct-tag column mapper of roughly 150 stdlib-only lines is the
ceiling — well short of an ORM, and a session decision (§9).

### 4.3 The `migrate` mechanism

Under §2.2 a migration is SQL text plus a thin mechanism: a version table, an ordered set of
embedded `.up.sql` / `.down.sql` files, each applied in a transaction under an advisory lock,
version recorded. Roughly 150 lines. It replaces golang-migrate, which in go-web-service was a
runtime dependency carrying thirty drivers for the sake of one — the weakest link in the module
graph under the dependency line, and present only because the CLI used it.

The mechanism designs explicitly for the cases a mature library has already met: dirty state
after a failed migration (recorded, reported, cleared only by an explicit force), engines
without transactional DDL (each statement recorded individually), and concurrent starters (the
lock). These are the session's acceptance criteria, not discoveries.

## 5. go-web-service

### 5.1 The organization domain

```
domain/organization/
  doc.go
  entities.go
  database.go      # source, projection, directives, scan, operations — the only file that touches query
  service.go
  handler.go
  handler_test.go
  sql/
    organization_view.sql    # the read model: columns + computed path via recursive CTE
    in_subtree.sql
    lock_tree.sql
    insert.sql
    edit.sql                 # guarded
    transfer.sql             # guarded
    delete.sql               # guarded
    version.sql
```

Every statement is SQL, in a file an editor can check, with a tier header. The recursive lineage
CTE that v0.3.0 expressed as `RecursivePath` — with `||` built by Go string concatenation — is
nine lines of SQL with `||` written as `||`. `database.go` holds the embedded `Source`, the
projection over `organization_view.sql` with its field map, the directive lowering, one scan
function, and operation functions that are each a transaction, one or more named statements,
and where relevant the guard. The consumer's `database.go` drops from ~300 lines to roughly
half, of which the projection, directives, and scan are about forty; the rest is operations
that read as what they do.

### 5.2 The composition root owns the mechanics

`cmd/db` is removed. Schema versioning, verification, and seeding become library mechanisms
exposed by the service through triggers. Three startup concerns, kept separate because their
risk profiles differ:

- **Verify** — always on, cheap, safe. The migration version matches the embedded set and every
  statement prepares against the live schema. Failing fast here is pure upside.
- **Apply** — a decision, behind config (`schema = "verify" | "apply"`). Concurrent replicas
  need the advisory lock; some migrations need the old code stopped first; and in regulated
  postures a process that serves traffic holding DDL privileges is a least-privilege question.
  The expected production shape is a one-shot invocation of the same binary under a migration
  role (`server --migrate up`, exit), with steady-state replicas on verify. That flag is the
  surviving fragment of the CLI: a mode of the service binary sharing its composition root, not
  a second command.
- **Seed** — reference data production needs is a migration. Seeds are dev and test tooling,
  disabled by config outside those environments.

### 5.3 The management surface

A separate listener — its own port or socket, authenticated, unreachable from the public API's
network path, with audit logging on anything that mutates. That isolation is a design
constraint, not a deployment detail. This surface absorbs the maintenance-module concept
(captured 2026-08-28, culled into `v1.data.sql.startup` at the retrospective); its constraints
carry over — separate listener, disabled unless configured, auth gates the mutating half, and
config rendering waits on a redaction contract in go-core. The initial set:

- the former CLI verbs: version, up, down, steps, force, seed;
- verification on demand;
- the statement inventory from every domain's `Source`, with tier headers — which is also the
  port work list;
- diagnostics the CLI never had: pool statistics (`sql.DBStats`), ping latency, server version
  and dialect, and native-tier views such as active and long-running sessions.

`down` and `force` are destructive and require an explicit confirmation token. Every endpoint
calls the same go-database function startup calls: a trigger over a mechanism, which is the
shape §2.1 prescribes for the whole layer.

### 5.4 The tier declaration

Each `.sql` file carries a header:

```sql
-- tier: standard
```
or
```sql
-- tier: native
-- native: postgres — RETURNING, now(). Ports: OUTPUT / RETURNING INTO; CURRENT_TIMESTAMP.
```

A `standard` file uses ISO/IEC 9075 forms only; a `native` file names the engine and the port.
The header is parsed by `Source`, surfaced by the management surface, and checked in CI — header
presence at minimum, and optionally a keyword lint over `standard` files. This is the render-time
capability check of v0.3.0 translated into authored SQL, and it is also the concrete form of the
meta-language concept's compiler policy ("a unit that reaches for a native feature declares the
reach"): a phase-one form of a settled direction, not a regression.

## 6. The pattern catalog

As the prototype built it (2026-09-03): a **pattern** is protocol SQL any package publishes
under a namespace, authored as a `.sql` file with a tier and slots; the library's own SQL is
patterns too (namespace `sql`), an application registers its namespace beside them, and an
engine overlays the one pattern it spells differently. A statement includes a pattern at
compile with `{{> sql.guard_where}}`; the collection read composes the request-time patterns
(`collection`, `count`, `one`, `where`, the operators, `order`, `paging`) at request time from
a base and the read's signature. The catalog is built once at the composition root and every
statement compiles against it; `sqlint.toml` names the same sources so the lint resolves
includes as the runtime does. The admission rule below stands; the shapes are now files.

Conventions the standard names. Each is a SQL shape; a mechanism function is listed only where
one exists.

| Pattern | SQL shape | Mechanism |
|---|---|---|
| Collection | a base query naming the read model; contract fields map to its columns | projection + directives → page and count |
| Single by key | `WHERE <unique> = ?` over the same base | `One` over the projection, or a named statement |
| Guarded mutation | fixed SET, `WHERE key = ? AND version = ?`, `version = version + 1`; a version-check statement | `Guard` |
| Identity-returning insert | `INSERT … RETURNING` (native) or insert + lookup (standard) | `Scalar`/`Rows` |
| Transactional side effects | several named statements in one `ExecTx` | `ExecTx` |
| Lineage / hierarchy | recursive CTE as the base of a projection | none |

Admission rule for a new pattern: it has been needed by at least one domain, its SQL shape is
stated, and it earns a mechanism only if it carries a protocol the SQL cannot guarantee alone.

## 7. What changes, by artifact

| Artifact | Change |
|---|---|
| sqlate | new repository and module from the prototype's `lib/` and `cmd/sqlint` (§4.1); v0.1.0 with `sqlate/postgres` |
| go-database | v0.4.0: reduced to the infrastructure service plus `admin` over sqlate; `ast`, `operation`, `exec`, `seed`, the seam, the dialect, and the constraint classes removed; `layers.md` rewritten |
| go-web-sdk | the If-Match precondition parse, the error writer's detail option, the strict decode and respond plumbing; the operator-syntax decision for the query parser |
| go-web-sdk-template | scaffolds `internal/data` (the session-and-catalog grouping, migrations, the application's patterns, the seeds behind a seeder, the statements registry), `admin/database` over go-database's admin service, the composition root as one file per layer, `sqlint.toml`, and the mise tasks |
| go-web-service | `cmd/db` and golang-migrate removed; `statements/` per domain; `database.go` rewritten over sqlate's handles; the admin mount; the entity roles (validation methods, the tag conventions); the management listener |
| docs | the DSL-driven-services principle page (§2.1–2.5); the sqlate pages; the go-database pages rewritten; the grammar recorded as the standard's own artifact, sqlate its first host; the SQL meta-language concept reframed as having this experiment as its first phase; the architecture definition amended so a Domain Service anchors a domain, a composition of one or more Entities |
| claude-plugins | the sufficiency question (§2.4) enters the `plan` stage; the conventions are `sqlint` as a package the harness calls |

## 8. Sequence

`goals.v1.data.sql.integration` carries the task breakdown in dependency order: `sqlate` →
`database` → `websdk` → `template` → `service` (a coordinated session; go-database v0.4.0 and
the service change release together so the reference architecture demonstrates the split on
release day) → `listener`; then `docs`, `hardening`, and `suite` under `goals.v1.data.sql`.
The pre-split breakdown (`query`, `migrate`, `organization`, `startup`) is superseded.

## 9. Deferred to sessions

Named here so they are not mistaken for open strategy. Each is settled by the session that
builds the mechanism, under marathon's one-step rule.

- Exact `query` signatures, `Source` loading, placeholder rebinding details — the
  `v1.data.sql.plan` session.
- Catalog composition: whether the pattern catalog (§6) suffices, or build-time fragment
  composition (§3, last entry) earns a place.
- Whether row mapping stays scan-function-only or gains the struct-tag mapper.
- `Guard` as two statements versus two callbacks.
- The migration version table schema and the force/dirty semantics.
- Management surface authentication and the confirmation-token mechanism.
- Whether `Concat`-style expression helpers survive at all now that computed columns live in
  the base query.

## 10. Retrospective adjustments (2026-08-31)

The layer evaluation widened the v0.4 breaking window; these ride the same release because
they are breaking or blast-radius-coupled:

- **Error mapping moves inside `Session`/`Tx`.** Today raw driver errors return and `exec` is
  the only reliable `MapError` caller; deleting it would make constraint classification opt-in
  exactly as direct session use increases.
- **One transaction-runner shape** across `ExecTx`, `seed`, and `migrate`: recover on panic,
  rollback errors joined, `sql.TxOptions` reachable (the guard's serializable-isolation claim
  currently has no code path).
- **The `postgres` sub-module is provider-visible blast radius**: its dialect implements
  capabilities against `ast` (`ReturningRenderer`, `ast.Writer` — retired); `Placeholder`
  survives. Per-module `GOWORK=off` builds enter CI first — the committed `go.work` masked a
  broken pin at tag v0.3.0.
- **The salvage is ~250 lines, not the packages**: `operation`'s typed errors and `Directives`
  vocabulary, the key-tie-breaker and duplicate-field rules, `exec`'s generic `Scan`/`Query`,
  and the guard outcome logic. The filter lowering is rewritten against (fragment, args); the
  `ast` renderer, `RecursivePath`, and the compound machinery are not ported.
- **The field contract carries types**, so a malformed filter value is a typed 400, not an
  engine cast error surfacing as 500; `query` gets its own request-error sentinel (v0.3.0
  classifies a bad page as an invalid statement).
- **Verify needs `Session.PrepareContext` and a prepare-capable test harness**; the scripted
  driver fakes are promoted to a shared internal test package.

## 11. Plan session adjustments (2026-09-01)

The `v1.data.sql.plan` session worked the deferred items (§9) through to a reviewed API
design, then routed their settlement to evidence: `v1.data.sql.prototype`, an experiment in
go-database that builds the whole SQL ↔ Go layer in one template-generated service module
before v0.4 breaks the library, the provider, and the service in lockstep. The direction is
`go-database/context/concepts/sql-architecture.md`. Adjustments to this record:

- `seed` retires to a documented pattern (§4.1's `seed` line): under §2.4 a twenty-five-line
  loader over the transaction runner does not justify a package.
- §5.1 holds as stated: `database.go` is the domain's sole `query` importer, its SQL client,
  with the operations as its methods; `service.go` never touches the mechanism.
- Pattern templates are admitted under one split: a template carries only protocol (the
  guard frame, the version check, the collection wrap, the identity-returning insert frame),
  the domain all expressive content. Whether they stitch at load or generate at build is the
  prototype's first question (§3, last entry).
- Writes are not enforced by type. Every runner takes the session seam; a statement whose
  semantics need transaction scope declares `-- transaction: required` in its header.
- Parameters are named (`:name`), resolved to dialect positions once at load, in place of the
  ordinal `?` §4.2 states.

## 12. Prototype adjustments (2026-09-03)

The `v1.data.sql.prototype` experiment (`experiments/sql-dsl` at the coordinator; its
record is `NOTES.md`, its verdict `REVIEW.md`) settled the five questions the plan left to
evidence and changed this record where the sections above now say so: the split into
`sqlate` and go-database (§4.1, §7, §8); the file grammar of `{{name}}` parameters, list
expansion, and `--|` declarations (§4.2); the pattern catalog as authored, namespaced files
composed on demand (§6); PRQL ruled out (§3). Beyond this record it settled the service-side
shape the template scaffolds — `internal/data`, the admin mount over an admin domain, the
composition root as files, the entity roles — recorded in `REVIEW.md` §3 and §5 for the
integration tasks, and the vocabulary in `NOTES.md`'s Ontology section: pattern, slot,
namespace, source, catalog, statement, declaration, parameter, include, base, field
contract, directives, signature, handle, and the verbs compile, compose, execute.

## Appendix — review concerns

Positions that go beyond the original brief or trade something away, raised for review.

### A.1 Commands leave the library; only the guard protocol remains

Resolved by principle (§2.2) but worth confirming: the library no longer guarantees the guard's
SQL shape. A consumer can author a guarded update that forgets the increment or the version
predicate. Mitigation is a convention check — a file named or annotated as guarded must contain
both clauses — in place of the v0.3.0 type. The alternative, keeping write statements in a
vocabulary, contradicts §2.2 and reopens the library-axis cost.

### A.2 Portability by discipline rather than by construction

§2.3 states the trade. The angles: how much engine portability the organization needs enforced
by a compiler versus by review; whether the keyword lint over `standard` files is worth building
and where it lives; and whether the header should be treated as the phase-one form of the
meta-language concept's compiler policy, which would make it a step toward a settled direction
rather than a loss.

### A.3 Applying migrations from the service process

§5.2 recommends verify-by-default and apply-by-mode with a one-shot invocation for production.
The concern is least privilege in regulated postures: whether the serving role should ever hold DDL, and
whether the one-shot mode is enough separation or a distinct migration role and binary
invocation should be mandated by the standard.

### A.4 Replacing golang-migrate with a ~150-line mechanism

The one place this strategy replaces a mature library with new code. The dependency-line and
DSL-principle arguments are strong, but the acceptance criteria in §4.3 (dirty state, non-
transactional DDL, concurrent starters) are the cases where the library earned its size, and
the session that builds the mechanism must treat them as required, not nice-to-have.

### A.5 The management surface as new attack surface

§5.3 makes network isolation a design constraint. Whether that is sufficient, or whether the
standard should require the surface to be off by default and enabled per environment, is a
posture decision for the review.
