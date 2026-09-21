# blobfs experiment review

The organized record of `blobfs.experiment`, written at stage 16 for the architect's review.
`NOTES.md` is the chronological log the stages appended to; this file is the same material
organized by the three findings the experiment exists to produce. Its sections describe the
experiment as it stood at stage 16. The review's decisions, and the stages 17 to 32 that
implemented them, are in `DECISIONS.md` and the last section of `NOTES.md`; where a decision
changed a claim below, the passage says so. The sources of truth are the
code and the transcripts under `evidence/`, and every claim here names the test, statement, or
transcript section that supports it. Where a later stage corrected an earlier entry of
`NOTES.md`, this file states the corrected position and says so.

## Summary

The experiment built `blobfs`, a library that keeps a virtual directory tree and file metadata in
SQL for an object store, and a command-line file system that uses it. The library never calls the
object store. The experiment ran against Postgres 18 and Azurite, over published `sqlate` v0.1.1
and `go-storage` v0.1.0.

The library has these packages:

- `lib/blobfs` is the root package of entities, vocabulary, and errors.
- `lib/blobfs/data` is the persistence package.
- `lib/blobfs/migrations` is the migration source.
- `lib/blobfs/data/pgnative` is the Postgres variant.
- `lib/blobfs/data/datatest` is the conformance suite.
- `lib/migrator` is the multi-set migrator.

The consumer is an application under `cmd/`, `internal/`, `domain/`, `admin/`, `migrations/`, and
`output/`.

The experiment ran in sixteen stages. Stages 1 to 5 built a design with a volume table, and the
architect then removed the volume. Stages 6 to 15 rebuilt the schema around one seeded root and
added the listing, the variant seam, the write, bookmark, delete, and move paths, the migrator,
and the integration tier. Every stage was committed after its gates passed, and stage 16 is this
record.

The conclusions that matter most:

- The standard-tier baseline is complete on any engine, and only two operations need native SQL.
- The listing is one statement anchored on a directory, with its total in the same select list.
  `sqlate`'s projection cannot express it.
- A file's path stays a read-time computation, and the library stores no path.
- Ownership is a join in the consumer's tables, and the library holds no unit.
- The multi-set migrator ships integrated, and the `deleting` status is required.
- On the baseline, the tree lock is a requirement on the caller.

The decisions that need the architect are in "Decisions for the architect".

## Finding 1: the library

### The layers and their import boundaries

The library is one module with three layers, as the concept states, and the experiment adds two
packages beside the second layer.

| Package | What it holds | What it may import |
|---------|---------------|--------------------|
| `lib/blobfs` | The entity types `Directory`, `File`, and `Object`; `RootID`; the `Status` vocabulary and the transition table; `NewKey` and `SanitizeFilename`; `NormalizeName` and `ValidateName`; the `KeyValidator` interface; the constraint-name constants; the sentinel errors. | The standard library and `golang.org/x/text/unicode/norm`. Neither `sqlate` nor `go-storage`. |
| `lib/blobfs/data` | Twenty standard-tier statements, the published pattern namespace `blobfs`, the listing composer, the `Store` type whose methods take a `sqlate.Session`, and the `Variant` interface with its baseline `Standard`. | `sqlate` and the root package. Never `go-storage`, never `pgnative`. |
| `lib/blobfs/data/pgnative` | The Postgres `Variant`: two native-tier statements, each with its port note. | The persistence package and `sqlate`. No driver: the lock and the returning update are plain SQL through the session. |
| `lib/blobfs/data/datatest` | The conformance suite, `Run(t, db, store)`, a non-test package so another package's tests can import it. | The persistence package and `sqlate`. Never a variant, never the migrator, and no non-test file names it. |
| `lib/blobfs/migrations` | The embedded Postgres DDL, exported as a migration source under the history table `blobfs_schema_version`. | `sqlate/migrate` only. |
| `lib/migrator` | The multi-set migrator: several `migrate.Migrator` values under one outer lock. | `sqlate` and the standard library. |

The `split-check` task in `mise.toml` enforces these boundaries with `go list -deps` over each
package and a grep for the driver's import path under `lib/`, and it enforces the consumer's
elemental layout in ten numbered rules: `cmd/blobfs` imports only `internal/app`; no root-level
application package depends on `internal/`; no package under `lib/` depends on a package outside
`lib/`; `output` depends on no domain, admin, or library package; the domain and admin packages
do not depend on each other; only `domain/files/storage.go` names `go-storage`; only a domain or
admin package's `database.go` names `sqlate/query`; cobra appears only in the command layers; only
`domain/files/blobfs.go` and `domain/files/database.go` name `lib/blobfs/data`; and only
`internal/app/domain.go` names `pgnative`. Every rule added after stage 4 was proved by a
temporary violation before it landed (`NOTES.md`, stages 9 and 15).

Two findings about the layers. The root package is thin: it compiles alone and a consumer with
its own persistence can take it, but it is vocabulary and validation, not a capability. The
concept's claim that a second engine adds a directory and not a module no longer holds once
native variants exist: a second engine adds a migrations directory and a variant package for
each native operation, and the native files' port notes are that work list.

### The schema

The schema is two tables. `blobfs_directory(id, parent_id, name, version, created_at,
updated_at)` holds exactly one root: the row with no parent and no name, seeded by
`0001_directory.up.sql` with the nil UUID `blobfs.RootID`. The check constraint
`blobfs_cc_directory_root_name` states `(parent_id IS NULL) = (name IS NULL)`, and the partial
unique index `blobfs_uq_directory_root` over the expression `(parent_id IS NULL)` allows one row
without a parent, so a second root fails as a unique violation under that index's name
(`TestOneRoot`, `TestRootRule` in `lib/blobfs/migrations`). No library operation can create a
root, because `Mkdir` always binds a parent and a validated name. `blobfs_file(id, directory_id,
name, status, key, size, content_type, etag, version, created_at, updated_at)` is the file table;
`size` and `etag` are NULL until the object exists, and the status check constraint admits
`pending`, `available`, and `deleting`. The third migration, `0003_file_created_index`, adds the
index `blobfs_ix_file_directory_created` on `blobfs_file (directory_id, created_at)` as the
upgrade rehearsal (see "The migration set and the multi-set migrator").

Constraint names are public API and follow `blobfs_<kind>_<table>_<detail>`, where `kind` is
`pk`, `fk`, `uq`, `cc`, or `ix`. The constants live in the root package (`lib/blobfs/constraints.go`)
because the persistence package imports the root and must not import the migrations package;
`TestConstraintConstants` checks every constant against the DDL. The two foreign keys into
`blobfs_directory` have no cascading action, and they are the whole guard against removing a
non-empty directory. Every row carries a `version` column, which the query library's guarded
commands check.

An `EnsureRoot` operation was the alternative to seeding the root in the migration. The seed was
chosen because it needs no lifecycle step and gives every consumer the same id, and `Root` is a
read of `RootID` through `directory_by_id`. The review should confirm it (decision 1).

### The two-tier statement model and what is native

Every statement in `lib/blobfs/data/statements` is standard tier: `TestNew` counts twenty, all
standard, and `sqlint` holds them to the standard forms (no `RETURNING`, no `::`, no `now()`, no
`LIMIT`, and the rest of the list in `context/reset.md`). The baseline is complete on any engine
`sqlate` has a dialect for.

Two operations are variation points, named by the `data.Variant` interface (`lib/blobfs/data/variant.go`):

- `LockTree(ctx, sess)` and `Serializes() bool`. The Postgres variant runs `SELECT
  pg_advisory_xact_lock($1)` under the fixed key `pgnative.TreeLockKey`, the 64-bit FNV-1a hash of
  `pgnative.TreeLockName` (`blobfs_directory.tree`), pinned by `TestTreeLockKey`. The baseline's
  `LockTree` takes no lock and `Serializes` reports false: standard SQL has no statement that holds
  a lock to commit. Both refuse a session that is not a `*sqlate.Tx` with
  `query.ErrTransactionRequired`, so a caller sees the same refusal on every variant. `Serializes`
  is the capability probe: a caller that needs moves serialized checks it before the first move
  instead of learning from a cycle.
- `BeginFileDelete(ctx, sess, id)`. The Postgres variant is one `UPDATE ... RETURNING` whose `CASE`
  expressions keep a deleting row's version and `updated_at` unchanged, so a retry converges; it
  accepts the pool (`TestBeginFileDeleteAcceptsThePool`). The baseline is the update
  `begin_file_delete` (headed `transaction: required`) and the read-back `file_by_id`, which must
  share a transaction so the read sees the row the update locked
  (`TestStandardBeginRequiresTransaction`). The two return the same row for the same fixture
  (`TestVariantsAgree`).

What the baseline costs and cannot do: one extra round trip per file-delete begin, and no
serialization of directory moves. The write path (four statements), the delete's complete step,
the directory removal, and both moves needed no native form; `RETURNING` would save one read-back
per step everywhere, which the delete begin already measures at one round trip. The delete and
move stages each confirmed that no third variation point was needed.

A variant is supplied at construction: `data.New(catalog, dialect, data.WithVariant(v))`, where
`v` is built by its own constructor against the same catalog (`pgnative.New`, `data.NewStandard`),
because the variant's statements compile against the consumer's catalog like the store's do. The
default costs no second compile: `newStandard` binds the baseline over the statements `New`
already compiled. A consumer-supplied variant is a struct that embeds a base variant and
overrides one method; `TestConsumerVariantSwapsOneMethod` proves the store runs the override and
the base for the other method, in both directions. `Store.Statements()` appends the variant's
inventory and `Store.Verify` runs the variant's `Verify`, so `pgnative`'s two statements are listed
and prepared at startup with the rest.

