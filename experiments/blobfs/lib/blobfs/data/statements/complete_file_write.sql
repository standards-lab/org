--| tier: standard
-- The last step of a file write, as the guarded command of the query
-- library's optimistic-concurrency protocol: moves the pending row to
-- available with the size, content type, and entity tag the store
-- reported, advances the version, and stamps updated_at. The guard's
-- predicate names the id and the version the caller read from the pending
-- row; the status predicate keeps a row that is no longer pending (already
-- completed, or deleting) unchanged. The guard reports no row affected as a
-- version mismatch or a missing row, so the caller reads the row again to
-- tell a status refusal from a version conflict.
UPDATE blobfs_file
SET status = 'available',
    size = {{size:bigint}},
    content_type = {{content_type}},
    etag = {{etag}},
    {{> sql.guard_set}}
WHERE {{> sql.guard_where}} AND status = 'pending'
