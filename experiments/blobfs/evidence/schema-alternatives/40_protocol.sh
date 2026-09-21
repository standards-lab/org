#!/usr/bin/env bash
# The transaction protocol between the database record and the object store,
# measured as round trips and as latency.
#
# One pgbench script per design runs a whole file's life as the library
# sequences it at the standard tier: the write's begin step (the pending row
# and its read-back), the write's complete step (the guarded update and its
# read-back), the delete's begin step (the update to deleting and its
# read-back, which the baseline runs in one transaction), and the delete's
# complete step (the removal). A second script runs a directory's life:
# create, read back, remove.
#
# Every statement pgbench sends is one round trip, and an explicit BEGIN and
# COMMIT are round trips too, so the transaction count pgbench reports is the
# round-trip count of the protocol.
#
# The parent directory is the small one (9 files at depth 6), so the
# measurement does not change the biggest directory's statistics.
set -euo pipefail
S="$(cd "$(dirname "$0")" && pwd)"
OUT="$S/protocol.txt"
PARENT='02d013ca-7000-8000-9000-0000000013ca'
SECONDS_PER_RUN=${SECONDS_PER_RUN:-10}

mk() { docker exec -i blobfs-postgres bash -c "cat > /tmp/$1" ; }

# ---------------------------------------------------------------- design A/B
# The two differ only in the table name and the kind predicate, so the scripts
# are generated from one template.
gen_flat() { # $1 table  $2 parent column  $3 extra insert cols  $4 extra insert vals  $5 kind predicate
  local T=$1 P=$2 XC=$3 XV=$4 KP=$5
  cat <<EOF
\set n random(1, 1000000000000)
-- write begin: one insert and one read-back, both on the pool
INSERT INTO $T (id, $P, name$XC, status, key, content_type)
VALUES (CAST(lpad(to_hex(:n), 32, '0') AS uuid), CAST('$PARENT' AS uuid), 'b' || :n || '.txt'$XV,
        'pending', lpad(to_hex(:n), 32, '0') || '/b' || :n || '.txt', 'text/plain');
SELECT f.id, f.$P, f.name, f.status, f.key, f.size, f.content_type, f.etag, f.version, f.created_at, f.updated_at
FROM $T f WHERE f.id = CAST(lpad(to_hex(:n), 32, '0') AS uuid)$KP;
-- write complete: the guarded update and its read-back, both on the pool
UPDATE $T SET status = 'available', size = 1024, content_type = 'text/plain', etag = 'e' || :n,
    updated_at = CURRENT_TIMESTAMP, version = version + 1
WHERE id = CAST(lpad(to_hex(:n), 32, '0') AS uuid) AND version = 1 AND status = 'pending';
SELECT f.id, f.$P, f.name, f.status, f.key, f.size, f.content_type, f.etag, f.version, f.created_at, f.updated_at
FROM $T f WHERE f.id = CAST(lpad(to_hex(:n), 32, '0') AS uuid)$KP;
-- delete begin: the baseline's two statements, which must share a transaction
BEGIN;
UPDATE $T SET status = 'deleting', version = version + 1, updated_at = CURRENT_TIMESTAMP
WHERE id = CAST(lpad(to_hex(:n), 32, '0') AS uuid) AND status <> 'deleting';
SELECT f.id, f.$P, f.name, f.status, f.key, f.size, f.content_type, f.etag, f.version, f.created_at, f.updated_at
FROM $T f WHERE f.id = CAST(lpad(to_hex(:n), 32, '0') AS uuid)$KP;
COMMIT;
-- delete complete: one statement on the pool
DELETE FROM $T WHERE id = CAST(lpad(to_hex(:n), 32, '0') AS uuid) AND status = 'deleting';
EOF
}

gen_flat blobfs_file directory_id "" "" "" | mk file_a.sql
gen_flat blobfs_entry parent_id ", kind" ", 'file'" " AND f.kind = 'file'" | mk file_b.sql