Every native file declares `--| tier: native` and `--| native: <feature and port>`, and `sqlint`
holds the variant's directory to the native-forms check keyed on each file's declared tier: a
native file is exempt by its tier and must carry its port note, and a file in that directory that
declares the standard tier is still held to the standard forms. The conformance suite
(`datatest.Run`) passes over both variants: `TestStandardConformance` in the persistence package
and `TestConformance` in `pgnative`.

### The listing

The listing is not a projection. `ListFiles` and `Children` are each one authored statement
anchored on a directory id (`files_in_directory`, `children_of_directory`, and their `_with_total`
twins), with the caller's filters, sort, and page appended in Go by the composer in
`lib/blobfs/data/listing.go` from the query library's own clause patterns, read through
`Catalog.Patterns()`. There is no derived-table wrap: the clauses attach at the statement's own
level, so the engine sees one flat query over one table and pages a name sort through the unique
index. The statements alias their table as `q` because the clause patterns qualify every field as
`q.<field>`.

The total travels in the page. Under `TotalExact` (the zero value) the `_with_total` statement
carries `COUNT(*) OVER () AS total` in its select list, evaluated over the rows the WHERE clause
keeps and before the paging clause cuts them, so the total cannot disagree with its page under
any isolation level. `TestListingCarriesItsTotal` proves one query per page, the window count
present under `TotalExact` and absent under `TotalNone`; `TestListingMatchesForest` proves the rows
and the total equal a whole-forest recursion's answer for every directory of a fixture, four
sorts, two filters, and four page sizes; `TestExactTotalUnderConcurrentInserts` proves the total
agrees with its own rows while a second connection inserts between calls.

Paging is by number (`OFFSET ... FETCH NEXT`) or by keyset cursor, and both walk the same order
(`TestCursorWalkMatchesOffsetWalk`). The sort is the caller's terms followed by `name`, the key,
as the tie-breaker when the caller did not name it. The tie-breaker takes the terms' direction
when they share one, so `created_at:desc` orders by `created_at DESC, name DESC` and a descending
sort is the exact reverse of the ascending one; mixed caller terms get `name` ascending. Stage 6
appended `name` ascending always, and stage 8 changed the rule so that a descending sort can take
a cursor; the stage 6 and 7 engine baselines were updated for the new order (decision 3).

The cursor (`lib/blobfs/data/cursor.go`) is base64url over an eight-byte SHA-256 prefix and a JSON
body holding the encoding version, the name of the statement that issued it, the sort terms it
was issued under, and the sort values of the page's last row as text. The checksum is an
integrity check, not authentication: a forged well-formed cursor positions the listing where a
filter could and nothing more. A cursor binds the listing and the sort, not the directory. It is
refused, with a `data.CursorError` that unwraps to `query.ErrDirectives`, when it is malformed or
edited, was issued by the other listing or under other terms or directions, or when the sort
cannot be continued: terms up to the key that mix directions, or a term naming a field that can be
NULL (`size`, `etag`, `parent_id`), which the entity's pointer fields declare
(`TestCursorRefusals`, `TestNullableSortIssuesNoCursor`, `TestCraftedCursorsAreRefused`). A cursor
page carries `NoTotal` whatever `Total` says, because a window count under the keyset predicate
would count the rows after the cursor, which is a different quantity; a caller reads page one with
its total and then walks by cursor, and `Next` is filled on offset pages too. The composer fetches
one row beyond every page, offset or cursor, and drops it, so the page size plus one is the bound
fetch count. The row's presence is `Page.More`, which says whether rows remain whatever the sort
and the total; `Next` is set when `More` is true and the sort can be continued. Stage 16 fetched
the extra row only when the sort could be continued (decision 5 of `DECISIONS.md` changed that).

The `NoTotal` edge: the window count travels on rows, so an empty first page has the exact total
0 (no row matched) and an empty page after the first reports `NoTotal` (-1), because the composer
runs no count statement (`TestListingPagesPastTheEnd`). The consumer's projections, whose count is
a separate statement, report the exact total on that same edge, so the two read models the binary
ships differ there and `output` renders both (`total unknown (the page is empty)` against
`total 5`) (decision 2).

The store's `Verify` has two halves: every statement prepares as authored, and each listing
prepares once more as a canonical rendering with every declared field filtered and sorted, plus a
cursor rendering over every field a cursor can continue. `TestVerify` counts twenty-six prepares:
twenty statements, four offset renderings, and two cursor renderings.

What the listing costs is in `evidence/read-model.txt` (proof V3) and `evidence/sort-index.txt`,
summarized under "The proofs".

### Error mapping

Constraint-to-sentinel mapping is per operation, because one constraint means different things on
different statements. `blobfs_fk_directory_parent` means a missing parent on an insert or a move
and a non-empty directory on a delete. `lib/blobfs/data/errors.go` therefore keeps two tables:
`writeSentinels` maps the two name uniques to `ErrNameTaken`, the root index to
`ErrRootDirectory`, and the two foreign keys to `ErrNotFound`; `deleteSentinels` maps the same two
foreign keys to `ErrNotEmpty`. Each mapping is keyed on the constraint name and checked against the
class `sqlate` reports, and the `sqlate.ConstraintError` stays reachable through `errors.As`
(`TestClassifyWrite`, `TestClassifyDelete`).

