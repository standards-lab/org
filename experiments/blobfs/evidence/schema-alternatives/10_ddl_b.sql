-- Design B: one table with a kind column, the file-only columns nullable on
-- directory rows, per-kind check constraints, and one UNIQUE (parent_id,
-- name), so directories and files share one name space.
--
-- Two typing devices, both declarative and both invisible to the
-- application, replace what design C's subtype tables provide:
--   * UNIQUE (id, kind) makes (id, kind) a foreign-key target, so a
--     reference can name a kind.
--   * parent_kind is a STORED generated column holding 'directory' for every
--     non-root row, and the parent foreign key is over (parent_id,
--     parent_kind), so a row's parent must be a directory. Nothing writes
--     the column.
-- A consumer's own table types its reference the same way, with a constant
-- generated column of its own (see bookmark below).
--
-- Indexes beside the primary key:
--   blobfs_uq_entry_parent_name       UNIQUE (parent_id, name): the name
--       space, and the index an interleaved listing and its cursor read.
--   blobfs_ix_entry_parent_kind_name  (parent_id, kind, name): the index a
--       one-kind listing reads, so listing a directory's subdirectories does
--       not scan its files.
--   blobfs_ix_entry_parent_kind_created (parent_id, kind, created_at): the
--       counterpart of design A's blobfs_ix_file_directory_created.
--   blobfs_uq_entry_id_kind           UNIQUE (id, kind).

CREATE TABLE blobfs_entry (
  id uuid NOT NULL,
  kind text NOT NULL,
  parent_id uuid,
  parent_kind text GENERATED ALWAYS AS (CASE WHEN parent_id IS NULL THEN NULL ELSE 'directory' END) STORED,
  name text,
  status text,
  key text,
  size bigint,
  content_type text,
  etag text,
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT blobfs_pk_entry PRIMARY KEY (id),
  CONSTRAINT blobfs_uq_entry_id_kind UNIQUE (id, kind),
  CONSTRAINT blobfs_fk_entry_parent FOREIGN KEY (parent_id, parent_kind) REFERENCES blobfs_entry (id, kind),
  CONSTRAINT blobfs_uq_entry_parent_name UNIQUE (parent_id, name),
  CONSTRAINT blobfs_cc_entry_kind CHECK (kind IN ('directory', 'file')),
  CONSTRAINT blobfs_cc_entry_name CHECK (name <> ''),
  CONSTRAINT blobfs_cc_entry_parent_not_self CHECK (parent_id <> id),
  CONSTRAINT blobfs_cc_entry_root_name CHECK ((parent_id IS NULL) = (name IS NULL)),
  CONSTRAINT blobfs_cc_entry_root_kind CHECK (parent_id IS NOT NULL OR kind = 'directory'),
  CONSTRAINT blobfs_cc_entry_directory CHECK (
    kind <> 'directory'
    OR (status IS NULL AND key IS NULL AND size IS NULL AND content_type IS NULL AND etag IS NULL)),
  -- Every conjunct is written NULL-safe. A CHECK passes when its expression
  -- is unknown, so `status IN (...)` alone lets a row with a NULL status
  -- through; the IS NOT NULL tests are what make the constraint bite.
  CONSTRAINT blobfs_cc_entry_file CHECK (
    kind <> 'file'
    OR (status IS NOT NULL AND status IN ('pending', 'available', 'deleting')
        AND key IS NOT NULL AND content_type IS NOT NULL AND name IS NOT NULL))
);

CREATE UNIQUE INDEX blobfs_uq_entry_root ON blobfs_entry ((parent_id IS NULL)) WHERE parent_id IS NULL;
CREATE INDEX blobfs_ix_entry_parent_kind_name ON blobfs_entry (parent_id, kind, name);
CREATE INDEX blobfs_ix_entry_parent_kind_created ON blobfs_entry (parent_id, kind, created_at);

INSERT INTO blobfs_entry (id, kind) VALUES ('00000000-0000-0000-0000-000000000000', 'directory');

-- The consumer's tables. Each reference is typed by a composite foreign key
-- over (id, kind) with a constant generated column on the consumer's row, so
-- the consumer writes the same columns it writes today.
CREATE TABLE directory_owner (
  directory_id uuid NOT NULL,
  directory_kind text GENERATED ALWAYS AS ('directory') STORED,
  unit_id uuid NOT NULL,
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT pk_directory_owner PRIMARY KEY (directory_id),
  CONSTRAINT fk_directory_owner_directory FOREIGN KEY (directory_id, directory_kind) REFERENCES blobfs_entry (id, kind)
);
CREATE INDEX ix_directory_owner_unit ON directory_owner (unit_id);

CREATE TABLE bookmark (
  unit_id uuid NOT NULL,
  file_id uuid NOT NULL,
  file_kind text GENERATED ALWAYS AS ('file') STORED,
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT pk_bookmark PRIMARY KEY (unit_id, file_id),
  CONSTRAINT fk_bookmark_file FOREIGN KEY (file_id, file_kind) REFERENCES blobfs_entry (id, kind)
);
CREATE UNIQUE INDEX uq_bookmark_active ON bookmark (unit_id) WHERE active;
