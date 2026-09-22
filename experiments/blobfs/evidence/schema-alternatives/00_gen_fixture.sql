-- Generates the shared fixture, once, into a database of its own, so every
-- candidate design loads byte-identical rows.
--
-- Shape follows lib/blobfs/data/evidence_integration_test.go's seedForest:
-- three trees under the root, about one directory per ten files, 100,000
-- file rows, a tenth of them in one directory at depth two (the biggest),
-- depth six reached. The parent choice is a regular fan-out instead of the
-- Go generator's uniform random choice, so the fixture is reproducible from
-- SQL alone; the properties the measurements depend on (the directory
-- count, the depth, the biggest directory's size, one small directory at
-- the deepest level) are the same.
--
-- Unlike the shipped fixture, 1 file in 100 is pending and 1 in 200 is
-- deleting, so a status filter has something to select.

DROP TABLE IF EXISTS gen_dir;
DROP TABLE IF EXISTS gen_file;

-- Levels: 3 + 20 + 120 + 700 + 4000 + 5160 = 10003 directories, depth 1..6.
CREATE TABLE gen_dir (
  n         integer PRIMARY KEY,   -- 0-based generation index
  id        uuid NOT NULL,
  parent_n  integer,               -- NULL for a depth-one directory (parent is the root)
  depth     integer NOT NULL,
  name      text NOT NULL
);

WITH levels (depth, cnt, base) AS (
  VALUES (1, 3, 0), (2, 20, 3), (3, 120, 23), (4, 700, 143), (5, 4000, 843), (6, 5160, 4843)
),
nodes AS (
  SELECT l.depth,
         l.base + g.i AS n,
         g.i AS within,
         l.cnt AS cnt,
         l.base AS base
  FROM levels l
  CROSS JOIN LATERAL generate_series(0, l.cnt - 1) AS g(i)
)
INSERT INTO gen_dir (n, id, parent_n, depth, name)
SELECT n.n,
       -- a stable uuid per index, version-7-shaped in its first bytes so the
       -- ordering by id matches insertion order
       (
         '02d0' || lpad(to_hex(n.n), 4, '0') || '-7000-8000-9000-' ||
         lpad(to_hex(n.n), 12, '0')
       )::uuid,
       CASE WHEN n.depth = 1 THEN NULL
            ELSE (SELECT p.base FROM levels p WHERE p.depth = n.depth - 1)
                 + (n.within % (SELECT p.cnt FROM levels p WHERE p.depth = n.depth - 1))
       END,
       n.depth,
       CASE WHEN n.depth = 1 THEN 't' || n.within ELSE 'd' || (n.n - 3) END
FROM nodes n;

-- The biggest directory is gen_dir 3 ("d0", the first depth-two directory),
-- matching the shipped fixture's /t2/d0 at depth two.

CREATE TABLE gen_file (
  n           integer PRIMARY KEY,
  id          uuid NOT NULL,
  dir_n       integer NOT NULL,
  name        text NOT NULL,
  key         text NOT NULL,
  size        bigint,
  status      text NOT NULL,
  content_type text NOT NULL,
  created_at  timestamptz NOT NULL
);

INSERT INTO gen_file (n, id, dir_n, name, key, size, status, content_type, created_at)
SELECT i,
       ('01f' || lpad(to_hex(i), 5, '0') || '-7000-8000-9000-' || lpad(to_hex(i), 12, '0'))::uuid,
       CASE WHEN i % 10 = 0 THEN 3 ELSE (i * 7919) % 10003 END,
       'f' || i || '.txt',
       ('01f' || lpad(to_hex(i), 5, '0') || '-7000-8000-9000-' || lpad(to_hex(i), 12, '0')) || '/f' || i || '.txt',
       CASE WHEN i % 100 = 0 THEN NULL ELSE i % 4096 END,
       CASE WHEN i % 100 = 0 THEN 'pending'
            WHEN i % 200 = 137 THEN 'deleting'
            ELSE 'available' END,
       'text/plain',
       TIMESTAMPTZ '2025-09-21 00:00:00+00' + ((i * 3137) % 31536000) * INTERVAL '1 second'
FROM generate_series(0, 99999) AS g(i);

ANALYZE gen_dir;
ANALYZE gen_file;
