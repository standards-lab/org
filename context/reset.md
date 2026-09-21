# reset · blobfs-experiment

- **Status:** handoff
- **Session:** experiment
- **Project:** standards-lab
- **Branch:** blobfs-experiment

## Disposition

- **Integrated:** nothing decayed.
- **Promoted:** nothing. Every result of the experiment and of the review is evidence or a recorded
  decision under `experiments/blobfs/`. The concept and design-note edits wait for the closeout
  and are listed in `experiments/blobfs/DECISIONS.md`, section "Edits queued for the close".
- **Culled:** nothing in `context/` changed this session.
- **Retained:** `concepts/blobfs.md` as written, provisional. The amendments in `REVIEW.md`,
  section "Amendments to make at close", and the additions in `DECISIONS.md` are made at the
  closeout, after the architect's final review.
- **Roadmap:** no manifest edit. `blobfs.experiment` stays the task in flight, and the roadmap does
  not advance on a handoff. The rewrite of the `blobfs.sources` summary and the requirements for
  `blobfs.build` wait for the closeout. `DECISIONS.md`, section "Requirements carried to later
  tasks", holds them.
- **Architecture layer:** nothing promoted on a handoff. A candidate for the closeout: the review's
  principles may have generalized past this repository, namely navigation one directory at a
  time, engine packages that own their DDL and native variants, migration sets as layers, and ids
  as the primary handle. Promotion waits for the closeout.
- **Cross-repo:** none.

## Next-focus

Task `blobfs.experiment`, an `experiment` session in `standards-lab`, on branch
`blobfs-experiment` (open, unpublished), in `standards-lab/experiments/blobfs`.

**Phase 2 is complete and Phase 3 has not started.** The architect reviewed the eight layers of the
built experiment, saw the live demonstration and the guided tour, and answered all fifteen
questions of `REVIEW.md`. Every decision, adjustment, and edit is recorded in
`experiments/blobfs/DECISIONS.md`. Read it in full first. It carries no provisional entry and no
open question. `GUIDE.md` (the tour of the built experiment) and `evidence/schema-alternatives/`
(the four-design measurements behind the schema decision) are also new files in the tree. The
original sixteen stages are done and committed.

**Stages: 16 of 32 done. Phase 3 stages 17 to 32 are approved, and none has started.** The list, in
dependency order, lowest first. Each entry names the adjustment of `DECISIONS.md` it implements.

17. Decompose `domain/files`: split `blobfs.go` and `database.go` by concern, role as prefix
    (adjustment 11). No behavior change.
18. Schema amendments: name the root `/`, make `Directory.Name` a `string`, and remove the
    `created_at` index migration, with a fixture third migration in the migrator tests and the
    golden hashes re-pinned (adjustments 13 and 15). Existing databases must be reset first
    (adjustment 17).
19. Engine package: create `lib/blobfs/postgres` from `pgnative` and the Postgres DDL, exporting the
    whole migration set, and remove `lib/blobfs/migrations` (adjustments 14 and 16). Update the
    consumer, `split-check`, and the `mise` tasks.
20. Constraint error wrapper: the message prints the sentinel and the constraint name, and the
    driver error stays reachable through `errors.As` (adjustment 12).
21. `Page.More`: one extra row on every page, the projections derive it from their count, and the
    tool prints a `more:` line (decision 5 of the review additions).
22. Seeding operations in the library: `EnsureDirectory`, `BeginOrResumeFileWrite`, and optional
    caller-supplied ids (adjustment 18).
23. The bookmark race: `HoldFile` and `rm` beginning before it counts, with tests of both
    interleavings (adjustment 10).
24. Relative path resolution from a directory id (adjustment 9).
25. Id-keyed consumer operations with path wrappers, the scope check by id through `IsWithin`, and a
    bookmark read model that returns ids and computes the path on request (adjustment 9).
26. File `cp` (adjustment 2).
27. Tool: an id column in `ls`, `stat` of directories, the `id:<uuid>` form, `--cursors`, and
    `--filter` (adjustments 3, 4, and 9).
28. Native variation points, measurement: `RETURNING` for the begin and complete write steps and for
    `Mkdir`, path resolution in one statement, and a row-value keyset predicate, measured against
    the standard tier with the round-trip method of `evidence/schema-alternatives/40_protocol.sh`
    (adjustment 14). Report the numbers before any implementation.
29. Native variation points, implementation: only the measured winners, in `lib/blobfs/postgres`,
    each proven by the conformance suite on both variants (adjustment 14).
