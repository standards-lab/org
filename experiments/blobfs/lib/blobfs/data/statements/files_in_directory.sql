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
-- The file listing of one directory, without a total: ListFiles under
-- TotalNone. The composer appends the caller's predicates with AND, then
-- ORDER BY and the paging clause, to this text as it stands, so the
-- statement ends with its WHERE clause. The correlation name is q because
-- the clause patterns the composer fills qualify every field as q.<field>.
-- name is the key: (directory_id, name) is unique, so a sort by name is
-- total and index-ordered in either direction.
SELECT q.id, q.directory_id, q.name, q.status, q.key, q.size, q.content_type, q.etag, q.version, q.created_at, q.updated_at
FROM blobfs_file q
WHERE q.directory_id = {{directory_id:uuid}}
