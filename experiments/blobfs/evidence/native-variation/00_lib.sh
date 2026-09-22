#!/usr/bin/env bash
# Shared helpers for the native-variation measurements (stage 28, adjustment
# 14 of DECISIONS.md). Every measurement script sources this file.
#
# What it provides:
#   build_db NAME      creates the scratch database blobfs_m28_NAME on the
#                      compose Postgres (docker exec blobfs-postgres), applies
#                      the shipped DDL from lib/blobfs/postgres/migrations
#                      (root named /, two migrations), loads the
#                      schema-alternatives fixture (10,003 directories in three
#                      trees to depth 6, 100,000 files, the biggest directory
#                      holding 10,009 files) and adds three chains of four
#                      directories under three depth-6 directories, so a path
#                      reaches depth 10. VACUUM ANALYZE runs after loading.
#   drop_db NAME       drops blobfs_m28_NAME.
#   explain_median     runs EXPLAIN (ANALYZE, BUFFERS) once to warm the cache
#                      and then RUNS times inside BEGIN ... ROLLBACK, picks the
#                      run with the median execution time, and prints a summary
#                      line (planning ms, execution ms, buffers, plan shape)
#                      followed by that run's plan. Buffers are the top plan
#                      node's shared hit + read, in 8 KB pages, as the other
#                      evidence transcripts count them. A rolled-back run does
#                      not change the rows, so a write statement is measured
#                      repeatably.
#   bench              runs pgbench inside the container with two scripts
#                      chosen at random per transaction (so the two forms
#                      alternate), one client, after a discarded warm-up run,
#                      and reports the median per-transaction latency of each
#                      script from pgbench's per-transaction log, plus
#                      pgbench's per-statement average latencies (-r). Every
#                      statement in a script is one round trip, counted from
#                      the script text, the way 40_protocol.sh counts them.
#
# Wall time is secondary evidence: the client and the server share one
# laptop, so the numbers are noisy and do not carry across machines. Round
# trips and buffers are the primary evidence.
set -euo pipefail

S="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BLOBFS="$(cd "$S/../.." && pwd)"
ROOT='00000000-0000-0000-0000-000000000000'
RUNS=${RUNS:-5}
BENCH_TXNS=${BENCH_TXNS:-400}   # about 200 per script
WARM_TXNS=${WARM_TXNS:-50}

PSQL() { docker exec -i blobfs-postgres psql -X -U app -v ON_ERROR_STOP=1 "$@"; }
ADMIN() { docker exec blobfs-postgres psql -X -U app -d postgres -v ON_ERROR_STOP=1 -c "$1"; }
mk() { docker exec -i blobfs-postgres bash -c "cat > /tmp/$1"; }

header() { # $1 db
  echo "# engine: $(PSQL -d "$1" -tAc 'SELECT version()')"
  echo "# date: $(date +%F)"
  echo "# host: $(uname -srm); client and server on one machine, everything in shared buffers."
}

