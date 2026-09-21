# blobfs review decisions

The architect's decisions from the post-execution review of `blobfs.experiment`, recorded as they
are made. `REVIEW.md` holds the findings and the fifteen open questions. This file holds the
answers to those questions, the decisions the review added, the adjustments queued for
implementation, and the edits queued for the close. The reset file records only where the session
stands.

Each entry has a status. Decided means the architect has settled it. Provisional means the
architect leans that way and has not confirmed it. Queued means decided and waiting for its stage.

## Answers to the questions in REVIEW.md

- Decision 1, the seeded root. Decided: keep the migration's seed of `blobfs.RootID`, the nil
  UUID. `blobfs_file.directory_id` is NOT NULL and references `blobfs_directory`, so a file at `/`
  needs a single root row to point at. With null-parent roots there would be many roots and no
  `/`. The seed needs no lifecycle step and gives every consumer the same id.
- Decision 12, the two cursor flags. Decided: keep `--after-dirs` and `--after-files`. They are
  harmless in the experiment, and directories and files are separate resources in a consumer's
  API.
- Decision 9, the key validator as a parameter of the begin step. Decided: keep the per-call
  parameter, so the persistence package builds without the object store and the consumer opens
  the store lazily.
- Decision 10, the bookmark-versus-delete race. Decided: close it, and do not accept it as
  built. A guarded update of the file row makes the two operations serialize on that row's lock,
  at the standard tier and with no third variation point (adjustment 10). The interleavings were
  run against Postgres: today's order leaves a `deleting` file with a bookmark, and the proposed
  order refuses the second operation in both directions.
- Decision 13, `Directory.Name` as `*string`. Decided: the root directory is named `/`, and the
  entity's `Name` becomes a plain `string` (adjustment 13).
- Decision 4, moving a directory on the baseline. Decided: document serializable isolation with a
  retry on SQLSTATE 40001 as the caller's route, and build no second baseline lock until a
  consumer on an engine without a native lock exists. `sqlate` maps 40001 first (Finding 2,
  item 15).
- Decision 7, where the Postgres variant lives. Decided: in the library module, because it adds no
  dependency weight, in the engine package of decision 11 (adjustment 14).
- Decision 8, `datatest` under `lib/`. Decided: keep it, because a consumer that supplies a variant
  of its own needs the conformance suite too.
- Decision 5, the `created_at` index. Decided: remove `blobfs_ix_file_directory_created` from the
  library's migration set and document it, so a consumer's own migration set adds it when it
  wants the index (adjustment 15).
- Decision 15, the root package as a layer of its own. Decided: keep it, because the constraint
  constants and sentinels must sit below the persistence package and a consumer with its own
  persistence still needs them.
- Decision 2, `NoTotal` on an empty page after the first. Decided: keep the edge and document it.
  `Page.More` tells a client that no rows remain, so an empty last page no longer reads as a
  fault.
- Decision 3, the tie-breaker's direction. Decided: keep the rule that the appended `name` follows
  the sort's direction, so a descending page is the exact reverse of the ascending page and a
  descending sort can take a cursor.
- Decision 6, `mv --unit`. Decided: no flag until `go-auth`. A unit's right to move within its
  scope is authorization, and the scope check by id belongs to that layer.
- Decision 11, a capped total. Decided: no cap. `TotalNone` makes the cost zero, and the cursor
  pages a large directory without a total. A consumer with very large directories asks for the
  total once on page 1.
- Decision 14, `rmdir` and the owner row. Decided: keep removing the owner row with the
  directory in one transaction. Refusing would make an owned directory impossible to delete.

All fifteen questions in `REVIEW.md` are answered.

## Decisions the review added

