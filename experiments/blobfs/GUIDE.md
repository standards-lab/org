# blobfs guided tour

A walk through what the `blobfs` command-line file system and its library do, one capability at a
time. Each step gives a summary, the files that implement it, and commands to run yourself.
`README.md` is the short reference for starting the stack and the commands, and `REVIEW.md`
records the decisions behind the design and the findings. All paths in
this guide are relative to `experiments/blobfs`.

## Setup

The tour needs the compose stack running (`mise run up`), the binary built, and a throwaway
database and container of its own. The tour runs against `blobfs_tour` and never touches the
compose stack's default `app` database.

```bash
cd experiments/blobfs
mise exec -- go build -o /tmp/blobfs ./cmd/blobfs

DB=blobfs_tour CT=blobtour        # the database and the object-store container
DB2=blobfs_tour2 CT2=blobtour2    # a second configuration, used in step 16
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
database, and mise's value overrides what you pass on the command line. A database that an earlier
commit installed holds a root directory with no name, so use fresh databases.

Three points on reading the output:

- A success prints one line to stdout, and `ls` prints a table.
- An error is a chain from the outermost operation to the innermost cause, so read its last
  segment first. A refusal that a database constraint caused ends with the reason and the
  constraint's name, such as `blobfs: name taken (constraint blobfs_uq_directory_parent_name)`.
- Every file command runs `Store.Verify` (in `domain/files/database.go`) before it does anything.
  It prepares each SQL statement against the database.

## What the two stores hold

Postgres holds the tree and each file's metadata: names, status, size, and the key of the stored
object. The Azurite object store holds the file bytes. The `lib/blobfs` library touches only
Postgres and never calls the object store. The consumer application in `domain/files` sequences
the two stores, and its `Store` type is where each command's logic lives.

## Step 1: applying the schema

The application has two migration sets: the `blobfs` tables, then the consumer's tables
(`directory_owner` and `bookmark`). Each set is a layer with its own name, history table, and
version numbers. `schema up` applies both in that order under one lock, and `schema down`
reverts them in the opposite order. `schema reset --yes` also drops the history tables and
refuses without `--yes`. The `blobfs` set ships two migrations, `directory` and `file`. It ships
no index on `created_at`, and the package comment of `lib/blobfs/postgres` says what that index
costs and what it buys, so a consumer adds it in its own set.

Files:

- `admin/schema/commands.go` and `admin/schema/database.go` define the `schema` command and the
  two sets.
- `lib/migrator/migrator.go` runs several sets under one lock. It needs two pool connections at
  once, which `lib/migrator/doc.go` explains.
- `lib/blobfs/postgres/migrations.go` returns the `blobfs` set, and
  `lib/blobfs/postgres/migrations/0001_directory` and `0002_file` (`.up.sql` and `.down.sql`) hold
  its DDL.
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
seeds one root row with the nil UUID and the name `/`, and every path starts there. A path
resolves one segment at a time from the root, using one `directory_child` query per segment on
the standard variant. `mkdir` creates only the last segment, so a missing parent is an error.

Files:

- `lib/blobfs/postgres/migrations/0001_directory.up.sql` defines the constraints: one root, a
  unique `(parent_id, name)`, and no cascading delete.
- `ResolveDirectory` and `ResolveDirectoryFrom` in `lib/blobfs/data/paths.go`.
- `Mkdir` in `lib/blobfs/data/directories.go`, and the consumer's `Mkdir` in
  `domain/files/blobfs_write.go`.

```bash
bfs mkdir /reports
bfs mkdir /reports/2026
bfs mkdir /a/b          # refused: the parent /a does not exist
bfs mkdir /reports      # refused: name taken (constraint blobfs_uq_directory_parent_name)
for n in alpha bravo charlie delta echo foxtrot; do bfs mkdir /reports/$n; done
bfs ls /reports
bfs stat /reports       # a directory's row: path, id, parent, name, version, timestamps
```

## Step 3: listing, paging, sorting, filtering, and totals

`ls` prints two halves, directories first and then files. Each half is its own page with its own
summary line, and each row ends with its id. A page's rows and its total come from one SQL
statement. The total is a window count, `COUNT(*) OVER ()`, in the same select list, so the total
and the page cannot disagree. `--total none` drops the count and the cost that goes with it. Each
half also prints `more: yes` or `more: no`, which says whether rows remain after the page,
whatever the total says.

The Go code appends the filter, sort, and page clauses to an authored statement. The library does
this itself, because `sqlate`'s projection cannot express a listing anchored on one directory.
`ls` runs the path resolution and both halves in one read-only repeatable-read transaction.

`--filter <field>:<op>:<value>` is repeatable. A filter applies to the file half when the file
listing declares its field, and to the directory half only for the fields both listings declare
(`id`, `name`, `version`, `created_at`, `updated_at`), the rule that `--sort` follows.

Files:

- `lib/blobfs/data/statements/files_in_directory_with_total.sql` and
  `children_of_directory_with_total.sql`.
- `lib/blobfs/data/listing.go` is the composer, and `Page` there carries `Total`, `More`, and
  `Next`.
- `List` and `ListDirectory` in `domain/files/blobfs_read.go`.
- `output/rows.go` renders the table.

```bash
bfs ls /reports --size 3 --page 2
bfs ls /reports --sort name:desc --size 3
bfs ls /reports --size 3 --total none
bfs ls /reports --filter name:like:%a% --filter name:ne:alpha
bfs ls /reports --filter nosuchfield:eq:1       # refused by the library before any query
```

## Step 4: cursors and the three page states

Page numbers use `OFFSET`, which reads and discards every earlier row. A cursor continues from
the last row of the previous page instead. With `--cursors`, each half that has a next page prints
`next-dirs:` or `next-files:` with an opaque cursor. The cursor is base64 of a checksum plus JSON
that names the statement that issued it, the sort, and the last row's sort values. A cursor is
refused if it was edited, came from the other half, or was issued under a different sort.

A page reads as one of three states. `more: no` means no rows remain. `more: yes` with a cursor
means continue by cursor. `more: yes` with no cursor means the sort cannot be continued, such as
a sort on a column that can be NULL like `size`, so read the next page by number.

Files: `lib/blobfs/data/cursor.go`, and the paging logic in `lib/blobfs/data/listing.go`.

```bash
bfs ls /reports --size 3 --total none --cursors
C=$(bfs ls /reports --size 3 --total none --cursors | sed -n 's/^next-dirs: //p')
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

