# blobfs experiment record

The record of `blobfs.experiment`, replacing what were three files — `REVIEW.md`, `DECISIONS.md`,
and `NOTES.md` — with one. It describes the experiment as it closed, after stage 32 and the
follow-up commits (`3ba8178` through `8c12919`), not as it stood partway through. Where a Phase 3
stage (17 to 32) changed a finding from the post-execution review, this record states the finding
in its final form and says which stage changed it and why. The sources of truth are the code on
the `blobfs-experiment` branch, the transcripts under `evidence/`, and `GUIDE.md` for the tour of
each capability; every claim below names the test, statement, or transcript section that supports
it.

`DECISIONS.md` and `NOTES.md` were folded into this file and are removed; git history keeps them.
What left the experiment for the workspace's durable context is in `context/concepts/blobfs.md`,
`blobfs-api.md`, `blobfs-composition.md`, `migration-sets.md`, and `sqlate-library-support.md`, and
in the amendments to `context/design/auth-strategy.md` and `context/roadmap.toml`; see "What left
the experiment."

## Summary

The experiment built `blobfs`, a library that keeps a virtual directory tree and file metadata in
SQL for an object store, and a command-line file system that uses it. The library never calls the
object store. The experiment ran against Postgres 18 and Azurite, over published `sqlate` v0.1.1
and `go-storage` v0.1.0.

The library has these packages, as of stage 32:

- `lib/blobfs` is the root package of entities, vocabulary, and errors.
- `lib/blobfs/data` is the persistence package, with `lib/blobfs/data/datatest` beside it as the
  conformance suite.
- `lib/blobfs/postgres` is the Postgres engine: the variant and the DDL as a migration set.
- `lib/migrator` is the multi-set migrator, a documented shim over `sqlate` v0.1.1.

The consumer is an application under `cmd/`, `internal/`, `domain/`, `admin/`, `migrations/`, and
`output/`, and it is not promoted; it exists to prove the library and to rehearse what a real
consumer's own layer above it might look like.

The conclusions that matter most:

- The standard-tier baseline is complete on any engine, and the Postgres engine overrides six
  operations, each shipped only after a measurement showed a win: the tree lock, the file-delete
  begin, the write steps' `RETURNING` forms, one-statement path resolution, and the row-value
  keyset predicate.
- The listing is one statement anchored on a directory, with its total in the same select list and
  `Page.More` reporting whether rows remain independent of the total. `sqlate`'s projection cannot
  express it.
- A file's path stays a read-time computation, and the library stores no path.
- Ownership is a join in the consumer's tables at two grains, directory and file, and the library
  holds no unit; the two operations that reference a file (a bookmark, a delete) serialize on the
  file's row through the library's reference-then-delete rule.
- The multi-set migrator ships integrated as a shim, and the `deleting` status is required.
- Ids are the primary handle in the library and in the consumer above it; a caller that holds an
  id and a version from a listing acts without a read.

The decisions the architect made in the post-execution review are in "The review: the fifteen
questions and their answers" and "The review: decisions added"; the stages that implemented them
are in "Phase 3."

## How the experiment ran

The experiment ran in 32 stages on the branch `blobfs-experiment`, plus five close-out commits.
Stages 1 to 5 built a design with a volume table, one core table meant to segregate directories
inside one container. The architect reversed that decision: a volume is an opinionated way to
segregate directories that an application owner can build on top of the baseline, and `blobfs`'s
job is the container-based directory and file infrastructure underneath it. Making volume a core
table had real benefits — unique roots, a clean anchor for path resolution, a declarative rule
that a root belongs to exactly one volume — and real costs: an opinionated segregation baked into
the library, and a listing anchor that the consumer's ownership join could not compose with. The
schema returned to two tables with one seeded root row, and the consumer's own two tables
(`directory_owner`, `bookmark`) rehearse two ways an application owner can build isolation and
ownership on top of it.

Stages 6 to 15 rebuilt the schema around the one seeded root and added the listing, the variant
seam, the write, bookmark, delete, and move paths, the migrator, and the integration tier. Stage
16 produced the post-execution review (the original `REVIEW.md`) from the code and the evidence,
and the architect reviewed eight layers of the built experiment, watched a live demonstration, and
answered fifteen open questions and thirteen decisions the review added, recorded originally in
`DECISIONS.md`. Stages 17 to 32 implemented that review; see "Phase 3." Every stage was committed
after its own build, vet, lint, `split-check`, test, and integration gates passed, several with
their implementation delegated to another agent under full context and the diff read firsthand
before commit.

One naming note for reading old test names: stages 7 to 13 named the built-binary tests
`TestFileCommands`, `TestWriteCommands`, `TestBookmarkCommands`, `TestDeleteCommands`, and
`TestMoveCommands`; stage 15 folded them into the steps of `TestScript` in
`integration/integration_test.go` (`directories`, `writes`, `bookmarks`, `deletes`, `moves`), which
runs once per variant.

## The library, as built

### Layers and import boundaries

The library is one module in three layers, as the concept states.

| Package | What it holds | What it may import |
|---------|---------------|---------------------|
| `lib/blobfs` | The entity types `Directory`, `File`, and `Object`; `RootID`; `NewID` and `ParseID`; the `Status` vocabulary and the transition table; `NewKey` and `SanitizeFilename`; `NormalizeName` and `ValidateName`; the `KeyValidator` interface; the constraint-name constants; the sentinel errors and `ViolationError`. | The standard library and `golang.org/x/text/unicode/norm`. Neither `sqlate` nor `go-storage`. |
| `lib/blobfs/data` | Twenty-two standard-tier statements, the published pattern namespace `blobfs`, the listing composer, the `Store` type whose methods take a `sqlate.Session`, and the `Variant` interface (`LockTree`, `Serializes`, `BeginFileDelete`, `InsertFile`, `InsertDirectory`, `CompleteFileWrite`, `ResolvePath`, `Keyset`) with its baseline `Standard`. | `sqlate` and the root package. Never `go-storage`, never the engine package. |
| `lib/blobfs/data/datatest` | The conformance suite, `Run(t, db, store)`, a non-test package so another package's tests can import it. | The persistence package and `sqlate`. Never an engine package, never the migrator, and no non-test file names it. |
| `lib/blobfs/postgres` | The Postgres `Variant`, its six native statements each with a port note, and the DDL exported as a migration set (`Migrations() (migrator.Set, error)`). | The persistence package, `lib/migrator` (for the `Set` type), and `sqlate`. No driver: every statement runs as plain SQL through the session. |
| `lib/migrator` | The multi-set migrator: several `migrate.Migrator` values under one outer lock. | `sqlate` and the standard library. |

`split-check` in `mise.toml` enforces these boundaries with `go list -deps` over each package and a
grep for the driver's import path under `lib/`, and it enforces the consumer's elemental layout in
rules that grew from ten at stage 16 to accommodate the split. Every rule added after stage 4 was
proved by a temporary violation before it landed.