build_db() { # $1 short name
  local db="blobfs_m28_$1"
  ADMIN "DROP DATABASE IF EXISTS $db" >/dev/null
  ADMIN "CREATE DATABASE $db" >/dev/null
  PSQL -d "$db" -q -f - < "$BLOBFS/lib/blobfs/postgres/migrations/0001_directory.up.sql"
  PSQL -d "$db" -q -f - < "$BLOBFS/lib/blobfs/postgres/migrations/0002_file.up.sql"
  PSQL -d "$db" -q -f - < "$S/../schema-alternatives/00_gen_fixture.sql"
  PSQL -d "$db" -q <<SQL
INSERT INTO blobfs_directory (id, parent_id, name, created_at, updated_at)
SELECT g.id, COALESCE(p.id, '$ROOT'::uuid), g.name,
       TIMESTAMPTZ '2025-09-21 00:00:00+00', TIMESTAMPTZ '2025-09-21 00:00:00+00'
FROM gen_dir g LEFT JOIN gen_dir p ON p.n = g.parent_n
ORDER BY g.depth, g.n;

INSERT INTO blobfs_file (id, directory_id, name, status, key, size, content_type, etag, created_at, updated_at)
SELECT f.id, d.id, f.name, f.status, f.key, f.size, f.content_type,
       CASE WHEN f.status = 'available' THEN 'etag-' || f.n END,
       f.created_at, f.created_at
FROM gen_file f JOIN gen_dir d ON d.n = f.dir_n;

-- Three chains c7/c8/c9/c10 under three depth-6 directories: the small
-- directory the other evidence uses (gen_dir 5066), the first depth-6
-- directory (4843) and the last (10002). Ids are deterministic: 02dc<branch><level>.
INSERT INTO blobfs_directory (id, parent_id, name)
SELECT ('02dc' || lpad(to_hex(b.branch), 2, '0') || lpad(to_hex(l.level), 2, '0') || '-7000-8000-9000-' || lpad(to_hex(b.branch * 100 + l.level), 12, '0'))::uuid,
       CASE WHEN l.level = 7 THEN g.id
            ELSE ('02dc' || lpad(to_hex(b.branch), 2, '0') || lpad(to_hex(l.level - 1), 2, '0') || '-7000-8000-9000-' || lpad(to_hex(b.branch * 100 + l.level - 1), 12, '0'))::uuid END,
       'c' || l.level
FROM (VALUES (1, 5066), (2, 4843), (3, 10002)) AS b(branch, n)
JOIN gen_dir g ON g.n = b.n
CROSS JOIN generate_series(7, 10) AS l(level)
ORDER BY l.level;

DROP TABLE gen_dir;
DROP TABLE gen_file;
SQL
  docker exec blobfs-postgres psql -X -U app -d "$db" -qc "VACUUM ANALYZE" >/dev/null
  echo "# fixture $db: $(PSQL -d "$db" -tAc "SELECT (SELECT count(*) FROM blobfs_directory) || ' directories (incl. root), ' || (SELECT count(*) FROM blobfs_file) || ' files; biggest directory ' || (SELECT max(c) FROM (SELECT count(*) c FROM blobfs_file GROUP BY directory_id) x) || ' files; deepest directory at depth ' || (SELECT max(depth) FROM (WITH RECURSIVE w AS (SELECT id, 0 AS depth FROM blobfs_directory WHERE parent_id IS NULL UNION ALL SELECT d.id, w.depth + 1 FROM blobfs_directory d JOIN w ON d.parent_id = w.id) SELECT depth FROM w) y)")"
}

drop_db() { ADMIN "DROP DATABASE IF EXISTS blobfs_m28_$1" >/dev/null; echo "# dropped blobfs_m28_$1"; }

# ---------------------------------------------------------------- EXPLAIN
PLAN_NODES=("Recursive Union" "WorkTable Scan" "CTE Scan" "Subquery Scan" "Append"
  "Merge Append" "Seq Scan" "Sort" "Incremental Sort" "Aggregate" "WindowAgg" "Limit"
  "Nested Loop" "Hash Join" "Merge Join" "Materialize" "Memoize" "Insert on" "Update on"
  "Bitmap Heap Scan" "BitmapOr" "Filter:" "Index Cond:" "InitPlan" "Result")

shape() { # plan text on stdin
  local plan; plan="$(cat)"
  local seen=()
  local n
  for n in "${PLAN_NODES[@]}"; do
    if grep -qF -- "$n" <<<"$plan"; then seen+=("$n"); fi
  done
  local idx
  while read -r idx; do
    [ -n "$idx" ] && seen+=("idx:$idx")
  done < <(grep -oE '(Index Only Scan|Index Scan Backward|Index Scan) using [a-z_]+' <<<"$plan" | awk '{print $NF}' | awk '!s[$0]++')
  local IFS=', '
  echo "${seen[*]}"
}

