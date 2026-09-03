--| tier: standard
UPDATE person
SET given_name = {{given_name}}, family_name = {{family_name}}, email = {{email}},
    updated_at = CURRENT_TIMESTAMP, version = version + 1
WHERE id = {{id}} AND version = {{version}}
