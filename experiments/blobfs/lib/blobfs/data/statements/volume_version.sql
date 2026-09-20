--| tier: standard
-- The guard's check: the volume's current version, or no row.
SELECT version FROM blobfs_volume WHERE id = {{id}}
