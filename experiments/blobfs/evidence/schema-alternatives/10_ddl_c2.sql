-- Design C2: the variation of C I judge strongest. blobfs_node holds the
-- tree and the shared columns, as in C, but there is no blobfs_directory
-- table: a directory is a node whose kind is 'directory' and nothing else.
-- Only files have a subtype table.
--
-- What this drops from C: the empty table, the circular foreign key and the
-- ALTER that the circle forces, the ordering rule on a bulk load, and the
-- second statement per directory create and per directory remove.
-- What it keeps from C: NOT NULL file columns in a table of their own, and a
-- consumer's bookmark referencing blobfs_file (node_id) with no extra column.
-- What it gives up against C: a consumer's directory reference must be the
-- composite (id, kind) foreign key, the same one design B uses.

CREATE TABLE blobfs_node (
  id uuid NOT NULL,
  kind text NOT NULL,
  parent_id uuid,
  parent_kind text GENERATED ALWAYS AS (CASE WHEN parent_id IS NULL THEN NULL ELSE 'directory' END) STORED,
  name text,
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT blobfs_pk_node PRIMARY KEY (id),
  CONSTRAINT blobfs_uq_node_id_kind UNIQUE (id, kind),
  CONSTRAINT blobfs_fk_node_parent FOREIGN KEY (parent_id, parent_kind) REFERENCES blobfs_node (id, kind),
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

INSERT INTO blobfs_node (id, kind) VALUES ('00000000-0000-0000-0000-000000000000', 'directory');

CREATE TABLE directory_owner (
  directory_id uuid NOT NULL,
  directory_kind text GENERATED ALWAYS AS ('directory') STORED,
  unit_id uuid NOT NULL,
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT pk_directory_owner PRIMARY KEY (directory_id),
  CONSTRAINT fk_directory_owner_directory FOREIGN KEY (directory_id, directory_kind) REFERENCES blobfs_node (id, kind)
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
