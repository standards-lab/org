--| tier: native
--| native: UPDATE ... RETURNING, so the begin step is one round trip and needs no read-back. Port: an engine must return the updated row from the update statement (SQL Server's OUTPUT INSERTED, Oracle's RETURNING INTO, SQLite's RETURNING) or fall back to the standard baseline's two statements in one transaction.
-- The first step of a file delete in one statement: moves the row to
-- deleting, advances its version and stamps updated_at on the first begin
-- only, and returns the row as it now stands. The CASE expressions read the
-- old status, so a row that is already deleting keeps its version and
-- updated_at and is returned unchanged; a retry therefore converges to the
-- same row. A row that does not exist returns nothing, which the variant
-- reports as not found. The correlation name is f because the published
-- column list qualifies every column as f.<column>.
UPDATE blobfs_file AS f
SET status = 'deleting',
    version = CASE WHEN f.status = 'deleting' THEN f.version ELSE f.version + 1 END,
    updated_at = CASE WHEN f.status = 'deleting' THEN f.updated_at ELSE CURRENT_TIMESTAMP END
WHERE f.id = {{id:uuid}}
RETURNING {{> blobfs.file_columns}}
