#!/usr/bin/env bash
# Candidate 2 of stage 28: path resolution in one statement, measured against
# the standard tier's walk.
#
# The standard form is what lib/blobfs/data/paths.go ships: ResolveDirectory
# reads the root row (directory_by_id) and then runs one directory_child
# read per segment, each with the parent id the previous read returned, so
# a path of depth N is N + 1 round trips. The pgbench script chains them
# with \gset exactly that way, so every statement carries the id the
# statement before it returned.
#
# The native candidate is one recursive statement that starts at a start
# directory (the root for an absolute path, any directory for
# ResolveDirectoryFrom) and walks down a text[] of segments by array
# subscript, one index probe of (parent_id, name) per level. It returns the
# deepest row reached and its depth: depth = the number of segments means
# resolved; a smaller depth means segment depth + 1 named no directory, and
# the caller spells the failing prefix from the segments it already holds
# (the same "at /a/b/missing" the walk reports today); no row means the
# start directory does not exist. A join chain generated per depth is
# measured beside it as the floor: it cannot say which segment failed.
#
# What is measured, at depths 1, 3, 6 and 10 on one chain, with the last
# segment present and with it missing, and once with a missing segment in
# the middle:
#   round trips  statements a pgbench script sends (lines ending in ; or
#                \gset), as 40_protocol.sh counts them.
#   buffers      EXPLAIN (ANALYZE, BUFFERS), top node's shared hit + read,
#                median of 5 runs after a warm-up; the standard form is the
#                sum over its N + 1 statements, each with the literal parent
#                id the previous one returns.
#   wall time    secondary: median per-transaction pgbench latency, forms
#                alternating at random, one client in the container.
#
# Volume: the shared fixture (10,003 directories to depth 6, 100,000 files)
# plus the three depth-10 chains 00_lib.sh adds; the measured chain is
# /t0/d3/d123/d363/d1063/d5063/c7/c8/c9/c10.
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/00_lib.sh"
OUT="$S/path_resolution.txt"
DB=blobfs_m28_path
trap 'drop_db path' EXIT

DIRCOLS='d.id, d.parent_id, d.name, d.version, d.created_at, d.updated_at'
SEGS=(t0 d3 d123 d363 d1063 d5063 c7 c8 c9 c10)

# ---- the forms, spelled with literals ------------------------------------
path_of() { local n=$1; local IFS=/; echo "/${SEGS[*]:0:$n}"; }
array_of() { local IFS=,; echo "{${*}}"; }   # a text[] literal from names

