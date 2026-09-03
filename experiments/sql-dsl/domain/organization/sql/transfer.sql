--| tier: standard
UPDATE organization
SET parent_id = {{parent_id:uuid}}, updated_at = CURRENT_TIMESTAMP, version = version + 1
WHERE id = {{id}} AND version = {{version}}
