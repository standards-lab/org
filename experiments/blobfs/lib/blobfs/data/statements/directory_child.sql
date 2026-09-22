--| tier: standard
-- One step of path resolution: the child of parent_id named name. The
-- caller normalizes the name first.
SELECT {{> blobfs.directory_columns}}
FROM blobfs_directory d
WHERE d.parent_id = {{parent_id:uuid}} AND d.name = {{name}}
