--| tier: standard
UPDATE person
SET status = 'active', updated_at = CURRENT_TIMESTAMP, version = version + 1
WHERE id = {{id}} AND version = {{version}}