recursive() { # $1 start id  $2 text[] literal
  cat <<EOF
WITH RECURSIVE walk (id, parent_id, name, version, created_at, updated_at, depth) AS (
    SELECT $DIRCOLS, CAST(0 AS integer)
    FROM blobfs_directory d
    WHERE d.id = CAST('$1' AS uuid)
  UNION ALL
    SELECT $DIRCOLS, w.depth + 1
    FROM walk w
    JOIN blobfs_directory d ON d.parent_id = w.id AND d.name = (CAST('$2' AS text[]))[w.depth + 1]
)
SELECT w.id, w.parent_id, w.name, w.version, w.created_at, w.updated_at, w.depth
FROM walk w
ORDER BY w.depth DESC
FETCH FIRST 1 ROW ONLY
EOF
}
chain() { # $1 start id, then the segment names
  local start=$1; shift
  local -a names=("$@"); local n=${#names[@]}
  local sql="SELECT d$n.id, d$n.parent_id, d$n.name, d$n.version, d$n.created_at, d$n.updated_at FROM blobfs_directory d1"
  local i
  for ((i = 2; i <= n; i++)); do
    sql+=" JOIN blobfs_directory d$i ON d$i.parent_id = d$((i - 1)).id AND d$i.name = '${names[$((i - 1))]}'"
  done
  sql+=" WHERE d1.parent_id = CAST('$start' AS uuid) AND d1.name = '${names[0]}'"
  echo "$sql"
}
root_read="SELECT $DIRCOLS FROM blobfs_directory d WHERE d.id = CAST('$ROOT' AS uuid)"
child_read() { echo "SELECT $DIRCOLS FROM blobfs_directory d WHERE d.parent_id = CAST('$1' AS uuid) AND d.name = '$2'"; }

# The standard pgbench script: the root read, then one child read per
# segment, each taking :pid from the previous row through \gset. The last
# read of a missing path returns no row, so it ends with ; and no \gset.
std_script() { # names...
  local -a names=("$@"); local n=${#names[@]}
  echo "SELECT d.id AS pid, d.parent_id, d.name, d.version, d.created_at, d.updated_at FROM blobfs_directory d WHERE d.id = CAST('$ROOT' AS uuid) \\gset"
  local i
  for ((i = 0; i < n; i++)); do
    if [ $i -lt $((n - 1)) ]; then
      echo "SELECT d.id AS pid, d.parent_id, d.name, d.version, d.created_at, d.updated_at FROM blobfs_directory d WHERE d.parent_id = CAST(':pid' AS uuid) AND d.name = '${names[$i]}' \\gset"
    else
      echo "SELECT d.id AS pid, d.parent_id, d.name, d.version, d.created_at, d.updated_at FROM blobfs_directory d WHERE d.parent_id = CAST(':pid' AS uuid) AND d.name = '${names[$i]}';"
    fi
  done
}

{
echo "# Candidate 2: path resolution in one statement vs the standard walk (root read + one directory_child per segment)."
echo "# Round trips are statements sent (pgbench script lines ending in ; or \\gset); buffers are EXPLAIN (ANALYZE, BUFFERS)"
echo "# top-node shared hit+read per statement, summed over the walk; wall time is the median pgbench per-transaction"
echo "# latency (secondary, one machine)."
build_db path
header $DB
echo "# measured chain: $(path_of 10)"
echo
echo "==================== the native candidate as it would ship (sqlate spelling; sqlate's type token has no [], so the array cast is spelled) ===================="
cat <<'EOF'
-- resolve_path.sql (native): the deepest directory reached below start_id along segments, with its depth.
-- depth = array_length(segments) is the resolved directory; a smaller depth is a miss at segment depth + 1;
-- no row is a start that does not exist. Absolute paths pass blobfs.RootID as start_id; an empty
-- segment list returns the start itself at depth 0.
WITH RECURSIVE walk (id, parent_id, name, version, created_at, updated_at, depth) AS (
    SELECT {{> blobfs.directory_columns}}, CAST(0 AS integer)
    FROM blobfs_directory d
    WHERE d.id = {{start_id:uuid}}
  UNION ALL
    SELECT {{> blobfs.directory_columns}}, w.depth + 1
    FROM walk w
    JOIN blobfs_directory d ON d.parent_id = w.id AND d.name = (CAST({{segments}} AS text[]))[w.depth + 1]
)
SELECT w.id, w.parent_id, w.name, w.version, w.created_at, w.updated_at, w.depth
FROM walk w
ORDER BY w.depth DESC
FETCH FIRST 1 ROW ONLY
EOF
echo
# the ids along the chain, for the literal child reads
declare -a IDS=("$ROOT")
for ((i = 0; i < 10; i++)); do
  IDS+=("$(PSQL -d $DB -tAc "SELECT id FROM blobfs_directory WHERE parent_id = '${IDS[$i]}' AND name = '${SEGS[$i]}'")")
done
echo "# ids along the chain:"
for ((i = 1; i <= 10; i++)); do echo "#   depth $i $(path_of $i) = ${IDS[$i]}"; done

echo
echo "==================== 1. what the native statement returns (functional check: name|depth of the deepest row reached) ===================="
for n in 1 3 6 10; do
  names=("${SEGS[@]:0:$n}")
  echo "$(path_of $n) -> $(PSQL -d $DB -tAc "$(recursive $ROOT "$(array_of "${names[@]}")")" | awk -F'|' '{print $3 "|" $7}')"
  miss=("${names[@]:0:$((n - 1))}" missing)
  prefix="$(path_of $((n - 1)))"; [ "$prefix" = / ] && prefix=""
  echo "$prefix/missing -> $(PSQL -d $DB -tAc "$(recursive $ROOT "$(array_of "${miss[@]}")")" | awk -F'|' '{print $3 "|" $7}')   (the walk reports: at $prefix/missing)"
done
mid=("${SEGS[@]:0:2}" missing "${SEGS[@]:3:7}")
echo "/t0/d3/missing/d363/... (10 segments) -> $(PSQL -d $DB -tAc "$(recursive $ROOT "$(array_of "${mid[@]}")")" | awk -F'|' '{print $3 "|" $7}')   (the walk reports: at /t0/d3/missing)"
echo "/ (no segments) -> $(PSQL -d $DB -tAc "$(recursive $ROOT '{}')" | awk -F'|' '{print $3 "|" $7}')"
echo "start that does not exist -> $(PSQL -d $DB -tAc "$(recursive 0000000a-0000-0000-0000-00000000ab09 '{t0}')" | wc -l) row(s)"

echo
echo "==================== 2. round trips and wall time (pgbench, one client, ~200 transactions per form) ===================="
for n in 1 3 6 10; do
  names=("${SEGS[@]:0:$n}")
  std_script "${names[@]}" | mk "std_d$n.sql"
  echo "$(recursive $ROOT "$(array_of "${names[@]}")" | tr '\n' ' ');" | mk "rec_d$n.sql"
  echo "$(chain $ROOT "${names[@]}");" | mk "chain_d$n.sql"
  echo "---- depth $n: $(path_of $n) ----"
  BENCH_TXNS=600 bench $DB "depth $n, standard walk ($((n + 1)) statements)" "std_d$n.sql" "depth $n, native recursive (1)" "rec_d$n.sql" "depth $n, join chain (1)" "chain_d$n.sql"
  echo
done
miss=("${SEGS[@]:0:9}" missing)
std_script "${miss[@]}" | mk std_d10_miss.sql
echo "$(recursive $ROOT "$(array_of "${miss[@]}")" | tr "\n" " ");" | mk rec_d10_miss.sql
echo "---- depth 10 with the last segment missing: the walk stops at its 11th statement with no row; the native form returns depth 9 ----"
bench $DB "depth 10 missing last, standard walk (11)" std_d10_miss.sql "depth 10 missing last, native recursive (1)" rec_d10_miss.sql
echo
std_script "${mid[@]:0:3}" | mk std_mid_miss.sql   # the walk stops at the first miss: root read + t0 + d3 + missing
echo "$(recursive $ROOT "$(array_of "${mid[@]}")" | tr "\n" " ");" | mk rec_mid_miss.sql
echo "---- 10 segments with the third missing: the walk stops at its 4th statement; the native form returns depth 2 ----"
bench $DB "third of 10 missing, standard walk (4)" std_mid_miss.sql "third of 10 missing, native recursive (1)" rec_mid_miss.sql

echo
echo "==================== 3. buffers (EXPLAIN (ANALYZE, BUFFERS), median of $RUNS after a warm-up) ===================="
echo "---- the standard walk's statements, each once (the walk of depth N is the root read plus the first N child reads) ----"
explain_median $DB "root read (directory_by_id)" "$root_read"; rootbuf=$LAST_BUF
declare -a CHILDBUF=()
for ((i = 0; i < 10; i++)); do
  show=no; [ $i -eq 0 ] && show=yes; [ $i -eq 9 ] && show=yes
  explain_median $DB "child read $((i + 1)): '${SEGS[$i]}' under depth $i" "$(child_read "${IDS[$i]}" "${SEGS[$i]}")" "" $show
  CHILDBUF+=("$LAST_BUF")
done
explain_median $DB "child read of a missing name under depth 9" "$(child_read "${IDS[9]}" missing)" "" no; missbuf=$LAST_BUF
echo
for n in 1 3 6 10; do
  names=("${SEGS[@]:0:$n}")
  s=$rootbuf; for ((i = 0; i < n; i++)); do s=$((s + CHILDBUF[i])); done
  echo "---- depth $n: $(path_of $n) ----"
  echo "    => standard walk, $((n + 1)) statements: $s buffers"
  show=no; [ $n -eq 10 ] && show=yes; [ $n -eq 1 ] && show=yes
  explain_median $DB "depth $n native recursive" "$(recursive $ROOT "$(array_of "${names[@]}")")" "" $show; echo "    => native recursive, 1 statement: $LAST_BUF buffers"
  explain_median $DB "depth $n join chain" "$(chain $ROOT "${names[@]}")" "" $show; echo "    => join chain, 1 statement: $LAST_BUF buffers"
  echo
done
s=$rootbuf; for ((i = 0; i < 9; i++)); do s=$((s + CHILDBUF[i])); done; s=$((s + missbuf))
echo "---- depth 10 with the last segment missing ----"
echo "    => standard walk, 11 statements: $s buffers"
explain_median $DB "depth 10 missing last, native recursive" "$(recursive $ROOT "$(array_of "${miss[@]}")")" "" no; echo "    => native recursive, 1 statement: $LAST_BUF buffers"
echo
echo "---- 10 segments with the third missing ----"
explain_median $DB "child read of 'missing' under depth 2" "$(child_read "${IDS[2]}" missing)" "" no; m2=$LAST_BUF
echo "    => standard walk, 4 statements: $((rootbuf + CHILDBUF[0] + CHILDBUF[1] + m2)) buffers"
explain_median $DB "third of 10 missing, native recursive" "$(recursive $ROOT "$(array_of "${mid[@]}")")" "" no; echo "    => native recursive, 1 statement: $LAST_BUF buffers"
} 2>&1 | tee "$OUT"