# explain_median DB LABEL SQL [PRE] [SHOW_PLAN]
# PRE runs unexplained inside the same transaction, before SQL (for a
# read-back that follows an insert). SHOW_PLAN defaults to yes.
explain_median() {
  local db=$1 label=$2 sql=$3 pre=${4:-} show=${5:-yes}
  local i out plan ex pl hit read buf
  local -a runs=()
  for ((i = 0; i <= RUNS; i++)); do
    out=$(PSQL -d "$db" -qtA <<SQL
BEGIN;
${pre:+$pre;}
EXPLAIN (ANALYZE, BUFFERS) $sql;
ROLLBACK;
SQL
)
    [ "$i" -eq 0 ] && continue
    ex=$(grep -oE 'Execution Time: [0-9.]+' <<<"$out" | awk '{print $3}' || true)
    pl=$(grep -oE 'Planning Time: [0-9.]+' <<<"$out" | awk '{print $3}' || true)
    local bufline
    bufline=$(grep -m1 -E 'Buffers: shared' <<<"$out" || true)
    hit=$(grep -oE 'hit=[0-9]+' <<<"$bufline" | cut -d= -f2 || true)
    read=$(grep -oE 'read=[0-9]+' <<<"$bufline" | cut -d= -f2 || true)
    buf=$(( ${hit:-0} + ${read:-0} ))
    runs+=("$ex|$pl|$buf|$(base64 -w0 <<<"$out")")
  done
  local median
  median=$(printf '%s\n' "${runs[@]}" | sort -t'|' -k1,1n | sed -n "$(( (RUNS + 1) / 2 ))p")
  IFS='|' read -r ex pl buf plan <<<"$median"
  plan=$(base64 -d <<<"$plan")
  LAST_BUF=$buf; LAST_EX=$ex
  printf '%-60s plan %7s ms  exec %8s ms  buffers %6s  %s\n' "$label" "$pl" "$ex" "$buf" "$(shape <<<"$plan")"
  if [ "$show" = yes ]; then
    printf '%s\n' "$plan" | sed 's/^/    /'
  fi
}

# ----------------------------------------------------------------- pgbench
round_trips() { # $1 script file in the container: statements are lines ending in ; or in \gset
  docker exec blobfs-postgres bash -c "grep -cE '(;|\\\\gset)[[:space:]]*\$' /tmp/$1"
}

# bench DB LABEL_A SCRIPT_A LABEL_B SCRIPT_B [LABEL_C SCRIPT_C]
bench() {
  local db=$1; shift
  local -a labels=() scripts=() fargs=()
  while [ $# -gt 0 ]; do labels+=("$1"); scripts+=("$2"); fargs+=(-f "/tmp/$2"); shift 2; done
  docker exec blobfs-postgres bash -c 'rm -f /tmp/m28log*'
  docker exec blobfs-postgres pgbench -U app -d "$db" "${fargs[@]}" -n -c 1 -t "$WARM_TXNS" >/dev/null 2>&1
  local out
  out=$(docker exec blobfs-postgres pgbench -U app -d "$db" "${fargs[@]}" -n -c 1 -t "$BENCH_TXNS" -r -l --log-prefix=/tmp/m28log 2>&1)
  if grep -qE 'ERROR|failed transactions: [1-9]' <<<"$out"; then
    echo "!! pgbench reported errors:"; grep -E 'ERROR|failed' <<<"$out" | head -5
  fi
  local i
  for i in "${!labels[@]}"; do
    local stats
    stats=$(docker exec blobfs-postgres bash -c "cat /tmp/m28log* | awk -v s=$i '\$4 == s {print \$3}' | sort -n | awk '{a[NR]=\$1} END {n=NR; if (n==0) {print \"0 0 0 0\"; exit}; m=(n%2)?a[(n+1)/2]:(a[n/2]+a[n/2+1])/2; printf \"%d %.3f %.3f %.3f\", n, m/1000, a[int(n*0.1)+1]/1000, a[int(n*0.9)]/1000}'")
    read -r n med p10 p90 <<<"$stats"
    printf '%-52s %2s round trips  median %8s ms  p10 %8s  p90 %8s  (n=%s)\n' "${labels[$i]}" "$(round_trips "${scripts[$i]}")" "$med" "$p10" "$p90" "$n"
  done
  echo "  per-statement average latency in ms (pgbench -r; the column after it is the failure count):"
  printf '%s\n' "$out" | grep -E '^SQL script|latency average|^[[:space:]]+[0-9]+\.[0-9]+[[:space:]]+[0-9]+[[:space:]]' | grep -vE '\\set' | sed 's/^/    /'
}
