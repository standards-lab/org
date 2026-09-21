# blobfs guided tour

A walk through what the `blobfs` command-line file system does, one capability at a time. Each
step gives a summary, the files that implement it, and commands to run yourself. `README.md` is
the reference for the layout and the commands, and `REVIEW.md` is the record of findings and
decisions. All paths in this guide are relative to `experiments/blobfs`.

## Setup

The tour needs the compose stack running (`mise run up`), the binary built, and a throwaway
database and container of its own. The tour runs against `blobfs_tour` and never touches the
compose stack's default `app` database.

```bash
cd experiments/blobfs
mise exec -- go build -o /tmp/blobfs ./cmd/blobfs

DB=blobfs_tour CT=blobtour        # the database and the object-store container
DB2=blobfs_tour2 CT2=blobtour2    # a second configuration, used in step 11
docker exec blobfs-postgres psql -U app -d app -c "CREATE DATABASE $DB" -c "CREATE DATABASE $DB2"

bfs() {
  BLOBFS_STORAGE_ENDPOINT=http://127.0.0.1:10000/devstoreaccount1 \
  BLOBFS_STORAGE_ACCOUNT=devstoreaccount1 \
  BLOBFS_STORAGE_KEY='Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==' \
  BLOBFS_STORAGE_CONTAINER=$CT \
  /tmp/blobfs --dsn "postgres://app:app@127.0.0.1:5434/$DB?sslmode=disable" "$@"
}
bfs2() { DB=$DB2 CT=$CT2 bfs "$@"; }
```

The account and key are the Azurite emulator's published development credentials. Run the
binary as above and not through `mise run cli`: `mise.toml` sets `BLOBFS_DSN` to the `app`
database, and mise's value overrides what you pass on the command line.

Three points on reading the output:

- A success prints one line to stdout, and `ls` prints a table.
- An error is a chain from the outermost operation to the innermost cause, so read its last
  segment first.
- Every file command runs `Store.Verify` (in `domain/files/commands.go`) before it does anything.
  It prepares each SQL statement against the database.

## What the two stores hold

Postgres holds the tree and each file's metadata: names, status, size, and the key of the stored
object. The Azurite object store holds the file bytes. The `lib/blobfs` library touches only
Postgres and never calls the object store. The consumer application in `domain/files` sequences
the two stores, and its `Store` type is where each command's logic lives.

## Step 1: applying the schema

The application has two migration sets: the `blobfs` tables, then the consumer's tables
(`directory_owner` and `bookmark`). Each set has its own history table. `schema up` applies both
in that order under one lock, and `schema down` reverts them in the opposite order. `schema reset
--yes` also drops the history tables and refuses without `--yes`.

Files:

- `admin/schema/commands.go` and `admin/schema/database.go` define the `schema` command and the
  two sets.
- `lib/migrator/migrator.go` runs several sets under one lock.
- `lib/blobfs/migrations/postgres/0001_directory`, `0002_file`, and `0003_file_created_index`
  (`.up.sql` and `.down.sql`) hold the `blobfs` DDL.
- `migrations/postgres/` holds the consumer's DDL.

```bash
bfs ls /            # refused: the schema is not applied
bfs schema status
bfs schema up
bfs schema status
bfs ls /
```

The refusal comes from `Store.Verify`. It prints one line per failing statement, so read the
first line, which names `schema up`.

## Step 2: creating and finding directories

A directory is a row in `blobfs_directory` with a `parent_id` and a `name`. The first migration
seeds one root row with the nil UUID and no name, and every path starts there. A path resolves
one segment at a time from the root, using one `directory_child` query per segment. `mkdir`
creates only the last segment, so a missing parent is an error.

Files:

- `lib/blobfs/migrations/postgres/0001_directory.up.sql` defines the constraints: one root, a
  unique `(parent_id, name)`, and no cascading delete.
- `ResolveDirectory` in `lib/blobfs/data/paths.go`.
- `Mkdir` in `lib/blobfs/data/directories.go`, and the consumer's `Mkdir` in
  `domain/files/blobfs.go`.

