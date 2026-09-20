--| tier: standard
-- A file's path inside its volume: its directory's tree path, a slash, and
-- its name, over blobfs_file f joined to the tree row t on f.directory_id.
-- The including statement aliases it as path, and includes blobfs.tree.
t.path || '/' || f.name
