# blobfs experiment notes

The running record of `blobfs.experiment`. Each stage appends its findings and decisions here, and
the final stage turns this file and the evidence into `REVIEW.md`. The design under test is
`context/concepts/blobfs.md`. The stage list and the run protocol are in the reset file
(`context/reset.md`).

## What the experiment answers

The experiment produces three findings, and the review is organized by them:

1. **The library.** How `blobfs` should be written: the layers, the standard-tier baseline with
   native variants, the listing and error-mapping design, and what a consumer composes.
2. **`sqlate`.** The adjustments `blobfs` needs from `sqlate`, each with its evidence.
3. **`v1.storage`.** How `blobfs` is incorporated into the `go-web-service` storage layer.

## Position at the time of writing (2026-09-20)

Stages 1 to 11 are committed: the single-root schema, the consumer's `directory_owner` and
`bookmark` tables, the persistence package with the listing composer and its keyset cursor,
`domain/files`, the listing evidence, and the variant seam: the `data.Variant` interface with its
two variation points, the standard baseline, the Postgres variant in `lib/blobfs/data/pgnative`,
and the conformance suite in `lib/blobfs/data/datatest`. Stage 10 added the
two-phase write in the persistence package (`FileByName`, `BeginFileWrite`, `CompleteFileWrite`
over four new standard-tier statements), the one-method key-validation interface, the storage
adapter in `domain/files/storage.go` over `go-storage` and `azureblob`, the object store opened
by the composition root on the first file command, and the `put`, `cat`, and `stat` commands with
`put --fail-after insert|write`. Stage 11 added the bookmark commands
(`bookmark add|ls|rm`, each under `--unit`), the consumer's bookmark read model (`bookmarks`, a
projection over `bookmark` joined to `blobfs_file` with each row's path computed by a recursion
correlated on the file's directory), the classification of the bookmark table's three
constraints into consumer sentinels, and the second evidence transcript. The binary serves
`schema up|down`, `mkdir <path> [--unit]`, `ls <path>` with `--page`, `--size`, `--sort`,
`--total none`, `--after-dirs`, `--after-files`, and `--unit`, the three file commands, and the
three bookmark commands; it builds its store over the baseline. `mise run evidence` writes
`evidence/read-model.txt`, the cost of the shipped listing, and `evidence/bookmarks.txt`, the
cost of the bookmark read model. Deleting and moving are later stages.

## Running it

`mise run up` starts Postgres on port 5434 and Azurite on port 10000. `mise run test` runs the
hermetic tests, and `mise run integration` runs the tests that need the services. `mise run lint`
runs `golangci-lint` and `sqlint`, and `mise run split-check` enforces the import boundaries.
`mise run evidence` regenerates the read-model measurement. `README.md` has the layout.

## Decisions log

Newest first.

### 2026-09-20: stage 11 decisions the plan did not spell out

- **What is shipped: the projection, with the recursion correlated per row.** The plan
  described the read model as a projection whose recursion is anchored on the bookmarked
  files' directories, and noted that a projection base cannot bind the unit. Built that way
  (a top-level `WITH RECURSIVE` anchored on every bookmarked file, the unit as a filter
  directive outside the derived table), the walk starts from every bookmark of every unit
  before the filter discards the rest: 165 ms and 3,023 buffers for a unit with 10 bookmarks
  among 53,110, and the same for 1,000. The shipped base instead computes each row's path by
  a scalar subquery containing the recursion, correlated on the file's directory. That base is
  a plain join the planner pulls up, so the unit filter reaches the bookmark index first and
  the walk runs once per row the filter keeps: 0.27 ms and 245 buffers for 10 bookmarks. It
  stays a `query.Projection`, so `--page`, `--size`, `--sort`, and `--total` come from the
  query library. The anchored plain statement (the unit bound inside the recursion, the shape
  a parameterized base would give) was measured beside it and not shipped: it is slower for a
  small unit (4.1 ms, 886 buffers for 10 bookmarks, because the planner hashes the whole
  directory table per recursion level) and about equal for a large one (14 ms against 17 ms
  for 1,000, with JIT off). Both hand-written shapes live in the evidence test only.
- **`add` of a file the unit has bookmarked already is refused**, with `ErrAlreadyBookmarked`
  mapped from the primary key `pk_bookmark`, active or not. An idempotent activation was
  rejected: it would turn the add into an update that must also obey the partial index, and
  the gate asks for the primary key's violation to reach the caller classified. A bookmark is
  added once and removed once, like a directory name.
- **`--active` makes the bookmark the unit's one active bookmark and is refused while another
  is active**, with `ErrActiveBookmark` mapped from `uq_bookmark_active`. The other bookmark is
  left as it is; there is no `--replace-active`. The refusal is the gate, and a swap is two
  commands (`rm` then `add --active`), which is one round trip more than a flag and no new
  statement.
- **`rm` of the active bookmark is allowed.** The partial index bounds how many bookmarks are
  active, not whether the active one may go; a unit with no active bookmark is a legal state
  (it is the state before the first `add --active`), and the next `add --active` fills it.
  `rm` of a bookmark the unit does not hold is `ErrNoBookmark` (zero rows affected); a missing
  file is `blobfs.ErrNotFound`.
- **A pending file can be bookmarked; a deleting file cannot.** The bookmark written beside a
  pending row is the `v1.storage` shape: the service inserts the pending row and its own row
  in one transaction, then uploads. `stat` and `bookmark ls` show the status. A deleting file
  is refused with `ErrNotAvailable` naming the status, because a new bookmark would hold a
  delete that is under way through the foreign key.
- **The foreign key's violation is `blobfs.ErrNotFound`.** `fk_bookmark_file` fails when the
  file was removed between its resolution and the insert. `TestAddBookmarkOfAFileRemovedMeanwhile`
  reaches it on the engine: a second connection holds an uncommitted delete, the add resolves
  the file and blocks on the foreign key check, and the commit fails the insert. The mapping
  is the consumer's (`bookmarkSentinels` in `database.go`), keyed on the consumer's constraint
  names, which `errors.go` exports as constants and the migrations test checks against the DDL.
- **The constraint names were already explicit.** `pk_bookmark`, `fk_bookmark_file`, and
  `uq_bookmark_active` follow `<kind>_<table>_<detail>` without the `blobfs_` prefix, so the
  consumer migration was not amended and no hash moved.
