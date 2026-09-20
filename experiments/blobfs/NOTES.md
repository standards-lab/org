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

Stages 1 to 8 are committed: the single-root schema, the
consumer's `directory_owner` and `bookmark` tables, the persistence package with the listing
composer and its keyset cursor, `domain/files`, and the listing evidence. The binary serves
`schema up|down`, `mkdir <path> [--unit]`, and `ls <path>` with `--page`, `--size`, `--sort`,
`--total none`, `--after-dirs`, `--after-files`, and `--unit`. `mise run evidence` writes
`evidence/read-model.txt`, the cost of the shipped listing. The write path, the file commands,
and the bookmark commands are later stages.

## Running it

`mise run up` starts Postgres on port 5434 and Azurite on port 10000. `mise run test` runs the
hermetic tests, and `mise run integration` runs the tests that need the services. `mise run lint`
runs `golangci-lint` and `sqlint`, and `mise run split-check` enforces the import boundaries.
`mise run evidence` regenerates the read-model measurement. `README.md` has the layout.

## Decisions log

Newest first.

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
  0.07 ms and 21 buffers flat (an index scan in name order under a `Limit`) and in 5.2 ms and
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
  statement makes the engine read every row of the directory with its columns: 5.5 ms and 2434
  buffers for 10,008 files, against 0.02 ms and 12 buffers without the total and 0.77 ms and
  109 buffers for an index-only count twin after `VACUUM`. The total in the page statement
  agrees with its page, and it costs a heap read that a separate count avoids once the table is
  vacuumed. A library that offers both should say so.

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
| 0 | Count twin before `VACUUM` | | 1.98 ms, 2434 buffers, bitmap heap scan |
| 0 | Count twin after `VACUUM` | | 0.77 ms, 109 buffers, index-only scan |
| 0 | Shipped exact-total page, before and after `VACUUM` | | 5.9 and 5.7 ms, 2434 buffers both |
| a | Whole-forest baseline, count and page | 11.2 and 11.1 ms, 29,700 buffers | 13.5 and 18.0 ms, 29,800 and 32,100 buffers |
| b | Shipped listing, exact total, page 1 | 0.12 ms, 13 buffers | 5.5 ms, 2434 buffers |
| c | Shipped listing, no total, page 1 | 0.04 ms, 13 buffers | 0.02 ms, 12 buffers |
| d | Last page by offset, no total | | 7.9 ms, 2434 buffers |
| d | Last page by offset, exact total | | 11.2 ms, 2434 buffers |
| d | Last page by cursor | | 0.02 ms, 6 buffers |
| e1 | Wrap, unanchored base, count twin and page | | 0.76 ms, 109 buffers; 0.05 ms, 12 buffers |
| e2 | Wrap, anchored base; the same with the window count inside | | 0.04 ms, 12 buffers; 5.7 ms, 2434 buffers |
| e3 | Recursive base, flat and wrapped | | 0.07 ms, 21 buffers; 5.2 ms, 2443 buffers |

What the measurement shows, and the answers to the V3 questions as far as it supports them:

- **The whole-forest baseline costs far more than the shipped listing.** It reads about 30,000 buffers for any
  directory, because the recursion walks every directory and the join touches every file; the
  shipped listing reads 12 or 13 buffers for a page without a total. The v1 numbers (815 buffers
  for the same shape) were measured on the same fixture size but before `VACUUM` and with a
  different plan; the new fixture's plan joins through `blobfs_uq_directory_parent_name` and
  the file index, and the two are not comparable beyond their order of magnitude.
- **Does the exact total stay the default?** Yes for the default page size and ordinary
  directories: for the small directory the total is free (13 buffers either way). For a big
  directory the window count costs the whole directory's heap read (2434 buffers, 5.5 ms for
  10,008 files) on every page, because the page statement must read every row's columns to
  count them, and `VACUUM` does not help it. The default holds because a consumer that lists a
  directory expects its size, and the cost is bounded by the directory, never by the tree.
- **Is a capped total needed?** The measurement does not decide it. A capped total (count at
  most N and report "more than N") would bound the heap read a big directory pays; `TotalNone`
  already bounds it to zero, and the cursor pages the directory without it. The experiment
  shipped no cap, and the numbers say a consumer with directories of tens of thousands of files
  should ask for the total once, on page one, and walk by cursor.
- **Does the cursor earn its place?** Yes. The last page of the biggest directory costs 0.02 ms
  and 6 buffers by cursor (an index scan from the cursor's name under a `Limit`) against 7.9 ms
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
  whole directory above the join (5.2 ms, 2443 buffers) where the flat form pages through the
  index (0.07 ms, 21 buffers). So the composer's choice to append the clauses at the statement's
  own level is required for the recursive shape the consumer-anchored read models take (the
  bookmark projection of stage 11, and any per-row path), and the wrap suffices for the plain
  listings. A window count inside a wrapped base also blocks the pull-up, but the plan is then
  the flat exact statement's anyway.
- **The `VACUUM` caveat, settled.** The v1 caveat that the count "reads the heap through a
  bitmap scan because the fixture was never vacuumed" applies to a separate count statement: the
  count twin goes from 2434 buffers to an index-only 109 after `VACUUM`. The shipped window total
  is unaffected, because it travels in the page statement, which reads the rows.

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
proofs 2 to 7 and the variant seam, and stage 11 adds the bookmark read model's cost to the
evidence.