The id forms of the `Store` check a client-named scope instead. `InScope` reads the owner row of
the scope directory first, so a unit that names a directory it does not own learns nothing, and
then asks the library whether the target lies within it. The supplied scope id is an input to
check, not to trust. The tool passes no scope for an id form, so the tests cover this check.

Files:

- `migrations/postgres/` holds the consumer's `directory_owner` table.
- `domain/files/statements/owned_directories.sql` is the projection.
- `List` and `topLevel` in `domain/files/blobfs_read.go`, and `InScope` in
  `domain/files/blobfs_scope.go`.

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

The scope check by id has its own tests:

```bash
mise exec -- go test -tags integration -count=1 -run '^TestScopeByID$' -v ./domain/files
```

## Step 6: writing and reading files

A write cannot be one transaction, because Postgres and the object store cannot commit
together. `put` runs three steps:

1. In one transaction that commits first, it inserts the file row with status `pending`, or takes
   up a `pending` row that an earlier attempt left.
2. Outside any transaction, it writes the bytes to the object store under the row's key,
   `<id>/<name>`.
3. On the connection pool, it marks the row `available` and records the size and etag.

A stop between the steps leaves a `pending` row. `stat` shows it, and `cat` refuses it. A second
`put` of the same path finds that row and resumes it. `--fail-after insert|write` injects the
stop. The find-or-begin logic is the library's `BeginOrResumeFileWrite`, so `put`, `cp`, and a
service's seeder share one write protocol.

Files:

- `Put` in `domain/files/blobfs_write.go` sequences the three steps.
- `BeginOrResumeFileWrite` and `CompleteFileWrite` in `lib/blobfs/data/files.go`.
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

## Step 7: copying files

`cp` copies one file. The destination reads the way `mv` reads it: an existing directory receives
the copy under the source's name, and any other path is the copy's path, whose parent must exist.
The source must be an `available` file. A directory, a `pending` file, and a `deleting` file are
refused before the destination is read. A destination name that a file holds is refused as taken,
and nothing is overwritten. The bytes stream through the process from the source object to a new
object under the new row's key, around the same begin and complete steps as `put`. The copy
takes the source's content type, and its size and etag are what the store reports for the new
object. Bookmarks and owner rows stay with the source, and a copy may cross top-level
directories. Directory copy is not offered.

