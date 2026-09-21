--| tier: native
--| native: pg_advisory_xact_lock over one fixed bigint key, released when the transaction ends. Port: an engine must provide a lock taken inside a transaction and held until its commit or rollback, keyed by a value the port derives from the blobfs_ namespace, and every mover must take it; SQL Server has sp_getapplock with the Transaction owner; MySQL's GET_LOCK is session-scoped and SQLite has no lock function, so a port to them keeps the baseline's no-op and serializes moves outside the database.
--| transaction: required
-- The tree lock: every transaction that moves a directory takes it before
-- its cycle check, so two opposing moves run one after the other and the
-- second one's check sees the first one's result. The key is
-- postgres.TreeLockKey, bound by the variant, and the lock is
-- transaction-scoped, so a session that is not a transaction is refused
-- rather than take a lock that autocommit releases at once.
SELECT pg_advisory_xact_lock({{key:bigint}})
