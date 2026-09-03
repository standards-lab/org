--| tier: native
--| native: postgres — RETURNING. Ports: OUTPUT INSERTED (SQL Server), RETURNING INTO (Oracle), LAST_INSERT_ID + a read (MySQL).
INSERT INTO person (unit_id, given_name, family_name, email)
VALUES ({{unit_id:uuid}}, {{given_name}}, {{family_name}}, {{email}})
RETURNING id, version