30. Cost regression assertions where they add legitimate value (adjustment 19).
31. Evidence: regenerate the transcripts with `mise run evidence` (adjustment 5).
32. Documentation and cleanup: the library docs and `README.md` conventions, the `GUIDE.md` update,
    the `README.md` evidence table, the migrator and `NewStorage` notes, and the removal of the
    leftover Azurite containers (adjustments 7 and 8).

**After stage 32.** Validate the whole step: the whole-module build, the full test run, the
integration tier, and `mise run demo`. **The updated `GUIDE.md` drives the architect's final
review.** It must cover every capability, including those Phase 3 adds, each with a summary, the
key files, and the commands to run. Then set the requirements for `blobfs.build` with the
architect, and then run `close`: the edits queued in `DECISIONS.md`, the roadmap, and the publish
with `gh pr create`, which is the architect's decision at that point.

### The exact next move

Resume with `/marathon:marathon experiment`. LOCATE routes to `Status: handoff`, and 3R checks out
`blobfs-experiment` and reads this file. Do not re-enter SETTLE. The stage list is approved.

1. Read `DECISIONS.md` in full, then `GUIDE.md` and `README.md`, then look at the tree.
2. Start stage 17. Before it, state the delegation call out loud: implementation goes to `fable`
   under the model-routing convention, one stage at a time, briefed with full context. The
   orchestrator finishes the documentation, code comments, and commit messages.
3. Run the stage's check: `mise run build`, `vet`, `test`, `lint`, and `split-check`, and
   `mise run integration` for any stage that touches the database. Read what `fable` produced
   firsthand, the diff and the check output, and then report with `diff --stat`, the check result,
   the delegation call, and only the decisions the plan did not spell out. Leave the working tree
   uncommitted until the architect approves. Commit on approval, and the architect states whether a
   `reset` follows.
4. Move to the next stage only after the commit. A finding that reaches beyond one stage is a
   re-plan of the remaining stages.

### Cautions

- Baseline at this handoff: the build, `go vet`, `split-check`, `golangci-lint` (0 issues),
  `sqlint`, all hermetic tests, and the integration tier (about 50 seconds) pass. A stage that
  breaks one of them is not done.
- Run the binary only against a throwaway database, and give the object store its own container
  through `BLOBFS_STORAGE_CONTAINER`. `mise.toml` sets `BLOBFS_DSN` to the compose stack's default
  `app` database, and mise's value overrides a variable set on the command line, so `mise run cli`
  changes `app`. `GUIDE.md`, section "Setup", shows the safe wrapper.
- Never run `mise run down`, `mise run reset`, or `docker compose down`. Run `mise run evidence`
  only in stage 31, because it rewrites the committed transcripts that `NOTES.md` and `REVIEW.md`
  cite.
- Stage 18 changes migrations in place, and the migrator does not detect that. The databases
  `blobfs_review`, `blobfs_review2`, and `blobfs_tour` are throwaway from the review and can be
  dropped once the architect says so. The default `app` database holds the full schema at head from
  an earlier mistake, and nothing depends on it. Resetting it is the architect's to run or to
  authorize, for example with `schema reset --yes` against `app`. Never do it unasked.
- The compose stack (`blobfs-postgres` on port 5434, `blobfs-azurite` on port 10000) is running. If
  it is not, `mise run up` starts it.
- The internal `livetest` package deletes Azurite containers, and the CLI cannot. The empty
  containers `gcheck` and `gcheck2` remain from a guide check, and the architect's own `blobtour`
  containers may exist.
- The earlier notes called the file `cp` stage 17. In this list it is stage 26.
- Documentation, code comments, commit messages, and context notes are the orchestrator's, in the
  voice standard, whoever drafted them.

### The design after the review

`blobfs` keeps a virtual directory tree and file metadata in SQL for an object store, and it never
calls the object store. The schema is two tables, `blobfs_directory` and `blobfs_file`, with
separate name spaces. One seeded root row, named `/` and given the nil UUID `blobfs.RootID`, anchors
every path. The API is id-first and navigates one directory at a time, with filters, sort, and
pagination over that directory's children, offset or keyset, and a `Page` that says both whether
rows remain and whether a cursor can continue. A file write, delete, and copy are two-phase, with a
`pending` or `deleting` row that any retry converges from. Engines are packages: `lib/blobfs/postgres`
owns its DDL as a whole migration set, its native variant, and its port notes, and the standard tier
is the baseline and the fallback. Migration sets are layers with their own histories, run bottom-up
and reverted top-down. The service, not the library, seeds named states, and the library supplies
the idempotent operations that make seeding files safe.
