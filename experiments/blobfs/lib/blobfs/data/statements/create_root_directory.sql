--| tier: standard
--| transaction: required
-- Inserts a volume's root directory: no parent, no name, the volume's id.
-- It requires a transaction for the same reason create_volume does.
INSERT INTO blobfs_directory (id, volume_id)
VALUES ({{id:uuid}}, {{volume_id:uuid}})