# ------------------------------------------------------------------ design C
gen_sub() {
  cat <<EOF
\set n random(1, 1000000000000)
-- write begin: the node row, the file row, and the read-back, in one
-- transaction because two statements must land together
BEGIN;
INSERT INTO blobfs_node (id, kind, parent_id, name)
VALUES (CAST(lpad(to_hex(:n), 32, '0') AS uuid), 'file', CAST('$PARENT' AS uuid), 'b' || :n || '.txt');
INSERT INTO blobfs_file (node_id, status, key, content_type)
VALUES (CAST(lpad(to_hex(:n), 32, '0') AS uuid), 'pending', lpad(to_hex(:n), 32, '0') || '/b' || :n || '.txt', 'text/plain');
SELECT n.id, n.parent_id, n.name, f.status, f.key, f.size, f.content_type, f.etag, n.version, n.created_at, n.updated_at
FROM blobfs_node n JOIN blobfs_file f ON f.node_id = n.id WHERE n.id = CAST(lpad(to_hex(:n), 32, '0') AS uuid);
COMMIT;
-- write complete: the version guard is on the node, the columns that change
-- are on the file, so the step is two updates and a read-back in one
-- transaction
BEGIN;
UPDATE blobfs_node SET updated_at = CURRENT_TIMESTAMP, version = version + 1
WHERE id = CAST(lpad(to_hex(:n), 32, '0') AS uuid) AND version = 1;
UPDATE blobfs_file SET status = 'available', size = 1024, content_type = 'text/plain', etag = 'e' || :n
WHERE node_id = CAST(lpad(to_hex(:n), 32, '0') AS uuid) AND status = 'pending';
SELECT n.id, n.parent_id, n.name, f.status, f.key, f.size, f.content_type, f.etag, n.version, n.created_at, n.updated_at
FROM blobfs_node n JOIN blobfs_file f ON f.node_id = n.id WHERE n.id = CAST(lpad(to_hex(:n), 32, '0') AS uuid);
COMMIT;
-- delete begin
BEGIN;
UPDATE blobfs_node SET updated_at = CURRENT_TIMESTAMP, version = version + 1
WHERE id = CAST(lpad(to_hex(:n), 32, '0') AS uuid)
  AND EXISTS (SELECT 1 FROM blobfs_file f WHERE f.node_id = blobfs_node.id AND f.status <> 'deleting');
UPDATE blobfs_file SET status = 'deleting' WHERE node_id = CAST(lpad(to_hex(:n), 32, '0') AS uuid) AND status <> 'deleting';
SELECT n.id, n.parent_id, n.name, f.status, f.key, f.size, f.content_type, f.etag, n.version, n.created_at, n.updated_at
FROM blobfs_node n JOIN blobfs_file f ON f.node_id = n.id WHERE n.id = CAST(lpad(to_hex(:n), 32, '0') AS uuid);
COMMIT;
-- delete complete: the subtype row first, then the node row
BEGIN;
DELETE FROM blobfs_file WHERE node_id = CAST(lpad(to_hex(:n), 32, '0') AS uuid) AND status = 'deleting';
DELETE FROM blobfs_node WHERE id = CAST(lpad(to_hex(:n), 32, '0') AS uuid) AND kind = 'file';
COMMIT;
EOF
}
gen_sub | mk file_c.sql

# ------------------------------------------------------ directory lifecycles
cat <<EOF | mk dir_a.sql
\set n random(1, 1000000000000)
INSERT INTO blobfs_directory (id, parent_id, name)
VALUES (CAST(lpad(to_hex(:n), 32, '0') AS uuid), CAST('$PARENT' AS uuid), 'g' || :n);
SELECT d.id, d.parent_id, d.name, d.version, d.created_at, d.updated_at
FROM blobfs_directory d WHERE d.id = CAST(lpad(to_hex(:n), 32, '0') AS uuid);
DELETE FROM blobfs_directory WHERE id = CAST(lpad(to_hex(:n), 32, '0') AS uuid) AND parent_id IS NOT NULL;
EOF

cat <<EOF | mk dir_b.sql
\set n random(1, 1000000000000)
INSERT INTO blobfs_entry (id, kind, parent_id, name)
VALUES (CAST(lpad(to_hex(:n), 32, '0') AS uuid), 'directory', CAST('$PARENT' AS uuid), 'g' || :n);
SELECT d.id, d.parent_id, d.name, d.version, d.created_at, d.updated_at
FROM blobfs_entry d WHERE d.id = CAST(lpad(to_hex(:n), 32, '0') AS uuid) AND d.kind = 'directory';
DELETE FROM blobfs_entry WHERE id = CAST(lpad(to_hex(:n), 32, '0') AS uuid) AND kind = 'directory' AND parent_id IS NOT NULL;
EOF

