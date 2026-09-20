--| tier: standard
--| transaction: required
-- Inserts the consumer's ownership row for a volume. It requires a
-- transaction because the row is written in the same unit as the volume
-- and its root: a volume the consumer created never exists without its
-- owner. version and the timestamps take the table's defaults.
INSERT INTO volume_owner (volume_id, unit_id)
VALUES ({{volume_id:uuid}}, {{unit_id:uuid}})
