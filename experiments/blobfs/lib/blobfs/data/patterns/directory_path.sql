--| tier: standard
-- A directory's path inside its volume, over the tree row t joined on the
-- directory's id: / for a root, the tree's path otherwise. The including
-- statement aliases it as path, and includes blobfs.tree.
CASE WHEN t.parent_id IS NULL THEN '/' ELSE t.path END
