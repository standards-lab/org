#!/usr/bin/env bash
# Candidate 3 of stage 28: a row-value keyset predicate, measured against the
# expanded OR chain the listing composer renders.
#
# The standard form is the text lib/blobfs/data/listing.go composes for a
# cursor page (keyset): for the terms up to and including the key, name,
# the predicate (a > x) OR (a = x AND b > y) OR ..., every value bound
# through CAST(placeholder AS type), then ORDER BY the terms and the paging
# clause with offset 0 and fetch page size + 1 (21 for the page size 20 the
# other evidence uses). A sort by name alone has one term, so its predicate
# is the single comparison q.name > x and a row-value form of one column is
# the same expression: nothing to measure there, and the name pages are
# recorded as the reference cost only. The native candidate replaces the OR
# chain by one row comparison (q.created_at, q.name) > (x, y), or < under a
# descending sort. A sort that mixes directions has no row-value form, and
# the composer already refuses a cursor on such a sort, so nothing is lost.
#
# What is measured, on the biggest directory (10,009 files), with cursors
# near the start (after row 20), in the middle (after row 5,000) and near
# the end (after row 9,990) of the sort:
#   buffers and plan shape   EXPLAIN (ANALYZE, BUFFERS), top node's shared
#                            hit + read, median of 5 runs after a warm-up,
#                            every value a literal so each run is planned
#                            with its own values as the simple protocol does.
#                            The plan says whether the predicate is an Index
#                            Cond or a Filter, and which index serves it.
#   wall time                secondary: median per-transaction pgbench
#                            latency, forms alternating at random, one
#                            client in the container.
#   round trips              one in every form; the candidate changes the
#                            statement's text, not its count.
#
# Section 1 runs on the schema as shipped: the only index over blobfs_file
# that leads with directory_id is the unique (directory_id, name). Section 2
# creates blobfs_ix_file_directory_created (directory_id, created_at), the
# index the library no longer ships by default (adjustment 15), the way
# evidence/sort-index.txt built it, VACUUM ANALYZEs, and repeats the
# created_at pages; the created_at measurement is only meaningful with it.
# Section 2 adds one three-term sort (created_at, version, then the key).
#
# Volume: the shared fixture (10,003 directories, 100,000 files, biggest
# directory 10,009 files, created_at spread over a year).
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/00_lib.sh"
OUT="$S/keyset.txt"
DB=blobfs_m28_keyset
BIG='02d00003-7000-8000-9000-000000000003'
trap 'drop_db keyset' EXIT

BASE="SELECT q.id, q.directory_id, q.name, q.status, q.key, q.size, q.content_type, q.etag, q.version, q.created_at, q.updated_at
FROM blobfs_file q
WHERE q.directory_id = CAST('$BIG' AS uuid)"
PAGING="OFFSET 0 ROWS FETCH NEXT 21 ROWS ONLY"
TS="timestamp with time zone"

name_page()   { echo "$BASE AND q.name $1 CAST('$2' AS text) ORDER BY q.name$3 $PAGING"; }   # $1 > or <, $2 name, $3 '' or ' DESC'
or_page()     { echo "$BASE AND (q.created_at $1 CAST('$2' AS $TS) OR (q.created_at = CAST('$2' AS $TS) AND q.name $1 CAST('$3' AS text))) ORDER BY q.created_at$4, q.name$4 $PAGING"; }
rv_page()     { echo "$BASE AND (q.created_at, q.name) $1 (CAST('$2' AS $TS), CAST('$3' AS text)) ORDER BY q.created_at$4, q.name$4 $PAGING"; }
or3_page()    { echo "$BASE AND (q.created_at > CAST('$1' AS $TS) OR (q.created_at = CAST('$1' AS $TS) AND q.version > CAST('$2' AS bigint)) OR (q.created_at = CAST('$1' AS $TS) AND q.version = CAST('$2' AS bigint) AND q.name > CAST('$3' AS text))) ORDER BY q.created_at, q.version, q.name $PAGING"; }
rv3_page()    { echo "$BASE AND (q.created_at, q.version, q.name) > (CAST('$1' AS $TS), CAST('$2' AS bigint), CAST('$3' AS text)) ORDER BY q.created_at, q.version, q.name $PAGING"; }

cursor_at() { # $1 ORDER BY terms  $2 offset -> created_at|name|version of that row
  PSQL -d $DB -tAc "SELECT created_at, name, version FROM blobfs_file WHERE directory_id = '$BIG' ORDER BY $1 OFFSET $2 LIMIT 1"
}

declare -A POS=([start]=19 [middle]=4999 [end]=9989)
ORDER=(start middle end)