1. Schema shape. Decided: two tables, `blobfs_directory` and `blobfs_file`, with separate name
   spaces. The review evaluated four designs in scratch databases on Postgres 18 at 10,003
   directories and 100,000 files, with the biggest directory holding 10,009 files:
   - Design A, the two tables, won every measured read: 12 buffers for page 1 by name without a
     total, and 2,400 buffers for the exact-total page.
   - Design B, one `blobfs_entry` table with a `kind` column, served an interleaved listing in 14
     buffers. It cost about 6% more buffers on the exact-total page (2,555) and about 21% more
     storage, computed from index sizes. A consumer's typed reference to a file or a directory
     needs a constant generated `kind` column and a composite foreign key, and the per-kind check
     constraints have to be written NULL-safe.
   - Design C, a `blobfs_node` table with `blobfs_directory` and `blobfs_file` subtype tables,
     needed 19 round trips for a file lifecycle against 9, and page 1 by name cost 95 buffers.
     Because the version column and the status column sit in different tables, a file step is
     safe only as a two-statement transaction that checks the guard's row count. A node with no
     subtype row is representable, and it hides from the listing while holding its name and
     blocking its parent's removal.
   - Design D, design A plus a `UNION ALL` interleaved listing, cost 2,404 buffers for page 1 and
     2,383 for page 2 by cursor, because the union sorts the whole directory. A cursor near the
     end of the directory cost 13.

   Reason for A: the storage medium is a web-based virtual file system, and consumers address
   directories and files as separate resources, so an interleaved listing is not needed.
   Consequence: the concept records designs B and C as rejected alternatives with this evidence.

2. Name collisions. Decided: no constraint, and the library stays permissive. A directory and a
   file may share a name under one parent, and that is not a collision. A path resolves by kind:
   each command works on a defined kind, and `mv` resolves the directory first. A consumer keeps
   the two kinds as separate resources in its routes. A consumer that wants a single name space
   enforces a naming policy in its own layer. An optional name validator that a consumer supplies,
   like the key validator, is added only if a consumer asks.

3. Navigation principle. Decided: the core API navigates one directory at a time. Filters, sort,
   and pagination act on that directory's children. A search that descends a subtree is a
   separate, deferred API with its own cost profile. A web consumer traverses by directory id, so
   each step is one bounded query, and a path lookup remains a convenience entry point.

4. The cursor. Decided: keep it. It gives constant-cost deep paging and pages that stay correct
   when the directory changes, and it matches the continuation tokens of the underlying stores.
   Conventions to document:
   - A cursor applies to sequential walks of a directory's children on cursor-capable sorts:
     `name`, `created_at`, `updated_at`, `version`, and `id`. The pattern is to read page 1 with
     the exact total, then walk by cursor with the total off.
   - A cursor stays hidden where it does not apply: sorts on nullable columns and mixed
     directions, the consumer's projection read models, walkers that remove rows, and human CLI
     output.
   - A cursor is an opaque token. Clients must not parse it, it is not an authorization token, and
     it is valid only for the listing and sort that issued it.

5. Page state. Decided: `Page.More` reports whether rows remain after a page, independently of
   `Page.Next`, the cursor. The composer fetches one extra row on every page, including offset
   pages. The client reads three states: no more rows, continue by cursor, and more rows with no
   viable cursor so page by number. Today an empty `Next` means no more rows for a cursor-capable
   sort and unknown for any other sort.

6. File copy. Decided: add `cp <src> <dst>` for files as an additional stage. Directory copy is
   out of scope. The defaults are in the queued adjustment below.

7. The composition root's awkward call sites (Proof 8, layer 1). Decided: keep
   `files.WithVariant` with its type parameter, because the alternatives (a constructor that
   returns the interface, or registering variants by name) are not clearly better. Keep
   `Infrastructure.Storage` naming `files.Storage` in the command-line tool. A service composes
   the object store through `files.NewStorage(store)`, so the crossing does not arise there, and
   the documentation says so.

8. The migrator's two connections (Finding 2, item 27). Decided: add no pool validation to the
   shim. The migrations are a strict chain with nothing to run in parallel, and the second
   connection exists because the advisory lock is pinned to one connection while the inner
   migrator takes its own. The fix belongs in `sqlate`: `blobfs.sources` requires that a run
   uses one connection, with each set's migrations executing on the pinned connection.

