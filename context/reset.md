# reset · blobfs-experiment

- **Status:** handoff
- **Session:** experiment
- **Project:** standards-lab
- **Branch:** blobfs-experiment

## Disposition

- **Integrated:** nothing decayed.
- **Promoted:** nothing. Every result of the experiment is evidence in `experiments/blobfs/`, and
  the concept and design notes change only at the review that follows.
- **Culled:** nothing in `context/` changed this session.
- **Retained:** `concepts/blobfs.md` as written, provisional. The experiment showed that several
  of its paragraphs are wrong or changed. `experiments/blobfs/REVIEW.md`, section "Amendments to
  make at close", lists each edit as file, change, and reason. The edits wait for the architect's
  review and are not made yet.
- **Roadmap:** no manifest edit. `blobfs.experiment` stays the task in flight, and the roadmap does
  not advance on a handoff. The `blobfs.sources` summary needs a rewrite from `REVIEW.md`, Finding
  2, items 26 to 32, at close.
- **Cross-repo:** none.

## Next-focus

Task `blobfs.experiment`, an `experiment` session in `standards-lab`, on branch
`blobfs-experiment` (open, unpublished), in `standards-lab/experiments/blobfs`.

**Stages: 16 of 16 done, all committed.** The experiment is built, verified, and recorded. The
working tree is clean. The last experiment commit is `96ca8e2` ("Record the experiment's findings
in REVIEW.md"), and this handoff is the commit after it. Nothing is closed, pushed, or published.

The experiment's organized record is `experiments/blobfs/REVIEW.md`: the three findings (the
library, the `sqlate` adjustments, the `v1.storage` incorporation), the proofs table, the
amendments to make at close, fifteen decisions for the architect with a recommendation each, what
the experiment did not prove, and the commands that reproduce every result.
`experiments/blobfs/NOTES.md` is the chronological log, and `experiments/blobfs/evidence/` holds
the three cost transcripts.

### The exact next move

Resume with `/marathon:marathon experiment`. LOCATE routes to `Status: handoff`, and 3R checks out
`blobfs-experiment` and reads this file. Do not re-enter SETTLE and do not run any stage: all
sixteen are done. Then stop and wait. Do not start the review on your own. The architect starts it
by saying "start", and the protocol is "The post-execution review" below.

Before the review, read `experiments/blobfs/REVIEW.md` (the whole file) and
`experiments/blobfs/README.md`, then look at the tree. The review explains code, so read each
layer's files firsthand before presenting them.

### Cautions for the review

- Run the binary only against a throwaway database. `mise.toml` sets `BLOBFS_DSN` to the compose
  stack's default `app` database, and mise's value overrides a variable set on the command line, so
  `mise run cli -- schema up` changes `app`. Create a database (for example `CREATE DATABASE
  blobfs_review` through `docker exec blobfs-postgres psql -U app -d app`) and pass its DSN with
  `--dsn`. Give the object store its own container through `BLOBFS_STORAGE_CONTAINER`. Drop both at
  the end of the review, and never `mise run down`, `mise run reset`, or `docker compose down`.
- The default `app` database already holds the full schema, at head, with only the seeded root
  row. A stage 11 subagent applied it there by mistake. Nothing depends on it. The architect can
  restore it with `mise exec -- go run ./cmd/blobfs schema down` from `experiments/blobfs`, which
  is the architect's to run or to authorize. Do not run it unasked.
- `mise run evidence` rewrites the three committed transcripts under `evidence/`, and the figures
  in `NOTES.md` and `REVIEW.md` cite them. Do not run it during the review.
- The Docker stack (`blobfs-postgres` on port 5434, `blobfs-azurite` on port 10000) is running.
  If it is not, `mise run up` starts it.
- `mise run demo` runs the scripted end-to-end run against the built binary once per variant, in
  throwaway databases and containers, printing every command and its output. It is a safe way to
  show the transcript, but the live demonstration below runs the binary by hand.

### The post-execution review

When the architect says "start", run the review in two phases. Change no code during it unless the
architect asks for an adjustment, and keep every explanation short: the architect wants to
understand the mechanics without being overwhelmed. Route any adjustment that is technically
complex to `fable` under the standing model-routing convention, state the delegation call first,
and read what it produced firsthand before saying anything about it.

