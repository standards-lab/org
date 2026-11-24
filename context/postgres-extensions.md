# Postgres extensions at the native tier

How a Postgres extension is declared at the native tier: its install, its migration, its image pin,
and the packages allowed to use it, the terms of spike-ai-pipeline's evidence 10
(`data-intelligence.md`, "Evidence"). pgvector, for capstone-athena, is the first case, and no spike
ran it, so this note holds the design `v1.data.extensions` builds. An extension is treated like a
provider: reusable infrastructure in sub-modules a consumer opts into. sqlate owns the SQL layer and
go-database the Go layer. The `principles` task promotes this note into Go Elemental's principles
and retires it.

## The image is the version

Every Postgres image that carries an extension is `postgres:19.0-alpine` with each extension built
from source, so further extensions layer onto one baseline rather than across prebuilt images of
different distributions. sqlate's `compose/postgres/Dockerfile` is the canonical copy, and a
consumer copies it. pgvector builds from `ADD …pgvector.git#v${PGVECTOR_VERSION}` with
`make with_llvm=no OPTFLAGS=""`: the official image is built with JIT against clang-21, and
`OPTFLAGS=""` keeps the binary portable. The `ARG PGVECTOR_VERSION` line is the extension's one pin,
and `currency.sh` learns to read it beside the `FROM` line. Only a repository that declares an
extension switches to the extension image; the rest keep plain `postgres:19.0-alpine`.

The image is the sole source of truth for an extension's version, and no Go code declares one. An
extension's control file sets its `default_version`, which `pg_available_extensions` reports.

Rejected: `pgvector/pgvector:<v>-pg19-trixie`, which swaps the service layer to Debian for one
extension and freezes the Postgres minor to pgvector's releases; PGDG's package on a Debian image,
for the same distribution change; and a version declared in Go, which the image overrules anyway,
since a built image makes only its own version available.

## The extensions layer

sqlate's migrator gains an engine-neutral extension stage. The application lists one declaration per
extension in `migrate.Options.Extensions`: a name, and optionally a removed state, with no version.
The stage runs ahead of every set, under the migrator's lock on its pinned connection, so starting
replicas serialize their activations. It applies declarations in declared order, each in its own
transaction, with no opt-out. Each declaration is a check and an activation, and sqlate/postgres
supplies the standard pair:

- **check**: compares `installed_version` with the image's `default_version` in
  `pg_available_extensions`.
- **activation**, on a mismatch: `CREATE EXTENSION IF NOT EXISTS x; ALTER EXTENSION x UPDATE`.

There is no history table: every run reconciles against the catalog, which an operator's manual
`ALTER EXTENSION` can't drift from. Since activation, the one privileged step, runs only on a
mismatch, a database whose operator pre-installed the image's version passes under an unprivileged
role.

The stage fails before any set runs, with a typed error naming the extension and the versions, when
the extension isn't available in the image, or when the installed version is newer than the image's
and the image ships no update path down to it. pgvector ships forward upgrade scripts only, so
rolling its image back across a catalog change needs an operator; within 0.8.x the catalog differs
only in comments. A mismatch otherwise makes `Verify` return an error that unwraps to `ErrPending`,
so go-database's `admin.Service.Start` applies it unchanged. `Migrator.Extensions(ctx)` reports each
extension's state, and `admin.Status` gains an `extensions` field, with `Ready` false on a mismatch.
`pgtest.MigratedWith(t, env, prefix, opts, sets...)` passes the declarations to a scratch database.

A declaration marked removed runs `DROP EXTENSION IF EXISTS x RESTRICT` after the sets, so one deploy
can drop the dependent columns in a set migration and then the extension; RESTRICT fails the run on
any dependent object left behind. Deleting a declaration only stops managing the extension, and
never drops it. Reset leaves extensions installed.

Only the application declares extensions; a shipped migration set declares none
(`migration-sets.md`), since a shipper requiring one would force every consumer onto its image.

Rejected: an application-owned bottom set of up/down migrations, which accumulates a migration per
upgrade and has no honest down for a version with no downgrade script.

## sqlint

sqlint gains `extensions = [<module paths>]` in `sqlint.toml`: each listed module's native forms join
the engine's deny list, so a module that declares pgvector refuses its forms in standard-tier files.
An extension module exports forms whenever the extension adds any name a statement can use:
operators, types, index methods, functions, or views. One that adds none declares `forms = []` with
a `reason`, and sqlint refuses a listed module with neither. Review keeps the sqlint list and
`Options.Extensions` in step; sqlint can't read Go values. A native file's `native:` header names the
extension it uses (`native: postgres, pgvector — <=> cosine distance`).

## The modules

- **`sqlate/postgres/pgvector`**, its own module so sqlint resolves its `sqlint.toml`: the native
  forms, and a Go package holding the extension's name and a table mapping each metric (l2, cosine,
  inner product) to its operator and index operator class.
- **`go-database/postgres/pgvector`**: helpers over pgvector-go's root module, in `database/sql` text
  format, so pgvector-go stays out of domain packages. It depends on sqlate's package for the metric
  table, ships no SQL generation and no space registry, and is proven in go-database's new
  integration tier. pgvector-go's pgx module is out: go-database/postgres has no AfterConnect hook
  for its registration.

Importing either module is limited the way a provider's is: the composition root, the migrations,
and the packages the application's design documentation declares. An embedding space's HNSW index is
an application migration, a partial index over a typed cast
(`USING hnsw ((embedding::vector(768)) vector_cosine_ops) WHERE space = <id>`), and its queries
repeat the cast.

## Assumptions

- pgvector's catalog stays compatible across patch releases, so rolling an image back within a minor
  needs no operator.
- No shipped set needs an extension; the first that does reopens the shipper rule.
- go-database's `database/sql` text format is fast enough for the capstone's batch loads.
