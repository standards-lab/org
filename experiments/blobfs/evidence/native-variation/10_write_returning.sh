#!/usr/bin/env bash
# Candidate 1 of stage 28: RETURNING for the write protocol's begin and
# complete steps and for Mkdir, measured against the standard tier.
#
# What is measured, per step, standard against native:
#   round trips  every statement a pgbench script sends is one round trip,
#                counted from the script text the way 40_protocol.sh counts
#                them (an explicit BEGIN or COMMIT would count too; none is
#                needed here, every step is one autocommit statement or a
#                sequence of them on the pool). Each script also carries the
#                setup and cleanup statements that give it a row to act on
#                and remove it again; those are identical across the forms of
#                one step and are named in the label, so the step's own
#                count is the script count minus them.
#   buffers      EXPLAIN (ANALYZE, BUFFERS) of each statement inside
#                BEGIN ... ROLLBACK (1 warm-up + 5 runs, the median run by
#                execution time), top plan node's shared hit + read. A
#                multi-statement form is the sum of its statements' medians.
#                The foreign-key check of an insert runs as an after trigger
#                and its buffers are outside the top node in every form
#                alike.
#   wall time    secondary: median per-transaction latency from pgbench's
#                log, one client inside the container, the forms of one
#                step chosen at random per transaction so they alternate,
#                after a discarded warm-up run. Same machine for client and
#                server; not comparable across hosts.
#
# Volume: the shared fixture (10,003 directories, 100,000 files); the parent
# of every written row is the small depth-6 directory (9 files), as in
# 40_protocol.sh, so the writes never touch the biggest directory.
#
# The standard forms are the statements lib/blobfs/data ships
# (begin_file_write + file_by_id, complete_file_write under the query
# library's guard + file_version + file_by_id, create_directory +
# directory_by_id). The native forms are written as they would ship in
# lib/blobfs/postgres/statements with the published column patterns; the
# sqlate spelling is printed at the top of the transcript.
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/00_lib.sh"
OUT="$S/write_returning.txt"
DB=blobfs_m28_write
PARENT='02d013ca-7000-8000-9000-0000000013ca'   # /t0/d3/d123/d363/d1063/d5063, 9 files
trap 'drop_db write' EXIT

FILECOLS='f.id, f.directory_id, f.name, f.status, f.key, f.size, f.content_type, f.etag, f.version, f.created_at, f.updated_at'
DIRCOLS='d.id, d.parent_id, d.name, d.version, d.created_at, d.updated_at'

# ---- the statements, with pgbench's :n as the id -------------------------
ID="CAST(lpad(to_hex(:n), 32, '0') AS uuid)"
insert_pending="INSERT INTO blobfs_file (id, directory_id, name, status, key, content_type) VALUES ($ID, CAST('$PARENT' AS uuid), 'b' || :n || '.txt', 'pending', lpad(to_hex(:n), 32, '0') || '/b' || :n || '.txt', 'text/plain');"
insert_available="INSERT INTO blobfs_file (id, directory_id, name, status, key, size, content_type, etag) VALUES ($ID, CAST('$PARENT' AS uuid), 'b' || :n || '.txt', 'available', lpad(to_hex(:n), 32, '0') || '/b' || :n || '.txt', 1024, 'text/plain', 'e');"
insert_returning="INSERT INTO blobfs_file AS f (id, directory_id, name, status, key, content_type) VALUES ($ID, CAST('$PARENT' AS uuid), 'b' || :n || '.txt', 'pending', lpad(to_hex(:n), 32, '0') || '/b' || :n || '.txt', 'text/plain') RETURNING $FILECOLS;"
read_file="SELECT $FILECOLS FROM blobfs_file f WHERE f.id = $ID;"
read_version="SELECT f.version FROM blobfs_file f WHERE f.id = $ID;"
cleanup_file="DELETE FROM blobfs_file WHERE id = $ID;"
complete() { # $1 expected version
  echo "UPDATE blobfs_file SET status = 'available', size = 1024, content_type = 'text/plain', etag = 'e' || :n, updated_at = CURRENT_TIMESTAMP, version = version + 1 WHERE id = $ID AND version = $1 AND status = 'pending';"
}
complete_returning() {
  echo "UPDATE blobfs_file AS f SET status = 'available', size = 1024, content_type = 'text/plain', etag = 'e' || :n, updated_at = CURRENT_TIMESTAMP, version = f.version + 1 WHERE f.id = $ID AND f.version = $1 AND f.status = 'pending' RETURNING $FILECOLS;"
}
complete_cte() {
  cat <<EOF
WITH completed AS (UPDATE blobfs_file AS f SET status = 'available', size = 1024, content_type = 'text/plain', etag = 'e' || :n, updated_at = CURRENT_TIMESTAMP, version = f.version + 1 WHERE f.id = $ID AND f.version = $1 AND f.status = 'pending' RETURNING $FILECOLS) SELECT TRUE AS completed, $FILECOLS FROM completed f UNION ALL SELECT FALSE, $FILECOLS FROM (SELECT $FILECOLS FROM blobfs_file f WHERE f.id = $ID AND NOT EXISTS (SELECT 1 FROM completed) FOR UPDATE) f;
EOF
}
insert_dir="INSERT INTO blobfs_directory (id, parent_id, name) VALUES ($ID, CAST('$PARENT' AS uuid), 'g' || :n);"
insert_dir_returning="INSERT INTO blobfs_directory AS d (id, parent_id, name) VALUES ($ID, CAST('$PARENT' AS uuid), 'g' || :n) RETURNING $DIRCOLS;"
read_dir="SELECT $DIRCOLS FROM blobfs_directory d WHERE d.id = $ID;"
cleanup_dir="DELETE FROM blobfs_directory WHERE id = $ID;"