**Phase 1: orientation.** Explain how the API is structured and how the experiment runs, then
demonstrate everything that was built by running the built binary live against the compose stack,
in a throwaway database, one step at a time with a line of narration and the real output shown.
Do not point at integration tests instead. Cover, in an order that builds up: bringing the stack
up and applying the schema (`schema up`, `status`, and a `reset --yes`); `mkdir` and `ls` with
paging, sorting, `--total none`, and the `--after-dirs` and `--after-files` cursors; `--unit`
scoping at a depth-one directory and at `/`; the write path (`put`, `cat`, `stat`) including
`--fail-after` and how a `pending` row is found and completed by a retry; `mv` (a file, a directory,
the cycle refusal, the root refusal, and the scope refusal); `rm` with `--fail-after begin|object`
and a converging retry, `rmdir` (including the root refusal), and `rm -r`; `bookmark add|ls|rm` and
the one-active rule; the same script on the baseline and on the `pgnative` variant (`--variant
standard` and `--variant pgnative`; the tree-lock step is where the two differ from outside); a
second configuration to show isolation (a second database and container); and the migration set
(`schema status` shows both sets and their versions). The upgrade rehearsal cannot be shown from
the binary, because `schema up` applies the whole three-migration set, so point at
`TestUpgradeAfterRestart` in `lib/migrator/migrator_integration_test.go` and run that one test.
Add a short
map of the layers before the demonstration. Leave the stack running and the database available so
the architect can try commands too.

**Phase 2: layer-by-layer review, top down.** Choose the layer segregation yourself and say what
you chose. A sensible default, from the top: (1) the entry point and composition root
(`cmd/blobfs`, `internal/app`); (2) the command surface and output (`domain/files/commands.go`,
`admin/schema`, `output`); (3) the consumer domain (`domain/files`: entities, statements,
`database.go`, the `blobfs.go` and `storage.go` translation files); (4) the persistence layer
(`lib/blobfs/data`: statements, patterns, the listing composer, the cursor); (5) the variant seam
(`lib/blobfs/data/variant.go`, `lib/blobfs/data/pgnative`, `lib/blobfs/data/datatest`); (6) the
root package and schema (`lib/blobfs`, `lib/blobfs/migrations`); (7) the migrator
(`lib/migrator`); (8) the tests, integration tier, and evidence (`integration`, `evidence`). For
each layer give the architect, and nothing more: at most five files to read, in reading order,
with the line that says what each one is; the three to five things to understand about the layer,
including the decisions and the tradeoffs behind them; and the findings for that layer from
`REVIEW.md` that affect it. Then stop and wait for the architect to finish with that layer, take
their questions and requested adjustments, and only then move to the next one. The aim is to
confirm that the infrastructure is well formed and performs as intended, so name for each layer
what you believe is well formed, what you are least sure of, and what the evidence says about
cost.

After the last layer, collect the adjustments the architect asked for, apply them, and then plan
with the architect the amendments to the concept and the design notes (the list is in `REVIEW.md`),
the answers to the fifteen decisions, and the `close`. The `close` publishes the branch with `gh pr
create`, which is the architect's decision at that point.

### The design in one paragraph

`blobfs` provides only the container-based directory and file infrastructure. A configuration
points at one container, which is the root of the tree. A consumer that wants several isolated
trees runs several configurations, each with its own container and database. The schema is
`blobfs_directory` (one seeded root row with the well-known id `blobfs.RootID`, the nil UUID, no
name, one root enforced by the partial unique index `blobfs_uq_directory_root`) and `blobfs_file`.
The listing is a statement anchored on one directory with the total computed in the same statement
(`COUNT(*) OVER ()`), composed in Go over authored statements, with offset paging and a keyset
cursor, and it never walks the whole forest. The path stays a read-time computation. Native SQL is
confined to two variation points behind the `data.Variant` interface, the tree lock and the
file-delete begin, and the standard-tier baseline is complete on any engine. The consumer keeps
`directory_owner(directory_id, unit_id)` and `bookmark(unit_id, file_id, active)` in its own
migration set.
