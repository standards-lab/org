--| tier: native
--| native: postgres — pg_advisory_xact_lock over hashtext. Ports: sp_getapplock (SQL Server), GET_LOCK (MySQL), DBMS_LOCK (Oracle), each taking the name directly, or a FOR UPDATE mutex row.
--| transaction: required
-- Serializes every transfer: the cycle check reads one node's lineage while
-- the update moves another, so two transfers on any two nodes must not
-- interleave. The lock is named, not numbered, and held to transaction end.
SELECT pg_advisory_xact_lock(hashtext({{name}}))
