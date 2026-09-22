--| tier: standard
--| transaction: required
-- The first step of a file delete on the standard baseline: moves the row
-- to deleting, advances its version, and stamps updated_at. A row that is
-- already deleting is left as it is, so a retry changes nothing and the
-- version advances once per delete. The baseline reads the row back with
-- file_by_id in the same transaction, which is why a transaction is
-- required: the row lock this update takes holds until commit, so the
-- read-back sees the row as this statement left it and not a row a
-- concurrent complete step removed in between. A row that does not exist
-- affects nothing, and the read-back reports it as not found.
UPDATE blobfs_file
SET status = 'deleting', version = version + 1, updated_at = CURRENT_TIMESTAMP
WHERE id = {{id:uuid}} AND status <> 'deleting'
