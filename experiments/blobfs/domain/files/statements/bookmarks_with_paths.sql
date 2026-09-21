--| tier: standard
--| key: file_id
--| field: unit_id uuid
--| field: file_id uuid
--| field: directory_id uuid
--| field: active boolean
--| field: path text
--| field: name text
--| field: status text
--| field: size bigint
--| field: content_type text
--| field: created_at timestamp with time zone
--| field: updated_at timestamp with time zone
-- The bookmark read model with each file's full path: the bookmarks base
-- with the path column added, in the order BookmarkedFile scans them. It
-- is the projection bookmark ls lists through when the listing asks for
-- paths (Listing.Paths), filtered by unit_id, and the one a sort by path
-- needs.
--
-- The path is computed per row by a recursion anchored on the bookmarked
-- file's directory and walking upward to the root, the row whose parent
-- is NULL; the walk ends at the row that stepped past the root, and the
-- names are joined with slashes as the walk climbs. The root's own name,
-- /, contributes nothing, so a file in the root has the path /name. The
-- recursion is a scalar subquery correlated on the file's directory rather
-- than a top-level common table expression over every bookmark, because a
-- projection base binds no parameter: with the recursion at the top level
-- the walk would start from every bookmark of every unit before the
-- unit's filter, applied outside the derived table, discards the rest.
-- Correlated, the base is a plain join the planner pulls up, so the unit
-- filter reaches the bookmark index first and the walk runs once per row
-- the filter keeps: the cost is the unit's bookmark count times the depth,
-- never the size of the tree or of the bookmark table. The evidence in
-- evidence/bookmarks.txt measures both shapes. That cost is why the
-- default base, bookmarks, leaves the path out.
--
-- The key is file_id, as in bookmarks.
SELECT b.unit_id, b.file_id, f.directory_id, b.active,
  (WITH RECURSIVE up (directory_id, path) AS (
      SELECT d.parent_id, CASE WHEN d.parent_id IS NULL THEN '' ELSE '/' || d.name END
      FROM blobfs_directory d
      WHERE d.id = f.directory_id
    UNION ALL
      SELECT d.parent_id, CASE WHEN d.parent_id IS NULL THEN '' ELSE '/' || d.name END || up.path
      FROM up
      JOIN blobfs_directory d ON d.id = up.directory_id
  )
  SELECT up.path FROM up WHERE up.directory_id IS NULL) || '/' || f.name AS path,
  f.name, f.status, f.size, f.content_type, b.created_at, b.updated_at
FROM bookmark b
JOIN blobfs_file f ON f.id = b.file_id
