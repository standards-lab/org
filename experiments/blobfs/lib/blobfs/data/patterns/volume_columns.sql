--| tier: standard
-- The columns of a volume row in the order blobfs.Volume scans them, over
-- the correlation name v: the including statement reads FROM blobfs_volume v.
v.id, v.name, v.version, v.created_at, v.updated_at
