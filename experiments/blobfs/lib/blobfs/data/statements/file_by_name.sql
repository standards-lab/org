--| tier: standard
-- One file row by its directory and name, in the column order blobfs.File
-- scans, or no row. (directory_id, name) is unique, so at most one row
-- matches. The caller normalizes the name first. It is the read behind
-- FileByName, which resolves the last segment of a file's path and finds
-- the pending row a retried write resumes.
SELECT {{> blobfs.file_columns}}
FROM blobfs_file f
WHERE f.directory_id = {{directory_id:uuid}} AND f.name = {{name}}