A constraint `blobfs` does not own is handled by class on a delete and left alone on a write. A
`DELETE` of a `blobfs_file` row can violate only a foreign key that references `blobfs_file`, and
`blobfs`'s own DDL declares none, so any foreign-key violation on the file's removal is a
consumer's constraint by construction; on a directory removal, any foreign key that is not one of
`blobfs`'s two is likewise a consumer's. `classifyDelete` reports those as `blobfs.ErrReferenced`
with the constraint name reachable, and the consumer matches the name against its own
(`fileDeleteSentinels` in `domain/files/database.go` maps `fk_bookmark_file` to
`files.ErrBookmarked`). The library never names a consumer constraint. On a write, a consumer
constraint's violation returns as it came, wrapped with the operation's context, and the consumer
classifies it (`bookmarkSentinels` maps `pk_bookmark`, `uq_bookmark_active`, and `fk_bookmark_file`
to the consumer's sentinels). The engine proofs are the suite's `ReferencedRowStaysDeleting`, which
creates a reference table of its own, and `TestRemoveDirectoryOnTheEngine`.

Two refusals are made in Go before any SQL: `ErrRootDirectory` for a move, rename, or removal of
the root, and a `NameError` for a name `ValidateName` refuses. `sql.ErrNoRows` from a typed handle
is mapped to `ErrNotFound` by `notFound`.

### The write path

A file write is two steps around the object write, which the library never makes.
`BeginFileWrite(ctx, sess, keys, directoryID, name, contentType)` validates the name, mints the id
(`uuid.NewV7()` from the standard library), builds the key `id/sanitized-name` and validates it
against the caller's `blobfs.KeyValidator`, and inserts the row as `pending` through
`begin_file_write`, then reads it back with `file_by_id`. `CompleteFileWrite(ctx, sess, id,
version, obj)` runs the guarded update `complete_file_write` under the query library's version
guard (`file_version` is the check) and reads the row back. Both take `sqlate.Session`, not
`*sqlate.Tx`: `begin_file_write` carries no `transaction: required` header, because one insert and
its read-back are correct on the pool, and a consumer that wants the row beside its own passes its
transaction. `TestFileWriteComposesIntoTheCallersTransaction` proves the begin inside a consumer's
`Transact` beside a consumer row that references `blobfs_file`: a rollback leaves neither row and
a commit both.

There is no fail step and no `failed` status, by decision (stage 10). A stop between the steps
leaves the row `pending`, which is the queryable state proof 3 asks for; a retry of the same write
finds the row through `FileByName` and completes it (`TestFileWriteRetryCompletesAPendingRow`,
`TestPutStopsAndResumes`); an abandoned write is removed through the delete steps, which
`pending` already allows. The transition table in `lib/blobfs/status.go` has no row for a failed
write and its comment records the decision. The complete step stores what `Put` returned: the size
the provider counted, the entity tag it assigned, and the content type as sent (`TestPut` proves
the row equals the service's `Stat` of the object on Azurite).

The consumer's `put` is three steps with two transaction boundaries
(`TestPutIsThreeStepsWithTwoBoundaries`): the parent's resolution, the name lookup, and the
insert in one transaction that commits before any byte reaches the store; the object write outside
any transaction; the completion on the pool. `put --fail-after insert|write` stops after the first
or second step with exit code 1 and a message naming the pending row.

### The delete path

A file delete is two steps around the object delete. `BeginFileDelete` moves the row to `deleting`
through the variant and returns it. `CompleteFileDelete` runs `remove_file` (`DELETE ... WHERE id =
? AND status = 'deleting'`), the same standard statement on every variant, and when it removes
nothing it reads the row once and classifies: a row that is gone is success, a row that exists and
is not deleting is `blobfs.ErrNotDeleting`, and a row that became deleting meanwhile has the
removal repeated once. The retry rule per step is that the begin returns a deleting row unchanged,
the object delete succeeds on a missing object, and the complete succeeds on a missing row, so a
stop after any step leaves a state the next run finishes. The suite's `FileDelete` group proves it
on both variants (`RetryAtEachStepConverges`, `PendingIsDeletable`, `NotDeletingIsRefused`,
`ReferencedRowStaysDeleting`), and the consumer's `TestRemoveConvergesAtEachStep` proves it end to
end.

`deleting` is needed (proof 6). It is the durable marker that lets a retry finish once the object
is gone: without it a row whose object was deleted would read as `available`, a rerun could not
tell "resume the delete" from "delete a live file", `cat` would report a missing object as a store
fault, and a bookmark could be added to a file with no object. The status also keeps the
`(directory_id, name)` slot until the row goes, so a `put` of the same name during the delete is
`ErrNameTaken` and `rmdir` of the directory is `ErrNotEmpty` until the row goes.

`RemoveDirectory` is one statement, `remove_directory`, which never removes a row without a parent;
the root is refused in Go first. There is no cascade and no recursive delete in the library. The
consumer's `rm -r` walks the tree in pages of 100 rows, directories then files, removes what each
page holds until a page comes back empty, then removes the directory; it takes no lock, and a row
inserted meanwhile is either removed by a later pass or refuses the directory's removal through the
foreign key, in which case the walk empties the directory again up to three rounds before
reporting `ErrNotEmpty`, and three stalled passes in a row over one directory are
`files.ErrTreeBusy` (`TestRemoveTreeRacesAnInsert`, `TestRemoveTreeRacesAnInsertBetweenPasses`).
The delete takes no tree lock: a move racing a delete is decided by one foreign key or the other,
with the engine's row locks making the second operation wait for the first's outcome
(`TestMoveRacesADelete`).

### The move path

`MoveDirectory(ctx, sess, id, parentID, name, version)` runs three statements in the caller's
transaction, in this order: the variant's `LockTree`, the cycle check `directory_is_within`
(exported as `IsWithin`), and the guarded update `reparent_directory` with `directory_version` as
its check and `AND parent_id IS NOT NULL` so no statement of the library can move the root. The
cycle check is one upward walk from the new parent looking for the moved directory, so its cost is
the new parent's depth. The transaction requirement is enforced twice: by the lock's refusal of a
pool session on both variants and by the `transaction: required` header on `reparent_directory`.
The step takes the id and the version the caller read, because the guard needs exactly those two.
A rename is a move to the same parent and pays the lock and the check like any move.
`TestMoveDirectoryIsThreeStepsUnderOneLock` pins the order and the bound arguments hermetically.

`MoveFile(ctx, sess, id, directoryID, name, version)` is one guarded statement, `move_file`, with
`AND status <> 'deleting'`, on the pool or in a transaction: a file cannot be its own ancestor, so
it takes no lock and no check. The key is untouched, so a rename moves no object
(`TestMoveFileOnTheEngine`). Directories and files have separate name spaces: a directory may take
the name of a file beside it (`MoveDirectory/NameTaken`).

What the baseline requires of a caller (proof 4): the lock. On the baseline, the suite's
`MoveDirectory/OpposingConcurrentMoves` shows two opposing moves (X under Y and Y under X) both
pass the no-op lock, both checks pass against the same committed state, both commit, and
afterward X's parent is Y and Y's parent is X, neither reachable from the root. The same
interleaving on `pgnative` blocks the second move inside the lock until the first commits and
then refuses it with `ErrCycle`. The cycle check assumes an acyclic tree: `directory_is_within` and
`directory_ancestors` are unbounded upward walks that never terminate on a cycle, which is why the
suite repairs the cycle it forms and why the lock is a requirement and not a courtesy. A baseline
caller has three options, in order of preference: use a serializing variant; open every moving
transaction at serializable isolation and retry on SQLSTATE 40001, which
`OpposingSerializableMoves` proves refuses the second of two opposing moves on both variants with
no cycle; or serialize directory moves outside the database. Postgres already serializes the two
updates on its own row locks (an update of `parent_id` is a key update under
`blobfs_uq_directory_parent_name`, and the foreign-key check's `KEY SHARE` conflicts with it); the
cycle comes from the second check running before the first commit, which only a lock taken before
the check or a serializable snapshot prevents (decision 4).

The interleaving is made deterministic by a test-only gated variant that the suite wraps around
the store's variant through the public `data.New`; there is no production hook and no third
variation point.

### Keys (proof 7)

`blobfs.KeyValidator` has one method, `ValidateKey(key string) error`. The concept's `MaxKeyLength`
was dropped at stage 10: `azureblob`'s `ValidateKey` enforces its 1,024-rune limit itself, the
storage fake's default does the same, and `MaxNameLength` (255 runes) keeps every key `blobfs`
builds at 292 runes at most, so a length check in `blobfs` never fires against a real provider.
The consumer's adapter is one method, `Storage.ValidateKey`, over
`store.Capabilities().ValidateKey`. The rune-boundary proofs are `TestBeginFileWriteKeyBoundary` (a
validator with a 60-rune limit accepts a key of exactly 60 runes and 83 bytes and refuses one rune
over before any SQL), `TestNewKeyCountsRunes`, and end to end `TestPutNameAtTheRuneBoundary` (a
name of 255 `é`, a key of 292 runes and 546 bytes, stored in Azurite and read back; 256 is
`ErrInvalidName`). Azurite proves nothing about the limit: `TestAzuriteAndTheKeyLimit` shows it
accepts a key of 1,025 runes put straight through `storage.Store.Put`, so the provider's rule is
the whole defense. The validator is a parameter of the begin step rather than an option of
`data.New`, so the persistence package builds without the object store and the consumer opens the
store lazily (decision 9).

### The migration set and the multi-set migrator

`lib/blobfs/migrations` exports `Migrations(dialect)`, which selects the directory by the
dialect's name (Postgres only at v1; another dialect is `ErrUnsupportedEngine`), the source name
`Source` (`blobfs`), and the history table `Table` (`blobfs_schema_version`). The set is three
migrations; `TestGoldenHashes` pins each file's text and `TestUpgradeKeepsInstalledHashes` checks
that versions 1 and 2 still hash to the values pinned at stage 6 after migration 3 was added.
Migration text was amended in place through stage 6 because nothing is released; migration 3 is a
new version because the rehearsal's purpose is an upgrade over an installed database
(`TestUpgradeAfterRestart` builds the installed state from the set cut to two migrations, seeds
rows, opens a new migrator over the full set, and checks that only version 3 was applied).

`lib/migrator` (`migrator.go`, 303 lines including its comments) runs several sets under one
lock. `New(db, []Set, Options)` validates that every set has a name and that no two share a name or
a history table, and builds one `migrate.Migrator` per set with `Options.Unlocked`. Each run
asserts `sqlate.Locker` on the dialect, pins one connection from the pool, takes the lock under
`Options.LockName` (default `migrator.sets`), checks every set's history first (a dirty row or a
mismatched history is a `SetError` that unwraps to `migrate.ErrDirty` or
`migrate.ErrUnknownVersion`), runs the sets, and releases the lock under `context.WithoutCancel`.
`Up` runs the sets in declared order; `Down` reverts them in reverse and keeps the history tables; `Reset` reverts in reverse and then drops each set's history table with a `DROP TABLE` the
shim issues itself; `Status` returns one `SetStatus` per set (name, table, applied version, latest,
pending migrations, dirty mark) without the lock; `Force(ctx, set, version)` is the operator's
repair. The inner migrators run on their own pooled connections, so a run needs two connections.
The engine proofs are `TestFreshReplay`, `TestUpgradeAfterRestart`, `TestResetOrderAcrossForeignKeys`
(the wrong order fails at migration 2's `DROP TABLE blobfs_file` with SQLSTATE 2BP01 and leaves
blobfs's head at 2, because migration 3 was already reverted in its own transaction),
`TestConcurrentStartersSerialize`, and `TestDirtyRefusalOnEngine`. Proof 5's answer is integrated
shipping; the hooks the promotion needs are items 26 to 32 of Finding 2.

### What a consumer composes

The consumer's shape is `domain/files`, and it is the shape `v1.storage` will take.

- Its own tables, in its own migration set (`migrations/postgres`): `directory_owner(directory_id,
  unit_id, version, timestamps)` keyed on the directory with a foreign key into
  `blobfs_directory`, and `bookmark(unit_id, file_id, active, timestamps)` with the primary key
  `(unit_id, file_id)`, a foreign key into `blobfs_file`, and the partial unique index
  `uq_bookmark_active ON bookmark (unit_id) WHERE active`. Its constraint names follow
  `<kind>_<table>_<detail>` without the `blobfs_` prefix, and the two mappings never overlap.
- One pattern catalog for the program (`domain/files/database.go`): `query.NewCatalog(query.Patterns(),
  data.Patterns())`, against which `New` compiles `blobfs`'s statements, the variant's, and the
  consumer's. `Verify` prepares thirty-six: the consumer's eight statements and two projections,
  `blobfs`'s twenty, and the six listing renderings.
- Ownership joins. `owned_directories` is a projection over `{{> blobfs.directory_columns}}` joined
  to `directory_owner`, and `bookmarks` is a projection over `bookmark` joined to `blobfs_file` with
  each row's path computed by a recursion correlated on the file's directory. Both restate the
  library entity's columns by name, because the scanner does not flatten embedded structs.
- Transaction boundaries. `ls` runs the path resolution and both halves in one read-only
  repeatable-read transaction (`DB.Transact` with `sqlate.ReadOnly()` and
  `sqlate.Isolation(sql.LevelRepeatableRead)`), so the two halves are mutually consistent
  (`TestListRunsInOneReadOnlyRepeatableReadTransaction`, `TestListHalvesAgreeUnderConcurrentWrites`).
  `bookmark ls` does the same for its count and page. `put`, `rm`, `mkdir --unit`, `rmdir`, `mv`,
  and `bookmark add` each state their boundary in `domain/files/blobfs.go`.
- The variant choice. `files.New(db, opener, files.WithVariant(pgnative.New))` at the composition
  root (`internal/app/domain.go`), from `--variant standard|pgnative` or `BLOBFS_VARIANT`.
  `WithVariant` takes a type parameter so `pgnative.New` passes as it is without the composition
  root naming the query library's types, which `split-check` rules 7 and 9 reserve for
  `database.go`.
- The object-store adapter (`domain/files/storage.go`), the one application file that imports
  `go-storage` and `azureblob`: it is the `blobfs.KeyValidator`, it maps the store's sentinels
  onto the domain's, and the composition root opens and starts it on the first file command that
  needs it, so `mkdir` and `ls` run with Azurite down.

## Finding 2: the adjustments `sqlate` needs

One consolidated list, collected from the `sqlate` ledger and every stage's additions in
`NOTES.md` and deduplicated. Each item states what fails or is awkward, the evidence, the
workaround `blobfs` uses today, and the smallest change in `sqlate` that removes it.

### Projections and listings

1. A projection base cannot bind a parameter. A listing anchored on one directory therefore
   cannot be a projection, and a projection whose recursion must be a top-level common table
   expression walks from every row before the outer filter runs. Evidence: the composer exists
   (`lib/blobfs/data/listing.go`); `evidence/bookmarks.txt` section c (the top-level recursion
   costs 164.706 ms and 3023 buffers for a unit with 10 bookmarks among 53,110, and 167.220 ms for
   1,000) against section a (0.270 ms and 245 buffers for 10). Workaround: the library composes its
   listings outside the projection, and the bookmark read model moves its recursion into a scalar
   subquery correlated on each row's directory, which the planner pulls up. Change: a base that
   binds parameters, the arity-one lift `design/auth-strategy.md` section 4 names. The correlated
   form cannot take a downward walk or a walk shared by several output columns, which keeps the
   lift motivated.
2. The total belongs in the page statement, and a projection cannot skip its count.
   `Projection.List` always runs its count twin before the page, and a separate count can disagree
   with its page. Evidence: `TestListingCarriesItsTotal`; `ls / --unit --total none` and `bookmark
   ls --total none` read the count and drop it (`NOTES.md`, stages 7 and 11). Workaround: the
   library's `_with_total` statements carry `COUNT(*) OVER () AS total`, and the consumer drops the
   count it cannot skip. Change: a total mode on `Directives`, and the window count as an option of
   the collection pattern, with the edge documented that a window total travels on rows and an
   empty later page has none.
3. A derived-table wrap over a base that contains `WITH RECURSIVE` loses the index order and blocks
   the outer filter. Evidence: `evidence/read-model.txt` section e3 (the same base pages in 0.069 ms
   and 21 buffers flat and in 5.094 ms and 2443 buffers wrapped); sections e1 and e2 show the wrap
   costs nothing over a flat base. Workaround: the composer appends the clauses at the statement's
   own level. Change: let a projection compose its clauses at the base's level for a recursive
   base, or document that the authored base decides whether the directives can reach an index.
4. The catalog exposes its inventory and not its renderer. `Catalog.render` is unexported, so a
   composer outside the projection reads the clause patterns' text through `Catalog.Patterns()` and
   fills the slots with its own copy of the slot regex. Evidence: `slot` and `newClauses` in
   `listing.go`. Change: a `Catalog.Render(name, fill)` method, or an exported clause composer.
5. The clause patterns fix the correlation name `q`. `filter_*`, `order_term`, and
   `order_term_desc` spell every field as `q.<field>`. Evidence: every listing statement reads `FROM
   blobfs_file q` or `FROM blobfs_directory q`. Change: make the qualifier a slot, or publish
   unqualified terms.
6. `Projection` has no keyset paging. `Directives` carries a page number only. Evidence: `ls /
   --unit` refuses `--after-dirs` with `files.ErrNoCursorAtRoot`. Change: a cursor on `Directives`
   with the composer's rules (the key as the tie-breaker in the sort's direction, one direction, no
   nullable term).
7. A projection's key is one field. `--| key:` names one field, and the bookmark read model's
   unique key over the unfiltered base is the pair `(unit_id, file_id)`. Evidence:
   `domain/files/statements/bookmarks.sql` declares `file_id` and relies on every read filtering by
   the unit. Change: a composite key, or a key declared per filter.
8. `Statement` does not expose the dialect it compiled against, only its catalog. Evidence:
   `data.New` takes the dialect for the placeholders the composer appends after the statement's
   own. Change: a `Statement.Dialect()` accessor.
9. `Statement.Text()` ends where the file ends, and nothing in the loader states it. The composer
   relies on the loader trimming a trailing semicolon and whitespace, and a listing statement must
   end with its WHERE clause. Evidence: the hermetic tests pin the rendered suffix
   (`TestListingComposesClauses`). Change: document the rule.
10. A statement cannot resolve a path in one round trip. Standard SQL has no ordered array
    parameter, and `{{name...}}` renders an `IN` list. Evidence: `ResolveDirectory` is one
    `directory_child` read per segment, and `mv` resolves four times from the root. Change: none at
    the standard tier; a native variant could resolve a path in one statement, and `blobfs` could
    offer a `ResolveUnder(parentID, path)` to halve the consumer's repeats.

### Transactions and sessions

11. `sqlate.Session` cannot begin a transaction. A library cannot give a two-read operation a
    consistent snapshot, so the caller must. Evidence: `ls` and `bookmark ls` open the read-only
    repeatable-read transaction themselves (`TestListRunsInOneReadOnlyRepeatableReadTransaction`).
    The consumer-side answer works: `DB.Transact` takes `ReadOnly()` and `Isolation(...)` beside the
    function, and every library method takes the `*Tx` as its session. Change: document the
    pattern, or let a library method take an option that begins a read-only transaction when the
    session is the pool.
12. The transaction requirement lives in the statement, not in the operation. The baseline's
    delete begin is two statements that need one transaction, declared on the first statement's
    header and inherited by the second only because the first refused the pool; the baseline's
    no-op lock enforces the same requirement by a type assertion in Go, since no statement runs.
    Evidence: `Standard.LockTree` and `Standard.BeginFileDelete` in `variant.go`. Change: an
    operation-level requirement (a `*sqlate.Tx` parameter or a `Transact`-style contract) that
    states it once.

### Errors and constraint classification

13. `UnknownFieldError` unwraps to `ErrDirectives`, the client-error sentinel, so a forgotten scope
    filter fails as a client error unless the consumer matches the type. The error carries `Use:
    filter`, which tells them apart. Evidence: the composer's refusals (`TestListingRefusals`).
    Change: a separate sentinel for a field the statement did not declare, or a documented
    contract that the consumer matches the type.
14. `postgres.Dialect.MapError` does not map SQLSTATE 2BP01 (dependent objects still exist).
    Evidence: `TestResetOrderAcrossForeignKeys` and `TestWrongOrderDownIsRefused` match the code by
    hand through the driver's error. Workaround: the shim wraps the raw error in `SetError`.
    Change: map the class to a sentinel.
15. `postgres.Dialect.MapError` leaves SQLSTATE 40001 (serialization failure) unmapped, and the
    dialect's own test pins that. Evidence: the suite's `OpposingSerializableMoves` recognizes it
    through the driver's `interface{ SQLState() string }`. Change: a sentinel for the class, so a
    retry loop stays portable; this matters because serializable isolation is the standard-tier
    alternative to the tree lock.
16. `ConstraintError` carries no table name. On a foreign-key violation from a delete, Postgres
    reports the referencing table beside the constraint name, and `sqlate` exposes the name and
    the class only. Evidence: `blobfs.ErrReferenced`'s message cannot say which consumer table
    holds the reference. Workaround: the consumer maps the name. Change: a `Table` field.
17. `query.Guard` reports only a version mismatch and cannot carry a second predicate. A guarded
    statement that adds `AND status = 'pending'` (`complete_file_write`) or `AND status <>
    'deleting'` (`move_file`) affects nothing when the status refuses the row, and the guard's
    check then finds the expected version and reports `ErrVersionMismatch: expected 1, current 1`,
    which is false. `Guard.Run` also returns the new version and not the row, so a begin that
    needed the row could not use it. Evidence: `CompleteFileWrite` and `MoveFile` read the row
    after a mismatch and reclassify (`TestCompleteFileWrite`, `TestMoveFile`). Two of the library's
    four guarded statements carry the extra read. Change: a guard whose check returns the row, or
    a check the caller extends with the same extra predicate. A related guarantee the write path
    relies on should be documented: `Guard.Run` passes the command's `Args` to the check and `Args`
    ignores an extra name, so the size, content type, and etag bound for the update do not fail
    the version read.

### Statements, tiers, and native declarations

18. A `--| field:` timestamp type must be spelled `timestamp with time zone` in standard tier,
    because `timestamptz` is a native form, and `Verify` never checks field types, so a wrong
    spelling surfaces only when someone filters on the field. Evidence: every listing statement's
    header. Change: `Verify` checks each declared field's type against the cast it renders, or the
    loader normalizes the spelling.
19. A library that ships statements hard-codes the `sql.` namespace, so a consumer that aliases
    `sqlate`'s source with `As` breaks the library's compile. Evidence: `{{> sql.guard_where}}` in
    `reparent_directory` and `complete_file_write`. Change: resolve the library's namespace by
    identity rather than by name, or document that the library namespace is never aliased.
20. A native statement's declaration is one line. `--| native: <feature and port>` is free text on
    one line, so a port note of any length is one long line. Evidence:
    `lib/blobfs/data/pgnative/statements/lock_tree.sql`. Change: a multi-line header value, or a
    separate `port:` key.
21. `Verify` prepares every statement whatever its tier and never asks the dialect whether a
    native statement belongs to it, so a Postgres variant compiled against another engine's
    dialect fails at prepare, not at compile. Evidence: `pgnative.New` compiles against any
    dialect. Change: a dialect check at compile for a native file, or documentation that native
    statements are verified only against a live engine.
22. `sqlint`'s `native_forms` check keys on the tier, not the directory, so a directory listed
    under `[statements]` may mix tiers. Evidence: `sqlint.toml`'s comment on the variant's
    directory. Change: a per-directory `tier` requirement, so the configuration can state the
    layout rule the experiment follows by convention.
23. `Statement.Native()` is the only reader of the port note, and no tool lists a program's native
    statements with their ports. Evidence: `pgnative`'s test asserts each note contains `Port:` by
    convention. Change: an `sqlint` report, or a `query.Statements` method that lists native
    statements and their declarations, so the port notes become the work list.
24. A parameter inside a published pattern takes no cast. `sql.guard_where` binds `{{id}}`
    untyped, so `complete_file_write` sends the id as an untyped parameter and Postgres infers
    `uuid` from the column, while the library's own statements cast every uuid (`{{id:uuid}}`) for
    `Verify`'s sake. Change: typed slots in a pattern, or a way for the including statement to
    state the type.
25. `StandardCatalog.HistoryExists` does not qualify by schema, so a same-named table in any
    schema satisfies the check. Evidence: the ledger (stage 3). Change: qualify by the current
    schema.

### `migrate` and the multi-set migrator

The hooks that make `lib/migrator` disappear. The shim needed nothing outside the public API and
repeats one constant and one statement; `blobfs.sources` promotes its shape into `sqlate` as
`migrate.New(db, []migrate.Set{...}, opts)`, with the default set keeping `schema_version` so a
v0.1.1 database needs no history migration.

26. `migrate` does not export its default table name (`schema_version`) and offers no `Table()`
    accessor. Evidence: `defaultTable` in `migrator.go`, repeated to refuse two sets on one table
    and to report the table in `Status`. Change: export the constant, or add the accessor.
27. `migrate` cannot run on a caller's connection. The shim pins its own connection for the outer
    lock and each inner run pins a second one, so a run needs two pool connections. Evidence:
    `Migrator.locked` in `migrator.go`. Change: a `Migrator` that takes a `*sql.Conn`, or a multi-set
    `Migrator` that shares one.
28. `migrate` has no operation that drops its history table, so `Reset` runs `DROP TABLE` on its
    own. Evidence: `Reset` and `TestFreshReplay`. Change: a `Migrator.Drop` (or `Reset`) beside
    `Force`.
29. `Options.Unlocked` does what its comment says and is what the shim relies on. Its doc comment
    should say that a caller holding its own lock is the intended use, beside the dialect without
    the capability. Change: documentation.
30. `Version` and `Verify` read without a lock and are enough for a status report, at four
    queries per set. Change: a `Status` that returns head, dirty, and pending in one read.
31. `Steps` tolerates a count larger than the applied prefix, and `Down(len(set))` is how the shim
    reverts a whole set. Change: document it as a guarantee.
32. `sqlate.Locker` is enough for the outer lock: the Postgres dialect's `Lock` and `Unlock` work on
    any pinned connection, and `TestConcurrentStartersSerialize` shows two starters serialized
    under `migrator.sets` (the control without the lock fails with a duplicate object, SQLSTATE
    23505 on `pg_type_typname_nsp_index`). No change is needed; the finding supports the
    promotion. A related correction to the concept: multi-statement transactional migrations work
    on pgx, which uses the simple protocol when a statement has no arguments, so the concept's
    claim that v0.1.1 cannot host a source holds only for one merged `Migrator`; a `Migrator` per
    set with its own `Options.Table` runs on v0.1.1, which is what the shim is.

### Scanning

33. `Scanner` refuses a column with no field. A page statement that carries `COUNT(*) OVER () AS
    total` beside the entity columns cannot scan through `query.Scanner[T]`. Evidence: `scanner`
    in `listing.go`, which reads the entity's fields by tag and the total into an `int`. Change: a
    scanner that takes extra destinations, or an entity wrapper the mapper flattens.
34. The scanner does not flatten embedded structs, so a consumer read model restates every column
    of a library entity, including columns it never uses. Evidence: `OwnedDirectory` restates
    `blobfs.Directory` beside `unit_id` and converts back to the library type, and `BookmarkedFile`
    restates the file columns (`domain/files/entities.go`). Change: flatten embedded structs in
    the mapper.

### What each adjustment unblocks

| Roadmap task | What it needs from this list | What the evidence supports |
|--------------|------------------------------|----------------------------|
| `blobfs.sources` (the multi-set migrator in `sqlate` v0.2.0) | Items 26 to 31 are the hooks; item 32 confirms the lock; item 14 lets the promoted migrator classify a refused revert. | The shim's API is the multi-set API the concept describes, and the promotion adds exactly those hooks. |
| `blobfs.build` (the library's own repository) | Nothing blocks it: the library builds and passes over v0.1.1 with the workarounds in place. Items 4, 5, 8, 9, 33 remove code from the composer; item 17 removes the extra read from two guarded steps; item 15 makes the documented serializable-isolation route portable; item 20 and item 23 make the port notes readable and listable. | The composer, the scan, and the two reclassifying reads are the code that leaves `blobfs` once these land. |
| `v1.storage.service` (the consumer) | Items 2, 6, 11, and 34 are the consumer's awkward call sites (the dropped count, the missing cursor on the owner read model, the transaction the consumer opens, the restated columns). Item 1 is motivated, not required: the correlated shape serves the bookmark read model, and the parameterized base is needed only for a directory-anchored listing that must be a projection. | `evidence/bookmarks.txt` shows the correlated shape at 0.270 ms for 10 bookmarks; the parameterized base's shape (section d) is slower for a small unit. |

## Finding 3: incorporation into `v1.storage`

### One install per configuration and per database

An install is one database and one container, with fixed `blobfs_` object names. A service that
serves several isolated trees runs one configuration per tree, a DSN and a container each.
`TestIsolation` (`integration/integration_test.go`) is the evidence that two configurations share
nothing: over the same binary and the same environment otherwise, both build `/docs/a.txt` with
different bytes; `ls /` in each shows its own entries only; `cat` returns each configuration's
bytes; a unit's directory and active bookmark in A are absent in B; each database holds exactly
its own `blobfs_file`, `bookmark`, and `directory_owner` rows and each container exactly its own
objects under the keys `stat` reports; `rm -r /docs` in A leaves B's row and object; `schema
reset --yes` in A leaves B's tables; and after both are torn down neither database nor container
exists. Nothing in the library or the consumer names a second database or container: no
`volume_id`, no key prefix, no schema qualifier. One observation for the review: the object keys
are `<file id>/<name>` and carry no mark of the configuration, so two configurations sharing one
container would not collide (the ids are UUIDv7), but the design keeps one container per
configuration so that a container-level operation belongs to exactly one tree.

The variant is a composition choice and reaches only the files store: a service on Postgres passes
`files.WithVariant(pgnative.New)`, and the schema step is the same on either variant.

### The migration source in the service's one migrator

The service builds one `migrator.Migrator` (after `blobfs.sources`, `sqlate`'s own) over
`[]Set{blobfs's set, its own set}`: the source's `Migrations(dialect)` under the source's `Table`,
and its own under the default table. `admin/schema/database.go` is the model: `Sets` is one short
function returning two `Set` literals, and `NewClient` is five lines over `migrator.New`. `Up` at
start takes one lock for both sets,
so several replicas starting together serialize and every one ends at head
(`TestConcurrentStartersSerialize`). `Up` refuses before running anything when any set is dirty or
its history does not match the binary's set, so a replica built from an older binary against a
newer database fails at start with `migrate.ErrUnknownVersion` naming the set. `Reset` reverts the
service's set before `blobfs`'s, so the service's foreign keys never block it, and drops both
history tables; `Down` keeps them. The upgrade path is a `go.mod` bump: the next start finds
`blobfs`'s new migration pending and applies only it, and the service's rows survive
(`TestUpgradeAfterRestart`). The order of `Up` is `blobfs` first, consumer last; the order of `Down`
and `Reset` is the reverse, and `TestResetOrderAcrossForeignKeys` shows the wrong order fails at
`DROP TABLE blobfs_file` and leaves `blobfs`'s set partially reverted, with the history agreeing
with the schema.

### What the admin surface must expose

`go-database`'s admin release (`blobfs.admin`) must expose `status` (one row per set: table,
applied version, latest, pending, dirty), `up`, `down`, `reset` behind an explicit confirmation,
and `force <set> <version>` for dirty recovery. The experiment's `schema` command exposes the
first four (`schema reset` requires `--yes` and refuses with `schema.ErrResetNotConfirmed` before
constructing the client) and leaves `force` out, because the stage list named four verbs; the
migrator has `Force`, and the admin surface needs it because `Reset` refuses a dirty set and the
operator's repair is to state the applied version through `Force` and then run `Up` or `Reset`. A
refused revert in the wrong order leaves a set partially reverted, so the admin's `status` after a
failed `reset` is what tells the operator where each set stands.

### Directory-grain ownership

The `directory_owner` rehearsal is the document hierarchy per organization. An owner row binds a
depth-one directory to a unit, written by `mkdir <path> --unit` in the same transaction as the
directory (`TestMkdirWithUnitIsOneTransaction`); `mkdir --unit` below depth one is
`files.ErrUnitDepth`, refused before any I/O. A scoped listing (`ls <path> --unit`) resolves the
depth-one ancestor of the path, reads its owner row once, and refuses with `files.ErrNotOwned`
before the rest of the path resolves, so a foreign unit learns nothing below the ancestor; the
listing statements carry no owner predicate, so the library's listings run unchanged under a
scope (`TestListScopedChecksTheAncestorOnce`). At `/` the scope is the owner projection filtered by
the unit (`TestListScopedAtRootUsesTheOwnerProjection`), and the file half is empty with total 0: a
file stored in the root has no top-level ancestor and belongs to no unit, so a service that keeps
files at the root has no scope for them. `rmdir` of an owned directory removes the owner row and
the directory in one transaction, because the owner row is the consumer's record of the directory
and has no life of its own; the alternative, an `ErrOwned` mapped from
`fk_directory_owner_directory`, would make an owned directory undeletable.

The cost of the scope check is one owner read per scoped command, plus the ancestor resolved twice
(`/first` for the check and the full path for the listing), because `ResolveDirectory` resolves
from the root only (Finding 2, item 10).

### File-grain ownership

The `bookmark` rehearsal is the `org_image` case. A `bookmark` row binds a file to a unit, the
partial unique index `uq_bookmark_active` allows one active bookmark per unit, and the three
constraints (`pk_bookmark`, `fk_bookmark_file`, `uq_bookmark_active`) reach the service as its own
sentinels (`ErrAlreadyBookmarked`, `blobfs.ErrNotFound` for a file removed meanwhile,
`ErrActiveBookmark`) through one table in its database file (`TestAddBookmarkClassifiesTheConstraints`,
`TestAddBookmarkOfAFileRemovedMeanwhile`, `TestBookmarkOneActivePerUnit`). The `org_image` case
maps onto it directly: the organization is the unit, the image row is the bookmark, "one logo per
organization" is the partial index, and the refusal of a second active row is a classified error
the handler turns into a conflict response. `add --active` is refused while another is active and
leaves the other as it is; a swap is `rm` then `add --active`. A pending file can be bookmarked,
which is the service's shape (the row beside the pending file row in the upload's first
transaction); a deleting file cannot. The foreign key holds the file: a delete of a bookmarked
file fails under `fk_bookmark_file` until the row goes.

The service's listing of a unit's files with their paths is the `bookmarks` projection: the
consumer's table joined to `blobfs_file`, the path computed per row by a recursion correlated on
the file's directory, paged, sorted, and counted through `query.Projection` unchanged, with the
count and the page in one read-only repeatable-read transaction
(`TestBookmarkTotalAgreesUnderConcurrentWrites`). The cost is the unit's row count times the
depth, and nothing grows with the tree or with other units' rows (`evidence/bookmarks.txt`,
section a: 245 buffers for 10 bookmarks, 2371 for 100, 21940 for 1,000, against a tree of 10,003
directories and a bookmark table of 53,110 rows).

### What a move may and may not do under directory-grain ownership

A move stays under one top-level directory (`files.ErrMoveAcrossScopes`, checked inside the
transaction after the destination resolves and before the source is touched,
`TestMoveStaysUnderOneTopLevelDirectory`). A top-level directory may be renamed, and its owner row,
keyed by id, stays; it may not be moved below another top-level directory. Nothing moves up to
the top level or across two top-level directories, and a file in the root may be renamed but not
moved under a top-level directory. The reason is structural: the owner row binds a top-level
directory and the scope is checked once at that ancestor, so a move across two top-level
directories would carry an entry from one unit's scope into another's without either unit's say,
and a move that changed a top-level directory's depth would leave an owner row at a depth the
scope check never reads. A move of a document across two organizations' trees is therefore not a
move but a copy and a delete, cheap on the SQL side and expensive on the store side. `mv` takes no
`--unit`: a unit's right to move within its own scope is the authorization the experiment does
not prove (decision 6).

### The write, delete, and move protocols as the service must sequence them

- The upload. In one transaction: the service resolves the parent, inserts the pending row through
  `BeginFileWrite`, and writes its own rows (the owner or image row) beside it; the transaction
  commits before any byte reaches the store. Outside any transaction: the service uploads under
  the row's key. On the pool: `CompleteFileWrite` with the row's id and version and what the store
  reported. A service that already runs `storage.Store` under its lifecycle hands the started
  store to the adapter through `files.NewStorage`, and the adapter is the `KeyValidator` the begin
  step takes. A `pending` row is the service's own state to sweep: no `failed` status exists, a
  retry of the same upload resumes the row, and an abandoned row goes through the delete steps. A
  time-bounded sweeper lists `status = 'pending'` under `updated_at < threshold` through the
  library's listing filters and deletes each.
- The delete. In one transaction: the service checks its own rows that reference the file (the
  bookmark count) and refuses while any exist, then runs `BeginFileDelete`. Outside any
  transaction: it deletes the object. On the pool: `CompleteFileDelete`. The service's foreign key
  into `blobfs_file` is the backstop for a reference that arrives after the check, and the service
  maps that key's name to its own error at the complete step. A sweeper for abandoned deletes lists
  `status = 'deleting'` under `updated_at < threshold` and runs the same idempotent steps. A
  directory removal removes the service's own rows about the directory in the same transaction as
  the library's removal, and a recursive removal is the service's walk, children first.
- The move. The service resolves both paths and runs `MoveDirectory` or `MoveFile` in one
  transaction; a directory move takes the tree lock through the variant. On the baseline the
  service serializes directory moves itself: one mover per process, or every moving transaction at
  serializable isolation with a retry on SQLSTATE 40001; on `pgnative` the library's lock does it.

### Deployment hazards the evidence found

- The JIT threshold on the correlated recursion. The planner costs the recursive subquery at about
  720 units per row, so a page over a unit with more than about 140 bookmarks crosses
  `jit_above_cost` (100,000) on a server with the default settings and is JIT-compiled:
  139.246 ms for 1,000 bookmarks with JIT on against 16.899 ms with it off
  (`evidence/bookmarks.txt`, sections a and f), and the compile is more than the walk. A sort by
  the key stays under the threshold and reads only the page's rows (0.234 ms and 493 buffers for
  1,000 bookmarks). A service with large units sorts by the key or lowers `jit_above_cost` for the
  session, which is a native form the standard tier cannot state.
