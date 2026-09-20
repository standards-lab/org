--| tier: standard
--| key: id
--| field: id uuid
--| field: name text
--| field: version bigint
--| field: created_at timestamp with time zone
--| field: updated_at timestamp with time zone
--| field: unit_id uuid
-- The consumer's volume read model: every volume with the unit that owns
-- it, in the order VolumeEntry scans them. The owner join is an inner join,
-- so a volume without an owner row is not listed; the consumer creates the
-- two rows together. --unit is a filter on unit_id.
SELECT {{> blobfs.volume_columns}}, o.unit_id
FROM blobfs_volume v
JOIN volume_owner o ON o.volume_id = v.id
