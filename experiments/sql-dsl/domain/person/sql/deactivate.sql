--| tier: standard
UPDATE person
SET status = 'inactive', updated_at = CURRENT_TIMESTAMP, version = version + 1
WHERE id = {{id}} AND version = {{version}}
