CREATE TABLE person (
  id uuid PRIMARY KEY DEFAULT uuidv7(),
  unit_id uuid NOT NULL REFERENCES organization (id),
  given_name text NOT NULL,
  family_name text NOT NULL,
  email text NOT NULL,
  status text NOT NULL DEFAULT 'pending',
  version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT uq_person_email UNIQUE (email),
  CONSTRAINT cc_person_status CHECK (status IN ('pending', 'active', 'inactive')),
  CONSTRAINT cc_person_given_name CHECK (given_name <> ''),
  CONSTRAINT cc_person_family_name CHECK (family_name <> ''),
  CONSTRAINT cc_person_email CHECK (email ~ '^[^@[:space:]]+@[^@[:space:]]+$')
);