Two findings hold from stage 16 and one is corrected by Phase 3. The root package is thin: it
compiles alone, and a consumer with its own persistence can take it, but it is vocabulary and
validation, not a capability. The concept's claim that a second engine adds a directory and not a
module holds in a stronger form after stage 19: an engine is a package a consumer selects by
import, with no registry, no init, and no flag; a second engine adds a package of its own, owning
its DDL as a whole migration set and its native variant, and the native files' port notes are its
work list.

### The schema, and the designs it was chosen over

The schema is two tables. `blobfs_directory(id, parent_id, name, version, created_at,
updated_at)` holds exactly one root, seeded by `0001_directory.up.sql` with the nil UUID
`blobfs.RootID` and named `/` (stage 18; before it, the root had no name and `Directory.Name` was
a pointer). The check constraint `blobfs_cc_directory_root_name` states `(parent_id IS NULL) =
(name = '/')`, `name` is `NOT NULL`, and the partial unique index `blobfs_uq_directory_root` over
the expression `(parent_id IS NULL)` — over the expression rather than `NULLS NOT DISTINCT`, for
portability across engines — allows one row without a parent, so a second root fails as a unique
violation under that index's name. No library operation can create a root, because `Mkdir` always
binds a parent and a validated name. `blobfs_file(id, directory_id, name, status, key, size,
content_type, etag, version, created_at, updated_at)` is the file table; `size` and `etag` are
NULL until the object exists, and the status check constraint admits `pending`, `available`, and
`deleting`.

Constraint names are public API and follow `blobfs_<kind>_<table>_<detail>`, where `kind` is `pk`,
`fk`, `uq`, `cc`, or `ix`. The constants live in the root package because the persistence package
imports the root and must not import the engine package. The two foreign keys into
`blobfs_directory` have no cascading action, and they are the whole guard against removing a
non-empty directory. Every row carries a `version` column, which the query library's guarded
commands check. Directories and files have separate name spaces, so a directory may take the name
of a file beside it, and a path resolves by kind: a command that resolves one kind sees a file
before a directory of the same name (`cp` and `stat`, stage 26 and 27).

**Designs rejected.** The post-execution review measured four schema designs on Postgres 18 at
10,003 directories and 100,000 files. Design B, one `blobfs_entry` table with a `kind` column,
served an interleaved listing in 14 buffers but cost about 6% more buffers on the exact-total page
and about 21% more storage, and needed a generated `kind` column and NULL-safe check constraints
per kind. Design C, a `blobfs_node` table with subtype tables, needed 19 round trips for a file
lifecycle against 9, and page 1 by name cost 95 buffers against design A's 12; a node with no
subtype row was representable, hiding from the listing while holding its name. Design D, design A
plus a `UNION ALL` interleaved listing, cost about the same as A for a name-sorted page and had no
own DDL. Design A, the two tables the experiment built, won every measured read, because the
storage medium is a web-based virtual file system and consumers address directories and files as
separate resources; an interleaved listing was not needed. See
`evidence/schema-alternatives/README.md` for the method and every number.

**The `created_at` index.** The library ships no index on `blobfs_file (directory_id, created_at)`
(decision 5; removed at stage 18). It would turn a page sorted by `created_at` without a total into
an index read (25 buffers against about 2,400 for the biggest measured directory) and buys nothing
for an exact-total page, which reads every row for the window count regardless; it costs about
3.99 MB for 100,000 files against the name index's 8.78 MB. A library that ships an index imposes
its write cost on every consumer, so `blobfs`'s package comment documents the index and its cost
and a consumer's own migration set adds it if wanted.

