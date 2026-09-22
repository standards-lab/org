--| tier: standard
-- How many units bookmark one file. rm reads it in the transaction that
-- begins the file's delete, after the begin has locked the row, and
-- refuses the delete while the count is not zero, so a bookmarked file's
-- object is never deleted. The begin's lock is what makes the count
-- reliable: a bookmark add holds the row before it inserts, so an add
-- that holds first has committed before this reads, and one that arrives
-- later waits and then refuses. The foreign key fk_bookmark_file would
-- refuse the row's removal anyway, but only after the object is gone.
SELECT COUNT(*) AS bookmarks
FROM bookmark b
WHERE b.file_id = {{file_id:uuid}}
