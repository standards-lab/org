--| tier: standard
-- The directory forest with each directory's path inside its volume, as a
-- recursive common table expression named tree. The anchor is every root
-- (the rows that carry a volume_id), so the volume id travels down to
-- every descendant. A root's path is the empty string and a child's is its
-- parent's path, a slash, and its name, so a path starts at / inside the
-- volume and never contains the volume's name. A consumer's base
-- statement includes this clause first, then joins tree t to the rows it
-- projects; blobfs.file_path and blobfs.directory_path compose a row's
-- path from t. The anchor's path is cast so the recursive column has a
-- type on every engine.
WITH RECURSIVE tree (id, volume_id, parent_id, path) AS (
    SELECT d.id, d.volume_id, d.parent_id, CAST('' AS text)
    FROM blobfs_directory d
    WHERE d.volume_id IS NOT NULL
  UNION ALL
    SELECT d.id, t.volume_id, d.parent_id, t.path || '/' || d.name
    FROM blobfs_directory d
    JOIN tree t ON t.id = d.parent_id
)