**The schema is public API.** A violation of a documented constraint reaches the caller as a
`blobfs.ViolationError` naming the sentinel and the constraint, with the underlying
`sqlate.ConstraintError` reachable through `errors.As` (stage 20; before it, the message carried
the driver's raw text). On a delete, a consumer's foreign key is classified by class as
`blobfs.ErrReferenced` with the constraint name reachable, never by name; on a write, a consumer
constraint's violation returns as it came. A released migration's text never changes, and a change
to seeded data — such as the root row — is a new migration, never an amendment (adjustment 17).

### The standard tier and the Postgres engine package

Every statement in `lib/blobfs/data/statements` is standard tier: `TestNew` counts twenty-two
(twenty at stage 16; stage 23 added `hold_file` and `hold_file_at_version`), all standard, and
`sqlint` holds them to the standard forms. The baseline is complete on any engine `sqlate` has a
dialect for, and it is the reference semantics, the fallback, and the port template.

The Postgres engine (`lib/blobfs/postgres`, stage 19; before it, the variant was
`lib/blobfs/data/pgnative` and the DDL was a separate `lib/blobfs/migrations` package with a
dialect switch) overrides six operations on the `Variant` interface, each shipped only after
stage 28 measured a win against the standard tier and stage 29 implemented it:

- **`LockTree` and `Serializes`.** Unchanged since stage 16: `pg_advisory_xact_lock` under the
  fixed key `postgres.TreeLockKey`; the baseline's lock is a no-op reporting `Serializes() ==
  false`.
- **`BeginFileDelete`.** Unchanged in shape: one `UPDATE ... RETURNING` against the baseline's
  update-then-read; the round trip saved was already measured at stage 16.
- **`InsertFile`, `InsertDirectory`, `CompleteFileWrite`** (stage 29, decision 14's third bullet).
  `INSERT ... RETURNING` for a file's begin and for `Mkdir` save one round trip each (measured: 72
  to 68 buffers, 92 to 89); a successful `CompleteFileWrite` drops from two round trips to one, and
  a refused one from three to two, because the variant reads the row once after an empty
  `RETURNING` to classify a version mismatch from a status refusal, sharing the classifier
  `data.CompleteRefusal` with the baseline.
- **`ResolvePath`** (stage 29). One recursive statement resolves a path of any depth in one round
  trip, against the baseline's one read per segment; measured at depth 10, 11 round trips become 1,
  with 3 extra buffers from the statement's final selection, which selecting the row at the
  maximum depth removes rather than sorting and cutting. The segments bind as one `[]string`
  parameter the driver encodes as `text[]`, so no name is ever spliced into SQL text, verified
  against names carrying quotes, backslashes, braces, commas, a SQL injection string, and unicode.
- **`Keyset`** (stage 29). A sort with two or more terms renders a row-value comparison
  (`(q.a, q.b) > (x, y)`) instead of the baseline's expanded OR chain; measured at 35 buffers at
  any cursor position in a directory that has an index on the sort column, against up to 5,055 for
  the OR chain at a cursor in the middle. A single-term sort (the listings' default, by `name`)
  renders identically on both tiers.

Every native statement declares `--| tier: native` and `--| native: <feature and port>`, and
`sqlint` holds the engine package's directory to the native-forms check keyed on each file's
declared tier. The conformance suite (`datatest.Run`) runs both variants and compares their rows
and error text for every operation, including the new ones; `datatest` also builds a second store
over the baseline against the same database, so the suite is variant-agnostic by construction. A
consumer variant embeds a base variant and overrides one method, proved unchanged since stage 16.

What the baseline costs and cannot do: one extra round trip per file-delete begin and per write
step, a path walk of one round trip per segment, the expanded-chain keyset cost at scale, and no
serialization of directory moves. Adjustment 14's `sqlate` note ("engine-keyed statement overlays
for statements of the same shape, and a way to declare a statement that returns the changed row")
is expected to remove four of the eight `Variant` methods' reasons to exist once `sqlate` lands
pattern overlays for `RETURNING`; see `context/concepts/sqlate-library-support.md`.

### The listing

The listing is not a projection. `ListFiles` and `Children` are each one authored statement
anchored on a directory id, with the caller's filters, sort, and page appended in Go by the
composer in `lib/blobfs/data/listing.go` from the query library's own clause patterns. There is no
derived-table wrap: the clauses attach at the statement's own level, so the engine sees one flat
query over one table and pages a name sort through the unique index.

The total travels in the page. Under `TotalExact` (the zero value, rejected as the cursor's
default early on precisely because it is the zero value and a cursor page must default to no
total) the `_with_total` statement carries `COUNT(*) OVER () AS total`, evaluated over the rows the
WHERE clause keeps and before the paging clause cuts them, so the total cannot disagree with its
page under any isolation level.

The composer fetches one row beyond every page, offset or cursor, and drops it (stage 21; at
stage 16 the extra row was fetched only when the sort could be continued by a cursor). The row's
presence is `Page.More`, reporting whether rows remain whatever the sort and the total say; `Next`,
the cursor, is filled when `More` is true and the sort can be continued. A page therefore reads as
one of three states: no rows remain; more rows, continue by cursor; or more rows with no cursor,
so read the next page by number. The `NoTotal` edge holds: an empty first page has the exact total
0, and an empty page after the first reports `NoTotal` because the composer runs no count
statement. The sort is the caller's terms followed by `name`, the key, as the tie-breaker when the
caller did not name it, taking the terms' shared direction when they have one.

The cursor is base64url over an eight-byte SHA-256 prefix and a JSON body holding the statement's
name, the sort terms, and the last row's sort values. It is refused when malformed, edited, issued
by the other listing or under other terms, or when the sort cannot be continued (mixed directions,
or a nullable field such as `size` or `parent_id`).

`ls` gained an `ID` column, `--filter` (the same rule as `--sort`: a term applies to the file half
whenever the file listing declares its field, and to the directory half only for fields both
listings declare), and `--cursors` to opt into printing the cursor lines (stage 27); none of this
touches the library, which already carried every id and had no filter restriction of its own — the
narrowing by half is the tool's rule (`domain/files/database.go`), not the library's.

The store's `Verify` prepares twenty-eight statements on the standard tier (twenty-two statements,
four offset renderings, two cursor renderings; twenty-six at stage 16, before `hold_file` and
`hold_file_at_version`). A store built with the Postgres variant prepares thirty-four: the
twenty-two standard statements (compiled regardless, since the standard set is the store's own
inventory and the variant's is appended, not substituted) plus the variant's six native statements.
The consumer's own inventory prepares forty (thirty-six at stage 16; stage 23 added the two holds
and stage 25's second bookmark projection, `bookmarks_with_paths`, added one more).

### Error mapping

Constraint-to-sentinel mapping is per operation, because one constraint means different things on
different statements. `lib/blobfs/data/errors.go` keeps two tables: `writeSentinels` maps the two
name uniques to `ErrNameTaken`, the root index to `ErrRootDirectory`, the two foreign keys to
`ErrNotFound`, and the two primary keys (new at stage 22, for a caller-supplied id) to
`ErrIDTaken`; `deleteSentinels` maps the two foreign keys to `ErrNotEmpty`. Each mapping is now
wrapped in a `blobfs.ViolationError` (stage 20) that names the sentinel and the constraint and
keeps the `sqlate.ConstraintError` reachable through `errors.As`, never the driver's raw text.

A constraint `blobfs` does not own is handled by class on a delete and left alone on a write, as
at stage 16. The consumer maps a foreign key's name to its own sentinel through the same
`ViolationError` shape (`domain/files/database_bookmarks.go`). Two refusals are made in Go before
any SQL: `ErrRootDirectory` and a `NameError`/`IDError`.

### The write path

A file write is two steps around the object write, which the library never makes.
`BeginOrResumeFileWrite` (stage 22; the consumer's find-or-begin logic moved into the library)
looks the name up first and inserts only when absent, returning a `WriteOutcome` — created,
resumed a `pending` row, or already present — so `put`, `cp`, and a seeder share one write
protocol. `CompleteFileWrite` runs the guarded update and reads the row back on a refusal to
classify it. `WithID` lets a caller supply the row's id, so a seeded file keeps its id and its key
across resets (adjustment 18); a found row keeps its own id regardless.

There is no fail step and no `failed` status, unchanged since stage 10. A stop between the steps
leaves the row `pending`; a retry of the same write resumes it; an abandoned write is removed
through the delete steps.

### The delete path and the reference-then-delete rule

A file delete is two steps around the object delete, mirroring the write. `BeginFileDelete` moves
the row to `deleting`; `CompleteFileDelete` removes it. The retry rule per step holds from stage
16: the begin returns a deleting row unchanged, the object delete succeeds on a missing object,
and the complete succeeds on a missing row.

The bookmark-versus-delete race, open at stage 16, closed at stage 23. At stage 16, a delete
checked the bookmark count before its begin, and a bookmark added between that check and the
complete step made the foreign key refuse the row's removal after the object was already gone,
leaving a `deleting` row with a bookmark until the bookmark was removed by hand. The library's
`HoldFile` now takes the file's row lock through a guarded update that changes no value and no
version; `AddBookmark` holds the file before it inserts, and `deleteFile` begins the delete before
it reads the bookmark count, so the two operations serialize on the row. The rule for a consumer
is reference-then-delete: hold the file in the transaction that inserts a row referencing it. A
row inserted without the hold still meets the foreign key at the complete step, which leaves the
row `deleting` with its bookmark until the bookmark is removed. Both interleavings are proven
against Postgres with real concurrent transactions
(`TestBookmarkAddedDuringTheDeleteIsRefused`, `TestDeleteDuringTheBookmarkAddIsRefused`), on both
variants, with the earlier `TestRemoveMeetsABookmarkAfterTheBegin` removed.

`RemoveDirectory` and the consumer's `rm -r` walk are unchanged since stage 16: no cascade, no
recursive delete in the library, and the consumer's walk empties a directory in pages until it is
gone or gives up as `ErrTreeBusy` after three stalled passes.

### The move path

`MoveDirectory` and `MoveFile` are unchanged in shape since stage 16: the directory move takes the
variant's tree lock, the cycle check, and a guarded update in one transaction; the file move is one
guarded statement with no lock. What is corrected: `Directory.Name` is a `string`, not a pointer
(stage 18), so a rename no longer nil-checks the root's name. `MoveEntry` (stage 25) is the id
form: it reads the source row for its parent, name, and version, resolves both paths for the
result, and takes an optional `Scope`.

What the baseline requires of a caller holds from stage 16: the lock, or serializable isolation
with a retry on SQLSTATE 40001, or serializing moves outside the database.

### Path resolution and ids as the handle

`ResolveDirectoryFrom` (stage 24) resolves a relative path from a directory id, sharing its
segment-splitting and per-segment walk with the absolute-path `ResolveDirectory`, so both accept
and refuse the same names with identical messages. `IsWithin`, exported since stage 16, is the
cycle check used both by `Move`'s own check and by a consumer's scope check.

Ids are the primary handle in the library and in the consumer above it (adjustment 9). The
consumer's `Store` gained an id-keyed method beside each path-keyed one — `ListDirectory`,
`StatFile`, `OpenFile`, `PutFile`, `MoveEntry`, `RemoveFile`, `CopyFile` — each taking an optional
`Scope` (stage 25). A caller that holds a listing row's id and version acts on it without a read:
`RemoveFile` with a version holds the row at that version first, refusing before anything begins
if the row moved on.

### Keys

Unchanged since stage 16 (proof 7). `blobfs.KeyValidator` has one method,
`ValidateKey(key string) error`, no maximum length, and the consumer's adapter is one method over
the provider's own capability.

### The migration set the library ships

`lib/blobfs/postgres` exports `Migrations() (migrator.Set, error)` (stage 19), returning the whole
set — name `blobfs`, history table `blobfs_schema_version`, and the migrations — as one value,
rather than the name, the table, and the migration list assembled separately as at stage 16. The
set is two migrations (stage 18; a third, `0003_file_created_index`, existed at stage 16 as the
upgrade rehearsal and was removed with the index it added). The upgrade rehearsal now lives in the
migrator's own tests as a fixture third migration, not a released one, since nothing in `blobfs`'s
set is released.

The set's self-containment rules hold: it owns every object it creates under the `blobfs_` prefix,
never references a consumer's objects, and a golden-hash test pins every migration's text. The
multi-set migrator and a consumer's adoption of a shipped set are recorded in
`context/concepts/migration-sets.md`, not restated here.

### What a consumer composes

`domain/files` is the shape `v1.storage` will take, updated for Phase 3:

- **File copy** (stage 26). `Copy` and `CopyFile` are a third sequence over the two-phase write,
  reusing `Put`'s begin and complete steps to stream bytes from a source object to a new one. The
  destination reads like `mv`. The source must be `available`; a directory is the new `ErrNotAFile`
  sentinel. Bookmarks and owner rows stay with the source.
- **The scope check by id** (stage 25). `InScope` reads the owner row of a client-named scope
  directory first, then asks the library's `IsWithin` whether the target lies inside it, so a
  supplied scope id is checked and never trusted; a unit that names a directory it does not own
  learns nothing about what is under it. The path form of `ls --unit` keeps deriving its scope
  from the path, unchanged.
- **The bookmark read model** (stage 25). It returns `file_id` and `directory_id` and computes a
  row's path only when the caller asks (`bookmark ls` does); the default statement carries no
  recursion, which the previous read model always ran.
- **Seeding** (adjustment 18, stage 22). `EnsureDirectory` and `BeginOrResumeFileWrite` are the
  library's insert-or-find operations a seeder needs, with `WithID` for a stable id across resets.
- Every other item — the pattern catalog, the variant choice, the object-store adapter, the
  transaction boundaries — is unchanged from stage 16.

## The adjustments `sqlate` needs

The consolidated list from the post-execution review, sorted after the review into what
`blobfs.sources` addresses and what stays an unscheduled ledger. Each item states what fails or is
awkward, the evidence, the workaround `blobfs` uses today, and the smallest change in `sqlate` that
removes it. The full text of all 34 items, plus the three Phase 3 additions, is preserved below;
the sort and the shapes chosen for `blobfs.sources` are in
`context/concepts/sqlate-library-support.md` and `migration-sets.md`.

### Projections and listings

1. A projection base cannot bind a parameter. A listing anchored on one directory therefore cannot
   be a projection, and a projection whose recursion must be a top-level common table expression
   walks from every row before the outer filter runs. Evidence: the composer exists
   (`lib/blobfs/data/listing.go`); `evidence/bookmarks.txt` section c (the top-level recursion
   costs 164.706 ms and 3023 buffers for a unit with 10 bookmarks among 53,110, and 167.220 ms for
   1,000) against section a (0.270 ms and 245 buffers for 10). Workaround: the library composes its
   listings outside the projection, and the bookmark read model moves its recursion into a scalar
   subquery correlated on each row's directory, which the planner pulls up. Change: a base that
   binds parameters, sorted to `sources` (`sqlate-library-support.md`): `List`/`One` take a
   variadic `base ...Args`, so an unscoped call is unchanged and a scoped one passes one `Args`
   value built with the new package function `query.With(name, v) Args`. This is also a `v1.auth`
   requirement (`design/auth-strategy.md` section 4), independent of `blobfs`.
2. The total belongs in the page statement, and a projection cannot skip its count.
   `Projection.List` always runs its count twin before the page. Change: a total mode on
   `Directives` (exact, window, none), sorted to `sources`.
3. A derived-table wrap over a base that contains `WITH RECURSIVE` loses the index order and blocks
   the outer filter. Change: sorted to the backlog; item 1 removes most of the cost in the cases
   that matter, and the alternative composition contract would give up the projection's
   base-independence guarantee.
4. The catalog exposes its inventory and not its renderer. Sorted to the backlog: it serves only a
   composer built outside `Projection`, which items 1, 2, and 6 retire.
5. The clause patterns fix the correlation name `q`. Sorted to the backlog, for the same reason as
   item 4.
6. `Projection` has no keyset paging. Sorted to `sources`: a cursor on `Directives` with the
   composer's rules (the key as the tie-breaker, one direction, no nullable term), and the Postgres
   half as the first pattern overlay the engine module supplies (the row-value predicate).
7. A projection's key is one field, and the bookmark read model's unique key over the unfiltered
   base was the pair `(unit_id, file_id)`. Sorted to `sources` alongside item 6, at the same
   verification depth `sqlate` already applies to a single key (a declared-field check only, no
   schema introspection): composite-key grammar (`--| key: a, b`) is cheap to add and item 1
   removes the one case that needed it. Real schema-level uniqueness verification, for a single key
   or a composite one, is a separate, new capability and stays in the backlog.
8. `Statement` does not expose the dialect it compiled against. Sorted to the backlog: it serves
   only the outside composer.
9. `Statement.Text()` trims a trailing semicolon and nothing states it. Sorted to the backlog:
   already documented in `docs/features.md` and the parser's own comment; a godoc line on `Text`
   rides along with a later change.
10. A statement cannot resolve a path in one round trip at the standard tier. Closed: the
    one-statement Postgres form needs nothing from `sqlate`; a `text[]` parameter binds as one
    `Args` value, and the statement is native in an engine package.

### Transactions and sessions

11. `sqlate.Session` cannot begin a transaction. Sorted to the backlog: the library's own answer,
    typing the four transaction-only operations as `*sqlate.Tx`, is the better contract.
12. The transaction requirement lives in the statement, not the operation. Closed as a
    `blobfs.build` API decision, not a `sqlate` change: `Directories.Move`, `LockTree`, `Files.Hold`,
    and `Files.Delete` take `*sqlate.Tx`.

### Errors and constraint classification

13. `UnknownFieldError` unwraps to `ErrDirectives`, the client-error sentinel. Sorted to the
    backlog, as "no change; document the contract": the proposed separate sentinel would regress
    `go-web-service`'s mapping of an unknown field to a 400 response; the correct fix is a
    documented `errors.As` contract for a program that composes its own filters.
14. `postgres.Dialect.MapError` does not map SQLSTATE 2BP01. Assigned to `blobfs.sources`
    (`migration-sets.md`), since the multi-set migrator's `Down` refusal needs it.
15. `postgres.Dialect.MapError` leaves SQLSTATE 40001 unmapped. Assigned to `blobfs.sources`
    (`migration-sets.md`); this is also the sentinel a baseline caller's serializable-isolation
    retry needs (decision 4).
16. `ConstraintError` carries no table name. Sorted to `sources`: the driver already exposes the
    table and column, and a not-null violation (Postgres sets no constraint name for that class)
    has no usable handle today. Additive: `Table` and `Column` fields, no consumer change forced.
17. `query.Guard` reports only a version mismatch and cannot carry a second predicate, so a guarded
    statement with a status predicate misreports a status refusal as a version mismatch and cannot
    return the row. Sorted to `sources`: a real correctness defect for any guarded write with a
    lifecycle status, not only `blobfs`'s two instances. A typed guard whose check returns the row
    and distinguishes no row, a version mismatch, and a status refusal; the existing `Guard` keeps
    its shape for consumers with the plain predicate.

### Statements, tiers, and native declarations

18. A `--| field:` timestamp type must be spelled correctly for the standard tier, and `Verify`
    never checks it. Sorted to `sources`: startup verification is the library's stated promise, and
    every projection consumer is exposed. A companion `sqlint` change extends the native-forms
    check to header values, shipped in lockstep, not trailing.
19. A library that ships statements hard-codes the `sql.` namespace. Sorted to `sources`: settled
    before `go-auth` becomes the second shipper. The namespace is reserved and resolved by
    identity, and `As` no longer applies to it.
20. A native statement's declaration is one line. Sorted to `sources`: every native port note in
    the workspace is already an unreadably long single line. A header value may span lines under a
    `port:` key, with the one-line form still accepted; the matching `sqlint` support ships in the
    same release, not trailing.
21. `Verify` never asks the dialect whether a native statement belongs to it. Sorted to the
    backlog: the value is real only once a program can be compiled against more than one dialect,
    which needs item 20's structured engine name and a second engine to be worth the lint
    exemption.
22. `sqlint`'s native-forms check keys on the tier, not the directory. Sorted to the backlog: a
    `sqlint` release of its own, triggered when the promoted `blobfs` repository wants to state the
    rule.
23. No tool lists a program's native statements with their ports. Sorted to the backlog: a natural
    addition once item 20 structures the port, not needed now.
24. A parameter inside a published pattern takes no cast. Sorted to the backlog: neither consumer
    has a failure from it, only a style inconsistency, and the two proposed grammars have no
    evidence to choose between them.
25. `StandardCatalog.HistoryExists` does not qualify by schema. Sorted to `sources`: a correctness
    defect in `migrate` for any multi-schema consumer, and the history protocol is being reworked
    anyway.

### `migrate` and the multi-set migrator

The hooks that make `lib/migrator` disappear, absorbed into `sqlate` under `blobfs.sources` rather
than promoted with `blobfs`; the full design is in `context/concepts/migration-sets.md`.

26. `migrate` does not export its default table name or offer a `Table()` accessor.
27. `migrate` cannot run on a caller's connection, so a run needs two pool connections.
28. `migrate` has no operation that drops its history table.
29. `Options.Unlocked`'s doc comment should state that a caller holding its own lock is the
    intended use.
30. `Version` and `Verify` read without a lock at four queries per set; a `Status` returning head,
    dirty, and pending in one read is the fix.
31. `Steps` tolerates a count larger than the applied prefix, which should be documented as a
    guarantee.
32. `sqlate.Locker` is enough for the outer lock; no change is needed, and the finding supports the
    promotion. A related correction to the concept: multi-statement transactional migrations work
    on pgx's simple protocol, so v0.1.1 hosts one `Migrator` per set with its own `Options.Table`,
    which is what the shim is; the concept's claim that v0.1.1 cannot host a source holds only for
    one merged `Migrator`.

### Scanning

33. `Scanner` refuses a column with no field, so a page statement carrying a window total needs its
    own scanner. Sorted to the backlog, as "subsumed by item 2": once the total lives inside the
    projection, no consumer needs extra scan destinations.
34. The scanner does not flatten embedded structs. Sorted to `sources`: ordinary struct-mapper
    behavior every consumer benefits from, and `go-web-service` already has a case (an `Identity`
    type it cannot embed today).

### The Phase 3 additions

- **A. Engine-keyed statement overlays**, for a statement of the same shape declared once and
  respelled per engine. Sorted to the backlog: no instance exists even inside `blobfs` — the
  Postgres engine's `RETURNING` forms change the statement's shape from exec to query (addition B),
  and its other two native statements have no standard twin.
- **B. A statement that returns the changed row, declared once** (`RETURNING` on Postgres,
  insert-then-read on the baseline). Sorted to the backlog: the right shape is not knowable from
  one library; a second library with the same fallback need, or a settled protocol shape, would
  decide it.
- **C. The keyset predicate as a pattern overlay.** Sorted to `sources`, as the Postgres half of
  item 6.

### What each adjustment unblocks

| Roadmap task | What it needs from this list |
|--------------|-------------------------------|
| `blobfs.sources` (`sqlate` v0.2.0) | Items 26 to 31 are the migrator's hooks; 32 confirms the lock; 14 and 15 let the promoted migrator and a baseline caller classify SQLSTATEs; 1, 2, 6, 7, 16, 17, 18, 19, 20, 25, and C are the library-hosting adjustments the architect chose to fold into the same task. The task covers six areas — the collection read, the guard, verification and headers, the mapper, errors, and `migrate` — not the migrator alone. |
| `blobfs.build` | Nothing blocks it: the library builds and passes over v0.1.1 with today's workarounds. If `sources` lands first, `blobfs.build` can collapse `listing.go` and `cursor.go` onto `query.Projection` instead of carrying them, and the eight-method `Variant` interface is expected to shrink to four. |
| `v1.storage.service` | Items 2, 6, 11, and 34 are the consumer's awkward call sites; item 1 serves the directory-anchored listing a scoped read model needs. |

One answer stays only partly supported: whether any sort besides `created_at` earns an index was
never measured; the answer for `size`, `status`, and the rest rests on the plan shape, not a
measurement.

## Incorporation into `v1.storage`

### One install per configuration

Unchanged since stage 16. An install is one database and one container, with fixed `blobfs_`
object names; a service serving several isolated trees runs one configuration per tree.
`TestIsolation` is the evidence that two configurations share nothing.

### The migration source in the service's one migrator

Unchanged in shape; the migration source is now one value from `postgres.Migrations()` rather than
assembled from a name, a table, and a migration list. The service builds one `migrator.Migrator`
over `[]Set{blobfs's set, its own set}`, `blobfs`'s first in `Up`, last in `Down` and `Reset`.

### What the admin surface must expose

Unchanged: `status`, `up`, `down`, `reset` behind confirmation, and `force <set> <version>` for
dirty recovery, because `Reset` refuses a dirty set and the operator's repair states the applied
version through `Force`. `Force` rather than a flag on `Reset` was chosen because a flag that
cleared the dirty mark and tried the down would have to guess whether the failed migration's
objects still exist; naming the version explicitly does not.

### Directory-grain ownership

Unchanged since stage 16 (see "Findings against other repositories" for the auth-strategy
amendment this section motivates).

### File-grain ownership

Unchanged since stage 16, with the reference-then-delete rule (see "The delete path" above) now
the mechanism that closes the race a bookmark and a delete could otherwise hit.

### What a move may and may not do under directory-grain ownership

Unchanged since stage 16: a move stays under one top-level directory, because the owner row binds
one and the scope is checked once at that ancestor. A move across two top-level directories would
carry an entry from one unit's scope into another's without either unit's say. `mv` takes no
`--unit`; a unit's right to move within its own scope is the authorization the experiment does not
prove (decision 6).

### The write, delete, and move protocols as the service must sequence them

Unchanged in shape from stage 16, with two updates: the upload's begin step is now
`BeginOrResumeFileWrite`, shared with `put`, `cp`, and a seeder; the delete's first transaction now
holds the file (through `HoldFile`, from the row that references it) before checking or inserting
any row that references the file, per the reference-then-delete rule, rather than only checking a
bookmark count before the begin.

### Deployment hazards the evidence found

Unchanged since stage 16, with the bookmark-versus-delete race removed (closed at stage 23) and one
addition: the migrator needs two pool connections until `blobfs.sources` lands (Finding 2, item
27).

### What stays undecided until `go-auth`

Unchanged since stage 16.

## The proofs

| Proof | The answer | The decision it changes |
|-------|------------|--------------------------|
| 1. Read model | The path stays a read-time query, and no path or volume id is stored. | The parameterized base stays motivated for shapes the correlated form cannot take. |
| V3. Listing cost | The exact total stays the default; the cursor earns its place; the composer's design is required for a recursive base. | Decision 5 moved the `created_at` index to documentation. |
| 2. Statements and tiers | Twenty-two standard statements, six native on Postgres, each shipped after a measurement. | The variation-point count grew from two to six at stage 29, each justified individually. |
| 3. Two-phase write | The pending row survives a stop and a retry completes it; failure is not a state. | No fail step ships. |
| 4. Move under a lock | The lock closes the race on Postgres; the baseline forms a cycle without one. | The variant takes the lock; the baseline's requirement is documented. |
| 5. Migrator | Integrated shipping as a shim; the API is what `blobfs.sources` promotes. | `blobfs.sources` promotes the shim's shape with the migrate group's hooks. |
| 6. Delete | The delete converges under retry at each step; a bookmark meeting a delete now serializes on the file's row rather than racing it. | Adjustment 10 closed the race `TestRemoveMeetsABookmarkAfterTheBegin` once demonstrated. |
| 7. Key validation | One method, no maximum length; a filename at the rune boundary is accepted and one rune over is refused before any SQL. | The concept's key validation and length claim becomes key validation alone. |
| V. Variant seam | Both variants pass the conformance suite; a consumer variant swaps one method without a fork; the engine package adds no dependency weight. | Shipped in the module, as a package (`lib/blobfs/postgres`). |
| 8. Awkward call sites | Nineteen call sites at stage 16 had to work around what they were given, mostly in `sqlate`. | Each names the entry that holds its evidence; Finding 2's sort names which now have a path forward. |

## The evidence

Buffer counts and plan shapes carry across machines; milliseconds do not.

- **The listing cost (proof V3)**, `evidence/read-model.txt`. The exact-total page over the
  biggest measured directory (10,008 files) costs about 2,434 buffers against 12 without a total.
  A cursor page costs 6 buffers against 2,434 by offset at the end of the directory. Composing at
  the base's own level (rather than wrapping a recursive base as a derived table) matters: the same
  base pages in 21 buffers flat and 2,443 wrapped.
- **The bookmark read model**, `evidence/bookmarks.txt`. The consumer-anchored shape (a scalar
  subquery correlated on each row's directory) costs 245 buffers for 10 bookmarks, 2,371 for 100,
  and 21,940 for 1,000, against a tree of 10,003 directories and a bookmark table of 53,110 rows,
  and nothing grows with the tree. A top-level recursion over the whole table costs 164.7 ms and
  3,023 buffers for the same 10 bookmarks. Stage 25's default statement omits this recursion
  entirely; it runs only when a caller asks for paths.
- **The `created_at` index**, `evidence/sort-index.txt`. Regenerated at stage 31: the index turns a
  page sorted by `created_at` without a total into an index read at about 25 buffers against about
  2,436 without it, and costs 3.99 MB for 100,000 files. The transcript's headers now read "the
  library's set alone" and "after the index" rather than migration-version language, since the set
  is two migrations and the index is created by the measurement itself, not shipped.
- **`evidence/v1-read-model.txt`** is the volume-era measurement, kept as the record; nothing
  regenerates it.
- **The native variation points**, `evidence/native-variation/`, produced at stage 28. `RETURNING`
  saves one round trip on the write steps in every outcome (a refusal drops from three round trips
  to two). One-statement path resolution drops depth 10 from 11 round trips to 1, at a cost of 3
  extra buffers the final query removes by selecting the maximum-depth row directly. The row-value
  keyset predicate costs 35 buffers at any cursor position in an indexed directory, against up to
  5,055 for the expanded chain at a middle cursor.
- **The cost regression assertions**, stage 30 (`lib/blobfs/data/cost_integration_test.go`,
  `lib/blobfs/postgres/cost_integration_test.go`). Seven tests assert a plan shape and a buffer
  bound, with a wide margin, for the cursor page, the row-value cursor, the exact-total page,
  one-statement path resolution, one step of the standard walk, and the primary-key lookups of the
  write and delete steps, so a future plan regression fails a test rather than surfacing only in
  production.

One answer stays only partly supported: whether any sort besides `created_at` earns an index was
never measured.

## The review: the fifteen questions and their answers

The open questions the post-execution review raised, each with the architect's decision, folded
into the "as built" record above where the decision changed something; recorded here as the record
of what was decided and why.

1. **The seeded root.** Kept the migration's seed of `RootID`, the nil UUID; an `EnsureRoot`
   operation was the alternative. The seed needs no lifecycle step and gives every consumer the
   same id.
2. **`NoTotal` on an empty page after the first.** Kept the edge and documented it, rather than
   running a count statement for that one case or deriving the total from a prior cursor.
3. **The tie-breaker's direction.** Kept the rule that the appended `name` follows the sort's
   direction, so a descending page is the exact reverse of the ascending one and can take a cursor.
4. **`MoveDirectory` on the baseline.** Documented serializable isolation with a retry on SQLSTATE
   40001 as the caller's route, rather than building a second baseline lock, until a consumer on an
   engine without a native lock exists.
5. **The `created_at` index.** Moved to documentation before `blobfs.build`, rather than kept in
   the shipped set; the rehearsal had served its purpose and nothing in the set was released.
6. **Whether `mv` should take `--unit`.** No flag until `go-auth`; a unit's right to move within
   its scope is authorization, which the scope check by id belongs to that layer.
7. **Whether the Postgres variant ships in the module or a sub-module.** In the module, as a
   package beside the persistence package, since it adds no dependency weight.
8. **The `datatest` package living in `lib/` as a non-test package.** Kept; a consumer that
   supplies a variant of its own needs the conformance suite too.
9. **The key validator as a parameter of the begin step.** Kept per call, so the persistence
   package builds without the object store and the consumer opens the store lazily.
10. **The bookmark-versus-delete race.** Decided at the review to close it rather than accept it
    with the foreign key as the backstop; adjustment 10 (stage 23) built `HoldFile` and the
    reference-then-delete rule.
11. **A capped total.** No cap; `TotalNone` bounds the cost to zero and the cursor pages a large
    directory without a total.
12. **`ls`'s two cursor flags.** Kept `--after-dirs` and `--after-files`; directories and files are
    separate resources in a consumer's API.
13. **`Directory.Name` as `*string`.** Decided to change it: the root is named `/`, and the field
    becomes a plain `string` (stage 18), reversing the stage-16 position of keeping the pointer.
14. **`rmdir` removing the owner row silently.** Kept; the alternative would make an owned
    directory undeletable.
15. **The root package as a layer of its own.** Kept; the constraint constants and sentinels must
    sit below the persistence package.

## The review: decisions added

Beyond the fifteen questions, the review settled these:

1. **Schema shape.** Design A, the two-table schema, over designs B, C, and D (see "The schema, and
   the designs it was chosen over").
2. **Name collisions.** No constraint; a directory and a file may share a name under one parent,
   and a path resolves by kind.
3. **Navigation principle.** The core API navigates one directory at a time; a subtree search is a
   separate, deferred capability.
4. **The cursor.** Kept, with its conventions (which sorts, the opaque token, bound to the listing
   and sort).
5. **Page state.** `Page.More` reports whether rows remain, independent of the total (stage 21).
6. **File copy.** Added as its own stage (stage 26); directory copy stays out of scope.
7. **The composition root's call sites.** Kept `files.WithVariant` with its type parameter and
   `Infrastructure.Storage` naming `files.Storage`.
8. **The migrator's two connections.** No pool validation added to the shim; the fix belongs in
   `sqlate` (Finding 2, item 27).
9. **Ids are first class.** Ids are the primary handle in the library and the consumer above it
   (stage 25); paths are entry points and for display.
10. **The command-line tool's scope.** The tool proves the concept and is not promoted; its
    details are not reviewed further unless they block proving a library capability.
11. **Native features are welcome.** The library does not constrain itself to the standard tier
    where a native form measurably wins (stages 28 and 29).
12. **Frictions become `sqlate` evolution.** The frictions Finding 2 records are carried to
    `sqlate`'s roadmap, not fixed inside the experiment.
13. **Migration sets are layers.** A migration set is a self-contained layer with its own name,
    history table, and version numbering, declared bottom-first (see
    `context/concepts/migration-sets.md`).

## Phase 3: the adjustments and the stages that landed them

Each adjustment the review queued, and the stage that implemented it, all committed on
`blobfs-experiment`:

1. `Page.More` — stage 21.
2. File `cp` as its own stage — stage 26.
3. Tool only: `--cursors` for the cursor lines — stage 27.
4. Tool only: `--filter` on `ls` — stage 27.
5. Regenerate the evidence transcripts — stage 31.
6. Error output conventions — stage 20 (the library's side; the tool's prefixes were left optional
   and not built).
7. `GUIDE.md`, the evidence table, and the layout documentation — stage 32.
8. Document the migrator's two-connection requirement and `NewStorage`'s composition — stage 32.
9. Ids as the primary handle — stages 22 (seeding), 24 (relative path resolution), 25 (id-keyed
   consumer operations and the scope check by id), 27 (the tool's `id:<uuid>` form).
10. Close the bookmark-versus-delete race — stage 23.
11. Decompose the large files — stage 17 (`domain/files`); `lib/blobfs/data` was not split in the
    experiment (see "Findings against other repositories" for the promotion-time recommendation).
12. Error wrapper (`ViolationError`) — stage 20.
13. Root named `/` — stage 18.
14. Native engine strategy — stage 19 (the engine package) and stage 29 (the variation points),
    measured at stage 28.
15. Remove the `created_at` index from the shipped migrations — stage 18.
16. Engine package owns the DDL — stage 19.
17. Reset before Phase 3 code runs against an old database — stated as a caution throughout, not a
    stage of its own.
18. The library's seeding API surface — stage 22.
19. Cost regression assertions — stage 30.

Stages 31 and 32 close the sequence: 31 regenerates the three evidence transcripts (every buffer
count and plan shape unchanged; only timings and the random ids the fixtures mint differ), and 32
rewrites `README.md` to the architect's specified shape (overview, a short layout table, starting
up, the command guide, shutting down, remaining reference), rewrites `GUIDE.md` as the tour the
architect's final review followed, corrects two claims `REVIEW.md` had made stale (the extra row
fetched only when a cursor could continue, and the open bookmark-versus-delete race), and documents
the migrator's connection requirement and the `NewStorage` composition pattern in code comments.
Five commits follow stage 32 as further cleanup: a filter-example and cursor-output correction in
`GUIDE.md` found while the architect walked through it, and a modernization pass applying `gopls`'s
`modernize` analyzer to six loops (a range-over-int, a `slices.Backward` walk, a `reflect.Fields()`
iteration, and three `strings.SplitSeq` loops), all behavior-preserving.

## What left the experiment

Promoted into the workspace's durable context, all concept-tier because nothing here is built yet:

- `context/concepts/blobfs.md`, re-scoped to what the library is.
- `context/concepts/blobfs-api.md`, the proposed API for `blobfs.build`.
- `context/concepts/blobfs-composition.md`, how a consumer builds around the library.
- `context/concepts/migration-sets.md`, the multi-set migrator `blobfs.sources` promotes.
- `context/concepts/sqlate-library-support.md`, the scheduled and backlog `sqlate` adjustments.
- `context/design/auth-strategy.md` section 8, amended for the directory grain.
- `context/roadmap.toml`, `blobfs.sources`, `blobfs.build`, `blobfs.admin`, `v1.storage.tasks.service`,
  and `v1.storage.tasks.suite` recite the new concept documents; `blobfs.experiment` is removed and
  `next` advances.

Nothing moved into `context/design/`: the repository the library's design notes would be about does
not exist yet, and the API names are still proposals `blobfs.build`'s own SETTLE owns.

## The promotion candidates, reviewed for `blobfs.build`

Facts about the closed branch's code, for whoever starts `blobfs.build` to read from here rather
than rediscover. Scope: `lib/blobfs`, `lib/blobfs/data` (with `datatest`), `lib/blobfs/postgres`,
and `lib/migrator` only — the promotion candidates; the tool is not reviewed further (decision 10).

**`lib/blobfs`.** Seven non-test files, 549 lines, none over 110. Clean; stays a package.
`entities.go` holds `Directory`, `File`, `Object`, `RootID`, `NewID`, `ParseID`, and `IDError`
together and could split by type (`directory.go`, `file.go`, `id.go`) once `Directory`/`File`
become the organizing types elsewhere.

**`lib/blobfs/data`.** Thirty-two files, about 7,600 lines, with `datatest` a further 1,872 (of
which `datatest.go` alone is 1,209). Three files exceed 500 lines: `listing.go` (619), `data_test.go`
(619), `data_integration_test.go` (793). The five operation files are organized by which stage
added their contents, not by concern: `directories.go` holds the file listing, `variant.go` holds
three `Store` methods and the delete's begin, `files.go` holds the delete's end, `paths.go` holds a
`Standard` method, and `move.go` holds both types' moves. A split by entity (`Directory`/`File` as
the organizing types, following `go-web-service/context/design/domain-architecture.md`'s
role-prefix convention) resolves most of this; `listing.go` and `datatest.go` are generic over both
types and need a concern-based split of their own regardless (a `listing_scan.go` for the scanner
and clause-filling helpers is a real, non-cosmetic split). Other findings: `postgres.New` compiles
the baseline statement set a second time through `data.NewStandard`, when the variant could borrow
the store's own compiled set; `Store.Variant()` is unused by the library or the tool;
`EnsureDirectory` and `BeginOrResumeFileWrite` are the same insert-or-find algorithm on two types
and could share a private generic helper.

**`lib/blobfs/postgres`.** Three non-test files, 366 lines. `TreeLockKey` is computed at init from
a hash that a test already pins and could be a literal `const`. The package is otherwise the right
home for both the variant and the DDL, and nothing in it is tool-shaped.

**`lib/migrator`.** Four files, 1,322 lines with tests. One file of code over the public `migrate`
API, well-shaped for absorption into `sqlate` (see `migration-sets.md`), needing no review beyond
that disposition.

## Findings against other repositories

- **`go-storage` and `azureblob`.** `Put` returns the caller's content type, while `Stat` and `Get`
  return the server's. `Delete` on a missing container returns `ErrNotFound`, which contradicts its
  own doc comment — a real defect, routed to `go-storage` separately, not fixed here. `Store.Start`
  wraps every failure, rejected credentials included, as `ErrUnavailable`. `go-storage` has no
  container delete, so the experiment's own test support reaches for the Azure SDK directly
  (`internal/livetest/storage.go`); a `DeleteContainer` on the provider or on `storagetest` would
  remove that dependency. The storage configuration reads its own environment
  (`Config.Finalize(prefix)`), so a consumer wanting a flag beside `--dsn` has to bypass it. One
  correction to the concept: `go-storage`'s `*Store` has had `List` since v0.1.0
  (`go-storage/store.go:210`); the earlier ledger's claim that the interface has no listing
  operation was wrong. The argument the concept makes from it — that keys carry no directory prefix
  and so foreign keys never cascade — holds regardless.
- **`architecture/standards/go-elemental/principles/tests-and-docs.md`.** Its sentence "Production
  source is written without doc comments; the agent writes godoc" was read by the experiment as
  forbidding doc comments on exported identifiers; the experiment wrote godoc on every exported
  identifier anyway, following what the sentence most likely states — who writes them, not that
  none exist. Worth a one-line clarification there; not a finding of drift.

## What the experiment did not prove

- Authorization. `--unit` rehearses a directive filter's shape; the composed authorized listing
  under the auth strategy's scope predicate needs `go-auth` and is proven in `v1.auth`'s storage
  sweep.
- The admin surface through `go-web-service`. `admin/schema` stands in for `go-database`'s admin
  release; the migration path through the service's admin surface is `v1.storage.service`'s and
  `blobfs.admin`'s to prove.
- The documentation-only migrator. Estimated, not built.
- Other engines. The DDL is Postgres only; the port notes are the work list for a second engine.
  The tree lock's port is one sentence for the next engine to weigh: SQL Server has an equivalent
  advisory lock; MySQL's `GET_LOCK` is session-scoped, not transaction-scoped; SQLite has neither.
- Sustained load. Every measurement is a median of five runs on one machine with everything in
  shared buffers; no concurrency beyond two connections was measured.
- The sweeper for abandoned `pending` or `deleting` rows. The listing filters it would use exist;
  the concept's trigger is `v1.messaging`.
- Content replacement, versioning, soft delete, and checksums, which the concept defers.
- Key validation against a real Azure account; Azurite accepts keys `azureblob` rejects.
- `sqlint`'s `[export]` entry for the published namespace; v0.1.1 offers only a bare directory.
- The interleaved directory-and-file listing, which the concept defers to a folder browser.

## Reproducing the results

Every command runs from `experiments/blobfs`, with the toolchain `mise.toml` pins (Go 1.27 and
`golangci-lint` 2.13.2) and the compose stack up. Every engine-backed test creates and drops its
own database and container, and none touches the default `app` database. `README.md`'s "Remaining
reference" table is the current, short version of this list; the detail below is what
`README.md` intentionally leaves out.

- `mise run up` starts Postgres 18 and Azurite and waits for both to report healthy.
- `mise run test`, `build`, `vet` run the hermetic checks.
- `mise run integration` runs every integration-tagged test: the conformance suite on both
  variants, the persistence and consumer tests on the engine, the migrator's engine proofs, the
  cost regression assertions, and the integration package.
- `mise run demo` builds the binary and runs the scripted end-to-end script once per variant.
- `mise run evidence` regenerates the three transcripts under `evidence/`, each gated by
  `BLOBFS_EVIDENCE=1` and seeding its own throwaway database.
- `mise run split-check` and `mise run lint` run the layering check and `golangci-lint`/`sqlint`.
- `mise run cli -- <command>` runs the binary against `BLOBFS_DSN`, which names the compose stack's
  default `app` database by default; work in a throwaway database instead and pass `--dsn`
  explicitly.

**Adjustment 17's caution**, load-bearing for anyone resuming this branch or building against it:
a database installed before stage 18 keeps a root row with no name, and the new code's `NOT NULL`
column and `Directory.Name string` cannot scan it. Reset any such database — `app`,
`blobfs_review`, `blobfs_review2`, `blobfs_tour` — before running Phase 3 code against it. A
released migration is never amended this way; this applies only because nothing in `blobfs`'s set
was ever released.