- **The transaction boundaries.** `AddBookmark` runs the parent's resolution, the file's
  lookup, and the insert in one transaction, the pattern of `Put`'s first step and of `mkdir
  --unit`; `create_bookmark` carries no `transaction: required` header, because one insert is
  correct on the pool too and a service composes it into its own transaction.
  `RemoveBookmark` runs on the pool: the delete is keyed by the unit and the file's id, so
  nothing needs the resolution and the delete to share a snapshot. `ListBookmarks` runs the
  projection's count and page in one read-only repeatable-read transaction, as `ls` does.
- **The key and the default sort.** The key is `file_id`, the default sort is `path`. Within
  one unit, which every read filters by, a file is bookmarked at most once, so any sort
  followed by `file_id` is total; over the unfiltered base the unique key is the pair
  `(unit_id, file_id)`, which a projection's one-field key cannot state (ledger). `path` is
  what a reader wants, and it costs the unit's bookmark count times the depth, because the
  sort needs every row's path; `--sort file_id` is the bookmark index's own order and costs
  the page's rows times the depth (0.23 ms and 493 buffers for 1,000 bookmarks against 17 ms
  and 21,940 by path). The evidence records both.
- **`--total none` on the bookmark listing reads the count and drops it**, as `ls / --unit`
  does: `Projection.List` always runs its count twin. The count is cheap here (the planner
  drops the path subquery from a count over the pulled-up base: 0.06 ms and 35 buffers for 10
  bookmarks, 0.93 ms and 3,012 for 1,000), so the flag saves nothing measurable.
- **An empty later page keeps the exact total.** The projection's count is a statement of its
  own, so page 4 of a five-row listing reports total 5 where the library's window count
  reports `NoTotal`. The engine test pins the difference.
- **The evidence test is in the package itself.** `TestBookmarkCost` is `package files`, not
  `files_test`, so it can run the unexported `bookmarksOf` through a recording session and
  capture the SQL the projection composes; the hand-written comparison forms are constants in
  the test. `mise run evidence` runs it after the library's `TestListingCost` and writes
  `evidence/bookmarks.txt`.
- **The paging flags are shared.** `pageFlags` (`--page`, `--size`, `--sort`, `--total`) is the
  flag set both `ls` and `bookmark ls` bind; `listingFlags` embeds it and adds the cursors and
  the unit. `bookmark ls` takes `--unit` as a required flag and no positional argument.

### 2026-09-20: stage 10 decisions the plan did not spell out

- **Fail-write has no representation, by choice.** Of the three options (delete the pending
  row, a fourth `failed` status, or no fail step), the write path ships no fail step. A stop
  between the steps leaves the row `pending`, which is the queryable state the gate asks for,
  and a retry of the same write completes it; a `failed` status would name a state the delete
  path already handles, since `BeginFileDelete` is allowed from `pending`, and a delete of the
  pending row would erase the state a retry needs. What a caller does with an abandoned write:
  it retries (a `put` of the same path), or it removes the row through the delete steps (stage
  12's `rm`), whose object delete tolerates a missing key. A sweeper that lists `pending` rows
  older than a threshold and runs the delete steps is the consumer's, as the concept says. The
  transition table is unchanged, and its comment records the decision.
- **How a retry resumes.** `put` looks the name up in the parent before it inserts. A `pending`
  row under the name is taken up at its id, key, and version: the object is stored again under
  the same key, which replaces an object an earlier attempt left, and the complete step runs at
  the version read. An `available` or `deleting` row under the name is `blobfs.ErrNameTaken`
  with the status in the message, because the consumer has no content replacement. Refusing a
  pending name with `ErrNameTaken` was rejected: the gate says a retry completes the row, and a
  user who sees `pending` in `stat` needs one command to finish it.
- **The transaction boundaries of `put`.** Three steps, two boundaries. The first step is one
  transaction: resolve the parent, look the name up, insert the pending row, read it back, and
  commit. It commits before any byte reaches the store, so the row is durable, and it is the
  transaction a consumer extends with its own rows. The second step, the object write, runs
  outside any transaction. The third step runs on the pool: the guarded update and the
  read-back. `TestPutIsThreeStepsWithTwoBoundaries` pins the driver operations, and
  `TestFileWriteComposesIntoTheCallersTransaction` proves the begin step inside a consumer's
  `Transact` beside a consumer table that references `blobfs_file`: a rollback leaves neither
  row, a commit both. The object store is opened and started before the first transaction, so
  an unreachable store fails the put before any row is inserted.
- **The step methods take `sqlate.Session`, not `*sqlate.Tx`** (proof 3). `begin_file_write`
  carries no `transaction: required` header: one insert and its read-back are correct on the pool
  as `Mkdir` is, and a consumer that wants the row beside its own passes its transaction.
  `complete_file_write` is one guarded statement and runs on the pool. Nothing in the write path
  needs the header.
- **The key-validation interface has one method.** `blobfs.KeyValidator` lost `MaxKeyLength`,
  and `NewKey` runs the store's `ValidateKey` alone. Proof 7's measurement: `azureblob`'s
  `ValidateKey` enforces its 1,024-rune limit itself (`keys.go`), the storage fake's default does
  the same, and go-storage's `Capabilities` contract reads that way; and `MaxNameLength` (255
  runes) keeps every key blobfs builds at 292 runes at most, so a length check in blobfs never
  fires against a real provider. The adapter is one method over `store.Capabilities().ValidateKey`.
  The rune-boundary proofs: `TestBeginFileWriteKeyBoundary` (a validator with a 60-rune limit,
  a key of exactly 60 runes and 83 bytes accepted, one rune over refused before any SQL),
  `TestNewKeyCountsRunes` in the root package, and end to end `TestPutNameAtTheRuneBoundary`
  and the binary's `TestWriteCommands`: a name of 255 `é` (a key of 292 runes, 546 bytes) is
  stored in Azurite and read back, and 256 is `ErrInvalidName`.
- **The validator is a parameter of the begin step.** `BeginFileWrite(ctx, sess, keys, ...)`
  takes the `blobfs.KeyValidator` per call instead of `data.New` taking it as an option, so the
  persistence package builds without the object store and the store can be opened lazily by the
  consumer; the tests also swap validators without rebuilding the store. A store-level option
  is the alternative if the review prefers one wiring point.
- **The complete step stores what `Put` returned.** `blobfs.Object{Size, ContentType, ETag}` is
  built from the store's answer to the put: the size the provider counted, the entity tag the
  service assigned, and the content type as sent, which is the ledger's finding that `Put`
  echoes the caller's type. A second round trip (`Stat` after `Put`) to read the server's type
  was not added; `TestPut` proves the row's size, etag, and content type equal the service's
  `Stat` of the object on Azurite, so the two agree for what the consumer declares.
- **The declared content type.** `put --content-type` sets it; without the flag it is the type
  registered for the local file's extension (`mime.TypeByExtension`, so `.txt` declares
  `text/plain; charset=utf-8`), or `application/octet-stream`, which stdin always gets.
- **The steps `--fail-after` names.** `insert` (the pending row is committed, nothing reached
  the store) and `write` (the object is stored, the row is not completed). There is no stop
  after complete, because nothing follows it; `complete` is refused before the store is
  constructed. The stop is a `files.StopError` matching `ErrStopped`, exit code 1, with the
  pending row's id and the instruction to rerun `put`. A failed object write leaves the same
  state as a stop after `insert`, and its message says the row stays pending.
- **`cat` of a `pending` or `deleting` file.** Refused with `files.ErrNotAvailable` and the
  status in the message, before the store is asked: a pending file's object may not exist and a
  deleting file's is being removed. `stat` shows any status and never consults the store. An
  `available` row whose object the store does not hold is `files.ErrObjectMissing`.
- **The object store is opened lazily, by the composition root.** `Infrastructure.Storage(ctx)`
  opens and starts the store on its first call, as `Database` opens the pool, and `Close` shuts
  it down; `files.New(db, opener)` receives the opener and calls it on the first file operation.
  `mkdir` and `ls` therefore never read `BLOBFS_STORAGE_*` and run with Azurite down. The
  configuration is the storage package's own `Config.Finalize("blobfs")`, so the variable names
  are the library's, not the experiment's.
- **`Start` at the composition root, not in the adapter.** `files.OpenStorage` finalizes the
  config, builds the `azureblob` client and the `storage.Store`, and calls `Start` (container
  ensured, provider probed) before it returns, so a rejected credential or an unreachable
  endpoint fails the first file command as `ErrStorageUnavailable` before any row is written.
- **The adapter's error mapping.** `storage.ErrNotFound` is `files.ErrObjectMissing`,
  `storage.ErrUnavailable` and `storage.ErrNotReady` are `files.ErrStorageUnavailable`, and
  `storage.ErrTooLarge` is `files.ErrObjectTooLarge`, each with the store's sentinel still
  reachable; a failure of the body passes through unclassified, as the provider documents.
- **A per-test Azurite container.** `livetest.Container(t)` names a container
  `blobfs-test-<suffix>` and deletes it when the test ends, through the Azure SDK's container
  client, because go-storage has no container delete. The store's `Start` creates it, so the
  tests drive the binary's own path. The integration package passes the name to the child
  through `BLOBFS_STORAGE_CONTAINER`; the domain's tests set it with `t.Setenv`.
- **`split-check` is unchanged.** Rule 6 already named `domain/files/storage.go`; the helper
  in `internal/livetest/storage.go` names the Azure SDK and not go-storage, so no rule moved
  and none needed proving. The rule-6 grep was exercised by the new file itself: it is the one
  application file matching `"github.com/standards-lab/go-storage`.

### 2026-09-20: stage 9 decisions the plan did not spell out

- **The interface.** `data.Variant` has three methods: `LockTree(ctx, sess)`, `Serializes()
  bool`, and `BeginFileDelete(ctx, sess, id) (blobfs.File, error)`. `Serializes` is the
  capability probe: a caller that needs moves serialized checks it before the first move rather
  than learn from a cycle. Both operations take `sqlate.Session`, as every method of the store
  does, and the transaction requirement is enforced the way the concept says the write path's
  is: by the statement's `transaction: required` header, or by the same type assertion in Go
  where no statement runs.
- **How a variant is supplied.** `data.New(catalog, dialect, opts...)` takes a functional
  option, `data.WithVariant(v)`. A variant is built by its own constructor against the same
  catalog and dialect (`pgnative.New`, `data.NewStandard`), because the variant's statements
  compile against the consumer's catalog like the store's do and the store cannot build a type it
  does not import. The default, with no option, is `Standard` built over the store's own compiled
  statements, so the default costs no second compile. `Store.Variant()` returns the variant, so a
  consumer can reach a capability beyond the interface.
- **The store forwards.** `Store.LockTree`, `Store.Serializes`, and `Store.BeginFileDelete`
  forward to the variant and add the operation's context to an error; the consumer and the later
  stages call the store, never the variant. `Store.Statements()` appends the variant's inventory
  when the variant has one, and `Store.Verify` runs the variant's `Verify` when it has one, so
  `pgnative`'s two statements are listed and prepared at startup with the rest. `Standard`
  exposes neither, because its statements are the persistence package's own and the store already
  lists and verifies them.
- **The baseline's statements live in the package's statement directory.** `begin_file_delete`
  (the update, headed `transaction: required`) and `file_by_id` (the read-back) are ordinary
  statements of `lib/blobfs/data/statements`, and `file_by_id` also serves the new `Store.File`,
  which stage 10's `stat` will use. So the store compiles and verifies the baseline's begin even
  when `pgnative` replaces it; the baseline is complete on every engine and always verified.
- **What the baseline's tree lock does.** Nothing, and `Serializes` reports false. Standard SQL
  has no statement that holds a lock to commit: `SELECT ... FOR UPDATE` is not standard tier, and
  the update-the-root-row idiom (an `UPDATE blobfs_directory SET version = version WHERE id =
  root`, which takes a row lock held to commit on every mainstream engine) was considered and set
  aside because the standard says nothing about locks, so its guarantee would rest on each
  engine's implementation rather than on the tier, and because it writes a tuple to the root on
  every move. The no-op still refuses a pool session with `query.ErrTransactionRequired`, so a
  caller gets the same refusal on every variant. The consequence is the stage 13 gate: on the
  baseline, two opposing concurrent moves can form a cycle, and a consumer that needs the
  guarantee on an engine without a native variant serializes moves outside the database. The
  root-row idiom is recorded here as the candidate if the review wants a serializing baseline; it
  would be a second standard variant, not a third variation point.
