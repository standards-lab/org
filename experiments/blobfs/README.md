# blobfs

An experiment that tests the design in `context/concepts/blobfs.md`: a SQL-backed virtual
directory and file-metadata library over any object store. The experiment builds a command-line
file system over Postgres and Azure Blob Storage, and it records what the design gets right and
what it gets wrong. It is not a product. The results are evidence for the concept, and nothing
here moves into `design/` or into a member repository without a deliberate promotion.

Postgres holds the directory tree and each file's metadata: names, status, size, and the key of
the stored object. The object store holds the file bytes, and the library never calls it. The
consumer application in `domain/files` sequences the two stores.

Where to read next:

- `GUIDE.md` is a guided tour of every capability, with the files that implement each and the
  commands to run.
- `DECISIONS.md` records the decisions from the review of the first sixteen stages and the
  adjustments that the later stages implemented.
- `REVIEW.md` is the organized record of the findings, and `NOTES.md` is the chronological log.
- `evidence/` holds the measurements behind the decisions.

The experiment depends on published `sqlate` v0.1.1, `go-storage` v0.1.0, and `azureblob` v0.1.0.
It contains no `replace` directive and edits no member repository.

## Layout

The packages under `lib/` are the promotion candidates. The rest is the consumer that exercises
them, laid out as an elemental application: every package other than `cmd/` and `internal/app/`
sits at the module root and never imports `internal/*`.

| Directory | Contents |
|-----------|----------|
| `cmd/blobfs/`, `internal/app/` | The process entry and the composition root, which is the only place that opens a connection, names the driver, and chooses the variant. |
| `domain/files/` | The consumer's file-system layer: the `Store` with its path and id forms, the object-store adapter, and the commands. |
| `admin/schema/`, `migrations/` | The `schema` command, and the consumer's own migration set. |
| `output/` | The rendering every command shares. |
| `lib/blobfs/` | The root package: entity types, sentinel errors, key and name rules. It imports neither `sqlate` nor `go-storage`. |
| `lib/blobfs/data/` | The persistence package: standard-tier statements, the listing composer, the write, delete, and move protocol, and the `Variant` interface with its baseline. `datatest/` is the conformance suite a variant must pass. |
| `lib/blobfs/postgres/` | The Postgres engine: the variant with its native statements, and the DDL as a migration set. |
| `lib/migrator/` | The multi-set migrator, a shim over `sqlate`. |
| `integration/`, `evidence/`, `compose/` | The black-box tests over the built binary, the measurements, and the Postgres and Azurite services. |

## Starting up

The experiment carries its own toolchain in `mise.toml`, which pins Go and `golangci-lint`.

Start the services. Postgres listens on `127.0.0.1:5434` and Azurite on `127.0.0.1:10000`, and
the command waits until both are healthy:

```bash
cd experiments/blobfs
mise run up
```

Build the binary, and create a database and a container name of your own. The `[env]` table in
`mise.toml` points at the stack's default `app` database, and mise's value overrides a variable
set on the command line, so `mise run cli` changes `app`. The wrapper below passes every setting
explicitly and never touches `app`:

```bash
mise exec -- go build -o /tmp/blobfs ./cmd/blobfs

DB=blobfs_try CT=blobtry
docker exec blobfs-postgres psql -U app -d app -c "CREATE DATABASE $DB"

bfs() {
  BLOBFS_STORAGE_ENDPOINT=http://127.0.0.1:10000/devstoreaccount1 \
  BLOBFS_STORAGE_ACCOUNT=devstoreaccount1 \
  BLOBFS_STORAGE_KEY='Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==' \
  BLOBFS_STORAGE_CONTAINER=$CT \
  /tmp/blobfs --dsn "postgres://app:app@127.0.0.1:5434/$DB?sslmode=disable" "$@"
}

bfs schema up
```

The account and key are the Azurite emulator's published development credentials. Every file
command runs a check first, and a database whose schema is not applied is refused with a message
that names `schema up`. A database that an earlier commit installed must be reset before this
code runs against it, because the root directory's row changed.

## Using the commands

Paths are absolute: `/` is the root and `/reports/2026` is a directory two levels below it. The
commands `ls`, `stat`, `cat`, and `rm` also take an entry's id as `id:<uuid>` in place of its
path, and `put`, `cp`, and `mv` take a destination directory's id the same way. The root is
addressed only as `/`. The commands below assume the wrapper above.