- The exact total on a very large directory. The window count makes the page statement read every
  row of the directory with its columns: 6.275 ms and 2434 buffers for 10,008 files on every page,
  against 0.023 ms and 12 buffers without the total (`evidence/read-model.txt`, sections b and c),
  and `VACUUM` does not help it. The cost is bounded by the directory, never by the tree. A service
  with directories of tens of thousands of files asks for the total once, on page one, and walks
  by cursor (the last page costs 0.018 ms and 6 buffers by cursor against 7.696 ms and 2434
  buffers by offset, section d).
- The baseline's missing tree lock. On an engine without a native variant, two concurrent
  directory moves can form a cycle, and a cycle makes every upward walk loop forever. The service
  must serialize directory moves outside the database or run them at serializable isolation.
- The bookmark-versus-delete race, closed after stage 16 (decision 10 of `DECISIONS.md`). At
  stage 16, `rm` checked the bookmark count before the begin, and a bookmark added between that
  check and the complete step made the foreign key refuse the row's removal after the object was
  gone. The library's `HoldFile` now takes the file's row lock with an update that changes no
  value and no version, `AddBookmark` holds the file before it inserts, and `deleteFile` begins
  the delete before it reads the count, so the two operations serialize on the row. The rule for a
  consumer is reference-then-delete: hold the file in the transaction that inserts a row
  referencing it. A row inserted without the hold still meets the foreign key at the complete
  step, which leaves the row `deleting` with its bookmark until the bookmark is removed
  (`TestBookmarkAddedDuringTheDeleteIsRefused`, `TestDeleteDuringTheBookmarkAddIsRefused`).
