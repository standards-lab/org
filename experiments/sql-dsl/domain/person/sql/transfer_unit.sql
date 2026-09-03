--| tier: standard
UPDATE person
SET unit_id = {{unit_id:uuid}}, updated_at = CURRENT_TIMESTAMP, version = version + 1
WHERE id = {{id}} AND version = {{version}}
