# sql-dsl

The `v1.data.sql.prototype` experiment: the whole SQL ↔ Go layer of the Go Elemental
architecture built in one template-generated service module, nothing published. Direction and
the five questions it settles: `go-database/context/concepts/sql-architecture.md`. The
experiment's record, position, and findings: `NOTES.md`.

## Layout

- `lib/` — promotion candidates for go-database; never imports `internal/`
  (`mise run split-check` enforces it).
- `admin/` — the administrative layer: one admin domain per infrastructure service, mounted
  at `/admin`; `admin/database` owns the schema (migrations), the seed files, and their
  operations.
- `domain/` — the two domains, mounted at `/api` (stages 8 and 9).
- `internal/` — the service side: `config` with its `configtest` fixtures, the composition
  root (`app`, whose `infrastructure.go`, `admin.go`, `domain.go`, and `reactors.go` wire
  the layers and mount them), and sdk staging.
- `cmd/server` — the service binary.

## Running it

```sh
mise trust && mise install
mise run db-up                 # postgres:18 on 127.0.0.1:5433
mise run serve                 # http://127.0.0.1:8081; startup applies pending migrations
mise run test                  # hermetic
mise run test-compose          # against the live stack
```

The admin mount, `/admin/database`: `GET /diagnostics`, `GET /schema`, `POST
/schema/{verify,up,down,steps,force}` (bodies `{"steps": n}` and `{"version": v}`), and
`POST /seed`, each a trigger over the same function startup calls.

Seeding is development and test tooling behind `admin.seed` (`APP_ADMIN_SEED`): on in
`config.local.json`, off in the base config. When on, startup seeds once the schema is
current, and `POST /seed` runs the same loader; when off, the endpoint answers 403. The seed
files under `admin/database/seeds/` are idempotent through each table's unique constraint.

The service reads `config.json`, `config.local.json` (`APP_ENV=local`), and `APP_*`
variables; the compose password rides `APP_DATABASE_PASSWORD` from `mise.toml`.