Files:

- `Copy`, `CopyFile`, and `finishCopy` in `domain/files/blobfs_write.go`, which share `beginPut`
  and `finishPut` with `Put`.
- The `copy` command in `domain/files/commands.go`.
- `domain/files/copy_test.go` and `copy_integration_test.go`, and the `copies` step of
  `integration/integration_test.go`.

```bash
bfs mkdir /c; bfs mkdir /c/src; bfs mkdir /c/dst
echo copy-me | bfs put - /c/src/f.txt
bfs cp /c/src/f.txt /c/dst                          # keeps the name
bfs cp /c/src/f.txt /c/dst/g.txt                    # a new name
bfs cp /c/src/f.txt /reports                        # across top-level directories
bfs stat /c/src/f.txt; bfs stat /c/dst/f.txt        # separate ids and keys, equal size
bfs cat /c/dst/g.txt
bfs cp /c/src /c/dst                                # refused: the source is a directory
bfs cp /c/src/f.txt /c/dst                          # refused: name taken
bfs cp /c/src/f.txt /c/dst/h.txt --fail-after insert
bfs stat /c/dst/h.txt                               # status: pending
bfs cp /c/src/f.txt /c/dst/h.txt                    # resumes the pending row
```

## Step 8: moving and renaming

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

- `Move` and `MoveEntry` in `domain/files/blobfs_move.go`.
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

## Step 9: deleting files and directories

A file delete has the same three-step shape as a write:

1. In one transaction, `rm` marks the row `deleting`, and then it refuses and rolls back the mark
   if any unit bookmarks the file.
2. It deletes the object.
3. It removes the row.

A `deleting` row keeps its name, so `put` refuses that name, and `cat` refuses the file. A rerun
of `rm` finishes from wherever the earlier run stopped. `rmdir` removes an empty directory only,
which the two foreign keys enforce because nothing cascades. `rm -r` is the consumer's own walk,
which removes children before their parent.

Files:

- `Remove`, `RemoveFile`, `deleteFile`, `RemoveDirectory`, and `RemoveTree` in
  `domain/files/blobfs_delete.go`.
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

## Step 10: bookmarks and the reference-then-delete rule

A bookmark binds a file to a unit in the consumer's `bookmark` table. A partial unique index
allows one active bookmark per unit. The foreign key into `blobfs_file` also stops a delete of a
bookmarked file. The library reports which constraint fired, and
`domain/files/database_bookmarks.go` maps the constraint's name to the consumer's own error.

A bookmark added while the file is being deleted, or a delete begun while the bookmark is being
added, would leave a bookmark on a file whose object is gone. The library closes that race with
`HoldFile`, a guarded update that takes the file's row lock and changes no value and no version.
The rule for a consumer is reference-then-delete: hold the file in the transaction that inserts a
row referencing it. `AddBookmark` holds the file before it inserts, and `deleteFile` begins the
delete before it reads the bookmark count, so the two operations serialize on the row. Whichever
starts first wins: a delete that began first makes the add refuse a `deleting` file, and an add
that held first commits its bookmark before the delete reads the count, which then refuses.
`bookmark ls` computes each file's full path per row only when it prints one, and its rows carry
the file's id and the directory's id.

Files:

- `AddBookmark` in `domain/files/database_bookmarks.go`, and `HoldFile` in
  `lib/blobfs/data/files.go` with `statements/hold_file.sql`.
- `domain/files/statements/create_bookmark.sql`, `bookmarks.sql`, and
  `bookmarks_with_paths.sql`, the second of which holds the path recursion.
- `domain/files/database_bookmarks.go` holds the constraint-to-error tables.

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

The two interleavings of the race run against Postgres with real concurrent transactions:

```bash
mise exec -- go test -tags integration -count=1 \
  -run '^(TestBookmarkAddedDuringTheDeleteIsRefused|TestDeleteDuringTheBookmarkAddIsRefused)$' \
  -v ./domain/files
```

## Step 11: ids as the handle

