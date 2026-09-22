--| tier: standard
-- Inserts a non-root directory under its parent. The id is minted in Go;
-- version and the timestamps take the table's defaults and are read back
-- by directory_by_id. A missing parent fails the foreign key
-- blobfs_fk_directory_parent and a taken name the unique constraint
-- blobfs_uq_directory_parent_name.
INSERT INTO blobfs_directory (id, parent_id, name)
VALUES ({{id:uuid}}, {{parent_id:uuid}}, {{name}})
