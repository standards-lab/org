# blobfs evidence

The measurements behind the experiment's decisions. Each entry names what it measures and how to
rerun it. Buffer counts and plan shapes carry across machines, and milliseconds do not, so the
transcripts use buffers and round trips as the primary evidence.

| Path | What it measures | How it is produced |
|------|------------------|--------------------|
| `read-model.txt` | Proof V3: the cost of the shipped directory listing, by page, sort, and total mode, on a volume fixture. | `mise run evidence` runs `TestListingCost` in `lib/blobfs/data`. |
| `bookmarks.txt` | The bookmark read model against the shapes it was chosen over, including the per-row path recursion. | `mise run evidence` runs `TestBookmarkCost` in `domain/files`. |
| `sort-index.txt` | What an index on `blobfs_file (directory_id, created_at)` buys a listing sorted by creation time. The test creates the index in its own database, because the library's migration set does not ship it. | `mise run evidence` runs `TestSortIndexCost` in `lib/blobfs/data`. |
| `v1-read-model.txt` | Proof V1: the read-model cost by form against the volume-based schema of an earlier stage. The transcript is kept as a record and is not regenerated. | A test of an earlier stage that no longer exists. |
| `schema-alternatives/` | Four candidate schema designs measured on Postgres 18 at 10,003 directories and 100,000 files: listings, the file lifecycle protocol, races, and integrity. Its own `README.md` describes the method. | The numbered scripts in that directory. |
| `native-variation/` | The three native variation points measured against the standard tier: `RETURNING` for the write steps, path resolution in one statement, and the row-value keyset predicate. | `10_write_returning.sh`, `20_path_resolution.sh`, and `30_keyset.sh`, each of which builds and drops its own scratch database on the compose Postgres. |

`mise run evidence` rewrites the first three transcripts, which `REVIEW.md` cites.
Run it only when the transcripts should change. The tests behind the transcripts are skipped
unless `BLOBFS_EVIDENCE=1`, which the task sets, and each one creates and drops its own database.

The cost regression tests in `lib/blobfs/data` and `lib/blobfs/postgres` assert the values these
transcripts recorded, with a wide margin, so a plan regression fails a test.