Ids are the primary handle in the library and in the consumer above it. A listing row carries the
entry's id and version, so a caller that holds a row acts without a read: the library's guarded
steps take the version the caller read. Paths are entry points and display. The `Store` has an id
form beside each path form: `ListDirectory`, `StatFile`, `OpenFile`, `PutFile`, `MoveEntry`,
`RemoveFile`, and `CopyFile`, each taking an optional scope. In the tool, `id:<uuid>` stands in
for a path in `ls`, `stat`, `cat`, and `rm`, and for a destination directory in `put`, `cp`, and
`mv`. `stat` reads a directory as well as a file, and it prints a file when a file and a
directory share a name, because the library keeps the two kinds in separate name spaces. The
library's `ResolveDirectoryFrom` resolves a relative path below a directory id, so a caller that
holds an id walks from there and not from the root.

Files:

- `domain/files/blobfs_read.go`, `blobfs_write.go`, `blobfs_move.go`, and `blobfs_delete.go`
  hold the id forms beside the path forms.
- `ParseRef` in `domain/files/commands.go` parses the `id:<uuid>` form.
- `ResolveDirectoryFrom` in `lib/blobfs/data/paths.go`.
- `domain/files/ids_test.go` and `ids_integration_test.go`, and the `ids` step of
  `integration/integration_test.go`.

```bash
bfs mkdir /ids; bfs mkdir /ids/src; bfs mkdir /ids/dst; bfs mkdir /ids/sub
echo by-id | bfs put - /ids/src/f.txt
bfs ls /ids                                        # copy the ids of src, dst, and sub
bfs ls /ids/src                                    # copy the file's id
F=<the file id>; SRC=<id of /ids/src>; DST=<id of /ids/dst>; SUB=<id of /ids/sub>
bfs ls id:$SRC
bfs stat id:$F
bfs stat id:$SRC                                   # a directory, by id
bfs cat id:$F
bfs cp id:$F id:$DST                               # a copy in /ids/dst under the same name
bfs mv id:$F id:$SUB                               # the move keeps the name and the id
bfs stat id:$F                                     # the same id, now in /ids/sub
bfs rm id:$F
bfs stat id:not-a-uuid                             # refused before any query
```

## Step 12: constraint errors

A database constraint that the library or the consumer classifies becomes a `ViolationError`.
Its message prints the sentinel and the constraint name and never the driver's text. `errors.Is`
matches the sentinel, and `errors.As` reaches the `sqlate.ConstraintError` beneath it. Steps 2, 7,
and 10 show refusals of this kind. A violation that no classifier maps, such as a check
constraint, still passes through with the driver's text.

Files:

- `ViolationError` in `lib/blobfs/errors.go`.
- `classifyWrite` and `classifyDelete` in `lib/blobfs/data/errors.go`, and `classifyBookmark` and
  `classifyFileDelete` in `domain/files/database_bookmarks.go`.

```bash
bfs mkdir /reports 2>&1 | tail -1
bfs rmdir /c 2>&1 | tail -1                        # not empty (constraint blobfs_fk_directory_parent)
```

## Step 13: the two variants

The persistence package runs 22 statements that any SQL engine can run. Together they form the
standard variant, which is the reference semantics and the fallback. An engine is a package that
a consumer selects by importing it, and `lib/blobfs/postgres` is the Postgres engine. Its variant
replaces seven operations through the `data.Variant` interface:

- The tree lock. The variant takes `pg_advisory_xact_lock`, so two opposing moves cannot both
  pass their cycle checks. The standard variant's lock does nothing, and a caller on that variant
  serializes directory moves itself.
