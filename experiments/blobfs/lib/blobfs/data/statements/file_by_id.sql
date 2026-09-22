--| tier: standard
-- One file row by id, in the column order blobfs.File scans. The standard
-- baseline's file-delete begin reads the row back through it after its
-- update, and File reads through it on its own.
SELECT {{> blobfs.file_columns}}
FROM blobfs_file f
WHERE f.id = {{id:uuid}}
