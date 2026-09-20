-- The directory hierarchy. Ids are minted in Go, so id has no default. The
-- table holds exactly one root: the row with no parent and no name, seeded
-- below with the well-known id blobfs.RootID (the nil UUID) so every
-- install and every consumer address the root by the same id. Every other
-- directory has a parent and a name. blobfs_cc_directory_root_name states
-- that rule (no parent means no name and no name means no parent), and the
-- partial unique index blobfs_uq_directory_root allows one row without a
-- parent: a second root fails under the index's name as a unique violation.
-- The unique constraint on (parent_id, name) covers the non-root rows, since
-- the root's NULL parent and NULL name keep it out of play. The foreign key
-- has no cascading action: a directory that still has children refuses its
-- delete through blobfs_fk_directory_parent. This migration is three
-- statements in one transaction.
CREATE TABLE blobfs_directory (
  id uuid NOT NULL,
  parent_id uuid,
  name text,
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT blobfs_pk_directory PRIMARY KEY (id),
  CONSTRAINT blobfs_fk_directory_parent FOREIGN KEY (parent_id) REFERENCES blobfs_directory (id),
  CONSTRAINT blobfs_uq_directory_parent_name UNIQUE (parent_id, name),
  CONSTRAINT blobfs_cc_directory_name CHECK (name <> ''),
  CONSTRAINT blobfs_cc_directory_parent_not_self CHECK (parent_id <> id),
  CONSTRAINT blobfs_cc_directory_root_name CHECK ((parent_id IS NULL) = (name IS NULL))
);

CREATE UNIQUE INDEX blobfs_uq_directory_root ON blobfs_directory ((parent_id IS NULL)) WHERE parent_id IS NULL;

INSERT INTO blobfs_directory (id) VALUES ('00000000-0000-0000-0000-000000000000');
