--| tier: standard
SELECT {{> blobfs.volume_columns}}
FROM blobfs_volume v
WHERE v.id = {{id:uuid}}
