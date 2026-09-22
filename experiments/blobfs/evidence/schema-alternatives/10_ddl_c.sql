-- Design C: the architect's supertype and subtypes. blobfs_node holds the
-- tree and the shared columns; blobfs_directory and blobfs_file are the two
-- subtype tables, keyed by the node's id and tied to its kind through
-- UNIQUE (id, kind). One name space, one UNIQUE (parent_id, name).
--
-- The foreign keys between blobfs_node and blobfs_directory are circular, so
-- blobfs_node's parent key is added by ALTER after both tables exist. No
-- deferral is needed at run time as long as a directory is written node row
-- first and subtype row second, and removed subtype row first and node row
-- second.

CREATE TABLE blobfs_node (
  id uuid NOT NULL,
  kind text NOT NULL,
  parent_id uuid,
  name text,
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT blobfs_pk_node PRIMARY KEY (id),
  CONSTRAINT blobfs_uq_node_id_kind UNIQUE (id, kind),
  CONSTRAINT blobfs_uq_node_parent_name UNIQUE (parent_id, name),
  CONSTRAINT blobfs_cc_node_kind CHECK (kind IN ('directory', 'file')),
  CONSTRAINT blobfs_cc_node_name CHECK (name <> ''),
  CONSTRAINT blobfs_cc_node_parent_not_self CHECK (parent_id <> id),
  CONSTRAINT blobfs_cc_node_root_name CHECK ((parent_id IS NULL) = (name IS NULL)),
  CONSTRAINT blobfs_cc_node_root_kind CHECK (parent_id IS NOT NULL OR kind = 'directory')
);

CREATE UNIQUE INDEX blobfs_uq_node_root ON blobfs_node ((parent_id IS NULL)) WHERE parent_id IS NULL;
CREATE INDEX blobfs_ix_node_parent_kind_name ON blobfs_node (parent_id, kind, name);
CREATE INDEX blobfs_ix_node_parent_kind_created ON blobfs_node (parent_id, kind, created_at);

CREATE TABLE blobfs_directory (
  node_id uuid NOT NULL,
  kind text NOT NULL DEFAULT 'directory',
  CONSTRAINT blobfs_pk_directory PRIMARY KEY (node_id),
  CONSTRAINT blobfs_cc_directory_kind CHECK (kind = 'directory'),
  CONSTRAINT blobfs_fk_directory_node FOREIGN KEY (node_id, kind) REFERENCES blobfs_node (id, kind)
);

CREATE TABLE blobfs_file (
  node_id uuid NOT NULL,
  kind text NOT NULL DEFAULT 'file',
  status text NOT NULL,
  key text NOT NULL,
  size bigint,
  content_type text NOT NULL,
  etag text,
  CONSTRAINT blobfs_pk_file PRIMARY KEY (node_id),
  CONSTRAINT blobfs_cc_file_kind CHECK (kind = 'file'),
  CONSTRAINT blobfs_cc_file_status CHECK (status IN ('pending', 'available', 'deleting')),
  CONSTRAINT blobfs_fk_file_node FOREIGN KEY (node_id, kind) REFERENCES blobfs_node (id, kind)
);

ALTER TABLE blobfs_node
  ADD CONSTRAINT blobfs_fk_node_parent FOREIGN KEY (parent_id) REFERENCES blobfs_directory (node_id);

INSERT INTO blobfs_node (id, kind) VALUES ('00000000-0000-0000-0000-000000000000', 'directory');
INSERT INTO blobfs_directory (node_id) VALUES ('00000000-0000-0000-0000-000000000000');

-- The consumer's tables. Each reference is typed by the subtype table it
-- names, with no extra column on the consumer's row.
CREATE TABLE directory_owner (
  directory_id uuid NOT NULL,
  unit_id uuid NOT NULL,
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT pk_directory_owner PRIMARY KEY (directory_id),
  CONSTRAINT fk_directory_owner_directory FOREIGN KEY (directory_id) REFERENCES blobfs_directory (node_id)
);
CREATE INDEX ix_directory_owner_unit ON directory_owner (unit_id);

CREATE TABLE bookmark (
  unit_id uuid NOT NULL,
  file_id uuid NOT NULL,
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT pk_bookmark PRIMARY KEY (unit_id, file_id),
  CONSTRAINT fk_bookmark_file FOREIGN KEY (file_id) REFERENCES blobfs_file (node_id)
);
CREATE UNIQUE INDEX uq_bookmark_active ON bookmark (unit_id) WHERE active;