9. Ids are first class. Decided: ids are the primary handle in the library and in the consumer
   layer above it. Operations are optimized around ids, and paths appear only at entry points and
   in display, so the overhead of resolving paths is paid only where a caller has only a path. The
   library already takes ids in every operation except `ResolveDirectory`, and its guarded steps
   take the version the caller read, so a client that holds a listing can act without a read
   first. The consumer's `Store` and the command-line tool are path-first and change (adjustment
   9 below).

10. The command-line tool's scope. Decided: the tool proves the `blobfs` concept and is not
    promoted into the build. Its details are not reviewed further unless they block proving a
    library capability. Adjustments that concern only the tool are marked optional.

11. Native features are welcome. Decided: the library does not constrain itself to the standard
    tier where a native form performs better. The engine is specified once, by the dialect the
    consumer constructs the library with, and the engine determines which patterns and statements
    are compiled. The standard tier stays as the reference semantics, the fallback for an engine
    with no native form, and the port template. The mechanism is an engine package selected by
    import, with variant methods for operations whose shape differs and statement overlays for
    those whose shape does not (adjustment 14).

12. Frictions become `sqlate` evolution. Decided: the frictions that Finding 2 records (the
    composer's text splicing, the scanner, the guard, the projection limits, and the rest) are
    signals for how `sqlate` evolves. They are carried to `sqlate`'s roadmap and are not fixed
    inside the experiment.

13. Migration sets are layers. Decided: a migration set is a self-contained layer with its own name,
    its own history table, and its own version numbering, so `0001_directory` in one set never
    collides with `0001_organization` in another. Sets are declared bottom-first, a library never
    depends on its consumer, and the order is a linear stack: `Up` runs ascending, and `Down` and
    `Reset` run descending. A library ships its set whole, so the consumer lists sets in layer
    order and does not assemble a name, a table, and a migration list (requirements below).

## Adjustments queued for implementation

1. `Page.More` (decision 5). Change `Page[T]`, the composer's offset path, and the cursor tests.
   The CLI prints a `more:` line. Update the `README.md` and the library docs. The consumer's
   projection read models derive `More` from their count when a total is requested.
2. File `cp` as its own stage (stage 26 of the Phase 3 list in the reset file). Add `Store.Copy` and the `cp` command in `domain/files`, with tests and
   `README.md` and `GUIDE.md` entries. Defaults:
   - The destination reads like `mv`: an existing directory receives the copy under the
     source's name, and any other path is the new path, whose parent must exist.
   - The source must be an `available` file. A directory, a `pending` file, or a `deleting` file
     is refused with a clear message.
   - A destination name already held by a file is `ErrNameTaken`, so there is no overwrite.
   - The bytes stream through the process with `Open` and `Put` around the write path's begin
     and complete steps. The content type is copied from the source row, and the size and etag
     come from the store's report of the new object.
   - `--fail-after insert|write` matches `put`, and a retry resumes the pending row.
   - A copy may cross top-level directories, and bookmarks and owner rows are not copied.
3. Tool only, decided: show the `next-dirs:` and `next-files:` lines only with a flag such as
   `--cursors`.
4. Tool only, decided: expose the library's filters as `--filter` on `ls`.
5. Regenerate the evidence transcripts with `mise run evidence` if the listing changes.
6. Error output. The unapplied-schema refusal and the `files:` and `data:` prefixes belong to the
   tool and are optional. The library's raw driver text after a sentinel is addressed by
   adjustment 12.
7. Update `GUIDE.md` after implementation, including a section on what the tour does not cover.
   Add the `evidence/schema-alternatives/` directory to the evidence table in `README.md`. The
   updated `GUIDE.md` drives the architect's final review of Phase 3, so it must cover every
   capability, including those Phase 3 adds, each with a summary, the key files, and the commands
   to run.
8. Document in the package comment of `lib/migrator` that a run needs two pool connections until
   `sqlate` runs a set on the caller's connection (decision 8). Document in `domain/files` and the
   library docs that a service composes the object store through `files.NewStorage` (decision 7).

9. Ids as the primary handle (decision 9). Queued as its own stage:
   - Add id-keyed methods to the consumer's `Store`: list a directory, stat, open, and remove a
     file, and put, move, and copy into a directory. The path methods become one resolve followed
     by the id method. A caller that holds `(id, version)` from a listing acts without a read.
   - Add relative resolution to the library: a variant of `ResolveDirectory` that starts at a
     directory id and takes a relative path (Finding 2, item 10).
   - Change the bookmark read model to return `file_id` and `directory_id` and compute the path
     only on request, since the per-row path recursion is the costliest measured query.
   - In the tool, add an id column to `ls`, let `stat` accept directories, and accept an
     `id:<uuid>` form wherever a path is accepted. Keep this minimal.
   - Check ownership by id with `IsWithin(dirID, scopeRootID)` against a client-named scope
     directory, followed by the owner-row read. The client-supplied scope id is an input to
     check, not to trust.

10. Close the bookmark-versus-delete race (decision 10). Decided:
    - `AddBookmark` first runs a guarded update of the file row inside its transaction, with the
      predicate `status <> 'deleting'` and, when the caller supplies the version, `version = ?`.
      The update writes no new version, so it does not invalidate other holders of the file's
      version. It takes the row lock, and it matching no row refuses the add.
    - `deleteFile` runs `BeginFileDelete` first and reads the bookmark count second, in the same
      transaction, and it rolls back with `ErrBookmarked` when the count is above zero.
    - Offer the guarded update as a library operation, for example `HoldFile`, so a consumer calls
      it before inserting any row that references a file. Document it as the reference-then-delete
      rule.
    - Replace `TestRemoveMeetsABookmarkAfterTheBegin` with tests of the two interleavings, and
      simplify the race notes in `deleteFile` and in `REVIEW.md`'s deployment hazards.

11. Decompose the large files (decision from the layer 3 review). Decided: split `blobfs.go`
    (803 lines) and `database.go` (364 lines) as the first Phase 3 stage, before any other change,
    so later stages land in the right files. The workspace convention
    (`go-web-service/context/design/domain-architecture.md`) keeps role-named files and, when a
    role file outgrows navigability, splits it by concern with the role as the prefix, never per
    entity and never into sub-packages. The architect confirmed the map:
    - `blobfs.go`: `compileLibrary` and the shared path helpers.
    - `blobfs_read.go`: `List`, `contents`, `topLevel`, `Stat`, `Open`.
    - `blobfs_write.go`: `Mkdir`, `Put`, and later `Copy`.
    - `blobfs_move.go`: `Move`, `destination`, and the scope helpers.
    - `blobfs_delete.go`: `Remove`, `deleteFile`, `RemoveDirectory`, `RemoveTree`, and its walker.
    - `database.go`: `Store`, `New`, `Verify`, and the options.
    - `database_bookmarks.go` and `database_owners.go`: the consumer's bookmark and owner
      statements, their sentinel maps, and their read models, with the bookmark and owner
      operations that use them.
    The split changes no signature and no test. `commands.go` (690 lines) may follow the same
    pattern, and that is optional because the tool is not promoted.

12. Error wrapper (layer 4). Decided: a library error that classifies a constraint violation
    gets its own error type. Its message prints the sentinel and the constraint name, and the
    original `sqlate.ConstraintError` stays reachable through `errors.As` and `errors.Is`.
13. Root named `/` (decision 13). Decided:
    - The root row's `name` is `/`, and the check constraint becomes
      `(parent_id IS NULL) = (name = '/')`. `ValidateName` already refuses `/` in a name, so no
      other directory can hold it, and `name <> ''` stays.
    - `blobfs.Directory.Name` becomes `string`. `ParentID` stays a pointer, because the root has
      no parent and a self-parent would break the upward walks and the `parent_not_self` check.
    - Amend `0001_directory.up.sql` in place and re-pin the golden hashes. Update `TestRootRule`,
      the cursor's nullability map, the nil checks in the consumer and the tool, `DirectoryPath`,
      and the bookmark path recursion so the root contributes nothing to a path.
14. Native engine strategy (decision 11). Decided:
    - The engine is selected by an engine package, not by a registry or a flag. The Postgres
      variant moves from `lib/blobfs/data/pgnative` to `lib/blobfs/postgres`, which exports the
      variant and its constructor. The composition root names the engine by importing the
      package, as it already imports `sqlate/postgres` for the dialect and as `go-database`'s
      provider constructor names the engine there. There is no `init`, no global state, and no
      flag. `go-database` puts its provider in a separate module because of the driver's weight,
      and the engine package needs no driver, so it stays in the library module (decision 7).
    - The standard baseline is what `data.New` builds when no engine package is used. The
      conformance suite runs both. The tool keeps `--variant standard` as the opt-out.
    - Add variation points along the write, delete, and move protocol, each shipped only when
      measured to win: `RETURNING` for the begin and complete write steps and for `Mkdir` (one
      round trip fewer each), path resolution in one statement, and a row-value keyset
      predicate. The delete begin and the tree lock already exist. A variant embeds the base
      variant and overrides what it needs, so the interface can grow without burdening a variant.
    - Record for `sqlate`: engine-keyed statement overlays for statements of the same shape, as
      pattern overlays already work, and a way to declare a statement that returns the changed
      row, so the standard and native shapes are declared once.

15. Remove the `created_at` index from the library's migrations (decision 5). Decided:
    - Delete migration 3 from the `blobfs` set, so the set is two migrations, and re-pin the golden
      hashes. Document the index and its cost (3,992 kB for 100,000 files) and the query it
      serves: a listing sorted by `created_at` without a total, 34 buffers against about 2,400.
    - Keep the upgrade rehearsal by giving the migrator's tests a fixture migration of their own
      as the third migration. `TestUpgradeAfterRestart` and the golden test that checks versions
      1 and 2 keep their hashes are rewritten over that fixture.
16. Engine package owns the DDL (layer 6). Decided: engines own their DDL. The Postgres DDL moves into `lib/blobfs/postgres` beside the variant. It is exported as a
    function that returns the whole migration set (name, history table, and migrations), and `lib/blobfs/migrations` with its `switch` on the dialect's name and its
    `ErrUnsupportedEngine` goes away. The amended `0001` for the root named `/` lands there.

17. Existing databases must be reset before the Phase 3 code runs. The history table records only
    `version`, `name`, `applied_at`, and `dirty`, so the migrator does not notice a migration
    edited in place. A database installed from an earlier commit keeps the old root, whose name is
    NULL, and the new `Directory.Name string` fails to scan it. Reset or drop `app`,
    `blobfs_review`, `blobfs_review2`, and `blobfs_tour` before running the new code. This applies
    only while nothing is released. A released migration is never amended, and a change to seeded
    data is a new migration.

18. The library's API surface for seeding files. Decided: the service provides the seed
    implementation, and the library provides the operations that facilitate it:
    - `EnsureDirectory(ctx, sess, parentID, name)` returns the directory and whether it was
      created, so a seeder does not catch `ErrNameTaken` and look the name up itself.
    - `BeginOrResumeFileWrite(ctx, sess, keys, directoryID, name, contentType)` returns the file
      and an outcome: created, resumed a `pending` row, or already present as `available` or
      `deleting`. The find-or-begin logic that the consumer's `Put` holds today moves into the
      library, so `put`, `cp`, and a seeder share one write protocol. The caller decides what
      the already-present outcome means: `put` maps it to `ErrNameTaken`, and a seeder skips it.
    - Optional caller-supplied ids on `Mkdir` and the begin step, so a seeded directory or file
      keeps the same id across resets. Object keys are `<id>/<name>`, so a re-seeded file reuses
      its key and overwrites the earlier object instead of orphaning it.

19. Cost regression assertions. Decided, where they add legitimate value: add buffer-bound
    assertions for the paths whose cost the evidence established, so a plan regression fails a
    test. The candidates are the cursor page, the exact-total page, path resolution, and the
    protocol steps. A bound is set with a wide margin against the measured value and asserts a
    plan shape or a buffer count, never milliseconds, and it runs on a fixture small enough for
    the integration tier. A candidate that would be flaky or would restate an existing test is
    left out.

## Requirements carried to later tasks

- `blobfs.sources` (the multi-set migrator in `sqlate`). Decided, from the layers decision:
  - A `migrate.Set` value that a library ships whole: name, history table, and numbered
    migrations, with the file naming `NNNN_name.up.sql` and `.down.sql` unchanged and the
    sequence private to its set.
  - Layer order as a stated rule: sets are declared bottom-first, `Up` runs ascending, and `Down`
    and `Reset` run descending.
  - Verbs that take a set name, defaulting to the top layer: `Down`, `Steps`, and `Force`.
  - An up-front refusal: `Down` of a set is refused while any set above it has applied
    migrations, so the run does not fail partway with SQLSTATE 2BP01. No explicit `Requires` list
    is added.
- `blobfs.sources`, from the layer 7 review. Decided: export
  the default table name with an accessor for a set's table (item 26); run a set's migrations on
  one connection (item 27, decision 8); add a `Migrator.Drop` that quotes the identifier (item 28);
  document `Options.Unlocked` and the `Steps` behavior (items 29 and 31); report head, dirty, and
  pending in one read (item 30); map SQLSTATE 2BP01 and 40001 to sentinels (items 14 and 15).
- `blobfs.admin`. The admin service's verbs take a set name, `Reset` and `Status` cover every set,
  and `Start` reports pending migrations by set. The service needs a `force` verb (Finding 3).
- `blobfs.admin` and `v1.storage.service`, the seed model. Decided:
  - The root directory is a structural seed in the engine's DDL and never a named state. `Up`
    creates it and every `Reset` re-creates it.
  - Named states are the consumer's. In v1 they seed directories only, through the library's
    `Mkdir` with insert-or-find on `ErrNameTaken`, and the owner rows go in the same transaction.
  - Files are seeded by the service's seeder through the two-phase write with a bytes source, an
    embedded fixture or inline text. The library provides the API surface that facilitates it
    (adjustment 18). The seeder is idempotent by name: an `available` file is skipped, a
    `pending` one resumes, and the object store must be started before the seed step.
  - `Reset` leaves orphaned objects in the store. Keys are `<UUIDv7 id>/<name>`, so orphans never
    collide with new rows, and they are accepted in development.
- `v1.storage.service`. A service composes the object store through `files.NewStorage(store)`
  (decision 7 of the review additions).

## Edits queued for the close

- Amend `context/concepts/blobfs.md` with the amendments listed in `REVIEW.md`, and add the
  rejected alternatives, the navigation principle, the cursor conventions, the path-by-kind rule,
  and the deferred list (directory copy, subtree search, content replacement, versioning).
- Amend `context/design/auth-strategy.md` section 8 for the directory grain.
- Rewrite the `blobfs.sources` summary in `context/roadmap.toml`, including the requirement that a
  multi-set run uses one connection (decision 8), and set the requirements for `blobfs.build`.
- Read `context/design/storage-strategy.md` once for the volume and ownership sentences.
- Record in the concept that a released migration is never amended, and that a change to seeded
  data, such as the root row, is a new migration.
- Publishing the branch with `gh pr create` is the architect's decision at the close.

## Open questions

None.
