--| tier: standard
UPDATE organization
SET code = {{code}}, name = {{name}}, updated_at = CURRENT_TIMESTAMP, version = version + 1
WHERE id = {{id}} AND version = {{version}}