```bash
bfs mkdir /reports
bfs mkdir /reports/2026
bfs mkdir /a/b          # refused: the parent /a does not exist
for n in alpha bravo charlie delta echo foxtrot; do bfs mkdir /reports/$n; done
bfs ls /reports
```

## Step 3: listing, paging, sorting, and totals

`ls` prints two halves, directories first and then files. Each half is its own page with its own
summary line. A page's rows and its total come from one SQL statement. The total is a window
count, `COUNT(*) OVER ()`, in the same select list, so the total and the page cannot disagree.
`--total none` drops the count and the cost that goes with it.

The Go code appends the sort, filter, and page clauses to an authored statement. The library
does this itself, because `sqlate`'s projection cannot express a listing anchored on one
directory. `ls` runs the path resolution and both halves in one read-only repeatable-read
transaction.

Files:

- `lib/blobfs/data/statements/files_in_directory_with_total.sql` and
  `children_of_directory_with_total.sql`.
- `lib/blobfs/data/listing.go` is the composer.
- `List` in `domain/files/blobfs.go`.
- `output/rows.go` renders the table.

```bash
bfs ls /reports --size 3 --page 2
bfs ls /reports --sort name:desc --size 3
bfs ls /reports --size 3 --total none
```

## Step 4: cursors

Page numbers use `OFFSET`, which reads and discards every earlier row. A cursor continues from
the last row of the previous page instead. Each page that has a next page prints `next-dirs:` or
`next-files:` with an opaque cursor. The cursor is base64 of a checksum plus JSON that names the
statement that issued it, the sort, and the last row's sort values. A cursor is refused if it was
edited, came from the other half, or was issued under a different sort. A sort on a column that
can be NULL, such as `size`, issues no cursor.

Files: `lib/blobfs/data/cursor.go`, and the paging logic in `lib/blobfs/data/listing.go`.

```bash
C=$(bfs ls /reports --size 3 --total none | sed -n 's/^next-dirs: //p')
bfs ls /reports --size 3 --after-dirs "$C"
bfs ls /reports --size 3 --after-dirs "${C}x"                  # refused: edited
bfs ls /reports --size 3 --after-files "$C"                    # refused: wrong half
bfs ls /reports --size 3 --sort name:desc --after-dirs "$C"    # refused: different sort
```

## Step 5: scoping a listing to a unit

The library has no concept of an owner, so ownership is the consumer's own table.
`directory_owner(directory_id, unit_id)` binds a top-level directory to a unit. `mkdir --unit`
works only at top level and writes the directory and the owner row in one transaction. `ls
<path> --unit` reads the owner row of the path's top-level ancestor once and refuses if the unit
does not own it. At `/`, the listing is the unit's own top-level directories, read through a
projection joined to `directory_owner`, and it takes no cursor.

Files:

- `migrations/postgres/` holds the consumer's `directory_owner` table.
- `domain/files/statements/owned_directories.sql` is the projection.
- `List` and `topLevel` in `domain/files/blobfs.go`.

```bash
U1=11111111-1111-4111-8111-111111111111; U2=22222222-2222-4222-8222-222222222222
bfs mkdir /acme --unit $U1
bfs mkdir /acme/docs
bfs mkdir /globex --unit $U2
bfs mkdir /acme/docs/deep --unit $U1    # refused: --unit applies to a top-level directory only
bfs ls /acme --unit $U1
bfs ls /acme/docs --unit $U2            # refused: U2 does not own /acme
bfs ls / --unit $U1
bfs ls / --unit $U2
```

## Step 6: writing and reading files

A write cannot be one transaction, because Postgres and the object store cannot commit
together. `put` runs three steps:

1. In one transaction that commits first, it inserts the file row with status `pending`.
2. Outside any transaction, it writes the bytes to the object store under the row's key,
   `<id>/<name>`.
3. On the connection pool, it marks the row `available` and records the size and etag.

A stop between the steps leaves a `pending` row. `stat` shows it, and `cat` refuses it. A second
`put` of the same path finds that row and resumes it. `--fail-after insert|write` injects the
stop.

Files:

