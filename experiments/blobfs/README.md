# blobfs

An experiment that tests the design in `context/concepts/blobfs.md`: a SQL-backed virtual
directory and file-metadata library over any object store. The experiment builds a command-line
file system over Postgres and Azure Blob Storage, and records what the design gets right and what
it gets wrong. It is not a product. The results are evidence for the concept, and nothing here
moves into `design/` or into a member repository without a deliberate promotion.

The experiment depends on published `sqlate` v0.1.1, `go-storage` v0.1.0, and `azureblob` v0.1.0.
It contains no `replace` directive and edits no member repository.

## Layout

The packages under `lib/` are the promotion candidates. The rest is the consumer that exercises
them, laid out as an elemental application: `cmd/blobfs` is process entry, `internal/app` is the
composition root, and every other application package sits at the module root and never imports
`internal/*`. Directories marked planned do not exist yet; each stage of the experiment creates its
own.

| Directory | Contents |
|-----------|----------|
| `cmd/blobfs/` | Process entry: the signal context, `app.New(os.Stdout, os.Stderr).Run(ctx)`, and the exit code. It imports only `internal/app`. |
| `internal/app/` | The composition root, one file per layer: the root command's flags, the infrastructure (the database pool, the logger, the output), the domain layer, the admin layer, and the list of mounts. It is the only package that opens a connection or names the pgx driver. |
| `internal/livetest/` | The throwaway-database helper the integration-tagged tests share. |
| `domain/volume/` | The consumer's file-system layer over `blobfs`: the row types of the consumer's own tables, its two read models (`volume_view` and `file_view`, projection bases over `blobfs`'s published patterns joined to `volume_owner`), `database.go` as the sole importer of `sqlate/query`, `blobfs.go` as the translation over the library, the `<volume>:<path>` address parser, and the `volume`, `mkdir`, and `ls` commands. In a later stage it gains the storage adapter and the file commands. |
| `evidence/` | The transcripts the measurements write: `v1-read-model.txt` is proof V1, the read-model cost by form, with `EXPLAIN (ANALYZE, BUFFERS)` plans at 1k, 10k, and 100k file rows. |
| `admin/schema/` | The schema administration layer: the `schema` command, which applies and reverts the two migration sets in canonical order. |
| `migrations/` | The consumer's own migration set: `volume_owner` and `volume_bookmark`, run after `blobfs`'s set under `sqlate`'s default history table. |
| `output/` | The result rendering every command family shares: a one-line result to stdout, a listing as aligned rows with a total line, an error to stderr. |
| `integration/` | The integration tier, behind the `integration` build tag: the built binary driven black-box against the compose stack. |
| `lib/blobfs/` | The root package: entity types, status vocabulary, key construction, name normalization, and error types. It imports neither `sqlate` nor `go-storage`. |
| `lib/blobfs/data/` | The persistence package: statements, the published pattern namespace, and the methods that take a `sqlate.Session`. It holds the standard-tier baseline. |
| `lib/blobfs/data/pgnative/` | The Postgres variant of the persistence package's variation points. (planned) |
| `lib/blobfs/migrations/` | The embedded DDL, exported as a migration source under its own history table. |
| `lib/migrator/` | A migrator that runs several migration sets, each with its own history table. It imports only `sqlate` and the standard library. |
| `compose/` | The Postgres and Azurite services the experiment runs against. |

## Running it

The experiment carries its own toolchain in `mise.toml`: Go 1.27 and `golangci-lint`.

- `mise run up` starts Postgres on `127.0.0.1:5434` and Azurite on `127.0.0.1:10000`, and waits
  until both report healthy. `mise run down` stops them and keeps their data; `mise run reset`
  also drops the data.
- `mise run test` runs the hermetic tests. `mise run integration` runs the tests that need the
  services, behind the `integration` build tag; each one creates and drops its own database.
- `mise run lint` runs `golangci-lint` and `sqlint`. `mise run split-check` fails when a package
  imports what its layer may not.
- `mise run cli -- schema up` runs the command-line file system; `mise run cli -- --help` lists
  its commands. The database comes from `--dsn`, or from `BLOBFS_DSN` when the flag is not given.
  The commands so far: `schema up|down`; `volume create <name> --unit <uuid>`, `volume ls`, and
  `volume rename <name> <new-name>`; `mkdir <volume>:<path>`; and `ls <volume>:<path>`. A path is
  addressed as `<volume>:<path>`, so `docs:/` is the root of the volume `docs` and
  `docs:/reports/2026` a directory inside it. The two listings take `--page`, `--size`,
  `--sort <field>[:desc]` (repeatable), and `--unit <uuid>`, which filters by the owning unit.
- `mise run evidence` runs the read-model cost measurement (proof V1) against the compose stack
  and writes its transcript to `evidence/v1-read-model.txt`. It seeds 100,000 file rows, so
  `mise run integration` skips it; the test runs only under `BLOBFS_EVIDENCE=1`.

The DSN and the storage settings come from the `[env]` table in `mise.toml`. The Azurite account
and key are the emulator's published development credentials.
