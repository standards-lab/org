--| tier: standard
-- The first step of a file write: inserts the row as pending, before the
-- object exists, with the key the object will be stored under and the
-- content type the caller declared. The id is minted in Go and the key is
-- built from it and validated against the store before this runs. size and
-- etag stay NULL until the write completes; version and the timestamps
-- take the table's defaults and are read back by file_by_id. A directory
-- that does not exist fails the foreign key blobfs_fk_file_directory and a
-- taken name the unique constraint blobfs_uq_file_directory_name. The
-- statement accepts the pool and composes into a caller's transaction
-- alike, so a consumer writes its own rows in the same unit as the pending
-- row.
INSERT INTO blobfs_file (id, directory_id, name, status, key, content_type)
VALUES ({{id:uuid}}, {{directory_id:uuid}}, {{name}}, 'pending', {{key}}, {{content_type}})
