--| tier: standard
--| key: id
--| field: id uuid
--| field: volume_id uuid
--| field: unit_id uuid
--| field: directory_id uuid
--| field: name text
--| field: path text
--| field: status text
--| field: size bigint
--| field: content_type text
--| field: version bigint
--| field: created_at timestamp with time zone
--| field: updated_at timestamp with time zone
-- The consumer's file read model: every file with its path inside its
-- volume, the volume carried down blobfs's tree, and the unit that owns the
-- volume from the consumer's own volume_owner row. The tree pattern comes
-- first, then the file columns in the order FileEntry scans them. The base
-- binds no parameters: List appends the directory_id filter as a
-- directive, and --unit is a filter on unit_id, the shape of the auth
-- strategy's scope predicate. The owner join is an inner join, so a volume
-- without an owner row lists no files.
{{> blobfs.tree}}
SELECT {{> blobfs.file_columns}}, {{> blobfs.file_path}} AS path, t.volume_id, o.unit_id
FROM blobfs_file f
JOIN tree t ON t.id = f.directory_id
JOIN volume_owner o ON o.volume_id = t.volume_id
