--| tier: standard
DELETE FROM person
WHERE id = {{id}} AND version = {{version}}
