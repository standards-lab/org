--| tier: standard
-- An action's precondition read: the record's status and version, checked
-- against the transition rule before the guarded update runs.
SELECT status, version FROM person WHERE id = {{id}}