- `Put` in `domain/files/blobfs.go` sequences the three steps.
- `BeginFileWrite` and `CompleteFileWrite` in `lib/blobfs/data/files.go`.
- `lib/blobfs/status.go` defines the `pending`, `available`, and `deleting` statuses.
- `lib/blobfs/key.go` builds keys.
- `domain/files/storage.go` is the one file that imports the object-store library.

```bash
echo 'quarterly numbers' > /tmp/blobfs-q1.txt
bfs put /tmp/blobfs-q1.txt /reports/q1.txt
bfs cat /reports/q1.txt
bfs stat /reports/q1.txt
bfs put /tmp/blobfs-q1.txt /reports/q1.txt         # refused: name taken
echo draft | bfs put - /reports/draft.md --fail-after insert
bfs stat /reports/draft.md                         # status: pending
bfs cat /reports/draft.md                          # refused
echo draft | bfs put - /reports/draft.md           # resumes the pending row
echo second | bfs put - /reports/two.txt --fail-after write
echo second | bfs put - /reports/two.txt
bfs ls /reports --size 2
bfs ls /reports --size 2 --sort size:desc          # sorts the files; directories stay in name order
```

## Step 7: moving and renaming

`mv` reads its destination the way Unix does: an existing directory receives the source, and any
other path is the new path. A directory move runs three statements in one transaction:

1. It takes the variant's tree lock.
2. It runs the cycle check, a walk upward from the new parent that looks for the moved
   directory.
3. It runs a guarded update of `parent_id` and `name`.

Children follow by id, and no object moves, because a key never encodes a path. Every move stays
under one top-level directory, because ownership is checked at that ancestor. A top-level
directory can be renamed, and its owner row stays.

Files:

- `Move` in `domain/files/blobfs.go`.
- `MoveDirectory` and `MoveFile` in `lib/blobfs/data/move.go`.
- `lib/blobfs/data/statements/directory_is_within.sql` and `reparent_directory.sql`.

```bash
bfs mv /reports/q1.txt /reports/2026
bfs mv /reports/2026/q1.txt /reports/2026/q1-final.txt
bfs stat /reports/2026/q1-final.txt                # the key still ends in /q1.txt
bfs mv /reports/2026 /reports/alpha
bfs ls /reports/alpha/2026
bfs mv /reports/alpha /reports/alpha/2026          # refused: cycle
bfs mv / /reports                                  # refused: root
bfs mv /reports/alpha /acme                        # refused: crosses top-level directories
bfs mv /globex /globex-inc                         # allowed: a rename keeps the owner
bfs ls / --unit $U2
```

## Step 8: deleting files and directories

A file delete has the same three-step shape as a write:

1. `rm` marks the row `deleting`. It refuses first if any unit bookmarks the file.
2. It deletes the object.
3. It removes the row.

A `deleting` row keeps its name, so `put` refuses that name, and `cat` refuses the file. A rerun
of `rm` finishes from wherever the earlier run stopped. `rmdir` removes an empty directory only,
which the two foreign keys enforce because nothing cascades. `rm -r` is the consumer's own walk,
which removes children before their parent.

Files:

- `Remove`, `deleteFile`, `RemoveDirectory`, and `RemoveTree` in `domain/files/blobfs.go`.
- `BeginFileDelete` in `lib/blobfs/data/variant.go`.
- `lib/blobfs/data/statements/remove_file.sql` and `remove_directory.sql`.

```bash
bfs rm /reports/two.txt --fail-after begin
bfs stat /reports/two.txt                          # status: deleting
bfs cat /reports/two.txt                           # refused
echo x | bfs put - /reports/two.txt                # refused: the name is still held
bfs rm /reports/two.txt                            # finishes the delete
bfs rm /reports/draft.md --fail-after object
bfs rm /reports/draft.md                           # finishes
bfs rm /reports/draft.md                           # refused: not found
bfs rmdir /reports/alpha                           # refused: not empty
bfs rmdir /reports/bravo
bfs rmdir /                                        # refused: root
bfs rm -r /reports
```

## Step 9: bookmarks

A bookmark binds a file to a unit in the consumer's `bookmark` table. A partial unique index
allows one active bookmark per unit. The foreign key into `blobfs_file` also stops a delete of a
bookmarked file. The library reports which constraint fired, and `domain/files/database.go` maps
the constraint's name to the consumer's own error. `bookmark ls` computes each file's full path
per row with a correlated recursion.