cat <<EOF | mk dir_c.sql
\set n random(1, 1000000000000)
BEGIN;
INSERT INTO blobfs_node (id, kind, parent_id, name)
VALUES (CAST(lpad(to_hex(:n), 32, '0') AS uuid), 'directory', CAST('$PARENT' AS uuid), 'g' || :n);
INSERT INTO blobfs_directory (node_id) VALUES (CAST(lpad(to_hex(:n), 32, '0') AS uuid));
SELECT d.id, d.parent_id, d.name, d.version, d.created_at, d.updated_at
FROM blobfs_node d WHERE d.id = CAST(lpad(to_hex(:n), 32, '0') AS uuid) AND d.kind = 'directory';
COMMIT;
BEGIN;
DELETE FROM blobfs_directory WHERE node_id = CAST(lpad(to_hex(:n), 32, '0') AS uuid);
DELETE FROM blobfs_node WHERE id = CAST(lpad(to_hex(:n), 32, '0') AS uuid) AND parent_id IS NOT NULL;
COMMIT;
EOF

cat <<EOF | mk dir_c2.sql
\set n random(1, 1000000000000)
INSERT INTO blobfs_node (id, kind, parent_id, name)
VALUES (CAST(lpad(to_hex(:n), 32, '0') AS uuid), 'directory', CAST('$PARENT' AS uuid), 'g' || :n);
SELECT d.id, d.parent_id, d.name, d.version, d.created_at, d.updated_at
FROM blobfs_node d WHERE d.id = CAST(lpad(to_hex(:n), 32, '0') AS uuid) AND d.kind = 'directory';
DELETE FROM blobfs_node WHERE id = CAST(lpad(to_hex(:n), 32, '0') AS uuid) AND kind = 'directory' AND parent_id IS NOT NULL;
EOF

# ------------------------------------------------------------ the file moves
cat <<EOF | mk mvfile_a.sql
\set n random(1, 10000)
UPDATE blobfs_file SET directory_id = CAST('$PARENT' AS uuid), name = 'm' || :n || '.txt',
    updated_at = CURRENT_TIMESTAMP, version = version + 1
WHERE id = CAST(lpad(to_hex(:n), 32, '0') AS uuid) AND version = 1 AND status <> 'deleting';
SELECT f.id, f.directory_id, f.name, f.status, f.key, f.size, f.content_type, f.etag, f.version, f.created_at, f.updated_at
FROM blobfs_file f WHERE f.id = CAST(lpad(to_hex(:n), 32, '0') AS uuid);
EOF

run() { # $1 db  $2 script  $3 label  $4 round trips
  local tps lat
  out=$(docker exec blobfs-postgres pgbench -U app -d "$1" -f "/tmp/$2" -n -c 1 -T "$SECONDS_PER_RUN" -r 2>&1)
  tps=$(printf '%s\n' "$out" | grep -E '^tps' | head -1 | awk '{print $3}')
  lat=$(printf '%s\n' "$out" | grep 'latency average' | awk '{print $4}')
  printf '%-34s %-18s %6s round trips  %10s ms/cycle  %10s cycles/s\n' "$3" "$1" "$4" "$lat" "$tps"
  printf '%s\n' "$out" | grep -E 'number of failed|ERROR' | head -3
}

{
echo "# The protocol between the database record and the object store: round trips"
echo "# and latency per whole lifecycle, one client, ${SECONDS_PER_RUN} s per run, pgbench inside the"
echo "# container so the client and the server share a host. Every statement, and"
echo "# every explicit BEGIN and COMMIT, is one round trip."
echo "#"
echo "# The file lifecycle is the four steps the consumer sequences around the two"
echo "# object-store calls: write begin, write complete, delete begin, delete"
echo "# complete. The measurement omits the path resolution, which is the same in"
echo "# every design, and the object-store calls, which the library never makes."
echo ""
run blobfs_opus_bench_a file_a.sql  "file lifecycle, design A" 9
run blobfs_opus_bench_b file_b.sql  "file lifecycle, design B" 9
run blobfs_opus_bench_c file_c.sql  "file lifecycle, design C" 19
run blobfs_opus_bench_c2 file_c.sql "file lifecycle, design C2" 19
echo ""
run blobfs_opus_bench_a dir_a.sql   "directory lifecycle, design A" 3
run blobfs_opus_bench_b dir_b.sql   "directory lifecycle, design B" 3
run blobfs_opus_bench_c dir_c.sql   "directory lifecycle, design C" 9
run blobfs_opus_bench_c2 dir_c2.sql "directory lifecycle, design C2" 3
} | tee "$OUT"
