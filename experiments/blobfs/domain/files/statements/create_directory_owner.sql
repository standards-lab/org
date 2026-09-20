--| tier: standard
--| transaction: required
-- Inserts the consumer's ownership row for a depth-one directory. It
-- requires a transaction because the row is written in the same unit as
-- the directory it binds: a directory the consumer created with a unit
-- never exists without its owner row. version and the timestamps take the
-- table's defaults.
INSERT INTO directory_owner (directory_id, unit_id)
VALUES ({{directory_id:uuid}}, {{unit_id:uuid}})
