--| tier: standard
UPDATE person
SET status = 'inactive', {{> sql.guard_set}}
WHERE {{> sql.guard_where}}
