--| tier: standard
-- The last step of a file delete: removes the row, and only a row that is
-- deleting, so a row whose delete has not begun is left as it is and the
-- caller reads it again to tell that refusal from a row that is already
-- gone. It is one statement and accepts the pool; the same statement
-- serves every variant, since only the begin step varies. A foreign key
-- that references blobfs_file from a consumer's table refuses the removal
-- while the consumer's row exists; blobfs owns no such key, so a
-- foreign-key violation here is always a consumer's constraint and is
-- reported as ErrReferenced with the constraint's name reachable.
DELETE FROM blobfs_file
WHERE id = {{id:uuid}} AND status = 'deleting'
