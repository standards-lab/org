--| tier: standard
--| key: id
--| field: id uuid
--| field: parent_id uuid
--| field: name text
--| field: version bigint
--| field: created_at timestamp with time zone
--| field: updated_at timestamp with time zone
-- The directory listing's base: every directory, paged, sorted, and
-- filtered by the caller's directives. Children appends the parent_id
-- filter, so a listing is always scoped to one parent.
SELECT {{> blobfs.directory_columns}}
FROM blobfs_directory d
