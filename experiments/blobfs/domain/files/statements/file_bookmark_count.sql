--| tier: standard
-- How many units bookmark one file. rm reads it in the transaction that
-- begins the file's delete and refuses the delete while the count is not
-- zero, so a bookmarked file's object is never deleted; the foreign key
-- fk_bookmark_file would refuse the row's removal anyway, but only after
-- the object is gone.
SELECT COUNT(*) AS bookmarks
FROM bookmark b
WHERE b.file_id = {{file_id:uuid}}