- The migrator's connection count. A run needs two pool connections until `sqlate` takes the
  caller's connection (Finding 2, item 27).
- The recursive delete under sustained writes. `rm -r` is not atomic and can loop; the experiment
  bounds it (three rounds of emptying, three stalled passes) and reports `ErrNotEmpty` or
  `ErrTreeBusy` with the tree consistent.

### What stays undecided until `go-auth`

Authorization. `--unit` rehearses the directive-filter shape of a scope predicate and nothing
more: the experiment proves that the scope can be checked once at the depth-one ancestor and that
the file-grain join carries the unit, but not the composed authorized listing under the auth
strategy's scope predicate, which `v1.auth`'s storage sweep proves. Whether the directory-grain
case joins the file-grain case in `design/auth-strategy.md` section 8 is an amendment for the
review (see "Amendments to make at close"): the evidence shows the two grains differ in where the
predicate sits (once at the ancestor, or on every row of the join), and both are consumer joins.
A unit's right to move within its own scope, and whether `mv` should take `--unit`, wait for the
same layer.

## The proofs

| Proof | The answer | The evidence | The decision it changes |
|-------|------------|--------------|-------------------------|
| 1. Read model | The path stays a read-time query, and no path or volume id is stored. The directory listing is a statement anchored on the directory (13 buffers for a page of a 10-file directory with its total). The consumer-anchored read model costs the unit's bookmark count times the depth: 245 buffers for 10 bookmarks and 21940 for 1,000, and nothing grows with the tree. | `TestListingMatchesForest`, `TestBookmarkTotalAgreesUnderConcurrentWrites`; `evidence/read-model.txt` sections b and c; `evidence/bookmarks.txt` section a. | The path decision holds. The parameterized base stays motivated for the shapes the correlated form cannot take (Finding 2, item 1). |
| V3. Listing cost | The exact total stays the default: free on a small directory, and bounded by the directory's size on a large one (2434 buffers for 10,008 files). The cursor earns its place (6 buffers against 2434 for the last page). Composing at the base's level is required for a recursive base and the wrap suffices for a flat one. A capped total is not decided by the measurement. The `created_at` sort earns an index when listed without the total (25 buffers against 2436), and no other sort was measured. | `evidence/read-model.txt` sections 0, a to e; `evidence/sort-index.txt`. | The default holds; the cursor ships; the composer's design is required; the index's home is decision 5. |
| 2. Statements and tiers | Every statement of the persistence package is standard tier (twenty, prepared with six renderings at startup) and `sqlint` reports ok. Two operations need a native variant, the tree lock and the file-delete begin, and only the lock changes behavior; the begin saves one round trip. | `TestNew`, `TestVerify`, `mise run lint`; the two files under `lib/blobfs/data/pgnative/statements`. | The two variation points are the whole native surface; no third was needed by the delete or move stages. |
| 3. Two-phase write | The pending row survives a stop and a retry completes it. The step methods take `sqlate.Session`, not `*sqlate.Tx`, and the begin composes into the consumer's transaction. Failure is not a state; the pending row is. | `TestPutStopsAndResumes`, `TestFileWriteComposesIntoTheCallersTransaction`, `TestFileWriteRetryCompletesAPendingRow`, `TestPutIsThreeStepsWithTwoBoundaries`. | No fail step ships; the concept's "marks the write failed" is removed. |
| 4. Move under a lock | The lock closes the race on `pgnative`; the baseline forms a cycle, so the lock is a caller requirement on the baseline. Serializable isolation refuses one of two opposing moves on both variants with no cycle. | The suite's `MoveDirectory/OpposingConcurrentMoves` and `OpposingSerializableMoves` on `TestStandardConformance` and `TestConformance`; `TestMoveOpposingConcurrentMoves`. | The variant takes the lock, which the concept said `blobfs` could not; the baseline's requirement is documented (decision 4). |
| 5. Migrator | Integrated shipping. The shim is one file over the public API, needed nothing outside it, and its API is the multi-set API the concept describes; `Reset` and `Status` stay one operation each across both sets, an upgrade needs no consumer edit, and a consumer without `go-database` operates the schema in the two `Set` literals of `Sets` plus a command per verb. | `TestFreshReplay`, `TestUpgradeAfterRestart`, `TestResetOrderAcrossForeignKeys`, `TestConcurrentStartersSerialize`, `TestDirtyRefusalOnEngine`; `admin/schema/database.go`. | `blobfs.sources` promotes the shim's shape with the hooks of Finding 2, items 26 to 31. |
| 6. Delete | The delete converges under retry at each step on both variants, the recursive delete is correct under a concurrent insert, and a bookmark meeting a delete yields a classifiable error. `deleting` is needed. `blobfs` classifies a consumer's constraint by class (`ErrReferenced`), never by name. | The suite's `FileDelete` group; `TestRemoveConvergesAtEachStep`, `TestRemoveTreeRacesAnInsert`, `TestRemoveMeetsABookmarkAfterTheBegin`, `TestClassifyDelete`. | The concept's "leaves violations of a consumer's join-table constraints unclassified" becomes "classifies by class on a delete and leaves the name to the consumer". |
| 7. Key validation | The wiring is one method, `Storage.ValidateKey` over the provider's capability, and the interface carries no `MaxKeyLength`. A filename at the rune boundary is accepted and one rune over is refused before any SQL. The emulator proves nothing about the limit. | `TestBeginFileWriteKeyBoundary`, `TestNewKeyCountsRunes`, `TestPutNameAtTheRuneBoundary`, `TestAzuriteAndTheKeyLimit`, `TestStorageValidatesKeysWithTheProvidersRule`. | The concept's "key validation and a maximum key length" becomes "key validation". |
| V. Variant seam | Both variants pass the conformance suite, and a consumer-supplied variant swaps one method without a fork. `pgnative` imports no driver and adds no dependency weight, so the evidence supports shipping it in the module. | `TestStandardConformance`, `TestConformance`, `TestConsumerVariantSwapsOneMethod`; `split-check`'s rule for `pgnative`. | Module or sub-module is decision 7. |
| 8. Awkward call sites | Nineteen call sites had to work around what they were given: eleven sit in `sqlate` (Finding 2 has each), three in `go-storage` (no container delete, no blob listing, no flag beside the environment), four in the library's own layout rules (path resolution from the root, the shared field set, the storage type crossing the composition root, the variant constructor's type parameter), and one in the consumer's own constraint mapping, which is where it belongs. | `NOTES.md`, "Proof 8, the awkward call sites (stage 15)". | None directly; each names the entry that holds its evidence. |

