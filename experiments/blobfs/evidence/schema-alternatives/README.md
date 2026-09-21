# Schema alternatives evaluation

Measurements of four candidate schema designs for `blobfs`, made during the post-execution review
of the experiment. The architect kept design A, the two-table schema the experiment built.
`DECISIONS.md` records the decision and its reasons. This directory holds the evidence.

## The designs

- Design A is the built schema: `blobfs_directory` and `blobfs_file`, with separate name spaces.
- Design B is one `blobfs_entry` table with a `kind` column, the file columns nullable on
  directory rows, and one `UNIQUE (parent_id, name)`. `10_ddl_b.sql` is the first form. The tuned
  form, with a partial index over directory rows and a generated `parent_kind` column for a typed
  parent key, is `70_recommended.sql`.
- Design C is a `blobfs_node` table with `blobfs_directory` and `blobfs_file` subtype tables,
  keyed by the node's id, with a composite key over `(id, kind)` to type each subtype.
- Design C2 is design C without the directory subtype table.
- Design D is design A plus one `UNION ALL` statement that lists a directory's directories and
  files in one page. It needs no schema of its own, so it has no DDL file.

## Method

Every design ran in its own database on Postgres 18.4 in the compose stack, loaded with identical
rows (an md5 over the whole file set matched): 10,003 directories in three trees to depth six,
100,000 files, a biggest directory of 10,009 files and 6 subdirectories, a small directory of 9
files at depth six, 1,000 files `pending`, 500 `deleting`, and 50,000 bookmark rows.

Each query ran once to warm the cache and then five times under `EXPLAIN (ANALYZE, BUFFERS)`. The
transcripts report the run with the median execution time. A buffer count is the top plan node's
shared hit plus read, in 8 KB pages. Buffer counts and plan shapes carry across machines, and
milliseconds do not. Every table was `VACUUM ANALYZE`d after loading.

## Files

| File | Contents |
|------|----------|
| `00_gen_fixture.sql` | The fixture generator, run once and copied to each design. |
| `10_ddl_a.sql`, `10_ddl_b.sql`, `10_ddl_c.sql`, `10_ddl_c2.sql` | The DDL of designs A, B, C, and C2. |
| `70_recommended.sql` | The tuned form of design B that the evaluation recommended. The architect chose design A. |
| `20_load.sh` | Creates one database per design and loads the fixture. |
| `30_measure.py`, `31_measure2.py` | The listing, cursor, sort, filter, path, and read-model measurements. |
| `40_protocol.sh` | The round-trip and latency runs of a whole file lifecycle and directory lifecycle, with pgbench. |
| `50_races.py` | Two-session interleavings of a move against a delete begin and against a write complete. |
| `60_integrity.sh` | Thirteen probes of what each schema refuses declaratively. |
| `listing.txt`, `listing2.txt` | The listing measurements. The summary lines come first, and each plan follows. |
| `storage.txt` | Table and index sizes after `REINDEX` and `VACUUM ANALYZE`. |
| `protocol.txt` | Round trips and latency per lifecycle. |
| `races.txt` | The interleaving results. |
| `integrity.txt`, `sweep.txt` | The declarative-enforcement probes and the sweeper's read. |
| `recommended-check.txt` | The tuned design B exercised through a whole directory and file life. |

## Results

The buffer counts below are for the 10,009-file directory, in pages.

| Measure | A | B, tuned | C | D |
|---------|---|----------|---|---|
| Files, page 1 by name, no total | 12 | 16 | 95 | 12 |
| Files, page 1 by name, exact total | 2,400 | 2,555 | 3,052 | 2,400 |
| Interleaved page 1, no total | not expressible | 14 | 91 | 2,404 |
| Interleaved page 2 by cursor | not expressible | 11 | 93 | 2,383 |
| Round trips, whole file lifecycle | 9 | 9 | 19 | 9 |

## Reading the results

- Design A won every measured read. Its cost is that a path can name a directory and a file at
  once (`integrity.txt`, case 2).
- Design D reads the whole directory for an early page, because the union cannot merge two
  ordered index scans. A cursor near the end of the directory costs 13 buffers in `listing.txt`
  (`D12`), so its cost depends on where in the directory the cursor sits.
- Design C's version column and status column live in different tables. In `races.txt`, case 1
  shows a delete begin that touches only the file row letting a move run through. Case 2 shows the
  same interleaving blocked when the delete begin also bumps the node's version. Case 3 shows a
  write complete whose version guard fails still landing, because the harness ran the second
  statement without checking the first one's row count. So design C is safe only when every file
  step is a two-statement transaction that checks the guard's row count, which the schema cannot
  enforce, and that costs round trips and the ability to run a step on the pool.
- Design B's typed foreign keys need a constant generated column in every consumer table that
  references a file or a directory, and its per-kind check constraints must be written NULL-safe
  (`integrity.txt`, case 4, and `recommended-check.txt`).
- The 34 MB figure for tuned design B against 28 MB for design A is computed from index sizes.
  `storage.txt` measured design B with its untuned indexes.

## Reproducing

The scripts assume the compose stack is running (`mise run up`) and reach Postgres through
`docker exec blobfs-postgres psql`. They create databases named `blobfs_opus_*` and expect the
caller to drop them afterward. They are not wired into `mise`, and the fixture takes several
minutes to load.