```bash
bfs mkdir /reports                                # create a directory; the parent must exist
bfs mkdir /acme --unit <uuid>                     # a top-level directory owned by a unit
bfs ls /reports                                   # directories, then files, one page each
bfs ls /reports --size 10 --page 2 --sort name:desc --total none
bfs ls /reports --filter size:gt:100 --filter name:like:q%
bfs ls /reports --size 3 --cursors                # print the cursors that continue each half
bfs ls /reports --after-files <cursor>            # continue the file half from a cursor
bfs ls /acme --unit <uuid>                        # list as a unit that owns /acme
bfs put ./q1.txt /reports/q1.txt                  # upload; use - for stdin
bfs cat /reports/q1.txt                           # write the content to stdout
bfs stat /reports/q1.txt                          # a file's or a directory's row
bfs cp /reports/q1.txt /reports/2026              # into a directory, or to a new path
bfs mv /reports/q1.txt /reports/q1-final.txt      # move or rename; within one top-level directory
bfs rm /reports/q1-final.txt                      # delete a file
bfs rmdir /reports/2026                           # remove an empty directory
bfs rm -r /reports                                # remove a tree, children first
bfs bookmark add /acme/docs/logo.png --unit <uuid> --active
bfs bookmark ls --unit <uuid>
bfs bookmark rm /acme/docs/logo.png --unit <uuid>
```

What to know about the output:

- A success prints one line, and `ls` prints a table whose last column is each row's id.
- Each half of a listing prints `more: yes` or `more: no`, which says whether rows remain after
  the page. A cursor continues a half without an offset, and cursors print only with
  `--cursors`.
- An error prints the chain from the outermost operation to the innermost cause, so read the
  last segment first. A refusal that a database constraint caused names the reason and the
  constraint.
- `put` and `cp` take `--fail-after insert|write`, and `rm` takes `--fail-after begin|object`.
  Each stops after that step and leaves a `pending` or `deleting` row, and a rerun of the same
  command finishes it.

## Shutting down

Drop the database you created and remove the binary:

```bash
docker exec blobfs-postgres psql -U app -d app -c "DROP DATABASE $DB"
rm /tmp/blobfs
```

The command-line tool cannot delete an object-store container, so `$CT` stays in Azurite until
the stack's data is dropped. Stop the services, keeping their data or dropping it:

```bash
mise run down      # stop the services and keep their data
mise run reset     # stop the services and drop their data
```

## Remaining reference

The mise tasks:

| Task | What it does |
|------|--------------|
| `mise run up`, `down`, `reset` | Start the services and wait for health, stop them, or stop them and drop their data. |
| `mise run build`, `vet`, `fmt`, `tidy` | Build every package, run `go vet`, format the source, and run `go mod tidy`. |
| `mise run test` | Run the hermetic tests. |
| `mise run integration` | Run the tests that need the services, behind the `integration` build tag. Each test creates and drops its own database. |
| `mise run demo` | Build the binary and run the scripted run of `integration/` once per variant, printing every command and its output. |
| `mise run lint`, `split-check` | Run `golangci-lint` and `sqlint`, and check the import boundaries of the layers. |
| `mise run evidence` | Run the cost measurements against the stack and write `evidence/read-model.txt`, `bookmarks.txt`, and `sort-index.txt`. |
| `mise run cli -- <command>` | Run the binary. It reads `BLOBFS_DSN`, which names the `app` database. |

The command-line settings:

| Setting | Meaning |
|---------|---------|
| `--dsn`, `BLOBFS_DSN` | The database's connection string. |
| `--variant`, `BLOBFS_VARIANT` | `standard`, the baseline every engine runs, or `postgres`, the Postgres engine's variant. The default is `standard`. |
| `BLOBFS_STORAGE_ENDPOINT`, `_ACCOUNT`, `_KEY`, `_CONTAINER` | The object store. The first file command that needs it opens it, so `mkdir` and `ls` never read these. |

The schema commands:

```bash
bfs schema status         # one row per migration set: applied version, latest, pending, dirty
bfs schema up             # apply both sets, blobfs's first
bfs schema down           # revert both sets in the opposite order, keeping the history tables
bfs schema reset --yes    # revert both sets and drop the history tables
```

The other flags, by command:

| Command | Flags |
|---------|-------|
| `ls` | `--page`, `--size`, `--sort <field>[:desc]`, `--total exact\|none`, `--filter <field>:<op>:<value>`, `--cursors`, `--after-dirs`, `--after-files`, `--unit`. |
| `put` | `--content-type`, `--fail-after insert\|write`. |
| `cp` | `--fail-after insert\|write`. |
| `bookmark ls` | `--page`, `--size`, `--sort`, `--total`, `--unit`. |
