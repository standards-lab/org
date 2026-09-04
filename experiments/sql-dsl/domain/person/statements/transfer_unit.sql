--| tier: standard
UPDATE person
SET unit_id = {{unit_id:uuid}}, {{> sql.guard_set}}
WHERE {{> sql.guard_where}}
