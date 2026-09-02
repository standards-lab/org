package database

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

//go:embed seeds/*.json
var seedFiles embed.FS

// ErrSeedDisabled reports a seed request in an environment whose config
// does not enable seeding.
var ErrSeedDisabled = errors.New("seeding is disabled")

// Seeded counts the rows one seed run inserted per domain; a row already
// present counts nothing, so a seeded database reports zeros.
type Seeded struct {
	Organizations int `json:"organizations"`
	People        int `json:"people"`
}

// The seed files are each domain's reference data for development and
// test, in the vocabulary of its API: an organization names its parent by
// code, a person names its unit by code.
type organizationSeed struct {
	Parent string `json:"parent"`
	Code   string `json:"code"`
	Name   string `json:"name"`
}

type personSeed struct {
	Unit       string `json:"unit"`
	GivenName  string `json:"given_name"`
	FamilyName string `json:"family_name"`
	Email      string `json:"email"`
	Status     string `json:"status"`
}

// Seed loads the embedded seed files in one transaction, each table by its
// own seed function in dependency order: organizations, then the people who
// belong to them. It is idempotent through each table's unique constraint —
// an existing row is left as it is — so it runs at every startup of an
// environment that enables it and on demand from the admin mount.
func (s *Service) Seed(ctx context.Context) (Seeded, error) {
	if !s.seed {
		return Seeded{}, ErrSeedDisabled
	}
	var orgs []organizationSeed
	var people []personSeed
	if err := errors.Join(readSeed("organization", &orgs), readSeed("person", &people)); err != nil {
		return Seeded{}, err
	}
	return sqldb.Transact(ctx, s.db, func(tx *sqldb.Tx) (Seeded, error) {
		var n Seeded
		ids, err := seedOrganizations(ctx, tx, orgs, &n.Organizations)
		if err != nil {
			return n, err
		}
		return n, seedPeople(ctx, tx, ids, people, &n.People)
	})
}

// The per-table seeds. Each resolves the file's references by code, calls
// the table's insert once per row, and counts what it inserted. The SQL is
// the native tier — ON CONFLICT, RETURNING — and moves to authored files
// under the admin domain once query exists; seeds never port.

// seedOrganizations inserts the tree in file order, each parent before its
// children, and returns every organization's id by code.
func seedOrganizations(ctx context.Context, tx *sqldb.Tx, rows []organizationSeed, inserted *int) (map[string]string, error) {
	ids := make(map[string]string, len(rows))
	for _, o := range rows {
		var parent any
		if o.Parent != "" {
			id, ok := ids[o.Parent]
			if !ok {
				return nil, fmt.Errorf("seed organization %s: parent %q not seeded before it", o.Code, o.Parent)
			}
			parent = id
		}
		id, ok, err := insertOrganization(ctx, tx, parent, o)
		if err != nil {
			return nil, fmt.Errorf("seed organization %s: %w", o.Code, err)
		}
		ids[o.Code] = id
		if ok {
			*inserted++
		}
	}
	return ids, nil
}

// insertOrganization inserts one organization or finds the one already
// there, returning its id and whether this call inserted it.
func insertOrganization(ctx context.Context, tx *sqldb.Tx, parent any, o organizationSeed) (id string, inserted bool, err error) {
	if found, err := scanID(tx.QueryContext(ctx,
		"INSERT INTO organization (parent_id, code, name) VALUES ($1, $2, $3) ON CONFLICT ON CONSTRAINT uq_organization_parent_code DO NOTHING RETURNING id",
		parent, o.Code, o.Name)); err != nil || found != "" {
		return found, found != "", err
	}
	found, err := scanID(tx.QueryContext(ctx,
		"SELECT id FROM organization WHERE parent_id IS NOT DISTINCT FROM $1 AND code = $2", parent, o.Code))
	if err == nil && found == "" {
		err = errors.New("neither inserted nor found")
	}
	return found, false, err
}

// seedPeople inserts each person under the unit named by code.
func seedPeople(ctx context.Context, tx *sqldb.Tx, units map[string]string, rows []personSeed, inserted *int) error {
	for _, p := range rows {
		unit, ok := units[p.Unit]
		if !ok {
			return fmt.Errorf("seed person %s: unit %q is not a seeded organization", p.Email, p.Unit)
		}
		ok, err := insertPerson(ctx, tx, unit, p)
		if err != nil {
			return fmt.Errorf("seed person %s: %w", p.Email, err)
		}
		if ok {
			*inserted++
		}
	}
	return nil
}

// insertPerson inserts one person, reporting whether the row was new.
func insertPerson(ctx context.Context, tx *sqldb.Tx, unit string, p personSeed) (bool, error) {
	res, err := tx.ExecContext(ctx,
		"INSERT INTO person (unit_id, given_name, family_name, email, status) VALUES ($1, $2, $3, $4, $5) ON CONFLICT ON CONSTRAINT uq_person_email DO NOTHING",
		unit, p.GivenName, p.FamilyName, p.Email, p.Status)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// scanID reads the id of the first row, or "" for no rows.
func scanID(rows *sql.Rows, err error) (string, error) {
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	var id string
	if rows.Next() {
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
	}
	return id, rows.Err()
}

// readSeed decodes seeds/<domain>.json strictly: an unknown field is a
// defect in the file, not data to ignore.
func readSeed(domain string, v any) error {
	f, err := seedFiles.Open("seeds/" + domain + ".json")
	if err != nil {
		return fmt.Errorf("seed %s: %w", domain, err)
	}
	defer func() { _ = f.Close() }()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("seed %s: %w", domain, err)
	}
	return nil
}
