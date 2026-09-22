--| tier: standard
--| key: file_id
--| field: unit_id uuid
--| field: file_id uuid
--| field: directory_id uuid
--| field: active boolean
--| field: name text
--| field: status text
--| field: size bigint
--| field: content_type text
--| field: created_at timestamp with time zone
--| field: updated_at timestamp with time zone
-- The consumer's bookmark read model: every bookmark joined to its file,
-- with the file's id and directory id as the handles a caller acts with,
-- in the order BookmarkedFile scans them. It is the projection bookmark ls
-- lists through by default, filtered by unit_id. It computes no path:
-- the path costs one upward walk per row, the costliest measured read of
-- the consumer, so a listing pays it only when it asks, through
-- bookmarks_with_paths, which is this base with the path column added.
--
-- The key is file_id: within one unit, the unit every read filters by, a
-- file is bookmarked at most once, so a sort by any field followed by
-- file_id is total. created_at and updated_at are the bookmark's. The
-- library columns are restated by name, as the scanner requires.
SELECT b.unit_id, b.file_id, f.directory_id, b.active,
  f.name, f.status, f.size, f.content_type, b.created_at, b.updated_at
FROM bookmark b
JOIN blobfs_file f ON f.id = b.file_id
