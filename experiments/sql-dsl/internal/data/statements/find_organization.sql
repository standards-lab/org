--| tier: standard
SELECT id
FROM organization
WHERE parent_id IS NOT DISTINCT FROM {{parent:uuid}} AND code = {{code}}
