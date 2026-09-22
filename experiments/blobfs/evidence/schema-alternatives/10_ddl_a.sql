-- Design A: the current two-table schema, copied from
-- lib/blobfs/migrations/postgres/0001_directory.up.sql, 0002_file.up.sql and
-- 0003_file_created_index.up.sql, with timestamptz spelled as in the shipped
-- DDL. Two name spaces: a directory and a file may share a name in one parent.

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

CREATE TABLE blobfs_file (
  id uuid NOT NULL,
  directory_id uuid NOT NULL,
  name text NOT NULL,
  status text NOT NULL,
  key text NOT NULL,
  size bigint,
  content_type text NOT NULL,
  etag text,
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT blobfs_pk_file PRIMARY KEY (id),
  CONSTRAINT blobfs_fk_file_directory FOREIGN KEY (directory_id) REFERENCES blobfs_directory (id),
  CONSTRAINT blobfs_uq_file_directory_name UNIQUE (directory_id, name),
  CONSTRAINT blobfs_cc_file_name CHECK (name <> ''),
  CONSTRAINT blobfs_cc_file_status CHECK (status IN ('pending', 'available', 'deleting'))
);

CREATE INDEX blobfs_ix_file_directory_created ON blobfs_file (directory_id, created_at);

-- The consumer's two tables, as migrations/postgres ships them.
CREATE TABLE directory_owner (
  directory_id uuid NOT NULL,
  unit_id uuid NOT NULL,
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT pk_directory_owner PRIMARY KEY (directory_id),
  CONSTRAINT fk_directory_owner_directory FOREIGN KEY (directory_id) REFERENCES blobfs_directory (id)
);
CREATE INDEX ix_directory_owner_unit ON directory_owner (unit_id);

CREATE TABLE bookmark (
  unit_id uuid NOT NULL,
  file_id uuid NOT NULL,
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT pk_bookmark PRIMARY KEY (unit_id, file_id),
  CONSTRAINT fk_bookmark_file FOREIGN KEY (file_id) REFERENCES blobfs_file (id)
);
CREATE UNIQUE INDEX uq_bookmark_active ON bookmark (unit_id) WHERE active;
