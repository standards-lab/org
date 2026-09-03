--| tier: standard
DELETE FROM person
WHERE {{> sql.guard_where}}
