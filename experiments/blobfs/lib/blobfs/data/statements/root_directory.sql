--| tier: standard
-- A volume's root directory, found through the back-reference: the one
-- directory row that carries the volume's id. The volume row holds no
-- forward reference to its root.
SELECT {{> blobfs.directory_columns}}
FROM blobfs_directory d
WHERE d.volume_id = {{volume_id:uuid}}
