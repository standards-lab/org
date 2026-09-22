--| tier: native
--| native: INSERT ... RETURNING, so the begin step is one round trip and needs no read-back. Port: an engine must return the inserted row with the defaults the table filled (SQL Server's OUTPUT INSERTED, Oracle's RETURNING INTO, SQLite's RETURNING) or fall back to the standard baseline's insert and read by id.
-- The first step of a file write in one statement: inserts the row as
-- pending, before the object exists, and returns it as the database
-- holds it, the version and the timestamps the table defaulted included.
-- The columns and values are the standard statement's; only the
-- correlation name f, which the published column list qualifies every
-- column with, and the RETURNING clause are added. A directory that does
-- not exist fails the foreign key blobfs_fk_file_directory and a taken
-- name the unique constraint blobfs_uq_file_directory_name, and the
-- store classifies both as the baseline's are.
INSERT INTO blobfs_file AS f (id, directory_id, name, status, key, content_type)
VALUES ({{id:uuid}}, {{directory_id:uuid}}, {{name}}, 'pending', {{key}}, {{content_type}})
RETURNING {{> blobfs.file_columns}}
