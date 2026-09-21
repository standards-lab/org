--| tier: standard
-- A file move or rename, as the guarded command of the query library's
-- optimistic-concurrency protocol: sets the file's directory and name,
-- advances the version, and stamps updated_at. The key is untouched: it
-- was built from the id and the name at the insert and the object stays
-- under it, so a rename moves no object. The guard's predicate names the
-- id and the version the caller read; the status predicate keeps a
-- deleting row unchanged, since its object is being removed. The guard
-- reports no row affected as a version mismatch or a missing row, so the
-- caller reads the row again to tell the status refusal from a version
-- conflict. A directory that does not exist fails the foreign key
-- blobfs_fk_file_directory, and a name already held in the directory the
-- unique constraint blobfs_uq_file_directory_name. One statement, so it
-- accepts the pool and composes into a caller's transaction alike; a file
-- cannot form a cycle, so no lock and no check precede it.
UPDATE blobfs_file
SET directory_id = {{directory_id:uuid}},
    name = {{name}},
    {{> sql.guard_set}}
WHERE {{> sql.guard_where}} AND status <> 'deleting'