R='\set n random(1, 1000000000000)'
w() { local name=$1; shift; { echo "$R"; printf '%s\n' "$@"; } | mk "$name"; }

w begin_std.sql     "$insert_pending" "$read_file" "$cleanup_file"
w begin_ret.sql     "$insert_returning" "$cleanup_file"
w complete_std.sql  "$insert_pending" "$(complete 1)" "$read_file" "$cleanup_file"
w complete_ret.sql  "$insert_pending" "$(complete_returning 1)" "$cleanup_file"
w complete_cte.sql  "$insert_pending" "$(complete_cte 1)" "$cleanup_file"
w mismatch_std.sql  "$insert_pending" "$(complete 7)" "$read_version" "$read_file" "$cleanup_file"
w mismatch_ret.sql  "$insert_pending" "$(complete_returning 7)" "$read_file" "$cleanup_file"
w mismatch_cte.sql  "$insert_pending" "$(complete_cte 7)" "$cleanup_file"
w status_std.sql    "$insert_available" "$(complete 1)" "$read_version" "$read_file" "$cleanup_file"
w status_ret.sql    "$insert_available" "$(complete_returning 1)" "$read_file" "$cleanup_file"
w status_cte.sql    "$insert_available" "$(complete_cte 1)" "$cleanup_file"
w notfound_std.sql  "$(complete 1)" "$read_version"
w notfound_ret.sql  "$(complete_returning 1)" "$read_file"
w notfound_cte.sql  "$(complete_cte 1)"
w mkdir_std.sql     "$insert_dir" "$read_dir" "$cleanup_dir"
w mkdir_ret.sql     "$insert_dir_returning" "$cleanup_dir"

# ---- literal ids for the EXPLAIN runs ------------------------------------
NEW='0000000a-0000-0000-0000-00000000ab01'   # inserted inside BEGIN ... ROLLBACK
PEND='0000000a-0000-0000-0000-00000000aa01'  # committed pending row, version 1
AVAIL='0000000a-0000-0000-0000-00000000aa02' # committed available row, version 1
NONE='0000000a-0000-0000-0000-00000000ab09'  # no row
lit() { sed "s/CAST(lpad(to_hex(:n), 32, '0') AS uuid)/CAST('$2' AS uuid)/g; s/:n/$3/g; s/;\$//" <<<"$1"; }

