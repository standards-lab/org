# sql-dsl

The `v1.data.sql.prototype` experiment: the whole SQL ↔ Go layer of the Go Elemental
architecture built in one template-generated service module, nothing published. Direction and
the five questions it settles: `go-database/context/concepts/sql-architecture.md`. The
experiment's record, position, and findings: `NOTES.md`.

## Layout

- `lib/` — promotion candidates for `sqlate`, the SQL templating library: its build graph
  holds no go-database, no service package, and no driver (`mise run split-check` enforces
  it); `sqldb` wraps a plain `*sql.DB` with a `sqldb.Dialect`, and the provider's dialect
  satisfies it structurally. The handle constructors are Go 1.27 generic methods on
  `Statement` (`Scan`, `Project`, `Guarded`) and `DB` (`Transact`). A parameter written
  `{{ids...}}` or `{{ids...:type}}` expands at bind to one placeholder per element of its
  list. `lib/query/patterns/` is the library's own SQL, namespace `sql`:
  the patterns the collection read composes at request time and the ones a statement includes
  with `{{> sql.name}}`. Every pattern source carries a `sqlint.toml` whose `[export]` names
  what a consumer reads from it; `lib/pgdialect`'s declares the engine's native forms.
- `admin/` — the administrative layer: one admin domain per infrastructure service, mounted
  at `/admin`; `admin/database` is the operations over the data layer — verify, up, down,
  steps, force, seed, diagnostics, the catalog read — and the policy of when the seed runs.
- `domain/` — the two domains, mounted at `/api`: `organization` (stage 8), `person`
  (stage 9). Each holds its statements in `statements/` and its SQL client in `database.go`.
- `internal/` — the service side: `config` with its `configtest` fixtures, `data` (the
  service's database infrastructure: the session-and-catalog grouping the domains take, the
  schema in `migrations/`, the application's pattern namespace `app` in `patterns/`, the seed
  files and their statements behind a `Seeder`; domains define no patterns), the composition root
  (`app`, whose `infrastructure.go` builds the catalog and, with `admin.go`, `domain.go`, and
  `reactors.go`, wires the layers and mounts them), and sdk staging.
- `cmd/server` — the service binary.

## Running it

```sh
mise trust && mise install
mise run db-up                 # postgres:18 on 127.0.0.1:5433
mise run serve                 # http://127.0.0.1:8081; startup applies pending migrations
mise run test                  # hermetic
mise run test-compose          # against the live stack
```

The API mount, `/api/organizations`: `GET` (paged, `?page=&size=&sort=&<field>=`), `GET /{id}`,
`GET /path/{path...}`, `POST`, `PATCH /{id}`, `POST /{id}/transfer`, `DELETE /{id}`; the
guarded commands take `If-Match: "<version>"`. Paging policy is the `reads` config block.

`/api/people`: `GET` (and `GET ?id=a,b,c`, the batch fetch by key through the expanded
statement, unpaged and bounded by the size limit), `GET /{id}`, `POST`, `PATCH /{id}`,
`DELETE /{id}`, and the actions `POST /{id}/activate`, `POST /{id}/deactivate`,
`POST /{id}/transfer-unit`.

`mise run lint` runs golangci-lint and `cmd/sqlint`, the SQL conventions lint, configured by
`sqlint.toml`, one per module: a table per role with its directory globs and switches, the
pattern sources and the engine as paths. A producer (a Go package path resolved through `go
list`, or a directory holding its own `sqlint.toml`) declares what a consumer reads in its
`[export]`; a bare directory is the service's own pattern files. The library and the engine are
named by package path and resolved through `go list`, the form they keep after the split. The
engine's export names its native forms as regular expressions, matched against code only.

The admin mount, `/admin/database`: `GET /diagnostics`, `GET /schema`, `GET /patterns` (the
catalog as the library holds it: every namespace and pattern, with tier, slots, and text),
`GET /statements` (the registry: every domain's compiled inventory with declarations,
parameters, and text), `POST
/schema/{verify,up,down,steps,force}` (bodies `{"steps": n}` and `{"version": v}`), and
`POST /seed`, each a trigger over the same function startup calls.

Seeding is development and test tooling behind `admin.seed` (`APP_ADMIN_SEED`): on in
`config.local.json`, off in the base config. When on, startup seeds once the schema is
current, and `POST /seed` runs the same loader; when off, the endpoint answers 403. The seed
files under `admin/database/seeds/` are idempotent through each table's unique constraint.

The service reads `config.json`, `config.local.json` (`APP_ENV=local`), and `APP_*`
variables; the compose password rides `APP_DATABASE_PASSWORD` from `mise.toml`.
