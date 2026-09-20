--| tier: standard
-- The guarded rename: the new name, the protocol columns advanced, under
-- the id and expected version.
UPDATE blobfs_volume
SET name = {{name}}, {{> sql.guard_set}}
WHERE {{> sql.guard_where}}