- The file-delete begin, as one `UPDATE ... RETURNING`.
- The write steps: `INSERT ... RETURNING` for a file's begin and for `Mkdir`, and `UPDATE ...
  RETURNING` for the complete step. Each is one round trip where the baseline runs two, and a
  refused complete step is two round trips where the baseline runs three.
- Path resolution as one recursive statement over the segments, which is one round trip where
  the baseline runs one per segment plus one.
- The keyset predicate as a row-value comparison, which stays cheap at any cursor position where
  a sort column has an index. The baseline renders the expanded OR chain.

Each variation point shipped only after a measurement showed a win, and the measurements are in
`evidence/native-variation/`. The conformance suite runs every operation over both variants and
compares the rows and the error text, so the two behave identically. The composition root chooses
the variant from `--variant` or `BLOBFS_VARIANT`. The lock's key is the 64-bit FNV-1a hash of
`blobfs_directory.tree`, exported as `postgres.TreeLockKey`.

Files:

- `lib/blobfs/data/variant.go` defines the `Variant` interface and the standard variant.
- `lib/blobfs/postgres/postgres.go` with the statements under `lib/blobfs/postgres/statements/`,
  each carrying its port note.
- `lib/blobfs/data/datatest/` is the conformance suite.
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
hold; time bfs --variant postgres mv /lock/p1 /lock/p2     # postgres: waits about 5 seconds
wait
```

The round trips of each variant are asserted by hermetic tests over a scripted driver:

```bash
mise exec -- go test -count=1 -run 'RoundTrips|IsOneStatement|Walk' -v ./lib/blobfs/data ./lib/blobfs/postgres
```

## Step 14: the cost assertions

Seven integration tests explain a statement with `EXPLAIN (ANALYZE, BUFFERS)` on a seeded fixture
and assert a plan shape and a buffer bound, so a plan regression fails a test. They cover the
cursor page, the row-value cursor with an index, the exact-total page, one-statement path
resolution, one step of the standard walk, and the primary-key lookups of the protocol steps.
Each bound sits well above the measured value and well below what the regression would read, and
no test asserts a time. Each test logs its measured value under `-v`.

Files: `lib/blobfs/data/cost_integration_test.go`, `lib/blobfs/postgres/cost_integration_test.go`,
and the explain runner and fixture builder in `internal/livetest/`.

```bash
mise exec -- go test -tags integration -count=1 -run 'Cost|StepPlans' -v ./lib/blobfs/data ./lib/blobfs/postgres
```

## Step 15: seeding operations

A service seeds named states through the library. The library provides the operations that make
a seed safe to repeat, and the tool exposes none of them, so the tests are the way to see them.
`EnsureDirectory` returns a directory and whether the call created it. `BeginOrResumeFileWrite`
returns a file and an outcome: created, resumed from a `pending` row, or already present. `WithID`
supplies the id of the row an operation inserts, so a seeded directory or file keeps its id
across resets, and a re-seeded file reuses its key. Both operations look the name up first, so
they compose into a caller's transaction, and a lost race on the pool is resolved by a second
lookup.

Files: `lib/blobfs/data/directories.go`, `files.go`, and `ids.go`, with `seed_test.go` and
`seed_integration_test.go`.

```bash
mise exec -- go test -tags integration -count=1 \
  -run 'EnsureDirectory|BeginOrResumeFileWrite|SuppliedIDs' -v ./lib/blobfs/data
```

## Step 16: a second configuration

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

## Step 17: the upgrade rehearsal

The `blobfs` set ships two migrations, so the migrator's tests keep a fixture migration of their
own as a third. The test installs the real set and seeds rows. It then opens a new migrator over
the set with the fixture appended, applies only the fixture, and checks that every row survived.

Files: `TestUpgradeAfterRestart` in `lib/migrator/migrator_integration_test.go`.

```bash
mise exec -- go test -tags integration -count=1 -run '^TestUpgradeAfterRestart$' -v ./lib/migrator
```

## What the tour does not cover

The tour follows the command-line tool. These parts of the experiment have no command:

- The library's seeding operations, `HoldFile`, `WithID`, and `ResolveDirectoryFrom`, and the
  id-keyed `Store` methods with a scope. Steps 5, 10, 11, and 15 name the tests that exercise
  them.
- The interleavings of two concurrent transactions. Step 10 runs the two tests that force them.
- The cost measurements. They live in `evidence/`, with a `README.md` that says what each file
  measures and how to rerun it.
- Directory copy, a search that descends a subtree, replacing a file's content, and versioning.
  The concept defers them, and the library and tool offer none.

## Cleaning up

```bash
docker exec blobfs-postgres psql -U app -d app -c "DROP DATABASE $DB" -c "DROP DATABASE $DB2"
rm /tmp/blobfs /tmp/blobfs-q1.txt
```

The object store has no container delete in the CLI, so the `blobtour` and `blobtour2`
containers stay in Azurite until `mise run reset` drops the stack's data.
