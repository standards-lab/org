--| tier: standard
--| key: name
--| field: id uuid
--| field: parent_id uuid
--| field: name text
--| field: version bigint
--| field: created_at timestamp with time zone
--| field: updated_at timestamp with time zone
-- The directories under one parent with their total: Children under
-- TotalExact. The total is the window count over the rows the WHERE clause
-- keeps, computed in the same statement as the page, so it cannot disagree
-- with the page under any isolation level. The composer appends the
-- caller's predicates with AND before the window is evaluated, then ORDER
-- BY and the paging clause; the statement ends with its WHERE clause. The
-- correlation name is q because the clause patterns the composer fills
-- qualify every field as q.<field>.
SELECT q.id, q.parent_id, q.name, q.version, q.created_at, q.updated_at, COUNT(*) OVER () AS total
FROM blobfs_directory q
WHERE q.parent_id = {{parent_id:uuid}}
