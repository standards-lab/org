#!/usr/bin/env bash
# Builds one scratch database per candidate design and loads the shared
# fixture from blobfs_opus_gen into it. Every design gets byte-identical
# rows: the same uuids, names, keys, sizes, statuses and creation times.
set -euo pipefail
S="$(cd "$(dirname "$0")" && pwd)"
PSQL() { docker exec -i blobfs-postgres psql -U app -v ON_ERROR_STOP=1 "$@"; }
ADMIN() { docker exec blobfs-postgres psql -U app -d postgres -c "$1"; }

ROOT='00000000-0000-0000-0000-000000000000'

for d in a b c; do
  db="blobfs_opus_$d"
  ADMIN "DROP DATABASE IF EXISTS $db" >/dev/null
  ADMIN "CREATE DATABASE $db" >/dev/null
  PSQL -d "$db" -f - < "$S/10_ddl_$d.sql" >/dev/null
  # the generator tables travel as plain text through the client
  docker exec blobfs-postgres psql -U app -d blobfs_opus_gen -c "\copy gen_dir TO STDOUT" > /tmp/gd.tsv
  docker exec blobfs-postgres psql -U app -d blobfs_opus_gen -c "\copy gen_file TO STDOUT" > /tmp/gf.tsv
  PSQL -d "$db" -c "CREATE TABLE gen_dir (n integer PRIMARY KEY, id uuid NOT NULL, parent_n integer, depth integer NOT NULL, name text NOT NULL)" >/dev/null
  PSQL -d "$db" -c "CREATE TABLE gen_file (n integer PRIMARY KEY, id uuid NOT NULL, dir_n integer NOT NULL, name text NOT NULL, key text NOT NULL, size bigint, status text NOT NULL, content_type text NOT NULL, created_at timestamptz NOT NULL)" >/dev/null
  PSQL -d "$db" -c "\copy gen_dir FROM STDIN" < /tmp/gd.tsv >/dev/null
  PSQL -d "$db" -c "\copy gen_file FROM STDIN" < /tmp/gf.tsv >/dev/null
done
rm -f /tmp/gd.tsv /tmp/gf.tsv

# ---- design A -------------------------------------------------------------
PSQL -d blobfs_opus_a <<SQL >/dev/null
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
SQL

# ---- design B -------------------------------------------------------------
PSQL -d blobfs_opus_b <<SQL >/dev/null
INSERT INTO blobfs_entry (id, kind, parent_id, name, created_at, updated_at)
SELECT g.id, 'directory', COALESCE(p.id, '$ROOT'::uuid), g.name,
       TIMESTAMPTZ '2025-09-21 00:00:00+00', TIMESTAMPTZ '2025-09-21 00:00:00+00'
FROM gen_dir g LEFT JOIN gen_dir p ON p.n = g.parent_n
ORDER BY g.depth, g.n;

INSERT INTO blobfs_entry (id, kind, parent_id, name, status, key, size, content_type, etag, created_at, updated_at)
SELECT f.id, 'file', d.id, f.name, f.status, f.key, f.size, f.content_type,
       CASE WHEN f.status = 'available' THEN 'etag-' || f.n END,
       f.created_at, f.created_at
FROM gen_file f JOIN gen_dir d ON d.n = f.dir_n;
SQL

# ---- design C -------------------------------------------------------------
# The directory half must be loaded one depth at a time: blobfs_node's
# parent key names blobfs_directory, so a level's node rows cannot be
# inserted before the level above has its subtype rows. That ordering
# requirement is design C's, not the loader's.
PSQL -d blobfs_opus_c <<SQL >/dev/null
DO \$\$
DECLARE d integer;
BEGIN
  FOR d IN 1..6 LOOP
    INSERT INTO blobfs_node (id, kind, parent_id, name, created_at, updated_at)
    SELECT g.id, 'directory', COALESCE(p.id, '$ROOT'::uuid), g.name,
           TIMESTAMPTZ '2025-09-21 00:00:00+00', TIMESTAMPTZ '2025-09-21 00:00:00+00'
    FROM gen_dir g LEFT JOIN gen_dir p ON p.n = g.parent_n
    WHERE g.depth = d;
    INSERT INTO blobfs_directory (node_id) SELECT g.id FROM gen_dir g WHERE g.depth = d;
  END LOOP;
END
\$\$;

INSERT INTO blobfs_node (id, kind, parent_id, name, created_at, updated_at)
SELECT f.id, 'file', d.id, f.name, f.created_at, f.created_at
FROM gen_file f JOIN gen_dir d ON d.n = f.dir_n;
INSERT INTO blobfs_file (node_id, status, key, size, content_type, etag)
SELECT f.id, f.status, f.key, f.size, f.content_type,
       CASE WHEN f.status = 'available' THEN 'etag-' || f.n END
FROM gen_file f;
SQL

# ---- the consumer's rows, identical in all three ---------------------------
# 200 units; unit u owns top-level directory (u mod 3); 53,110 bookmark rows
# is the scale evidence/bookmarks.txt used, so the same shape is seeded here:
# every file whose index mod 2 = 0 is bookmarked by unit (n mod 200).
for d in a b c; do
  PSQL -d "blobfs_opus_$d" <<SQL >/dev/null
INSERT INTO directory_owner (directory_id, unit_id)
SELECT g.id, ('01b0' || lpad(to_hex(g.n), 4, '0') || '-7000-8000-9000-' || lpad(to_hex(g.n), 12, '0'))::uuid
FROM gen_dir g WHERE g.depth = 1;

INSERT INTO bookmark (unit_id, file_id, active)
SELECT ('01b0' || lpad(to_hex(f.n % 200), 4, '0') || '-7000-8000-9000-' || lpad(to_hex(f.n % 200), 12, '0'))::uuid,
       f.id, false
FROM gen_file f WHERE f.n % 2 = 0 AND f.status <> 'deleting';
SQL
done

for d in a b c; do
  db="blobfs_opus_$d"
  PSQL -d "$db" -c "DROP TABLE gen_dir; DROP TABLE gen_file;" >/dev/null
  docker exec blobfs-postgres psql -U app -d "$db" -c "VACUUM ANALYZE" >/dev/null
  echo "== $db"
  docker exec blobfs-postgres psql -U app -d "$db" -tAc "SELECT relname, n_live_tup FROM pg_stat_user_tables ORDER BY relname"
done