- **The Postgres lock key.** `pg_advisory_xact_lock` takes a bigint, so the key is the 64-bit
  FNV-1a hash of `pgnative.TreeLockName` (`blobfs_directory.tree`), exported as
  `pgnative.TreeLockKey` and pinned by a test, so a consumer that takes advisory locks of its own
  can avoid it and a port can derive the same one. The lock statement is headed `transaction:
  required`, because a transaction-scoped lock taken under autocommit is released at once.
- **Idempotence of the begin.** A begin on a row already deleting changes nothing and returns
  the row: the baseline's update carries `AND status <> 'deleting'`, and the Postgres statement's
  `CASE` expressions keep the version and `updated_at` when the old status is deleting. The
  version therefore advances exactly once per delete, on the first begin, in both variants. The
  transition table in `lib/blobfs/status.go` already allows pending, available, and deleting to
  deleting, so no status change was added; stage 12's complete step needs the `DELETE ... WHERE
  status = 'deleting'` and the cleanup of a bookmark, neither of which stage 9 adds.
- **The pool on the begin.** The baseline requires a transaction, because its read-back needs
  the update's row lock to hold so a concurrent complete cannot remove the row between the two
  statements. The Postgres begin is one statement and accepts the pool; the interface's contract
  says a portable caller passes a transaction. The conformance suite runs every begin in a
  transaction, and each variant's own tests pin its pool behavior.
- **The conformance suite's home.** `lib/blobfs/data/datatest` is a non-test package with one
  exported function, `Run(t, db, store)`, because a helper in a `_test.go` file cannot be
  imported by another package's tests and `pgnative`'s tests must run the same checks as the
  persistence package's. It takes the migrated database from the caller and imports no test
  infrastructure of the experiment, so it stays under `lib/` as a promotion candidate beside the
  package it tests, the way `net/nettest` sits beside `net`. A consumer that supplies a variant of
  its own runs it too. `split-check` holds it to the persistence package and `sqlate`, never a
  variant, and forbids any non-test file from naming it; both rules were proved by a temporary
  violation, and so was `pgnative`'s rule (an import of `lib/migrator`).
- **Two directories for the statements, one lint.** `sqlint`'s native-forms check keys on each
  file's declared tier, not on its directory, so the Postgres variant's directory is added to
  `[statements].dirs` with the check left on: a native file is exempt by its tier and must carry
  its port note to compile, and a file in that directory that declares the standard tier is still
  held to the standard forms. Both refusals were exercised by hand (a lock statement without its
  port note, and the same statement declared standard).
- **The consumer is unchanged.** `domain/files` builds its store with `data.New` and no option,
  so the binary runs the baseline. Stage 15's per-variant run will need the composition root to
  choose the variant; nothing in stage 9 adds a flag for it.

### 2026-09-20: stage 8 decisions the plan did not spell out

- **The cursor's encoding.** A cursor is base64url over an eight-byte SHA-256 prefix and a JSON
  body holding the encoding version, the name of the statement that issued it, the sort terms
  (field and direction) it was issued under, and the sort values of the last row of the page as
  text: a uuid in canonical form, a bigint in decimal, a timestamp in RFC 3339 at nanosecond
  precision in UTC, text as it is. The checksum is an integrity check and not authentication:
  an edited or truncated cursor is refused before any SQL, and so is one whose values do not
  parse as their field's declared type. A cursor carries no secret because a forged well-formed
  cursor positions the listing where a filter could, and nothing more. Every refusal is a
  `data.CursorError` that unwraps to `query.ErrDirectives`.
- **What a cursor binds.** The listing and the sort, not the directory. A cursor issued for one
  directory continues the same listing of another directory: it is a position in a sort order.
  A cursor from the other listing, or one issued under other terms or directions, is refused
  with a message naming both.
- **The total under a cursor.** A cursor page carries `NoTotal` whatever `Total` says, and it
  runs the plain statement. The keyset predicate is a WHERE predicate, so a window count under it
  would count the rows after the cursor, which is a different quantity and must not be called
  the total. A caller that wants the total reads an offset page under `TotalExact`; `Next` is
  filled on offset pages too, so page one with its total and then a cursor walk is the intended
  shape. Refusing `TotalExact` with `After` was rejected because `TotalExact` is the zero value
  of `Total`, so every cursor call would have had to say `TotalNone`.
- **The page number under a cursor.** Ignored; it may be zero. The consumer's `--page` applies
  to a half read by number and is ignored by a half read after a cursor.
- **The extra row.** Whenever the sort can be continued by a cursor, the composer fetches one row
  beyond the page and drops it; its presence fills `Next`. This holds under `TotalExact` too,
  where the total would have told, so one rule serves both modes. The bound fetch count is
  therefore the size plus one, and the hermetic tests pin it.
- **The tie-breaker's direction.** The appended `name` takes the direction of the caller's terms
  when they share one, so `created_at:desc` orders by `created_at DESC, name DESC` and a
  descending sort is the exact reverse of the ascending one. Stage 6 appended `name` ascending;
  under that rule every descending sort would have mixed directions and no descending sort but
  `name` could take a cursor. Mixed caller terms still get `name` ascending. The stage 6 and 7
  engine baselines were updated for the new order.
- **Mixed directions are refused, by choice.** The expanded `OR` form handles a term-by-term
  direction, so the refusal is not a limit of the form; it keeps the cursor's contract one
  sentence long, and the query library's `Projection`, which has no cursor, sets no precedent.
- **Nullable fields come from the entity type.** A field whose Go field is a pointer (`size`,
  `etag`, `parent_id`) is nullable; the key is exempt. A sort whose terms up to the key name one
  cannot be continued: `After` is refused and `Next` is empty, and the page fetches exactly its
  size. The pointer fields are the entity's documented NULL contract, so no second declaration
  was added.
- **Terms after the key.** The cursor terms are the ORDER BY terms up to and including the key,
  because a term after a unique key orders nothing. `--sort name --sort size` continues by name
  alone, and `size` being nullable does not matter there.
- **`Verify` prepares a cursor rendering.** One per listing, over every field a cursor can
  continue, so the keyset predicate prepares against each declared type at startup: fourteen
  prepares in the library, eighteen in the consumer.
- **Two flags for two halves.** `ls` has `--after-dirs` and `--after-files`, and prints
  `next-dirs:` and `next-files:` lines, one per half that has a next page. A half without a
  cursor is read by number. A single `--after` was rejected because a cursor is a position in one
  half's order and `ls` prints two halves; a single flag with `--only dirs|files` would have
  added a listing mode to carry one flag.
- **The owner read model takes no cursor.** `ls / --unit` reads the consumer's projection, and
  `query.Projection` pages by number only, so a cursor there is `files.ErrNoCursorAtRoot`, refused
  before any I/O. Recorded in the `sqlate` ledger.
- **The evidence moved into the library's test tier.** `TestListingCost` lives in
  `lib/blobfs/data` under the `integration` tag and `BLOBFS_EVIDENCE=1`, because the shipped
  listing is the library's and the measurement needs no consumer table. `mise run integration`
  skips it. The fixture is seeded through `unnest` and vacuumed after seeding, and section 0
  measures the biggest directory's count before and after `VACUUM`.

### 2026-09-20: stage 7 decisions the plan did not spell out

- **The transaction options.** `Store.List` runs through `DB.Transact` with `sqlate.ReadOnly()`
  and `sqlate.Isolation(sql.LevelRepeatableRead)`. pgx renders them as `BEGIN ISOLATION LEVEL
  REPEATABLE READ READ ONLY`. The hermetic recording pins both options on the one begin, and
  the engine test lists while a second connection commits a directory and a file together and
  sees equal totals on every run.
- **A sort term and the two halves.** The file half takes every `--sort` term and refuses a field
  it does not declare. The directory half takes the terms whose field both listings declare
  (`id`, `name`, `version`, `created_at`, `updated_at`) and ignores the rest, so `--sort
  size:desc` orders the files by size and leaves the directories in name order. `parent_id` is
  not in the set: every row of one listing shares it. The owner projection takes the same set.
  The set is restated in the consumer and pinned by a hermetic test.
- **How `ls` shows a total.** One line per half: the rows on the page, the page number and size,
  and `total N`, `total not counted` under `--total none`, or `total unknown (the page is empty)`
  for an empty page after the first, where the window count travels on rows and the page has
  none. An empty first page says `total 0`.
- **How `--unit` refusals classify.** A unit that does not own the top-level ancestor, and a
  top-level directory with no owner row, are `files.ErrNotOwned`. The check runs before the rest
  of the path resolves, so a foreign unit learns nothing below the ancestor; an ancestor that
  does not exist is `blobfs.ErrNotFound`. `mkdir --unit` below depth one is `files.ErrUnitDepth`,
  refused before any I/O. The `--unit` value is parsed and re-rendered in canonical form, so the
  comparison with the engine's text is case-insensitive to what was typed.
- **`ls / --unit`.** The directory half is the owner projection filtered by the unit, converted
  to `blobfs.Directory` so the result has one shape. The file half is empty with total 0: a file
  in the root has no top-level ancestor and belongs to no unit.
- **`mkdir` and its parents.** There is no `-p`; a missing parent is `blobfs.ErrNotFound`. `mkdir
  /` is `blobfs.ErrRootDirectory`, a trailing slash `blobfs.ErrInvalidPath`. Without `--unit` the
  two statements run on the pool; with `--unit` the parent's resolution, the insert, the read-back,
  and the owner insert run in one transaction, and a refused owner rolls the directory back.
- **The ancestor is resolved twice.** `ResolveDirectory` resolves from the root only, so a scoped
  `ls /a/b` resolves `/a` for the scope check and then `/a/b` for the listing; the root read and
  the first child read run twice. Recorded in the library ledger.
- **`--after` is not registered.** Stage 8 adds the flag with the cursor.
- **The rendering.** `mkdir` prints one result line through `Line`. `ls` prints through
  `output.Listing`, which takes `output.Entry` rows and one `output.Page` per half.
- **The `evidence` task stays stale.** It still points at `./domain/volume`. A path-only fix would
  let a run overwrite the V1 transcript with an empty run, since the test does not exist yet;
  stage 8 rewrites the task with the test.

### 2026-09-20: stage 6 decisions the plan did not spell out

- **The page and total convention.** `Page[T]` carries `Rows`, `Total`, and `Next`. `Total` is
  the window count the rows carried, or `NoTotal` (-1) when the listing asked for `TotalNone`. An
  empty first page has the exact total 0, because no row matched. An empty page after the first
  also reports `NoTotal`: the window count travels on rows, and a page past the end has none. The
  composer runs no count statement in that case, so the "one statement" rule holds everywhere.
- **The cursor field.** `Listing.After` and `Page.Next` are declared now with the shape stage 8
  fills. A non-empty `After` is refused with `query.ErrDirectives` until then, and `Next` is empty.
- **The root sentinel.** `blobfs.ErrRootDirectory` names a refusal that targets the root: a
  second row with no parent, and the delete, move, and rename of the root in later stages. The
  write mapping maps `blobfs_uq_directory_root` to it. No library statement can violate that
  index, because `create_directory` always binds a parent and a validated name, so the mapping is
  proved by a table test on the classifier and the constraint by the migrations test.
- **The root in listings.** `Children` never lists the root: the root has no parent, so no
  `parent_id` equals its parent. `Children(RootID)` lists the depth-one directories. `Root` is a
  read of `RootID` through `directory_by_id`, and it fails with `ErrNotFound` on a database whose
  schema is not applied.
- **`Mkdir` and the root.** `Mkdir` refuses an empty name through `ValidateName`, and every row
  it writes has a parent, so no call of it creates a root. The schema's seed is the only way a
  root row exists.
- **Statement names.** The listing statements are `files_in_directory`,
  `files_in_directory_with_total`, `children_of_directory`, and `children_of_directory_with_total`:
  two authored files per listing, so the window count is authored SQL and the composer inserts
  nothing into a select list. `directory_ancestors` is the upward walk behind `DirectoryPath`.
- **The correlation name `q`.** The listing statements alias their table as `q`, because the query
  library's clause patterns qualify every field as `q.<field>` (see the ledger). The published
  column-list patterns keep `d` and `f`, so the listing statements spell their columns out.
- **The consumer in the meantime.** `domain/volume` is deleted whole (every file was volume-based),
  `internal/app/domain.go` mounts nothing, the integration script checks `schema up|down` and the
  seeded root, and `sqlint.toml` lists only the library's statements. `README.md`, the
  `split-check` renames, and the `evidence` task wait for stages 7 and 8.
- **Unique index, not `NULLS NOT DISTINCT`.** `blobfs_uq_directory_root` is a partial unique
  index over the expression `(parent_id IS NULL)`, so it needs no Postgres 15 feature and reads
  the same on any engine with partial indexes.

### 2026-09-20: volume leaves blobfs, one root per install

The architect reversed the decision to make `volume` a core table. A volume is an opinionated way
to segregate directories inside one container, and an application owner can build that on top of
the baseline. `blobfs` provides the container-based directory and file infrastructure. A
configuration points at one container, which is the root of the tree, and a consumer that wants
several isolated trees runs several configurations, each with its own container and database.

The schema returns to two tables with one seeded root row (`RootID`, the nil UUID), enforced by a
partial unique index. An `EnsureRoot` operation is the alternative to seeding. The migration seed
was chosen because it needs no lifecycle step and gives every consumer a known id, and the review
should confirm it.

Making volume a core table had real benefits: unique roots, a clean anchor for path resolution,
and a declarative rule that a root belongs to exactly one volume. The costs were an opinionated
segregation in the library and a listing anchor that the consumer's ownership join could not
compose with. The application owner can build the same segregation on the baseline, and the
consumer side of this experiment rehearses two ways to do that.

### 2026-09-20: the listing is anchored on a directory and returns its own total

The V1 measurement showed the file listing built from a whole-forest recursion costs in proportion
to the number of directories, whatever the size of the listed directory. The architect ruled that
a listing must be built around the most performant execution and that its total must agree
exactly with its page. The listing is therefore a statement anchored on one directory, with the
total computed in the same statement by `COUNT(*) OVER ()`, composed in Go over authored
statements. A keyset cursor ships beside offset paging. The path stays a read-time computation, and
no `volume_id` or stored path is added.

### 2026-09-20: the consumer keeps two tables of its own

`directory_owner(directory_id, unit_id)` rehearses the document hierarchy per organization: the
scope is checked once at the depth-one ancestor. `bookmark(unit_id, file_id, active)` with a
partial unique index rehearses the `org_image` case. Both are consumer tables. The library holds no
owner and no unit.

### 2026-09-19 and 2026-09-20: earlier decisions still in force

- One `blobfs` install per database, with fixed `blobfs_` object names.
- Native-tier statements are allowed as variants behind a narrow Go interface. The standard-tier
  baseline is complete on any engine, and published patterns stay standard tier.
- The documentation-only variant is estimated and not built. The migrator shim is a drop-in for
  the multi-set API that `blobfs.sources` describes. Ids are minted in Go.
- The consumer follows the `slab` elemental layout, and cobra is adopted for the consumer only.

## Ledger: what the experiment has found

### Adjustments `sqlate` needs

- **A parameterized projection base.** A projection base cannot bind a parameter, so a listing
  anchored on one directory cannot be a projection. The parameterized base the auth strategy names
  as the arity-one lift is the fix, and this experiment gives it a second motivating case besides
  the scope predicate.
- **The total in the page statement.** A total computed by a separate count statement can disagree
  with its page. The total belongs in the select list of the page statement.
- **Clause composition at the base's level.** The derived-table wrap forces a `SubqueryScan` for a
  base that contains `WITH RECURSIVE`. Stage 8 measures whether composing the clauses inside the
  statement is required or the wrap suffices.
- **`UnknownFieldError` unwraps to `ErrDirectives`.** That is the sentinel a generic handler maps
  to a client error, so a forgotten scope filter fails as a client error unless the consumer
  matches the type. The error carries `Use: filter`, which lets a consumer tell them apart.
- **`postgres.Dialect.MapError` does not map SQLSTATE `2BP01`.** Dropping a table that another
  object depends on fails with `dependent objects still exist`, and the error reaches the caller
  unclassified.
- **`migrate` exports no default table name and no accessor, and cannot take a connection or a
  lock from the caller.** A multi-set migrator with one outer lock therefore repeats the default
  table name and pins its own connection.
- **`Steps` tolerates a count larger than the applied prefix.** The shim relies on it to revert a
  whole set. It should be a documented guarantee.
- **`StandardCatalog.HistoryExists` does not qualify by schema.** A same-named table in any schema
  satisfies the check.
- **`query.Guard` reports only a version mismatch.** A mutation refused for a `deleting` status
  needs its own check, because the guard's check statement returns only a version.
- **`--| field:` timestamp types.** In standard tier the type must be spelled `timestamp with time
  zone`, because `timestamptz` is a native form. `Verify` never checks field types, so a wrong
  spelling surfaces only when someone filters on the field.
- **A library that ships statements hard-codes the `sql.` namespace.** A consumer that aliases
  `sqlate`'s source with `As` breaks the library's compile.
- **The scanner does not flatten embedded structs.** A consumer read model restates every column
  of a library entity, including columns it does not use.
- **`sqlate.Session` cannot begin a transaction.** A library cannot give an operation that reads
  twice a consistent snapshot, so the caller must open a read-only repeatable-read transaction.
- **Path resolution takes one round trip per segment.** Standard SQL has no ordered array
  parameter, and `{{name...}}` renders an `IN` list.
- **Multi-statement transactional migrations work on pgx.** pgx uses the simple protocol when a
  statement has no arguments. The concept's claim that v0.1.1 cannot host a source holds only for
  one merged `Migrator`: a `Migrator` per set with its own `Options.Table` runs on v0.1.1.
- **The catalog exposes its inventory and not its renderer (stage 6).** `Catalog.render` is
  unexported, so a composer outside the projection reads the clause patterns' text through
  `Catalog.Patterns()` and fills the slots with its own copy of the slot regex. The composer in
  `lib/blobfs/data/listing.go` is that copy. A `Catalog.Render(name, fill)` method, or an
  exported clause composer, removes the duplication.
- **The clause patterns fix the correlation name `q` (stage 6).** `filter_*`, `order_term`, and
  `order_term_desc` spell every field as `q.<field>`, the derived table's alias. A statement that
  composes the clauses at its own level must therefore alias its table as `q`, which is why the
  listing statements read `FROM blobfs_file q`. Making the qualifier a slot, or publishing
  unqualified terms, would let a statement keep its own alias.
- **`Scanner` refuses a column with no field (stage 6).** A page statement that carries
  `COUNT(*) OVER () AS total` beside the entity columns cannot scan through `query.Scanner[T]`,
  because the total has no field on the entity. The composer keeps a scan of its own that reads
  the entity's fields by tag and the total into an `int`. A scanner that takes extra
  destinations, or an entity wrapper the mapper flattens, would remove it.
- **The window total travels on rows (stage 6).** `COUNT(*) OVER ()` gives every row the total,
  and an empty page carries none. An empty first page is the exact total 0; an empty later page
  has no total from the statement. The composer reports `NoTotal` there rather than run a count
  twin. A library that offers the window total should document this edge.
- **The composer needs the dialect (stage 6).** `Statement` does not expose the dialect it
  compiled against, only its catalog, so the composer takes the dialect from `New` for the
  placeholders it appends after the statement's own.
- **`Statement.Text()` ends where the file ends (stage 6).** The composer appends `AND`, `ORDER
  BY`, and the paging clause to `Text()`, which works because the loader trims a trailing
  semicolon and whitespace. A listing statement must therefore end with its `WHERE` clause; the
  hermetic test pins the rendered suffix, and nothing in the loader states the rule.
- **A projection cannot skip its count (stage 7).** `Projection.List` always runs the count twin
  before the page. The consumer's `ls / --unit --total none` reads the count and drops it. A
  total mode on `Directives`, or the window count in the collection pattern, would remove the
  statement.
- **The transaction options compose as needed (stage 7).** `DB.Transact` takes `ReadOnly()` and
  `Isolation(sql.LevelRepeatableRead)` beside the function, and every library method takes the
  `*Tx` as its session, so the consumer gave `ls` one snapshot with no library change. This is
  the consumer-side answer to the finding that `sqlate.Session` cannot begin a transaction.
- **The scanner rule reaches the consumer's read model (stage 7).** `OwnedDirectory` restates
  every column of `blobfs.Directory` beside `unit_id`, `parent_id` included though the read model
  never uses it, and converts back to the library type for the result. An instance of the
  embedded-struct finding above.
- **`Projection` has no keyset paging (stage 8).** `Directives` carries a page number only, so
  the consumer-anchored read model cannot continue by cursor, and `ls / --unit` refuses
  `--after-dirs`. A cursor on `Directives`, with the composer's rules (the key as the
  tie-breaker in the sort's direction, one direction, no nullable term), would let a projection
  page the way the library's listings do.
- **The wrap over a recursive base loses the index order (stage 8, confirmed).** Section e3 of
  the evidence: the same base, the upward walk joined to the files of one directory, pages in
  0.07 ms and 21 buffers flat (an index scan in name order under a `Limit`) and in 5.1 ms and
  2443 buffers wrapped (a bitmap scan of the whole directory and a top-N sort above the join).
  PostgreSQL 18 shows no `Subquery Scan` node, because a trivial one is elided, but a subquery
  that contains a CTE is not pulled up and is planned as its own unit, so the outer `ORDER BY`
  and `FETCH` cannot reach the index. The ledger's earlier wording named the node; the mechanism
  is the missing pull-up.
- **The wrap over a flat base costs nothing (stage 8).** Sections e1 and e2: a base over one
  table with no window function is pulled up, with the anchor outside as a directive or inside
  as a bound parameter, and the plan equals the flat statement's (12 buffers, 0.02 ms). A window
  count inside the base blocks the pull-up, and the plan equals the flat exact statement's,
  which reads the whole directory anyway. So composing at the base's level is required for a
  base that contains `WITH RECURSIVE`, and the wrap suffices for a flat base.
- **The window total costs the directory's heap read (stage 8).** `COUNT(*) OVER ()` in the page
  statement makes the engine read every row of the directory with its columns: 6.3 ms and 2434
  buffers for 10,008 files, against 0.02 ms and 12 buffers without the total and 0.79 ms and
  109 buffers for an index-only count twin after `VACUUM`. The total in the page statement
  agrees with its page, and it costs a heap read that a separate count avoids once the table is
  vacuumed. A library that offers both should say so.
- **A native statement declares its tier and its port in the header (stage 9).** `--| tier:
  native` and `--| native: <feature and port>` as free text on one line; the loader refuses a
  native file without the declaration and a standard file with one, and `sqlint` reports both.
  The declaration is one line, so a port note of any length is one long line; a multi-line value,
  or a separate `port:` key, would let the note read as prose.
- **`Verify` covers native statements (stage 9).** `Statements.Verify` prepares every statement's
  text whatever its tier, so `pg_advisory_xact_lock` and `UPDATE ... RETURNING` are prepared at
  startup with the rest. Nothing in `Verify` asks the dialect whether a native statement belongs
  to it, so a Postgres variant compiled against another engine's dialect fails at prepare, not
  at compile.
- **The transaction requirement lives in the statement, not in the operation (stage 9).** The
  baseline's begin is two statements that need one transaction, and the requirement is declared on
  the first statement's header, which the second inherits only because the first refused the pool
  before it ran. An operation-level requirement (a `Transact`-style contract on the method, or a
  `*sqlate.Tx` parameter) would state it once. The experiment kept `sqlate.Session` on the
  interface, as the concept prescribes for the write path, and enforces it in Go where no
  statement runs (the baseline's no-op lock), so the refusal is uniform across variants.
- **The `native_forms` check keys on the tier, not the directory (stage 9).** A directory listed
  under `[statements]` may mix tiers; the check exempts a file by its declared tier. The
  experiment keeps native files in a directory of their own for the layout's sake, not the
  lint's. A per-directory `tier` requirement (every file in this directory must declare native,
  or must declare standard) would let the configuration state the layout rule the experiment
  follows by convention.
- **`query.Guard` cannot carry a second predicate (stage 10).** `complete_file_write` includes
  `guard_where` and `guard_set` and adds `AND status = 'pending'`. When the status predicate
  refuses the row, the guard affects nothing, runs its check, finds the expected version, and
  reports `ErrVersionMismatch: expected 1, current 1`, which is false. `CompleteFileWrite`
  therefore reads the row after a mismatch and reclassifies: the expected version with another
  status is a `blobfs.TransitionError` (matching `ErrDeleting` for a deleting row), and only a
  moved version is the guard's conflict. A guard whose check returns the row, or a check the
  caller can extend with the same extra predicate, would remove the third statement.
- **The guard's check ignores extra arguments, and that is what makes one map work (stage
  10).** `Guard.Run` passes the command's `Args` to the check, and `Args` ignores an extra
  name, so the size, content type, and etag bound for the update do not fail the version read.
  The documentation says so for a "narrower check"; the write path relies on it.
- **A parameter inside a published pattern takes no cast (stage 10).** `sql.guard_where` binds
  `{{id}}` without a type, so `complete_file_write` sends the id as an untyped parameter and
  Postgres infers `uuid` from the column; the experiment's own statements cast every uuid
  (`{{id:uuid}}`) for `Verify`'s sake. A pattern with typed slots, or a way for the including
  statement to state the type, would make the two forms one.
- **`Statement.Native()` is the only reader of the port note (stage 9).** No tool lists the
  native statements of a program with their ports; `pgnative`'s test asserts each note contains
  `Port:` by convention. An `sqlint` report, or a `query.Statements` method that lists native
  statements and their declarations, would turn the port notes into the work list the ledger
  says they are.
- **The parameterized projection base, measured (stage 11).** The bookmark read model as the
  plan described it, a projection whose base is a top-level `WITH RECURSIVE` anchored on the
  bookmarked files' directories, cannot bind the unit, so the recursion walks upward from every
  bookmark of every unit and the unit filter, applied outside the derived table, discards the
  rest afterward: 165 ms and 3,023 buffers for a unit with 10 bookmarks among 53,110 (156 ms
  with JIT off), and the same order for 1,000. The anchored plain statement with the unit bound
  in the recursion's anchor, the shape a parameterized base would give, costs 4.1 ms and 886
  buffers for 10 and 14 ms and 6,472 for 1,000. So a parameterized base turns a cost
  proportional to every unit's bookmarks into one proportional to the unit's. The experiment
  did not need it for this read model, because the recursion can be moved into a scalar
  subquery correlated on each row's directory; the base is then a plain join the planner pulls
  up, the unit directive reaches the bookmark index, and the page costs 0.27 ms and 245
  buffers for 10 bookmarks and 17 ms and 21,940 for 1,000 (JIT off). The parameterized base
  stays motivated by the two cases the correlated form cannot take: a base whose recursion must
  be a top-level common table expression (a downward walk, or a walk shared by several output
  columns), and the directory-anchored listing of stage 6.
- **A correlated recursion inside a projection base is pulled up; a top-level one is not
  (stage 11).** The query library's collection and count patterns wrap the base as a derived
  table. A base with a `WITH RECURSIVE` at its top level is planned as its own unit, so the
  outer filter runs after it (the finding of stage 8 for the sort, seen again for the filter).
  A base whose recursion sits in a scalar subquery of the select list is a simple subquery, and
  the planner pulls it up: the outer `WHERE` becomes an index condition on `bookmark`, the
  count twin drops the subquery altogether (0.06 ms and 35 buffers for 10 bookmarks, 0.93 ms
  and 3,012 for 1,000, against 4.6 ms and 877 for the anchored form's count, which must run its
  recursion), and a sort by the key evaluates the subquery for the page's rows only (0.23 ms
  and 493 buffers for 1,000 bookmarks). The authored base decides whether the projection's
  directives can reach an index; the library's documentation should say so.
- **The correlated recursion trips the planner's JIT threshold (stage 11).** The planner costs
  the recursive subquery at about 720 units per row, so a page over a unit with more than about
  140 bookmarks crosses `jit_above_cost` (100,000) on a server with the default settings and is
  JIT-compiled: 139 ms for 1,000 bookmarks with JIT on against 17 ms with it off, and the
  compile is more than the walk. A sort by the key stays under the threshold (the `Limit` bounds
  the estimate), and the anchored form's estimate is 9,000. A consumer that ships this shape
  either sets `jit_above_cost` for the session (a native form the standard tier cannot state) or
  accepts it; the transcript's section f records both settings.
- **A projection's key is one field (stage 11).** `--| key:` names one field, and the
  projection appends it as the tie-breaker. The bookmark read model's unique key over the
  unfiltered base is the pair `(unit_id, file_id)`; the experiment declares `file_id` and
  relies on every read filtering by the unit. A composite key, or a key declared per filter,
  would let the header state the truth.
- **The count twin gives an empty later page its total (stage 11).** Unlike the window count,
  which travels on rows, the projection's separate count reports the exact total for a page
  past the end. The two read models the consumer ships therefore differ on that edge, and
  `output` renders both (`total 5` against `total unknown (the page is empty)`).
- **A consumer's constraints classify in the consumer (stage 11).** `blobfs`'s write mapping
  returns a violation of a consumer constraint as it came, with the `sqlate.ConstraintError`
  reachable, and the consumer keeps its own table from its constraint names to its sentinels
  (`bookmarkSentinels`), keyed on the names its migration declares and its tests check. The
  two mappings never overlap, because the names carry the owner's prefix or the lack of it.
  `sqlate` itself gives everything this needs; the finding is that the classification is
  per-owner and per-operation, and a library cannot classify a constraint it does not own.

### The library

- **Constraint-to-sentinel mapping is per operation.** `blobfs_fk_directory_parent` means a
  missing parent on a write and a non-empty directory on a delete, so each kind of operation
  carries its own table.
- **Constraint names live in the root package.** The persistence layer cannot import the
  migrations layer, so the constants are in `lib/blobfs`.
- **`Directory.Name` is a pointer.** A root has no name, so every non-root call site nil-checks or
  dereferences.
- **A second engine no longer adds only a directory.** Once native variants exist, a second engine
  adds a migrations directory and a variant for each native operation, and the native files' port
  notes are that work list.
- **The root layer is thin.** It holds validation helpers and vocabulary, and it stands alone as a
  compilable package but not as a capability.
- **`MaxKeyLength` on the key-validation interface is nearly redundant.** `azureblob`'s own key
  check also enforces the length. Proof 7 measures it.
- **Fail-write has no representation.** The status table cannot express a failed write without a
  fourth status or a delete of the pending row. Stage 10 decides.
- **`tests-and-docs.md` says production source has no doc comments,** and no repository in the
  workspace follows that sentence. The experiment follows the practice: godoc on every exported
  identifier, and the package comment in `doc.go` for a multi-file package.
- **History tables survive a full `Down`.** Stage 14 decides whether `Reset` drops them.
- **One seeded root, one partial unique index (stage 6).** `blobfs_directory` holds exactly one
  row with no parent, seeded with `RootID` by the migration and guarded by
  `blobfs_uq_directory_root` over the expression `(parent_id IS NULL)`. A second root is a unique
  violation under the index's name, and `TestOneRoot` shows the primary key and the index are
  distinct guards. No library operation can create a root, because `Mkdir` always binds a parent
  and a validated name.
- **The listing composes at the statement's level and carries its total (stage 6).** `ListFiles`
  and `Children` are one statement each, anchored on a directory id, with the caller's predicates,
  sort, and page appended in Go from the query library's clause patterns and the total from
  `COUNT(*) OVER ()` in the same select list. `TestListingCarriesItsTotal` proves one query per
  page and the window count present under `TotalExact` and absent under `TotalNone`;
  `TestListingMatchesForest` proves the rows and the total equal a whole-forest recursion's
  answer for every directory of a fixture, four sorts, two filters, and four page sizes;
  `TestExactTotalUnderConcurrentInserts` proves the total agrees with its own rows while a second
  connection inserts between calls, on the pool and under a repeatable-read transaction.
- **`name` is the key of both listings (stage 6).** `(directory_id, name)` and `(parent_id,
  name)` are unique, so a sort by name is total in either direction and needs no tie-breaker;
  every other sort gains `name` as the tie-breaker. No index was added.
- **`DirectoryPath` is one upward walk (stage 6).** `directory_ancestors` recurses from the
  directory to the root, so its cost is the depth. `ResolveDirectory` stays one round trip per
  segment from the root.
- **The store's `Verify` has two halves (stage 6).** Every statement prepares as authored, and
  each listing statement prepares once more as a canonical rendering with every declared field
  filtered and sorted and the paging clause, so a field the table lacks fails at startup.
  Twelve prepares in all: eight statements and four renderings.
- **`ResolveDirectory` resolves from the root only (stage 7).** A consumer that needs the
  depth-one ancestor of a path, as the scope check does, resolves `/first` and then the full
  path, so the root read and the first child read run twice per scoped `ls`. A `ResolveUnder
  (parentID, path)`, or a resolution that returns the chain it walked, would remove the repeat.
- **A listing's declared fields are reachable only through the statement inventory (stage 7).**
  `Store.Statements()` exposes them by statement name, so a consumer that routes sort terms
  between the two halves restates the shared field set and pins it by a test. A `Fields()`
  accessor per listing would let the consumer ask.
- **The variant seam costs one interface, one option, and three forwarding methods (stage 9).**
  `data.Variant` has three methods, `data.New` takes `WithVariant`, and the store forwards
  `LockTree`, `Serializes`, and `BeginFileDelete`. A consumer-supplied variant is a struct that
  embeds a base variant and overrides one method; `TestConsumerVariantSwapsOneMethod` proves the
  store runs the override and the base for the other method, in both directions, with no change
  to the library. `datatest.Run` passes on both shipped variants (`TestStandardConformance`,
  `TestConformance`).
- **What the tree lock costs (stage 9).** On Postgres, one `SELECT pg_advisory_xact_lock($1)`
  per moving transaction, held to its end, and `TestConformance/LockTree` shows a second
  transaction blocks for as long as the first holds it (500 ms in the test) and proceeds at its
  commit or rollback; `TestTreeLockIsAnAdvisoryLock` shows the lock in `pg_locks` inside the
  transaction and gone after it. On the baseline the lock costs nothing and serializes nothing:
  standard SQL has no lock held to commit, so the honest baseline is a no-op that reports
  `Serializes() == false`, and stage 13 proves the cycle it allows. A second engine with a
  transaction-scoped lock (SQL Server's `sp_getapplock`) ports the variant; one without (MySQL's
  `GET_LOCK` is session-scoped, SQLite has none) keeps the no-op and serializes moves outside the
  database. The lock's port note says so.
- **What the file-delete begin costs (stage 9).** One round trip on Postgres (`UPDATE ...
  RETURNING` over the published column list, the `CASE` expressions keeping a deleting row
  unchanged) against two on the baseline (the update, then `file_by_id`) plus the transaction
  the baseline requires. Both return the same row for the same fixture (`TestVariantsAgree`) and
  the retry converges to the same row in both. The one-statement form accepts the pool; the
  two-statement form refuses it with `query.ErrTransactionRequired` before any SQL, so the
  transaction requirement is a property of the variant and the interface's contract is the
  stricter one.
- **How the guard composes with the begin (stage 9).** The begin does not take an expected
  version, so it does not use `query.Guard`: a `rm` names a path, not a version, and a guarded
  begin would also have to return the row, which `Guard.Run` does not (it returns the new
  version). A consumer that wants an optimistic begin composes `guard_where` into a statement of
  its own and reads the row back, and the ledger's earlier finding holds: the guard's check
  reports only a version mismatch, so a refusal for status needs its own read.
- **The write path is four standard statements and no variation point (stage 10).**
  `begin_file_write` (the insert), `file_by_id` (its read-back), `complete_file_write` (the
  guarded update), and `file_version` (the guard's check), plus `file_by_name` for the retry
  and for `stat`. None needs a native form: the insert takes the pool, the update is one
  statement, and `RETURNING` would save one read-back per step, which stage 9's delete begin
  already measured at one round trip. `TestNew` counts fourteen statements, all standard tier,
  and `Verify` twenty prepares.
- **Both steps take the pool and compose into a transaction (stage 10, proof 3).** The begin
  step ran inside a consumer's `Transact` beside a consumer row that references `blobfs_file`
  through a foreign key, and the foreign key held against the pending row inside the
  transaction; a rollback removed both rows, a commit kept both. The complete step is one
  guarded statement on the pool. Failure is not a state: the pending row is the state, and the
  delete path is the fail step.
- **The key-validation wiring is one method (stage 10, proof 7).** `Storage.ValidateKey` is
  `store.Capabilities().ValidateKey(key)`, and `blobfs.KeyValidator` has no `MaxKeyLength`: the
  provider's rule enforces its own length in runes, and `MaxNameLength` keeps every key at 292
  runes or fewer, so the interface's length was unreachable against any real provider. The
  concept's "key validation and a maximum key length" becomes "key validation".
- **The consumer-anchored read model costs the unit's bookmarks times the depth (stage 11).**
  `bookmarks.sql` is one projection base: `bookmark` joined to `blobfs_file`, with the path
  computed per row by a recursion from the file's directory to the root, names joined with
  slashes as the walk climbs and the root contributing nothing. The measurement: the page by
  path costs 0.27 ms and 245 buffers for 10 bookmarks, 1.0 ms and 2,371 for 100, and 17 ms and
  21,940 for 1,000 (JIT off); buffers grow with the depth (14,349 at mean depth 3.0 against
  23,349 at depth 6.0 for 1,000 bookmarks); and nothing grows with the size of the tree
  (10,003 directories, 100,000 files) or of the bookmark table (53,110 rows). The path is a
  read-time computation, as settled, and the consumer stores none.
- **The bookmark's foreign key holds the file (stage 11).** A raw delete of a bookmarked file
  fails under `fk_bookmark_file`, so stage 12's `rm` meets a classifiable error from the
  consumer's constraint, which `blobfs` leaves unclassified and the consumer maps. The
  bookmark row is the consumer's, so the consumer decides whether `rm` removes the bookmark
  first or refuses.
- **The consumer's `List` is one transaction and two library listings (stage 7).**
  `TestListRunsInOneReadOnlyRepeatableReadTransaction` proves one begin with both options, the
  root read, the two halves, and the commit, and nothing else; `TestListHalvesAgreeUnderConcurrentWrites`
  proves the halves agree on the engine while another connection commits directory-and-file
  pairs between them. `TestListScopedChecksTheAncestorOnce` proves the owner row is read once,
  before the path resolves further, and that neither listing statement carries an owner
  predicate.

### `go-storage` and `azureblob`

- `Put` returns the caller's content type, while `Stat` and `Get` return the server's.
- `Delete` on a missing container returns `ErrNotFound`, which contradicts its own doc comment.
- `MaxKeyLength` counts runes, and Azurite accepts keys that `azureblob`'s rules reject, so key
  validation is proved by unit tests only.
- `Store.Start` wraps every failure as `ErrUnavailable`, including rejected credentials.
- No Azurite compose file existed in the workspace, and the image defines no health check. The
  experiment's file uses `nc -z` on the blob port.
- Azurite accepted a key of 1,024 runes (2,048 bytes) and one of 1,025 runes put straight
  through `storage.Store.Put`, which validates no key, and served both back (stage 10,
  `TestAzuriteAndTheKeyLimit`). Only `azureblob`'s `ValidateKey` refuses the longer one, so the
  provider's rule is the whole defense and the emulator proves nothing about the limit.
- `Put`'s returned `Object` agrees with `Stat`'s on Azurite for the size, the etag, and the
  content type when the caller declares one (stage 10, `TestPut`); the ledger's difference
  shows only when the caller declares none and the service defaults the type.
- go-storage has no container delete, so a test that creates a container of its own reaches
  for the Azure SDK's container client to remove it (`internal/livetest/storage.go`). A
  `DeleteContainer` on the provider, or on `storagetest`, would keep the SDK out of a
  consumer's test support.
- The storage configuration reads its own environment (`Config.Finalize(prefix)`), so a consumer
  that wants a flag for the container, as it has `--dsn` for the database, has to set the
  variable itself or bypass `Finalize`; the experiment kept the variables.

### `v1.storage` incorporation (to develop through the remaining stages)

- One install per configuration and per database. The fixed table names make a second isolated
  tree a second database.
- Directory-grain ownership: a consumer table keyed on a depth-one directory, checked once at the
  ancestor, rehearses the document hierarchy per organization.
- Directory-grain ownership as built (stage 7): the scope check is one read of the owner row for
  the top-level ancestor of the path, before the rest of the path resolves, and the listing
  statements carry no owner predicate, so the library's listings run unchanged under a scope.
  At the root the scope is the owner projection filtered by the unit; a file stored in the root
  belongs to no unit, so a service that keeps files at the root has no scope for them. Creating
  the organization's tree is one `mkdir --unit` at depth one: the directory and the owner row in
  one transaction.
- File-grain ownership: a consumer join table with a partial unique index rehearses the logo.
- The migration source is added to the service's one migrator, ahead of the service's own set.
  Reverting runs the service's set first.
- `org_image` is hypothetical. No partial unique index existed in the workspace before this
  experiment.
- The two-phase write as built (stage 10): the service inserts the pending row in the
  transaction that writes its own rows (an owner, a document record), commits, uploads under the
  row's key, and completes on the pool. The consumer's transaction boundary sits around the
  first step only; no transaction spans the upload. A service that already runs `storage.Store`
  under its lifecycle hands the started store to the adapter through `files.NewStorage`, and the
  adapter is the `blobfs.KeyValidator` the begin step takes, so the wiring is the one line that
  passes the adapter.
- A `pending` row is the service's own state to sweep: no `failed` status exists, a retry of the
  same upload resumes the row, and an abandoned row goes through the delete steps. A service
  that wants a time bound lists `status = 'pending'` under `updated_at < threshold` through the
  library's listing filters and deletes each.
- The composition root names the domain's `files.Storage` type, because only the domain's
  `storage.go` may import go-storage. A service whose composition root owns the storage store
  would build the adapter in the domain the same way and keep the store itself in its
  infrastructure; the type crossing is the cost of keeping the provider import in one file.
- File-grain ownership as built (stage 11): the `bookmark` row binds a file to a unit, at most
  one active per unit under the partial unique index `uq_bookmark_active`, and the three
  constraints (`pk_bookmark`, `fk_bookmark_file`, `uq_bookmark_active`) reach the service as
  its own sentinels through one table in its database file. The `org_image` case maps onto it
  directly: the organization is the unit, the image row is the bookmark, "one logo per
  organization" is the partial index, and the refusal of a second active row is a classified
  error the handler turns into a conflict response. The service writes the row beside the
  pending file row in the upload's first transaction (a pending file can be bookmarked), and
  the delete of a file the row references fails under the foreign key until the row goes.
- The service's listing of a unit's files with their paths is a projection over its own table
  joined to `blobfs_file`, with the path computed per row by a recursion correlated on the
  file's directory, and it pages, sorts, and counts through `query.Projection` unchanged. The
  count and the page run in one read-only repeatable-read transaction so they agree. The cost
  is the unit's row count times the depth; a sort by the row's key costs the page's rows
  instead. A unit with more than a few hundred rows pays a JIT compile on a default Postgres
  unless the session lowers `jit_above_cost`.

## Evidence: the cost of the shipped listing (proof V3, measured 2026-09-20, PostgreSQL 18.4)

The measurement is `evidence/read-model.txt`, written by `mise run evidence` from
`lib/blobfs/data/evidence_integration_test.go` (`TestListingCost`, gated by `BLOBFS_EVIDENCE=1`
and the `integration` tag) against the shipped schema and the shipped listing statements as the
store composes them. The fixture is 100,000 file rows and 10,003 directories in three trees under
the root, a tenth of the files in one depth-two directory (10,008 files, the biggest), and a
10-file directory at depth six (the small one). Both tables were `VACUUM ANALYZE`d after seeding,
except in section 0. The numbers are medians of five `EXPLAIN (ANALYZE, BUFFERS)` runs in
milliseconds; plan shapes and buffer counts are the durable facts, and the milliseconds depend
on the machine (a laptop, everything in shared buffers). `EXPLAIN` ran over pgx's simple protocol,
so every run was planned with its literal values, as a custom plan is.

| Section | Query | Small (10 files) | Biggest (10,008 files) |
|---------|-------|------------------|------------------------|
| 0 | Count twin before `VACUUM` | | 1.96 ms, 2434 buffers, bitmap heap scan |
| 0 | Count twin after `VACUUM` | | 0.79 ms, 109 buffers, index-only scan |
| 0 | Shipped exact-total page, before and after `VACUUM` | | 5.8 and 6.1 ms, 2434 buffers both |
| a | Whole-forest baseline, count and page | 11.3 and 11.1 ms, 29,700 buffers | 13.6 and 17.9 ms, 29,800 and 32,100 buffers |
| b | Shipped listing, exact total, page 1 | 0.05 ms, 13 buffers | 6.3 ms, 2434 buffers |
| c | Shipped listing, no total, page 1 | 0.04 ms, 13 buffers | 0.02 ms, 12 buffers |
| d | Last page by offset, no total | | 7.7 ms, 2434 buffers |
| d | Last page by offset, exact total | | 11.1 ms, 2434 buffers |
| d | Last page by cursor | | 0.02 ms, 6 buffers |
| e1 | Wrap, unanchored base, count twin and page | | 0.75 ms, 109 buffers; 0.02 ms, 12 buffers |
| e2 | Wrap, anchored base; the same with the window count inside | | 0.02 ms, 12 buffers; 5.9 ms, 2434 buffers |
| e3 | Recursive base, flat and wrapped | | 0.07 ms, 21 buffers; 5.1 ms, 2443 buffers |

What the measurement shows, and the answers to the V3 questions as far as it supports them:

- **The whole-forest baseline costs far more than the shipped listing.** It reads about 30,000 buffers for any
  directory, because the recursion walks every directory and the join touches every file; the
  shipped listing reads 12 or 13 buffers for a page without a total. The v1 numbers (815 buffers
  for the same shape) were measured on the same fixture size but before `VACUUM` and with a
  different plan; the new fixture's plan joins through `blobfs_uq_directory_parent_name` and
  the file index, and the two are not comparable beyond their order of magnitude.
- **Does the exact total stay the default?** Yes for the default page size and ordinary
  directories: for the small directory the total is free (13 buffers either way). For a big
  directory the window count costs the whole directory's heap read (2434 buffers, 6.3 ms for
  10,008 files) on every page, because the page statement must read every row's columns to
  count them, and `VACUUM` does not help it. The default holds because a consumer that lists a
  directory expects its size, and the cost is bounded by the directory, never by the tree.
- **Is a capped total needed?** The measurement does not decide it. A capped total (count at
  most N and report "more than N") would bound the heap read a big directory pays; `TotalNone`
  already bounds it to zero, and the cursor pages the directory without it. The experiment
  shipped no cap, and the numbers say a consumer with directories of tens of thousands of files
  should ask for the total once, on page one, and walk by cursor.
- **Does the cursor earn its place?** Yes. The last page of the biggest directory costs 0.02 ms
  and 6 buffers by cursor (an index scan from the cursor's name under a `Limit`) against 7.7 ms
  and 2434 buffers by offset (a bitmap scan of the directory and a top-N sort). Offset paging
  costs the pages skipped; the cursor costs the page.
- **Does any sort earn an index?** Not on this evidence. Every measured query sorts by `name`,
  which `blobfs_uq_file_directory_name` orders in either direction. A sort by another field is
  an in-memory sort of the directory (the same shape as the exact-total page, a bitmap scan and
  a top-N sort), so its cost is the directory's size, as stage 6 said. The measurement did not
  time one; the stage 14 rehearsal migration adds `(directory_id, created_at)` and can measure
  the difference then.
- **Is composing at the base's level required, or does the wrap suffice?** Both, by base. A flat
  base (one table, no window function) is pulled up by the planner, and the wrap's plan equals
  the flat statement's, with the anchor outside as a directive or inside as a bound parameter.
  A base that contains `WITH RECURSIVE` is not pulled up: the wrapped e3 form scans and sorts the
  whole directory above the join (5.1 ms, 2443 buffers) where the flat form pages through the
  index (0.07 ms, 21 buffers). So the composer's choice to append the clauses at the statement's
  own level is required for the recursive shape the consumer-anchored read models take (the
  bookmark projection of stage 11, and any per-row path), and the wrap suffices for the plain
  listings. A window count inside a wrapped base also blocks the pull-up, but the plan is then
  the flat exact statement's anyway.
- **The `VACUUM` caveat, settled.** The v1 caveat that the count "reads the heap through a
  bitmap scan because the fixture was never vacuumed" applies to a separate count statement: the
  count twin goes from 2434 buffers to an index-only 109 after `VACUUM`. The shipped window total
  is unaffected, because it travels in the page statement, which reads the rows.

## Evidence: the cost of the bookmark read model (stage 11, measured 2026-09-20, PostgreSQL 18.4)

The measurement is `evidence/bookmarks.txt`, written by `mise run evidence` from
`domain/files/evidence_integration_test.go` (`TestBookmarkCost`, gated by `BLOBFS_EVIDENCE=1`
and the `integration` tag) against the shipped schema and the shipped projection as the store
composes it. The fixture is 100,000 file rows and 10,003 directories in three trees under the
root to depth six, 50 background units with 1,000 bookmarks each, and the measured units: 10,
100, and 1,000 bookmarks of random files (mean directory depth about 5.5), and 1,000 bookmarks
of files at depth three ("shallow", the least depth holding a thousand files) and at depth six
("deep"). The three tables were `VACUUM ANALYZE`d after seeding. The numbers are medians of five
`EXPLAIN (ANALYZE, BUFFERS)` runs in milliseconds, over pgx's simple protocol; plan shapes and
buffer counts are the durable facts. JIT is the server's default (on) except in section f.

| Section | Query | 10 bookmarks | 100 | 1,000 |
|---------|-------|--------------|-----|-------|
| a | Shipped projection, count twin | 0.06 ms, 35 buffers | 0.26 ms, 306 | 0.93 ms, 3,012 |
| a | Shipped projection, page 1 by path | 0.27 ms, 245 | 1.0 ms, 2,371 | 139 ms, 21,940 (JIT) |
| b | Shipped projection, page 1 by `file_id` | 0.15 ms, 245 | | 0.23 ms, 493 |
| c | Projection over a top-level recursion, count and page | 165 and 167 ms, 3,023 and 3,032 | | 167 and 170 ms, 6,000 and 7,009 |
| d | Anchored plain statement, count and page | 4.6 and 4.1 ms, 877 and 886 | | 13.8 and 14.3 ms, 5,463 and 6,472 |
| d | Anchored plain statement, page wrapped as a derived table | | | 14.5 ms, 6,472 |
| e | Shipped page by path, shallow (depth 3.0) and deep (depth 6.0) | | | 134 and 138 ms, 14,349 and 23,349 (JIT) |
| e | Anchored page, shallow and deep | | | 13.3 and 14.1 ms, 6,475 and 6,476 |
| f | Shipped page by path, JIT off | | 1.6 ms, 2,371 | 16.9 ms, 21,940 |
| f | Shipped page by path, JIT off, shallow and deep | | | 13.9 and 17.4 ms, 14,349 and 23,349 |
| f | Top-level recursion, JIT off, count and page | 156 and 158 ms | | 160 and 160 ms |

What the measurement shows:

- **The shipped shape costs the unit's bookmarks times the depth.** From 10 to 100 to 1,000
  bookmarks the page by path reads 245, 2,371, and 21,940 buffers, about 22 per bookmark at a
  mean depth of 5.5, and from depth 3.0 to 6.0 it reads 14,349 against 23,349 for the same
  1,000 bookmarks. Nothing in it scales with the 53,110 bookmarks of the other units or with the
  tree's 10,003 directories.
- **The top-level recursion costs every unit's bookmarks.** Section c is the shape the plan
  described and the only one a projection base can take when the recursion is a top-level
  common table expression: 165 ms for a unit with 10 bookmarks and 167 ms for one with 1,000,
  because the walk starts from all 53,110 bookmarks before the unit filter runs. That is the
  cost a parameterized projection base removes, and the finding the ledger records with these
  numbers.
- **The anchored plain statement is what a parameterized base would give**, and it is not
  better than the correlated form: 4.1 ms and 886 buffers for 10 bookmarks (the planner hashes
  the whole directory table on every recursion level, so the small unit pays for the tree) and
  14 ms for 1,000, against 0.27 ms and 17 ms (JIT off) for the shipped shape. Its count twin
  must run the recursion (4.6 ms for 10 bookmarks) where the shipped count drops the path
  subquery (0.06 ms). The wrap costs nothing here (14.5 against 14.3 ms) because the sort is by
  a computed column and no index order is lost.
- **JIT dominates the shipped page at a thousand bookmarks.** 139 ms with the default settings
  against 17 ms with `jit = off`; the planner's estimate of the correlated recursion crosses
  `jit_above_cost` at about 140 rows. The sort by the key stays under the threshold and reads
  only the page's rows (0.23 ms and 493 buffers for 1,000 bookmarks), so a consumer with large
  units sorts by the key or lowers the threshold for the session.
- **The path stays a read-time computation.** The read model stores nothing, and its cost is
  bounded by what the unit holds.

### Earlier evidence: read-model cost by form (proof V1, 2026-09-20, volume-era schema)

The record is `evidence/v1-read-model.txt`, produced by a test deleted with the volume package
and kept untouched. Medians of five runs, in milliseconds, 100,000 file rows and about 10,000
directories in three trees, analyzed and never vacuumed.

| Form | What it is | Listed directory (10 files) | Biggest directory (10,008 files) |
|------|------------|-----------------------------|----------------------------------|
| 1 | Projection over a whole-forest recursion, filter applied after | 11 ms per query, 815 buffers | 14 ms count, 17 ms page |
| 1b | Form 1 without the owner join | 11 ms, no measurable difference | 15 ms count, 18 ms page |
| 2 | Plain statement anchored on the directory, path from an upward walk | 0.1 ms, 35 buffers | 2.5 ms count, 0.1 ms page |
| 3 | `volume_id` stored on file rows, flat listing, no path | 0.08 ms, 5 buffers | 1.1 ms count, 0.05 ms page |

Form 1 walked every directory for the count and again for the page, so its cost was linear in
the number of directories. The owner join cost nothing measurable. Form 2 cost the directory's
depth plus the page. Form 3 was the cheapest and returned no path, and the architect deferred it.
The 2.5 ms count of the biggest directory read the heap through a bitmap scan because the fixture
was never vacuumed; section 0 of the V3 measurement settles that caveat.

## Not proven yet

Authorization, which needs `go-auth` and is proven under `v1.auth`. The migration path through
`go-web-service`'s admin surface, which `v1.storage.service` proves. The remaining stages answer
proofs 2 to 7 and the variant seam; the bookmark read model's cost is in the evidence.