Files:

- `AddBookmark` in `domain/files/blobfs.go`.
- `domain/files/statements/create_bookmark.sql` and `bookmarks.sql`.
- `domain/files/database.go` holds the constraint-to-error tables.

```bash
echo a | bfs put - /acme/docs/logo.png
echo b | bfs put - /acme/docs/terms.pdf
bfs bookmark add /acme/docs/logo.png --unit $U1 --active
bfs bookmark add /acme/docs/terms.pdf --unit $U1 --active   # refused: one active bookmark per unit
bfs bookmark add /acme/docs/terms.pdf --unit $U1
bfs bookmark ls --unit $U1
bfs rm /acme/docs/logo.png                                   # refused: bookmarked
bfs bookmark rm /acme/docs/logo.png --unit $U1
bfs rm /acme/docs/logo.png
```

## Step 10: the two variants

The persistence package runs 20 statements that any SQL engine can run. Together they form the
standard variant. Two operations can be replaced by engine-specific code through the
`data.Variant` interface, and `pgnative` is the Postgres variant that replaces both:

- The tree lock. `pgnative` takes `pg_advisory_xact_lock`, so two opposing moves cannot both
  pass their cycle checks. The standard variant's lock does nothing, and a caller on that
  variant serializes directory moves itself.
- The file-delete begin. `pgnative` runs one `UPDATE ... RETURNING`, which saves one round trip.

The composition root chooses the variant from `--variant` or `BLOBFS_VARIANT`. The lock's key is
the 64-bit FNV-1a hash of `blobfs_directory.tree`, exported as `pgnative.TreeLockKey`.

Files:

- `lib/blobfs/data/variant.go` defines the `Variant` interface and the standard variant.
- `lib/blobfs/data/pgnative/pgnative.go` with `statements/lock_tree.sql` and
  `statements/begin_file_delete.sql`.
- `variantOptions` in `internal/app/domain.go`.

To see the difference from outside, hold the lock from another session and time a move on each
variant:

```bash
bfs mkdir /lock; for d in s1 s2 p1 p2; do bfs mkdir /lock/$d; done
KEY=-8521165719926625175
hold() {
  docker exec blobfs-postgres psql -q -U app -d $DB \
    -c "BEGIN; SELECT pg_advisory_xact_lock($KEY); SELECT pg_sleep(6); COMMIT;" >/dev/null &
  sleep 1
}
hold; time bfs mv /lock/s1 /lock/s2                        # standard: returns at once
wait
hold; time bfs --variant pgnative mv /lock/p1 /lock/p2     # pgnative: waits about 5 seconds
wait
```

## Step 11: a second configuration

An install is one database and one container. A second isolated tree is a second configuration,
which means another DSN and another container. Nothing in the library or the consumer names
either one.

```bash
bfs2 schema up
bfs2 mkdir /docs
echo in-B | bfs2 put - /docs/a.txt
bfs mkdir /docs
echo in-A | bfs put - /docs/a.txt
bfs cat /docs/a.txt          # in-A
bfs2 cat /docs/a.txt         # in-B
bfs ls /
bfs2 ls /
```

## Step 12: the upgrade rehearsal

Migration 3 is a real upgrade over an installed database. The test installs the set cut at two
migrations and seeds rows. It then opens a new migrator over all three migrations, applies only
migration 3, and checks that every row survived.

Files: `TestUpgradeAfterRestart` in `lib/migrator/migrator_integration_test.go`, and
`lib/blobfs/migrations/postgres/0003_file_created_index.up.sql`.

```bash
(cd lib/migrator && mise exec -- go test -tags integration -count=1 -run '^TestUpgradeAfterRestart$' -v .)
```

## Cleaning up

```bash
docker exec blobfs-postgres psql -U app -d app -c "DROP DATABASE $DB" -c "DROP DATABASE $DB2"
rm /tmp/blobfs /tmp/blobfs-q1.txt
```

The object store has no container delete in the CLI, so the `blobtour` and `blobtour2`
containers stay in Azurite until `mise run reset` drops the stack's data.
