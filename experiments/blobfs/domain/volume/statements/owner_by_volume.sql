--| tier: standard
-- The ownership row of one volume, in the order Owner scans it, or no row.
SELECT o.volume_id, o.unit_id, o.version, o.created_at, o.updated_at
FROM volume_owner o
WHERE o.volume_id = {{volume_id:uuid}}
