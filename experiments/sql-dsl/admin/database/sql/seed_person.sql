--| tier: native
--| native: postgres — ON CONFLICT DO NOTHING. Ports: MERGE (SQL Server, Oracle), INSERT IGNORE (MySQL).
INSERT INTO person (unit_id, given_name, family_name, email, status)
VALUES ({{unit}}, {{given_name}}, {{family_name}}, {{email}}, {{status}})
ON CONFLICT ON CONSTRAINT uq_person_email DO NOTHING
