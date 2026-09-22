--| tier: standard
--| key: name
--| field: id uuid
--| field: directory_id uuid
--| field: name text
--| field: status text
--| field: size bigint
--| field: content_type text
--| field: etag text
--| field: version bigint
--| field: created_at timestamp with time zone
--| field: updated_at timestamp with time zone
-- The file listing of one directory with its total: ListFiles under
-- TotalExact. The total is the window count over the rows the WHERE clause
-- keeps, computed in the same statement as the page, so it cannot disagree
-- with the page under any isolation level. The composer appends the
-- caller's predicates with AND before the window is evaluated, then ORDER
-- BY and the paging clause; the statement ends with its WHERE clause. The
-- correlation name is q because the clause patterns the composer fills
-- qualify every field as q.<field>.
SELECT q.id, q.directory_id, q.name, q.status, q.key, q.size, q.content_type, q.etag, q.version, q.created_at, q.updated_at, COUNT(*) OVER () AS total
FROM blobfs_file q
WHERE q.directory_id = {{directory_id:uuid}}
