--| tier: standard
UPDATE person
SET given_name = {{given_name}}, family_name = {{family_name}}, email = {{email}}, {{> sql.guard_set}}
WHERE {{> sql.guard_where}}
