--| tier: standard
-- Inserts a unit's bookmark of a file, active or not. The primary key
-- pk_bookmark refuses a second bookmark of the same file by the same unit,
-- the partial unique index uq_bookmark_active refuses a second active
-- bookmark for the unit, and the foreign key fk_bookmark_file refuses a
-- file that does not exist; database.go maps each name to its sentinel.
-- The timestamps take the table's defaults. The statement accepts the
-- pool and composes into a caller's transaction alike: the consumer runs
-- it in the transaction that resolved the file, and a service would run
-- it beside the pending row of its own upload.
INSERT INTO bookmark (unit_id, file_id, active)
VALUES ({{unit_id:uuid}}, {{file_id:uuid}}, {{active:boolean}})
