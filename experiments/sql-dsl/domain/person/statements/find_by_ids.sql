--| tier: standard
-- The batch fetch by key: the read model for a stated set of ids, in the
-- order the caller asks for them being the engine's; a lookup, not a
-- collection read, so it takes no paging. The list expands at bind.
SELECT id, unit_id, given_name, family_name, email, status, version, created_at, updated_at
FROM person
WHERE id IN ({{ids...:uuid}})
