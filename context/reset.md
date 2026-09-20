# reset · blobfs-experiment

- **Status:** handoff
- **Session:** experiment
- **Project:** standards-lab
- **Branch:** blobfs-experiment

## Disposition

- **Integrated:** nothing decayed.
- **Promoted:** nothing. Every result of the experiment is evidence in `experiments/blobfs/`, and
  the concept and design notes change only at the review after the last stage.
- **Culled:** nothing in `context/` changed this session.
- **Retained:** `concepts/blobfs.md` as written, provisional. The experiment has already shown that
  its `volume` and root-anchor paragraphs, its ownership paragraph, its assumption that a path
  query "walks from the roots", and its claim that a second engine adds only a directory are
  wrong or changed. The amendments are listed in `experiments/blobfs/NOTES.md` and the plan, and
  wait for the architect's review.
- **Roadmap:** no manifest edit. `blobfs.experiment` stays the task in flight, and the roadmap does
  not advance on a handoff.
- **Cross-repo:** none.

## Next-focus

Task `blobfs.experiment`, an `experiment` session in `standards-lab`, on branch `blobfs-experiment`
(open, unpublished), in `standards-lab/experiments/blobfs`. Resume with `/marathon:marathon
experiment` in auto mode.

**Stages: 5 of 16 done, next is stage 6.** Stages 1 to 5a are committed as their own commits.
Stage 5b is committed as a work-in-progress commit (`d2ec826`) because the architect then removed
`volume` from `blobfs`; stage 6 unwinds it. `ff52d58` seeds `experiments/blobfs/NOTES.md`, the
findings ledger and decisions log that every stage appends to. The tree is clean.

### The standing instruction

The architect has waived per-stage review for stages 6 to 16 and will review the whole experiment
once stage 16 is committed. Do not re-enter SETTLE. The plan is settled, and 3R resumes it.

1. For each stage in order, state the delegation call (every stage goes to `fable`), brief `fable`
   with full context (the stage row below, the decisions below, the previous stage's report, and
   the hard rules), and read what `fable` produced firsthand before anything else.
2. Rerun the stage's gates yourself instead of trusting the report. If a gate fails, send the
   failure back to `fable` and repeat, up to three rounds. If a stage is still red, or a finding
   invalidates a settled decision, stop, write a handoff reset naming the blocker, and wait for
   the architect. Otherwise never pause for review.
3. On green gates, commit the stage with `git add .` and a message that says what it adds and the
   decisions it made. Then start the next stage.
4. If context fills before stage 16, run `reset` (a handoff with the stage position and the exact
   next move) and continue in the fresh context.
5. Stop after stage 16 is committed. Do not run `close`, do not push, and do not publish. Report
   what was built, the answer to each proof, and the findings grouped by the three review goals
   (the library, `sqlate`, `v1.storage`), and say anything that changed a settled decision.

The common gates, run from `experiments/blobfs`: `go build ./...`, `go vet ./...`, `go vet -tags
integration ./...`, `go test -race ./...`, `mise run integration`, `mise run split-check`, `mise run
lint` (0 issues and `sqlint: ok`), `gofmt -l .` empty, and `go mod tidy` leaving `go.mod` and
`go.sum` unchanged. The Docker stack (`mise run up`) must be running. Every engine-backed test uses
its own throwaway database. No `blobfs_*` database may remain afterward. Never run `mise run
down`, `mise run reset`, or `docker compose down`.

`fable` never commits, never pushes, and never writes the reset file. The orchestrator finishes
documentation, comments, and commit messages in its own voice, and stage 16 follows that: `fable`
drafts the evidence-heavy sections, and the orchestrator finishes and reads them firsthand.

### The design in one paragraph

