# sql-dsl

The `v1.data.sql.prototype` experiment: the whole SQL ↔ Go layer of the Go Elemental
architecture built in one template-generated service module, nothing published. Direction and
the five questions it settles: `go-database/context/concepts/sql-architecture.md`. The
experiment's record, position, and findings: `NOTES.md`.

## Layout

- `lib/` — promotion candidates for go-database; never imports `internal/`
  (`mise run split-check` enforces it).
- `internal/` — the service side: config, infrastructure, schema, sdk staging, and the two
  domains.
- `cmd/server` — the service binary; `-schema <verb>` is the one-shot schema mode.

## Running it

```sh
mise trust && mise install
mise run db-up                 # postgres:18 on 127.0.0.1:5433
mise run schema -- diag        # connection diagnostics
mise run serve                 # http://127.0.0.1:8081
mise run test                  # hermetic
mise run test-compose          # against the live stack
```

The service reads `config.json`, `config.local.json` (`APP_ENV=local`), and `APP_*`
variables; the compose password rides `APP_DATABASE_PASSWORD` from `mise.toml`.
