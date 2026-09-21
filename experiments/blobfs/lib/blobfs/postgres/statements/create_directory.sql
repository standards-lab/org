--| tier: native
--| native: INSERT ... RETURNING, so Mkdir is one round trip and needs no read-back. Port: an engine must return the inserted row with the defaults the table filled (SQL Server's OUTPUT INSERTED, Oracle's RETURNING INTO, SQLite's RETURNING) or fall back to the standard baseline's insert and read by id.
-- Inserts a non-root directory under its parent in one statement and
-- returns it as the database holds it, the version and the timestamps
-- the table defaulted included. The columns and values are the standard
-- statement's; only the correlation name d, which the published column
-- list qualifies every column with, and the RETURNING clause are added.
-- A missing parent fails the foreign key blobfs_fk_directory_parent and
-- a taken name the unique constraint blobfs_uq_directory_parent_name,
-- and the store classifies both as the baseline's are.
INSERT INTO blobfs_directory AS d (id, parent_id, name)
VALUES ({{id:uuid}}, {{parent_id:uuid}}, {{name}})
RETURNING {{> blobfs.directory_columns}}
