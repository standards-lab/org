--| tier: standard
-- The guard's check for complete_file_write: the current version of the
-- file with id, or no row. The query library runs it when the guarded
-- command affected no row, to tell a missing row from a version conflict.
SELECT f.version
FROM blobfs_file f
WHERE f.id = {{id:uuid}}
