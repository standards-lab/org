--| tier: standard
--| transaction: required
-- Inserts a volume row. The id is minted in Go; version and the timestamps
-- take the table's defaults and are read back by volume_by_id. It requires
-- a transaction because a volume is created together with its root
-- directory, and neither row may exist without the other.
INSERT INTO blobfs_volume (id, name)
VALUES ({{id:uuid}}, {{name}})
