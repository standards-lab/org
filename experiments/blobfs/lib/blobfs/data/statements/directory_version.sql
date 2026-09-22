--| tier: standard
-- The guard's check for reparent_directory: the current version of the
-- directory with id, or no row. The query library runs it when the
-- guarded command affected no row, to tell a missing row from a version
-- conflict.
SELECT d.version
FROM blobfs_directory d
WHERE d.id = {{id:uuid}}
