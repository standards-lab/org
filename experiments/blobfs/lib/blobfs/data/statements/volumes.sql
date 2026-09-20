--| tier: standard
--| key: id
--| field: id uuid
--| field: name text
--| field: version bigint
--| field: created_at timestamp with time zone
--| field: updated_at timestamp with time zone
-- The volume listing's base: every volume, paged, sorted, and filtered by
-- the caller's directives over the declared fields.
SELECT {{> blobfs.volume_columns}}
FROM blobfs_volume v
