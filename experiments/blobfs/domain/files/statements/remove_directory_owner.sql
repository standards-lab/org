--| tier: standard
--| transaction: required
-- Removes the consumer's ownership row of a directory, if it has one. It
-- requires a transaction because the row goes in the same unit as the
-- directory it binds: rmdir removes the owner row and then the directory,
-- and a directory the library refuses to remove keeps its owner row when
-- the transaction rolls back. No row affected means the directory had no
-- owner, which is not an error.
DELETE FROM directory_owner
WHERE directory_id = {{directory_id:uuid}}
