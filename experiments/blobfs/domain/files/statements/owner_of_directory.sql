--| tier: standard
-- The ownership row of one directory, in the order DirectoryOwner scans
-- it, or no row. The scope check reads it once, for the depth-one ancestor
-- of the listed path.
SELECT o.directory_id, o.unit_id, o.version, o.created_at, o.updated_at
FROM directory_owner o
WHERE o.directory_id = {{directory_id:uuid}}
