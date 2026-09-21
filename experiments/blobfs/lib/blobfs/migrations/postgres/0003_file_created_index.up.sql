-- The index a listing sorted by creation time uses: a directory's files in
-- created_at order, so a page sorted by created_at reads the page from the
-- index instead of sorting the directory. The unique constraint on
-- (directory_id, name) already orders a sort by name. This migration is the
-- upgrade rehearsal: a new version over an installed schema, applied to a
-- database whose rows survive it.
CREATE INDEX blobfs_ix_file_directory_created ON blobfs_file (directory_id, created_at);
