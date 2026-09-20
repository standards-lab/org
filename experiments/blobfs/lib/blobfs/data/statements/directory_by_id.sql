--| tier: standard
SELECT {{> blobfs.directory_columns}}
FROM blobfs_directory d
WHERE d.id = {{id:uuid}}
