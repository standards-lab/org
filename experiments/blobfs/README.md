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
them. Directories marked planned do not exist yet; each stage of the experiment creates its own.

| Directory | Contents |
|-----------|----------|
| `lib/blobfs/` | The root package: entity types, status vocabulary, key construction, name normalization, and error types. It imports neither `sqlate` nor `go-storage`. (planned) |
| `lib/blobfs/data/` | The persistence package: statements, the published pattern namespace, and the methods that take a `sqlate.Session`. It holds the standard-tier baseline. (planned) |
| `lib/blobfs/data/pgnative/` | The Postgres variant of the persistence package's variation points. (planned) |
| `lib/blobfs/migrations/` | The embedded DDL, exported as a migration source. (planned) |
| `lib/migrator/` | A migrator that runs several migration sets, each with its own history table, under one lock. It imports only `sqlate` and the standard library. (planned) |
| `consumer/` | The consumer's own tables (`volume`, `volume_bookmark`), statements, and lock registry. (planned) |
| `cmd/blobfs/` | The command-line file system. (planned) |
| `compose/` | The Postgres and Azurite services the experiment runs against. |

## Running it

The experiment carries its own toolchain in `mise.toml`: Go 1.27 and `golangci-lint`.

- `mise run up` starts Postgres on `127.0.0.1:5434` and Azurite on `127.0.0.1:10000`, and waits
  until both report healthy. `mise run down` stops them and keeps their data; `mise run reset`
  also drops the data.
- `mise run test` runs the hermetic tests. `mise run test-compose` runs the tests that need the
  services, behind the `compose` build tag.
- `mise run lint` runs `golangci-lint` and `sqlint`. `mise run split-check` fails when a package
  imports what its layer may not.
- `mise run cli` runs the command-line file system. (planned)

The DSN and the storage settings come from the `[env]` table in `mise.toml`. The Azurite account
and key are the emulator's published development credentials.
