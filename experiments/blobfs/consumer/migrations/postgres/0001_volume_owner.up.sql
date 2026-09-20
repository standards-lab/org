-- The consumer's ownership row: the auth strategy's join table at the
-- volume grain. It binds a blobfs volume to the unit that owns it, and
-- unit_id stands in for the scope's unit; only the shape of a filter on it
-- is rehearsed. The foreign key into blobfs_volume has no cascading action,
-- so a volume delete that meets an owner row fails with a foreign-key error
-- blobfs does not classify. This migration is two statements in one
-- transaction.
CREATE TABLE volume_owner (
  volume_id uuid NOT NULL,
  unit_id uuid NOT NULL,
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT pk_volume_owner PRIMARY KEY (volume_id),
  CONSTRAINT fk_volume_owner_volume FOREIGN KEY (volume_id) REFERENCES blobfs_volume (id)
);

CREATE INDEX ix_volume_owner_unit ON volume_owner (unit_id);