created_pages() { # $1 section tag
  local p dir cmp ob
  for dir in asc desc; do
    if [ $dir = asc ]; then cmp='>'; ob=''; else cmp='<'; ob=' DESC'; fi
    for p in "${ORDER[@]}"; do
      IFS='|' read -r t n v <<<"$(cursor_at "created_at$ob, name$ob" "${POS[$p]}")"
      echo "---- created_at $dir, cursor $p (after row $((POS[$p] + 1)): created_at $t, name $n) ----"
      explain_median $DB "$1 created_at $dir, $p, OR chain (standard)" "$(or_page "$cmp" "$t" "$n" "$ob")"; o=$LAST_BUF
      explain_median $DB "$1 created_at $dir, $p, row value (native)" "$(rv_page "$cmp" "$t" "$n" "$ob")"; r=$LAST_BUF
      echo "    => created_at $dir, $p: OR chain $o buffers, row value $r buffers"
      echo
    done
  done
}

{
echo "# Candidate 3: row-value keyset predicate vs the composer's expanded OR chain, on the biggest directory (10,009 files)."
echo "# Buffers are EXPLAIN (ANALYZE, BUFFERS) top-node shared hit+read (median of $RUNS after a warm-up); the plan shows"
echo "# Index Cond vs Filter and the index used; wall time is the median pgbench per-transaction latency (secondary)."
build_db keyset
header $DB
echo "# page size 20, so every page fetches 21 rows; cursors after rows 20, 5000 and 9990 of the sort."
echo "# distinct created_at values in the biggest directory: $(PSQL -d $DB -tAc "SELECT count(DISTINCT created_at) FROM blobfs_file WHERE directory_id = '$BIG'")"
echo
echo "==================== the native candidate as it would ship ===================="
cat <<'EOF'
-- The keyset predicate for terms t1..tn (all ascending, or all descending with <), as the
-- Postgres variant would render it in place of listing.go's expanded chain; every value still
-- binds its own placeholder through the value pattern:
--   (q.t1, q.t2, ..., q.tn) > (CAST($i AS type1), CAST($i+1 AS type2), ..., CAST($i+n-1 AS typen))
-- for the created_at sort: (q.created_at, q.name) > (CAST($2 AS timestamp with time zone), CAST($3 AS text))
EOF
echo
echo "==================== 1. as shipped: no created_at index ===================="
echo "---- reference: sort by name, one term, the predicate is one comparison in both forms ----"
for p in "${ORDER[@]}"; do
  IFS='|' read -r t n v <<<"$(cursor_at "name" "${POS[$p]}")"
  explain_median $DB "S1 name asc, $p (after '$n')" "$(name_page '>' "$n" '')" "" no
done
IFS='|' read -r t n v <<<"$(cursor_at "name DESC" "${POS[middle]}")"
explain_median $DB "S1 name desc, middle (before '$n')" "$(name_page '<' "$n" ' DESC')" "" no
echo
created_pages S1
IFS='|' read -r t n v <<<"$(cursor_at "created_at, name" "${POS[middle]}")"
echo "$(or_page '>' "$t" "$n" '');" | tr '\n' ' ' | sed 's/$/\n/' | mk or_mid.sql
echo "$(rv_page '>' "$t" "$n" '');" | tr '\n' ' ' | sed 's/$/\n/' | mk rv_mid.sql
echo "---- wall time, created_at asc, middle cursor ----"
bench $DB "S1 created_at asc middle, OR chain" or_mid.sql "S1 created_at asc middle, row value" rv_mid.sql

echo
echo "==================== 2. with blobfs_ix_file_directory_created (directory_id, created_at), as evidence/sort-index.txt built it ===================="
PSQL -d $DB -qc "CREATE INDEX blobfs_ix_file_directory_created ON blobfs_file (directory_id, created_at)"
docker exec blobfs-postgres psql -X -U app -d $DB -qc "VACUUM ANALYZE blobfs_file" >/dev/null
echo "# index size: $(PSQL -d $DB -tAc "SELECT pg_size_pretty(pg_relation_size('blobfs_ix_file_directory_created'))")"
echo
created_pages S2
echo "---- three terms: sort created_at, version, then the key name; middle cursor ----"
IFS='|' read -r t n v <<<"$(cursor_at "created_at, version, name" "${POS[middle]}")"
explain_median $DB "S2 created_at, version asc, middle, OR chain (3 disjuncts)" "$(or3_page "$t" "$v" "$n")"; o=$LAST_BUF
explain_median $DB "S2 created_at, version asc, middle, row value (3 columns)" "$(rv3_page "$t" "$v" "$n")"; r=$LAST_BUF
echo "    => created_at, version asc, middle: OR chain $o buffers, row value $r buffers"
echo
for p in "${ORDER[@]}"; do
  IFS='|' read -r t n v <<<"$(cursor_at "created_at, name" "${POS[$p]}")"
  echo "$(or_page '>' "$t" "$n" '');" | tr '\n' ' ' | sed 's/$/\n/' | mk "or_$p.sql"
  echo "$(rv_page '>' "$t" "$n" '');" | tr '\n' ' ' | sed 's/$/\n/' | mk "rv_$p.sql"
  echo "---- wall time, created_at asc, $p cursor ----"
  bench $DB "S2 created_at asc $p, OR chain" "or_$p.sql" "S2 created_at asc $p, row value" "rv_$p.sql"
  echo
done
} 2>&1 | tee "$OUT"
