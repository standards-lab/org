--| tier: native
--| native: UPDATE ... RETURNING, so a completed write is one round trip and a refused one two, where the baseline's guard runs two and three. Port: an engine must return the updated row from the update statement (SQL Server's OUTPUT INSERTED, Oracle's RETURNING INTO, SQLite's RETURNING) or fall back to the standard baseline's guarded update and read by id.
-- The last step of a file write in one statement: moves the pending row
-- to available with the size, content type, and entity tag the store
-- reported, advances the version, stamps updated_at, and returns the row
-- as it now stands. The SET list and the predicate are the standard
-- statement's with the query library's guard patterns spelled out: the id
-- and the version the caller read from the pending row, and the status
-- predicate that keeps a row no longer pending unchanged. No row returned
-- means the update changed none, and the variant reads the row once to
-- tell a missing row, a version conflict, and a status refusal apart.
UPDATE blobfs_file AS f
SET status = 'available',
    size = {{size:bigint}},
    content_type = {{content_type}},
    etag = {{etag}},
    updated_at = CURRENT_TIMESTAMP, version = f.version + 1
WHERE f.id = {{id:uuid}} AND f.version = {{version:bigint}} AND f.status = 'pending'
RETURNING {{> blobfs.file_columns}}
