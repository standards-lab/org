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

An install is one directory tree in one database. The schema seeds one root directory, with the
well-known id `blobfs.RootID` and no name, and every path starts at `/`. A consumer that wants
several isolated trees runs several configurations, each with its own database and container.

| Directory | Contents |
|-----------|----------|
| `cmd/blobfs/` | Process entry: the signal context, `app.New(os.Stdout, os.Stderr).Run(ctx)`, and the exit code. It imports only `internal/app`. |
| `internal/app/` | The composition root, one file per layer: the root command's flags, the infrastructure (the database pool, the logger, the output), the domain layer, the admin layer, and the list of mounts. It is the only package that opens a connection or names the pgx driver. |
| `internal/livetest/` | The throwaway-database helper the integration-tagged tests share. |
| `domain/files/` | The consumer's file-system layer over `blobfs`: the row type of the consumer's `directory_owner` table, its owner read model (`owned_directories`, a projection base over `blobfs`'s published column list joined to `directory_owner`), `database.go` as the sole importer of `sqlate/query`, `blobfs.go` as the translation over the library, and the `mkdir` and `ls` commands. In later stages it gains the storage adapter, the file commands, and the bookmark commands. |
| `evidence/` | The transcripts the measurements write: `read-model.txt` is proof V3, the cost of the shipped listing (`mise run evidence` regenerates it); `v1-read-model.txt` is proof V1, the read-model cost by form against the volume-based schema of an earlier stage, kept as the record. |
| `admin/schema/` | The schema administration layer: the `schema` command, which applies and reverts the two migration sets in canonical order. |
| `migrations/` | The consumer's own migration set: `directory_owner` and `bookmark`, run after `blobfs`'s set under `sqlate`'s default history table. |
| `output/` | The result rendering every command family shares: a one-line result to stdout, a directory listing as aligned rows with one line per half stating the page and the total or its absence, an error to stderr. |
| `integration/` | The integration tier, behind the `integration` build tag: the built binary driven black-box against the compose stack. |
| `lib/blobfs/` | The root package: entity types, the root's id, status vocabulary, key construction, name normalization, and error types. It imports neither `sqlate` nor `go-storage`. |
| `lib/blobfs/data/` | The persistence package: statements, the published pattern namespace, the listing composer, the methods that take a `sqlate.Session`, and the `Variant` interface with its standard-tier baseline, `Standard`. Every statement in it is standard tier. |
| `lib/blobfs/data/pgnative/` | The Postgres variant of the persistence package's two variation points, over two native-tier statements, each with its port note. It imports the persistence package and `sqlate` only. |
| `lib/blobfs/data/datatest/` | The conformance suite a variant must pass, run through a store built over the variant against a live database. The persistence package's tests run it over the baseline and `pgnative`'s over the Postgres variant. |
| `lib/blobfs/migrations/` | The embedded DDL, exported as a migration source under its own history table. |
| `lib/migrator/` | A migrator that runs several migration sets, each with its own history table. It imports only `sqlate` and the standard library. |
| `compose/` | The Postgres and Azurite services the experiment runs against. |

## The two variation points

The persistence package runs on any engine `sqlate` has a dialect for, and two of its operations
may be replaced by an engine's own statements through the `data.Variant` interface. `data.New`
runs the standard baseline unless it is given a variant with `data.WithVariant`, and a consumer
that wants a different behavior for one operation embeds a variant and overrides that method.

- **The tree lock** (`LockTree`, `Serializes`). A directory move takes the lock inside its
  transaction, before its cycle check, so two opposing moves run one after the other. The Postgres
  variant takes a transaction-scoped advisory lock under a fixed key (`pgnative.TreeLockKey`),
  which the engine releases when the transaction ends. Standard SQL has no lock that is held to
  commit, so the baseline's lock is a no-op and `Serializes` reports false: on the baseline, two
  opposing concurrent moves can form a cycle, and a consumer that needs the guarantee serializes
  moves outside the database.
- **The file-delete begin** (`BeginFileDelete`). The step moves the file to `deleting` and returns
  the row, so the caller can delete the object by its key and then complete the delete by removing
  the row. A retry returns the same row; a missing file is `blobfs.ErrNotFound`. The Postgres
  variant is one `UPDATE ... RETURNING`. The baseline is an update and a read-back, which must run
  in one transaction so the read sees the row the update locked; it refuses the pool with
  `query.ErrTransactionRequired`.

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
- `mise run evidence` runs the listing cost measurement (`TestListingCost` in `lib/blobfs/data`,
  skipped unless `BLOBFS_EVIDENCE=1`) against the compose stack in a throwaway database and
  writes the transcript to `evidence/read-model.txt`.

The DSN and the storage settings come from the `[env]` table in `mise.toml`. The Azurite account
and key are the emulator's published development credentials.

## The commands so far

Paths are absolute: `/` is the root and `/reports/2026` a directory two levels below it. A unit
id is a UUID and stands in for the auth strategy's unit.

- `schema up` applies both migration sets, `blobfs`'s first, and `schema down` reverts them in
  the opposite order.
- `mkdir <path>` creates the last segment under its parent, which must exist; there is no `-p`.
  `mkdir <path> --unit <uuid>` works at a top-level path only and writes the directory and its
  `directory_owner` row in one transaction.
- `ls <path>` lists the directories under the path and then the files in it, one page of each,
  with one line per half stating how many rows the page holds, the page number and size, and
  the total. `--page` and `--size` choose the page, `--sort <field>[:desc]` (repeatable) orders
  it, and `--total none` skips the total, which the page statement otherwise computes in the
  same query as the rows. A sort term applies to both halves when both declare its field
  (`name`, `created_at`, `updated_at`, `version`, `id`); a field only files have, such as
  `size`, sorts the files and leaves the directories in name order. A descending sort is the
  exact reverse of the ascending one: the `name` tie-breaker takes the sort's direction. An
  empty page after the first carries no total and says so. The resolution of the path and both
  halves run in one read-only repeatable-read transaction.
- A half that has a next page prints a line `next-dirs: <cursor>` or `next-files: <cursor>`.
  `ls <path> --after-dirs <cursor>` and `ls <path> --after-files <cursor>` continue that half
  from the cursor: the half then lists the rows after the last row of the earlier page, ignores
  `--page`, and reports `total not counted`, because a total computed under the cursor would
  count the rows after it and not the listing. The other half is still read by page number. A
  cursor is opaque; it is refused when it was edited, was issued by the other half, or was
  issued under a different `--sort`, and a sort by a field that can be NULL (`size`, `etag`)
  pages by number only. `ls / --unit` reads the owner read model, which pages by number only,
  so it refuses a cursor.
- `ls <path> --unit <uuid>` is the directory-grain ownership rehearsal: the unit must own the
  path's top-level directory, checked once at that ancestor, and is refused otherwise. At `/`
  the listing is the unit's own top-level directories, read through the consumer's owner
  projection, and no files.

A run against a database whose schema is not applied fails before any work and names
`schema up`.
