-- The recommended schema: one table, blobfs_entry, with a kind column.
-- This is design B with the index set tuned and the check constraints split
-- so each refusal carries its own name.
--
-- Migration 1 of the library's set. The root is seeded here, as it is today.

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

  -- (id, kind) is a foreign-key target, so a consumer's table can reference
  -- a file and not a directory, or a directory and not a file. Nothing in
  -- the library reads this index; it exists for the consumer's keys and for
  -- the parent key below.
  CONSTRAINT blobfs_uq_entry_id_kind UNIQUE (id, kind),

  -- One name space: a directory and a file cannot share a name in one
  -- parent. The root's parent_id and name are both NULL, which keeps it out
  -- of the constraint.
  CONSTRAINT blobfs_uq_entry_parent_name UNIQUE (parent_id, name),

  -- A row's parent must be a directory. parent_kind is derived, so the
  -- application never writes it and cannot get it wrong. No cascading
  -- action: a directory that still holds entries refuses its own delete
  -- through this key, which is the whole not-empty guard.
  CONSTRAINT blobfs_fk_entry_parent FOREIGN KEY (parent_id, parent_kind)
    REFERENCES blobfs_entry (id, kind),

  CONSTRAINT blobfs_cc_entry_kind CHECK (kind IN ('directory', 'file')),
  CONSTRAINT blobfs_cc_entry_name CHECK (name <> ''),
  CONSTRAINT blobfs_cc_entry_parent_not_self CHECK (parent_id <> id),

  -- The root is the one row with no parent and no name, and it is a
  -- directory.
  CONSTRAINT blobfs_cc_entry_root_name CHECK ((parent_id IS NULL) = (name IS NULL)),
  CONSTRAINT blobfs_cc_entry_root_kind CHECK (parent_id IS NOT NULL OR kind = 'directory'),

  -- A directory row carries none of the file columns.
  CONSTRAINT blobfs_cc_entry_directory_columns CHECK (
    kind <> 'directory'
    OR (status IS NULL AND key IS NULL AND size IS NULL
        AND content_type IS NULL AND etag IS NULL)),

  -- A file row carries the three columns that exist from the pending row on.
  -- Every conjunct is written NULL-safe: a CHECK passes when its expression
  -- is unknown, so `status IN (...)` alone would let a NULL status through.
  -- size and etag stay NULL until the object exists.
  CONSTRAINT blobfs_cc_entry_file_status CHECK (
    kind <> 'file' OR (status IS NOT NULL AND status IN ('pending', 'available', 'deleting'))),
  CONSTRAINT blobfs_cc_entry_file_key CHECK (kind <> 'file' OR key IS NOT NULL),
  CONSTRAINT blobfs_cc_entry_file_content_type CHECK (kind <> 'file' OR content_type IS NOT NULL)
);

-- One root per install.
CREATE UNIQUE INDEX blobfs_uq_entry_root ON blobfs_entry ((parent_id IS NULL)) WHERE parent_id IS NULL;

-- Listing one parent's subdirectories without reading its files. Partial over
-- directory rows, which are the small half of any tree, so the index is a
-- twenty-fifth of a full (parent_id, kind, name) index at this fixture's
-- shape. The file listing and the interleaved listing both read
-- blobfs_uq_entry_parent_name instead.
CREATE INDEX blobfs_ix_entry_parent_directory_name ON blobfs_entry (parent_id, name) WHERE kind = 'directory';

INSERT INTO blobfs_entry (id, kind) VALUES ('00000000-0000-0000-0000-000000000000', 'directory');

-- Migration 2, the counterpart of today's 0003_file_created_index and, like
-- it, a candidate for the consumer's own set rather than the library's
-- (architect's decision 5).
-- CREATE INDEX blobfs_ix_entry_parent_file_created ON blobfs_entry (parent_id, created_at) WHERE kind = 'file';

-- ---------------------------------------------------------------------------
-- What a consumer writes. Each reference names a kind through a constant
-- generated column, so the consumer's INSERT binds the same columns it binds
-- today and the foreign key still carries its own name to the error mapping.

CREATE TABLE directory_owner (
  directory_id uuid NOT NULL,
  directory_kind text GENERATED ALWAYS AS ('directory') STORED,
  unit_id uuid NOT NULL,
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT pk_directory_owner PRIMARY KEY (directory_id),
  CONSTRAINT fk_directory_owner_directory FOREIGN KEY (directory_id, directory_kind)
    REFERENCES blobfs_entry (id, kind)
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
