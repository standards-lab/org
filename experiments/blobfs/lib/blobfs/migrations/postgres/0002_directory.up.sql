-- The directory hierarchy. Ids are minted in Go, so id has no default. A
-- root (parent_id NULL) has no name of its own and belongs to exactly one
-- volume through volume_id; every other directory has a name and no
-- volume_id. The two root checks state that rule, and the unique constraint
-- on volume_id allows one root per volume. The unique constraint on
-- (parent_id, name) covers the non-root rows, since a root's NULL parent
-- and NULL name keep it out of play. Neither foreign key has a cascading
-- action: a directory that still has children refuses its delete through
-- blobfs_fk_directory_parent, and a volume whose root still exists refuses
-- its delete through blobfs_fk_directory_volume.
CREATE TABLE blobfs_directory (
  id uuid NOT NULL,
  parent_id uuid,
  volume_id uuid,
  name text,
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT blobfs_pk_directory PRIMARY KEY (id),
  CONSTRAINT blobfs_fk_directory_parent FOREIGN KEY (parent_id) REFERENCES blobfs_directory (id),
  CONSTRAINT blobfs_fk_directory_volume FOREIGN KEY (volume_id) REFERENCES blobfs_volume (id),
  CONSTRAINT blobfs_uq_directory_volume UNIQUE (volume_id),
  CONSTRAINT blobfs_uq_directory_parent_name UNIQUE (parent_id, name),
  CONSTRAINT blobfs_cc_directory_name CHECK (name <> ''),
  CONSTRAINT blobfs_cc_directory_parent_not_self CHECK (parent_id <> id),
  CONSTRAINT blobfs_cc_directory_root_volume CHECK ((parent_id IS NULL) = (volume_id IS NOT NULL)),
  CONSTRAINT blobfs_cc_directory_root_name CHECK ((parent_id IS NULL) = (name IS NULL))
);
