--| tier: standard
-- The name is compared as stored; the caller normalizes it first.
SELECT {{> blobfs.volume_columns}}
FROM blobfs_volume v
WHERE v.name = {{name}}