`blobfs` provides only the container-based directory and file infrastructure. A configuration
points at one container, which is the root of the tree. A consumer that wants several isolated
trees runs several configurations, each with its own container and database. `volume` is not in the
library. The schema is `blobfs_directory` (one seeded root row with the well-known id
`blobfs.RootID`, the nil UUID, no name, one root enforced by a partial unique index
`blobfs_uq_directory_root`) and `blobfs_file`. The listing is a statement anchored on one directory
with the total computed in the same statement (`COUNT(*) OVER ()`), composed in Go over authored
statements, with offset paging and a keyset cursor, and it never walks the whole forest. The path
stays a read-time computation (`DirectoryPath` walks upward), and no `volume_id` or stored path is
added. The consumer keeps `directory_owner(directory_id, unit_id)` (scope checked once at the
depth-one ancestor) and `bookmark(unit_id, file_id, active)` (partial unique index, one active
bookmark per unit).

The full design, the decisions, and the ledger are in the approved plan at
`/home/jaime/.claude/plans/experiment-structured-boole.md` (read it if it exists) and in
`experiments/blobfs/NOTES.md` (always present). If the two disagree, this file wins.

### Settled decisions that hold for every stage

- One install per database, with fixed `blobfs_` object names. Ids are minted in Go
  (`uuid.NewV7()` from the standard library) and carried as `string`.
- Native-tier statements are allowed as variants behind a narrow Go interface. The standard-tier
  baseline is complete on any engine. `lib/blobfs/data/pgnative` is the Postgres variant. The
  variation points are the tree lock and the file-delete begin, and a third needs a finding.
  Published patterns stay standard tier, and every native file carries its port note.
- The documentation-only variant is estimated, not built. The migrator shim is a drop-in for the
  multi-set API `blobfs.sources` describes, and it records every hook it needs from `sqlate`.
- The consumer follows the `slab` elemental layout: `cmd/blobfs` is process entry only,
  `internal/app` is the composition root, and `domain/files`, `admin/schema`, `migrations`, and
  `output` sit at module root level. Cobra is adopted for the consumer only. Constructors that can
  fail return `(*T, error)`.
- The `lib/` packages are promotion candidates. `split-check` enforces the import boundaries, and
  a new rule is proved by temporarily violating it.
- Standard-tier SQL may not use `RETURNING`, `::`, `now()`, `timestamptz`, `LIMIT`, `ILIKE`,
  `ON CONFLICT`, or `uuidv7()`. Use `CURRENT_TIMESTAMP`, `{{name:type}}` casts, and `OFFSET ...
  FETCH NEXT`. Name statement files by intent, never `insert_`, `select_`, `update_`, `upsert_`, or
  `merge_`. `{{` is reserved in every SQL body. Timestamp `--| field:` types are spelled
  `timestamp with time zone`.
- Doc comments: a multi-file package keeps its package comment in `doc.go`, every exported
  identifier carries godoc that opens with its name, and short interface-satisfying methods carry
  none. Voice: full sentences with real verbs, ordinary terms, no metaphors for mechanisms, no
  em-dash cadence, no "not X but Y" frame.
- Migration text is amended in place while unreleased, and the golden hashes are re-pinned with
  it. Constraint names follow `blobfs_<kind>_<table>_<detail>`.
- Every stage appends its findings and decisions to `experiments/blobfs/NOTES.md`.

### Stage list