One answer is only partly supported. V3's question whether any sort earns an index was answered
for `created_at` only; `size`, `status`, and the other declared fields were never timed, and the
answer for them rests on the plan shape (a bitmap scan of the directory and a top-N sort) rather
than a measurement.

## Amendments to make at close

The concrete edits the concept and the design notes need, listed and not made. Each is file, what
changes, and why.

| File | What changes | Why |
|------|--------------|-----|
| `context/concepts/blobfs.md`, "The schema" | Two tables with one seeded root (`RootID`, the nil UUID), the check constraint on the root's name, and the partial unique index `blobfs_uq_directory_root`; the sentence "Root directories need no constraint, since the consumer's table anchors every root" is removed; the constraint-naming scheme with `ix` is stated. | `0001_directory.up.sql`, `TestOneRoot`, `TestRootRule`. |
| `context/concepts/blobfs.md`, "Reading and composing ownership" | Ownership is a consumer join at the file grain (`bookmark`, the `org_image` case) or the directory grain (`directory_owner`, the scope checked once at the depth-one ancestor); the interim directive-filter paragraph is replaced by the listing as built: a statement anchored on one directory with the exact total in the same select list, composed in Go, with offset and keyset paging, and never a projection. The "path projection" pattern is gone; the published patterns are the two column lists. | Finding 1, "The listing"; `TestListScopedChecksTheAncestorOnce`. |
| `context/concepts/blobfs.md`, "The experiment", "Consumer side" | The `volume` and `volume_bookmark` paragraph is removed; the consumer keeps `directory_owner` and `bookmark`, and isolation is configuration (one database and one container per install). | The decisions log's "volume leaves blobfs" and `TestIsolation`. |
| `context/concepts/blobfs.md`, "Layers" | The two packages beside the persistence layer are named (`pgnative`, `datatest`), and "a second engine adds a directory and not a module" becomes "a directory and a variant per native operation". | Finding 1, "The layers and their import boundaries". |
| `context/concepts/blobfs.md`, "The write is a sequence of steps" | Step 3 loses "or marks the write failed"; "key validation and a maximum key length" becomes "key validation". | Proofs 3 and 7. |
| `context/concepts/blobfs.md`, "Moving" | "`blobfs` cannot take the lock itself" becomes: the variant takes it where the engine has a transaction-scoped lock, the baseline's lock is a documented no-op, and a baseline caller serializes moves at serializable isolation or outside the database. | Proof 4. |
| `context/concepts/blobfs.md`, "Deleting" | "leaves violations of a consumer's join-table constraints unclassified" becomes "classifies them by class on a delete as `ErrReferenced` and leaves the name to the consumer". | Proof 6. |
| `context/concepts/blobfs.md`, "Migration sources" | "`sqlate/migrate` v0.1.1 cannot host a source" becomes "cannot host several sets in one `Migrator`; one `Migrator` per set runs on v0.1.1, which the shim is". | Finding 2, item 32. |
| `context/concepts/blobfs.md`, "Assumptions" | "It walks from the roots" becomes "it walks upward from the directory"; "a file and a directory sharing a name stays unambiguous" gains "for every command that resolves one kind; `mv` resolves the directory first"; the "few lines" of key-validation wiring is confirmed at one method. | `directory_ancestors.sql`; the stage 13 decisions; proof 7. |
| `context/design/auth-strategy.md`, section 8 | Decide whether the directory-grain case joins the file-grain case. The section states the file-grain shape (the join table carries the unit and drives every authorized listing). The experiment adds the directory grain: the owner row binds a depth-one directory, the predicate runs once at the ancestor, and the library's listings run unchanged under the scope. The recommendation is to state both as consumer joins, with the directory grain's predicate at the ancestor and its move constraint (Finding 3, "What a move may and may not do"). | Finding 3, the two ownership sections. |
| `context/roadmap.toml`, `goals.blobfs.tasks.sources` | The summary is rewritten from Finding 2's `migrate` group: the multi-set `Migrator` over `Set` values with one lock and the hooks (exported default table, one connection per run, a drop of the history table, a one-read status, the documented `Steps` guarantee, and the mapped 2BP01). | Finding 2, items 26 to 32. |
| `context/design/storage-strategy.md` (if it names `blobfs`'s schema or the volume) | Checked at the review for the same volume and ownership sentences. | The reset file's "Retained" line names only the concept; the design note should be read once. |

## Decisions for the architect

The open review questions the stages flagged, each with its options and the recommendation the
evidence supports.

1. The seeded root. Options: keep the migration's seed of `RootID` (as built), or an `EnsureRoot`
   operation. Recommendation: keep the seed; it needs no lifecycle step, gives every consumer the
   same id, and `Root` fails with `ErrNotFound` on an unapplied schema, which is the right signal.
2. `NoTotal` on an empty page after the first. The library's window count travels on rows, so an
   empty later page reports `NoTotal` (-1) while a projection's count twin reports the exact total,
   and the binary's two read models differ on that edge. Options: keep the edge and document it
   (as built); run a count statement for that one case, which breaks the one-statement rule; or
   make the composer report the total from the previous page's cursor, which does not exist for an
   offset page. Recommendation: keep it, and let `sqlate` document the edge if it adopts the window
   total (Finding 2, item 2).
3. The tie-breaker's direction. Stage 8 changed the appended `name` to take the caller's
   direction when the terms share one, so a descending sort is the exact reverse of the ascending
   one and can take a cursor; the stage 7 offset order changed with it and the engine baselines
   were updated. Options: keep the rule (as built) or restore `name` ascending always and forgo
   cursors on descending sorts. Recommendation: keep the rule; a listing whose descending page is
   not the reverse of its ascending page surprises every reader.
4. `MoveDirectory` on the baseline. Options: a second standard variant that serializes through the
   root-row update idiom (`UPDATE blobfs_directory SET version = version WHERE id = root`, a row
   lock held to commit on every mainstream engine, but a guarantee resting on each engine's
   implementation rather than on the tier, and a write to the root on every move); document
   serializable isolation with a retry on SQLSTATE 40001, which `OpposingSerializableMoves` proves
   on both variants and which needs nothing from the variant seam; or both. Recommendation:
   document both and build neither until a consumer on an engine without a native lock exists;
   the isolation route is the one the evidence prefers, and `sqlate` should map 40001 first
   (Finding 2, item 15).
5. The `created_at` index. Migration 3 adds `blobfs_ix_file_directory_created`, which turns a
   page sorted by `created_at` without the total into an index read (2436 buffers to 25) and buys
   nothing for an exact-total page; it costs 3992 kB for 100,000 files against the name index's
   8776 kB, and a library that ships an index imposes its write cost on every consumer. Options:
   keep it in `blobfs`'s set (as built, because the rehearsal needed a real migration), or document
   the index and let a consumer's own set add it. Recommendation: move it to the documentation and
   the consumer's set before `blobfs.build`; the rehearsal has served its purpose, and nothing in
   the set is released.
6. Whether `mv` should take `--unit`. The scope rule is structural and needs no unit; a unit's
   right to move within its scope is authorization. Options: no flag (as built), or `--unit` that
   checks the source's top-level ancestor's owner. Recommendation: no flag until `go-auth`; the
   check would rehearse nothing the listing's check does not.
7. Whether `pgnative` ships in the module or a sub-module (proof V). The variant imports the
   persistence package and `sqlate` and no driver, so it adds no dependency weight; a sub-module
   would isolate nothing. Recommendation: in the module, as a package beside `data`.
8. The `datatest` package living in `lib/` as a non-test package. It is the only way another
   package's tests can import the suite, and it stays under `lib/` as a promotion candidate beside
   the package it tests, the way `net/nettest` sits beside `net`. Options: keep it (as built), or
   fold it into `data`'s tests and duplicate the checks in `pgnative`. Recommendation: keep it; a
   consumer that supplies a variant of its own needs it too.
9. The key validator as a parameter of the begin step (`BeginFileWrite(ctx, sess, keys, ...)`)
   rather than an option of `data.New`. Options: per call (as built, so the persistence package
   builds without the store and the consumer opens the store lazily), or a store-level option for
   one wiring point. Recommendation: per call; the lazy store is what lets the directory commands
   run without the object store's configuration.
10. The bookmark-versus-delete race. Options: accept the race with the foreign key as the backstop
    (as built), close it with serializable isolation on both first transactions and a retry, or a
    native row lock (`SELECT ... FOR SHARE`) as a third variation point. Recommendation: accept it;
    the outcome is a classified error and a state a rerun converges from, and the service's
    handler already has to report a conflict.
11. A capped total. The measurement does not decide it; `TotalNone` bounds the cost to zero and
    the cursor pages a large directory without it. Recommendation: no cap; a consumer with very
    large directories asks for the total once and walks by cursor.
12. `ls`'s two cursor flags (`--after-dirs`, `--after-files`). A single `--after` was rejected
    because a cursor is a position in one half's order. Recommendation: keep two; the service's
    API will page directories and files as separate resources anyway.
13. `Directory.Name` as `*string`. The root has no name, so every non-root call site nil-checks
    or dereferences. Options: keep the pointer (as built, the documented NULL contract the cursor
    also reads), or a string with the root's name empty and a check constraint that says so.
    Recommendation: keep the pointer; the nullable contract is what the cursor's refusal rule
    reads from the entity.
14. `rmdir` removing the owner row silently rather than refusing an owned directory. The
    alternative (`ErrOwned` mapped from `fk_directory_owner_directory`) would make an owned
    directory undeletable. Recommendation: keep it; the owner row is the consumer's record of the
    directory and nothing else removes it.
15. The root package as a layer of its own. It compiles alone but is vocabulary and validation, not
    a capability. Options: keep the three layers (as built), or fold the root into the persistence
    package. Recommendation: keep it; the constraint constants and the sentinels must live below
    the persistence package, and a consumer with its own persistence still takes them.

## What the experiment did not prove

- Authorization. `--unit` rehearses a directive filter's shape; the composed authorized listing
  under the auth strategy's scope predicate needs `go-auth` and is proven in `v1.auth`'s storage
  sweep.
- The admin surface through `go-web-service`. `admin/schema` stands in for `go-database`'s admin
  release; the migration path through the service's admin surface, including `Reset` and `force`,
  is `v1.storage.service`'s and `blobfs.admin`'s to prove.
- The documentation-only variant. It was estimated (the shim's 303 lines copied into every
  consumer, one `migrate.Migrator` per set with its own loop, lock, and reset) and not built.
- Other engines. The DDL is Postgres only, the baseline was run on Postgres only, and the port
  notes are the work list for a second engine; no dialect other than `postgres` was exercised.
- Sustained load. Every measurement is a median of five `EXPLAIN (ANALYZE, BUFFERS)` runs on one
  laptop with everything in shared buffers; no concurrency beyond two connections was measured,
  and the milliseconds depend on the machine, while the plan shapes and buffer counts do not.
- The sweeper. No sweeper for abandoned `pending` or `deleting` rows ships; the listing filters
  that a sweeper would use exist, and the concept's trigger is `v1.messaging`.
- Content replacement, versioning, soft delete, and checksums, which the concept defers.
- Key validation against a real Azure account. Azurite accepts keys `azureblob` rejects, so the
  provider's rule is proved by unit tests only.
- `sqlint`'s `[export]` entry for the published namespace. `sqlint.toml` registers the patterns as a
  bare directory under `[sources]`, which is what v0.1.1 offers.
- The interleaved directory-and-file listing, which the concept defers to a folder browser.

## Reproducing the results

Every command runs from `experiments/blobfs`, with the toolchain `mise.toml` pins (Go 1.27 and
`golangci-lint` 2.13.2) and the compose stack up. Every engine-backed test creates and drops its
own database and container, and none touches the default `app` database.

- `mise run up` starts Postgres 18 on `127.0.0.1:5434` and Azurite on `127.0.0.1:10000` and waits
  for both to report healthy.
- `mise run test` runs the hermetic tests (`go test -race ./...`). `go build ./...` and `go vet
  ./...` are `mise run build` and `mise run vet`.
- `mise run integration` runs every integration-tagged test (`go test -race -tags integration
  -count=1 ./...`): the migrations and the conformance suite on both variants, the persistence
  and consumer tests on the engine, the migrator's engine proofs, and the integration package.
- `mise run demo` builds the binary and runs `integration.TestScript` verbosely, once per variant,
  printing every command line and its output under `TestScript/<variant>/<step>`; the steps are
  `schema-up`, `directories`, `writes`, `bookmarks`, `deletes`, `moves`, `tree-lock`, and
  `schema-down`.
- `mise run evidence` regenerates the three transcripts under `evidence/`: `TestListingCost` in
  `lib/blobfs/data` writes `read-model.txt`, `TestBookmarkCost` in `domain/files` writes
  `bookmarks.txt`, and `TestSortIndexCost` in `lib/blobfs/data` writes `sort-index.txt`. Each is
  gated by `BLOBFS_EVIDENCE=1`, which the task sets, and each seeds its own throwaway database.
  `evidence/v1-read-model.txt` is the volume-era measurement, produced by a test deleted at stage
  6 and kept as the record; nothing regenerates it.
- `mise run split-check` fails when a package imports what its layer may not, and prints
  `split-check: ok` otherwise.
- `mise run lint` runs `golangci-lint` and `sqlint` (`sqlint: ok`).
- `mise run cli -- <command>` runs the binary against `BLOBFS_DSN` from `mise.toml`'s `[env]`
  table, which names the compose stack's default `app` database, and mise's value overrides a
  variable set on the command line. A command such as `schema up` therefore changes `app`. To
  work in a throwaway database, create one and pass `--dsn` explicitly. `mise run cli -- --help`
  lists the commands.
