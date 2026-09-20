--| tier: standard
--| key: name
--| field: id uuid
--| field: parent_id uuid
--| field: name text
--| field: version bigint
--| field: created_at timestamp with time zone
--| field: updated_at timestamp with time zone
--| field: unit_id uuid
-- The consumer's owner read model: every directory that has an owner row,
-- with the unit that owns it, in the order OwnedDirectory scans them. It is
-- the projection ls / --unit lists through, filtered by unit_id. The join
-- is an inner join, so a directory without an owner row is not listed. The
-- owner rows are written for depth-one directories only, so every row's
-- parent is the root and name is the key: (parent_id, name) is unique.
SELECT {{> blobfs.directory_columns}}, o.unit_id
FROM blobfs_directory d
JOIN directory_owner o ON o.directory_id = d.id