{
echo "# Candidate 1: RETURNING for the write protocol's begin and complete steps and for Mkdir."
echo "# Round trips are statements sent (pgbench script lines); buffers are EXPLAIN (ANALYZE, BUFFERS)"
echo "# top-node shared hit+read per statement, summed over a multi-statement form; wall time is the"
echo "# median pgbench per-transaction latency (secondary, one machine)."
build_db write
header $DB
echo "# parent of every written row: $PARENT (the small depth-6 directory)."
echo
echo "==================== the native candidates as they would ship (sqlate spelling) ===================="
cat <<'EOF'
-- begin_file_write.sql (native): one round trip, the row comes back from the insert
INSERT INTO blobfs_file AS f (id, directory_id, name, status, key, content_type)
VALUES ({{id:uuid}}, {{directory_id:uuid}}, {{name}}, 'pending', {{key}}, {{content_type}})
RETURNING {{> blobfs.file_columns}}

-- create_directory.sql (native)
INSERT INTO blobfs_directory AS d (id, parent_id, name)
VALUES ({{id:uuid}}, {{parent_id:uuid}}, {{name}})
RETURNING {{> blobfs.directory_columns}}

-- complete_file_write.sql (native, plain RETURNING): one round trip on success; no row returned
-- means the variant reads the row once (file_by_id) to tell not found / version mismatch /
-- wrong status apart, exactly as the standard guard's check + read-back does today.
UPDATE blobfs_file AS f
SET status = 'available', size = {{size:bigint}}, content_type = {{content_type}}, etag = {{etag}},
    updated_at = CURRENT_TIMESTAMP, version = f.version + 1
WHERE f.id = {{id:uuid}} AND f.version = {{version:bigint}} AND f.status = 'pending'
RETURNING {{> blobfs.file_columns}}

-- complete_file_write.sql (native, one-statement classification): every outcome in one round
-- trip. completed = TRUE carries the updated row; completed = FALSE carries the row as it stands
-- (locked FOR UPDATE, so under READ COMMITTED it is the latest committed version, not the
-- statement's snapshot) and the caller classifies by version and status; no row is not found.
WITH completed AS (
  UPDATE blobfs_file AS f
  SET status = 'available', size = {{size:bigint}}, content_type = {{content_type}}, etag = {{etag}},
      updated_at = CURRENT_TIMESTAMP, version = f.version + 1
  WHERE f.id = {{id:uuid}} AND f.version = {{version:bigint}} AND f.status = 'pending'
  RETURNING {{> blobfs.file_columns}}
)
SELECT TRUE AS completed, {{> blobfs.file_columns}} FROM completed f
UNION ALL
SELECT FALSE, {{> blobfs.file_columns}}
FROM (SELECT {{> blobfs.file_columns}} FROM blobfs_file f
      WHERE f.id = {{id:uuid}} AND NOT EXISTS (SELECT 1 FROM completed) FOR UPDATE) f
EOF
echo
echo "==================== 1. round trips and wall time (pgbench, one client, ~200 transactions per form) ===================="
echo "# The label names the shared setup/cleanup statements; the step's own round trips = script count minus those."
echo
echo "---- BeginFileWrite: insert + read-back (standard) vs INSERT ... RETURNING (native); +1 cleanup delete in both ----"
bench $DB "begin, standard (step 2 + 1 cleanup)" begin_std.sql "begin, native RETURNING (step 1 + 1 cleanup)" begin_ret.sql
echo
echo "---- CompleteFileWrite, success: guarded update + read-back (standard) vs UPDATE ... RETURNING vs the CTE form; +1 setup insert, +1 cleanup delete ----"
BENCH_TXNS=600 bench $DB "complete ok, standard (step 2 + 2 shared)" complete_std.sql "complete ok, native RETURNING (step 1 + 2 shared)" complete_ret.sql "complete ok, native CTE (step 1 + 2 shared)" complete_cte.sql
echo
echo "---- CompleteFileWrite, version mismatch: update (0 rows) + guard check + read-back (standard, 3) vs RETURNING (0 rows) + read-back (2) vs CTE (1); +2 shared ----"
BENCH_TXNS=600 bench $DB "complete mismatch, standard (step 3 + 2 shared)" mismatch_std.sql "complete mismatch, native RETURNING (step 2 + 2 shared)" mismatch_ret.sql "complete mismatch, native CTE (step 1 + 2 shared)" mismatch_cte.sql
echo
echo "---- CompleteFileWrite, wrong status (row available at the expected version): same shapes ----"
BENCH_TXNS=600 bench $DB "complete wrong status, standard (step 3 + 2 shared)" status_std.sql "complete wrong status, native RETURNING (step 2 + 2 shared)" status_ret.sql "complete wrong status, native CTE (step 1 + 2 shared)" status_cte.sql
echo
echo "---- CompleteFileWrite, not found: update (0 rows) + guard check (standard, 2) vs RETURNING (0 rows) + read-back (2) vs CTE (1); no setup or cleanup ----"
BENCH_TXNS=600 bench $DB "complete not found, standard (step 2)" notfound_std.sql "complete not found, native RETURNING (step 2)" notfound_ret.sql "complete not found, native CTE (step 1)" notfound_cte.sql
echo
echo "---- Mkdir: insert + read-back (standard) vs INSERT ... RETURNING (native); +1 cleanup delete in both ----"
bench $DB "mkdir, standard (step 2 + 1 cleanup)" mkdir_std.sql "mkdir, native RETURNING (step 1 + 1 cleanup)" mkdir_ret.sql
echo
echo "# rows left by pgbench (every script removed what it wrote; the not-found scripts wrote nothing):"
PSQL -d $DB -tAc "SELECT 'files under the parent now: ' || count(*) FROM blobfs_file WHERE directory_id = '$PARENT'"

echo
echo "==================== 2. buffers per statement (EXPLAIN (ANALYZE, BUFFERS), median of $RUNS after a warm-up, inside BEGIN ... ROLLBACK) ===================="
PSQL -d $DB -q <<SQL
INSERT INTO blobfs_file (id, directory_id, name, status, key, content_type) VALUES ('$PEND', '$PARENT', 'pend.txt', 'pending', '$PEND/pend.txt', 'text/plain');
INSERT INTO blobfs_file (id, directory_id, name, status, key, size, content_type, etag) VALUES ('$AVAIL', '$PARENT', 'avail.txt', 'available', '$AVAIL/avail.txt', 1024, 'text/plain', 'e');
SQL
sum() { echo "    => $1: $2 buffers"; }

echo
echo "---- BeginFileWrite ----"
explain_median $DB "begin std (1/2) insert" "$(lit "$insert_pending" $NEW 1)"; a=$LAST_BUF
explain_median $DB "begin std (2/2) read-back after the insert" "$(lit "$read_file" $NEW 1)" "$(lit "$insert_pending" $NEW 1)"; b=$LAST_BUF
sum "begin, standard, 2 statements" $((a + b))
explain_median $DB "begin native INSERT ... RETURNING" "$(lit "$insert_returning" $NEW 1)"; sum "begin, native, 1 statement" $LAST_BUF

echo
echo "---- CompleteFileWrite, success (pending row at version 1) ----"
explain_median $DB "complete ok std (1/2) guarded update" "$(lit "$(complete 1)" $PEND 1)"; a=$LAST_BUF
explain_median $DB "complete ok std (2/2) read-back after the update" "$(lit "$read_file" $PEND 1)" "$(lit "$(complete 1)" $PEND 1)"; b=$LAST_BUF
sum "complete ok, standard, 2 statements" $((a + b))
explain_median $DB "complete ok native UPDATE ... RETURNING" "$(lit "$(complete_returning 1)" $PEND 1)"; sum "complete ok, native RETURNING, 1 statement" $LAST_BUF
explain_median $DB "complete ok native CTE" "$(lit "$(complete_cte 1)" $PEND 1)"; sum "complete ok, native CTE, 1 statement" $LAST_BUF

echo
echo "---- CompleteFileWrite, version mismatch (pending row at version 1, caller expects 7) ----"
explain_median $DB "mismatch std (1/3) guarded update, 0 rows" "$(lit "$(complete 7)" $PEND 1)"; a=$LAST_BUF
explain_median $DB "mismatch std (2/3) guard check file_version" "$(lit "$read_version" $PEND 1)"; b=$LAST_BUF
explain_median $DB "mismatch std (3/3) read-back to classify" "$(lit "$read_file" $PEND 1)"; c=$LAST_BUF
sum "mismatch, standard, 3 statements" $((a + b + c))
explain_median $DB "mismatch native (1/2) UPDATE ... RETURNING, 0 rows" "$(lit "$(complete_returning 7)" $PEND 1)"; a=$LAST_BUF
explain_median $DB "mismatch native (2/2) read-back to classify" "$(lit "$read_file" $PEND 1)" "" no; b=$LAST_BUF
sum "mismatch, native RETURNING, 2 statements" $((a + b))
explain_median $DB "mismatch native CTE, completed = false" "$(lit "$(complete_cte 7)" $PEND 1)"; sum "mismatch, native CTE, 1 statement" $LAST_BUF

echo
echo "---- CompleteFileWrite, wrong status (available row at version 1, caller expects 1) ----"
explain_median $DB "status std (1/3) guarded update, 0 rows" "$(lit "$(complete 1)" $AVAIL 1)" "" no; a=$LAST_BUF
explain_median $DB "status std (2/3) guard check file_version" "$(lit "$read_version" $AVAIL 1)" "" no; b=$LAST_BUF
explain_median $DB "status std (3/3) read-back to classify" "$(lit "$read_file" $AVAIL 1)" "" no; c=$LAST_BUF
sum "wrong status, standard, 3 statements" $((a + b + c))
explain_median $DB "status native (1/2) UPDATE ... RETURNING, 0 rows" "$(lit "$(complete_returning 1)" $AVAIL 1)" "" no; a=$LAST_BUF
explain_median $DB "status native (2/2) read-back to classify" "$(lit "$read_file" $AVAIL 1)" "" no; b=$LAST_BUF
sum "wrong status, native RETURNING, 2 statements" $((a + b))
explain_median $DB "status native CTE, completed = false" "$(lit "$(complete_cte 1)" $AVAIL 1)" "" no; sum "wrong status, native CTE, 1 statement" $LAST_BUF

echo
echo "---- CompleteFileWrite, not found ----"
explain_median $DB "not found std (1/2) guarded update, 0 rows" "$(lit "$(complete 1)" $NONE 1)" "" no; a=$LAST_BUF
explain_median $DB "not found std (2/2) guard check file_version, 0 rows" "$(lit "$read_version" $NONE 1)" "" no; b=$LAST_BUF
sum "not found, standard, 2 statements" $((a + b))
explain_median $DB "not found native (1/2) UPDATE ... RETURNING, 0 rows" "$(lit "$(complete_returning 1)" $NONE 1)" "" no; a=$LAST_BUF
explain_median $DB "not found native (2/2) read-back, 0 rows" "$(lit "$read_file" $NONE 1)" "" no; b=$LAST_BUF
sum "not found, native RETURNING, 2 statements" $((a + b))
explain_median $DB "not found native CTE, no row" "$(lit "$(complete_cte 1)" $NONE 1)"; sum "not found, native CTE, 1 statement" $LAST_BUF

echo
echo "---- Mkdir ----"
explain_median $DB "mkdir std (1/2) insert" "$(lit "$insert_dir" $NEW 1)"; a=$LAST_BUF
explain_median $DB "mkdir std (2/2) read-back after the insert" "$(lit "$read_dir" $NEW 1)" "$(lit "$insert_dir" $NEW 1)"; b=$LAST_BUF
sum "mkdir, standard, 2 statements" $((a + b))
explain_median $DB "mkdir native INSERT ... RETURNING" "$(lit "$insert_dir_returning" $NEW 1)"; sum "mkdir, native, 1 statement" $LAST_BUF

echo
echo "==================== 3. outcomes the CTE form returns (functional check; columns: completed|id|directory_id|name|status|key|size|content_type|etag|version|created_at|updated_at) ===================="
cte_case() { # $1 label  $2 id  $3 expected version
  echo "-- $1"
  local rows; rows=$(PSQL -d $DB -tA -c "$(lit "$(complete_cte "$3")" "$2" 1)")
  [ -n "$rows" ] && sed 's/^/   /' <<<"$rows"
  echo "   ($(grep -c . <<<"$rows" || true) row(s))"
}
PSQL -d $DB -q -c "UPDATE blobfs_file SET status = 'pending', version = 1, size = NULL, etag = NULL WHERE id = '$PEND'"
echo "-- pending row at version 1, caller expects 1: completed = t, the row as updated"
PSQL -d $DB -tA -c "$(lit "$(complete_cte 1)" $PEND 1)" | sed 's/^/   /'
PSQL -d $DB -q -c "UPDATE blobfs_file SET status = 'pending', version = 1, size = NULL, etag = NULL WHERE id = '$PEND'"
cte_case "pending row at version 1, caller expects 7: completed = f, the row unchanged (version 1 <> 7: a version mismatch)" $PEND 7
cte_case "available row at version 1, caller expects 1: completed = f, the row unchanged (version matches, status is not pending: a transition refusal)" $AVAIL 1
cte_case "no row: nothing comes back (not found)" $NONE 1
} 2>&1 | tee "$OUT"
