--| tier: standard
--| key: name
--| field: id uuid
--| field: parent_id uuid
--| field: name text
--| field: version bigint
--| field: created_at timestamp with time zone
--| field: updated_at timestamp with time zone
-- The directories under one parent, without a total: Children under
-- TotalNone. The composer appends the caller's predicates with AND, then
-- ORDER BY and the paging clause, so the statement ends with its WHERE
-- clause. The correlation name is q because the clause patterns the
-- composer fills qualify every field as q.<field>. name is the key:
-- (parent_id, name) is unique, so a sort by name is total and
-- index-ordered in either direction. The root has no parent, so it never
-- appears in any listing.
SELECT q.id, q.parent_id, q.name, q.version, q.created_at, q.updated_at
FROM blobfs_directory q
WHERE q.parent_id = {{parent_id:uuid}}
