--| tier: native
--| native: postgres — ON CONFLICT DO NOTHING and RETURNING. Ports: MERGE (SQL Server, Oracle), INSERT IGNORE + a second read (MySQL).
INSERT INTO organization (parent_id, code, name)
VALUES ({{parent:uuid}}, {{code}}, {{name}})
ON CONFLICT ON CONSTRAINT uq_organization_parent_code DO NOTHING
RETURNING id
