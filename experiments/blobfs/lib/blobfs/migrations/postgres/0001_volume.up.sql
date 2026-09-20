-- A volume names one directory tree. Its name is the tree's one unique
-- name, and every path starts at / inside a volume. The volume holds no
-- owner and no unit: it anchors path resolution, and a consumer's own table
-- binds it to whatever the consumer authorizes by. Ids are minted in Go, so
-- id has no default.
CREATE TABLE blobfs_volume (
  id uuid NOT NULL,
  name text NOT NULL,
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT blobfs_pk_volume PRIMARY KEY (id),
  CONSTRAINT blobfs_uq_volume_name UNIQUE (name),
  CONSTRAINT blobfs_cc_volume_name CHECK (name <> '')
);
