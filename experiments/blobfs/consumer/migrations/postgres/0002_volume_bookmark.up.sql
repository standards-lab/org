-- A bookmark joins a volume to one of its files. Neither foreign key has a
-- cascading action, so a file delete that meets a bookmark fails with a
-- classifiable foreign-key error. The partial unique index allows one
-- active bookmark per volume: the guarded single-active-row shape. This
-- migration is two statements in one transaction.
CREATE TABLE volume_bookmark (
  volume_id uuid NOT NULL,
  file_id uuid NOT NULL,
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT pk_volume_bookmark PRIMARY KEY (volume_id, file_id),
  CONSTRAINT fk_volume_bookmark_volume FOREIGN KEY (volume_id) REFERENCES blobfs_volume (id),
  CONSTRAINT fk_volume_bookmark_file FOREIGN KEY (file_id) REFERENCES blobfs_file (id)
);

CREATE UNIQUE INDEX uq_volume_bookmark_active ON volume_bookmark (volume_id) WHERE active;