| # | Stage | Unit | Gate beyond the common gates |
|---|-------|------|------------------------------|
| 1 to 5a | done | committed | |
| 5b | done | WIP commit, superseded | |
| 6 | Single-root schema and persistence | Delete the volume migration; amend the directory migration (nullable `name`, `blobfs_cc_directory_root_name`, partial unique index `blobfs_uq_directory_root`, seed the root); keep the file migration; consumer migrations become `directory_owner` and `bookmark`; `lib/blobfs` loses `Volume` and `VolumeID`, gains `RootID` and the root-constraint constant; `lib/blobfs/data` loses volume operations, the `tree` pattern, and `ListIn`, and gains `Root`, `Directory`, `Mkdir`, `ResolveDirectory`, `DirectoryPath`, and the listing composer (`Listing`, `Page[T]`, `TotalMode`, exact totals, offset paging, `Verify`); golden hashes; tests | A second root is refused; exact totals hold while another connection inserts between calls; a hermetic test proves one statement carries `COUNT(*) OVER ()`; the listing returns the same rows as a whole-forest baseline on a fixture |
| 7 | The consumer over one root | Rename `domain/volume` to `domain/files`; delete address parsing and the volume commands; `mkdir` and `ls` (`--page`, `--size`, `--sort`, `--total none`, `--unit`), the `directory_owner` entity and statements, the owner projection for `ls /`, `internal/app/domain.go`, `output` rows, the `integration/` script, `split-check` renames, `README.md` | `mkdir` and `ls` through the built binary; `--unit` scopes correctly; one `ls` runs in one read-only repeatable-read transaction |
| 8 | Keyset cursor and the listing evidence | The cursor in the composer, `--after`, and `evidence/read-model.txt` regenerated by `mise run evidence` with `VACUUM ANALYZE` after seeding | Offset and cursor walks return the same rows; the transcript holds the whole-forest baseline, the shipped listing exact and with no total, the last page by offset and by cursor, and the derived-table wrap of the same base |
| 9 | Variant seam | The variant interface (tree lock, file-delete begin), the standard baseline, `lib/blobfs/data/pgnative`, a conformance suite parametrized by variant | The suite passes on both variants; a consumer-supplied method swaps without a fork |
| 10 | Write path and key validation | Insert pending, complete, fail; `domain/files/storage.go` (the `*storage.Store` adapter and key validator, `Store.Start`); `put`, `cat`, `stat`, `--fail-after` | Over Postgres and Azurite: the write composes into the consumer's transaction, a stop between steps leaves a queryable `pending` row, a retry completes it, and a filename at the rune boundary is accepted |
| 11 | Bookmarks: the consumer-anchored read model | `bookmark add\|ls\|rm`, the bookmark projection with a per-row path from a recursion anchored on the bookmarked files' directories, the one-active partial index | One active bookmark per unit is enforced with a classifiable error; the projection's total agrees with its page; the cost is recorded in the evidence |
| 12 | Deleting | File delete begin and complete through the variant, `rm`, `rmdir` (refuses a non-empty directory and refuses the root with `ErrRootDirectory`), and `rm -r` in the consumer | Per variant: convergence under retry at each step; a recursive delete racing an insert; a bookmark meeting a file delete yields a classifiable error |
| 13 | Moving | The cycle check, the move through the variant's lock, `mv` | Per variant: two opposing concurrent moves leave no cycle and one is refused; the baseline without a lock forms a cycle; moving the root is refused |
| 14 | Multi-source migrator and upgrade rehearsal | `lib/migrator` in full (one outer lock on its own pinned connection with `Unlocked` on every inner migrator, `Status`, `Reset`, dirty refusal); a rehearsal blobfs migration adding an index on `blobfs_file (directory_id, created_at)`; `admin/schema status\|up\|down\|reset` | Fresh replay; upgrade after restart; reset order across the bookmark and owner foreign keys; two concurrent starters serialized |
| 15 | Integration tier | `integration/`: the scripted run against the built binary once per variant, and a run against two configurations that shows two databases and containers are isolated | The suite passes against the compose stack |
| 16 | The record | `README.md`, `NOTES.md`, and `REVIEW.md` organized by the three findings (the library, the `sqlate` adjustments, the `v1.storage` incorporation) | Every proof has an answer with evidence; the `sqlate` adjustment list and the `v1.storage` notes are complete |

### The exact next move

Read `experiments/blobfs/NOTES.md` and the tree, then state the delegation call for stage 6 and
brief `fable`. Stage 6's diff will delete much of what stages 3, 5a, and 5b added, so tell `fable`
to keep every test that still applies (rewriting it for the single root) and to delete only what
the volume removal makes meaningless. Then verify, commit, and continue to stage 7.
