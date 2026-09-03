--| tier: standard
UPDATE person
SET status = 'active', {{> sql.guard_set}}
WHERE {{> sql.guard_where}}
