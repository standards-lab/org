--| tier: standard
--| key: id
--| field: id uuid
--| field: unit_id uuid
--| field: given_name text
--| field: family_name text
--| field: email text
--| field: status text
--| field: version bigint
--| field: created_at timestamp with time zone
--| field: updated_at timestamp with time zone
-- The read model: the person table as it stands, the plain-base case of
-- the collection pattern.
SELECT id, unit_id, given_name, family_name, email, status, version, created_at, updated_at
FROM person
