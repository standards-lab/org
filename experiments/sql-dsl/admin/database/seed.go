package database

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/standards-lab/org/experiments/sql-dsl/lib/query"
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
		ids, err := s.seedOrganizations(ctx, tx, orgs, &n.Organizations)
		if err != nil {
			return n, err
		}
		return n, s.seedPeople(ctx, tx, ids, people, &n.People)
	})
}

// The per-table seeds. Each resolves the file's references by code, calls
// the table's insert once per row, and counts what it inserted. The
// statements are the admin domain's authored files under sql/ — the native
// tier, ON CONFLICT and RETURNING, declared in their headers; seeds never
// port — held as query handles bound once in New.

// seedOrganizations inserts the tree in file order, each parent before its
// children, and returns every organization's id by code.
func (s *Service) seedOrganizations(ctx context.Context, tx *sqldb.Tx, rows []organizationSeed, inserted *int) (map[string]string, error) {
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
		id, ok, err := s.seedOrganization(ctx, tx, parent, o)
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

// seedOrganization seeds one organization or finds the one already there,
// returning its id and whether this call inserted it. The statement returns
// no row on conflict; sql.ErrNoRows is that signal.
func (s *Service) seedOrganization(ctx context.Context, tx *sqldb.Tx, parent any, o organizationSeed) (id string, inserted bool, err error) {
	id, err = s.seedOrg.One(ctx, tx, query.Args{"parent": parent, "code": o.Code, "name": o.Name})
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false, err
	}
	id, err = s.findOrg.One(ctx, tx, query.Args{"parent": parent, "code": o.Code})
	if errors.Is(err, sql.ErrNoRows) {
		err = errors.New("neither inserted nor found")
	}
	return id, false, err
}

// seedPeople seeds each person under the unit named by code.
func (s *Service) seedPeople(ctx context.Context, tx *sqldb.Tx, units map[string]string, rows []personSeed, inserted *int) error {
	for _, p := range rows {
		unit, ok := units[p.Unit]
		if !ok {
			return fmt.Errorf("seed person %s: unit %q is not a seeded organization", p.Email, p.Unit)
		}
		n, err := s.seedPerson.Exec(ctx, tx, query.Args{
			"unit": unit, "given_name": p.GivenName, "family_name": p.FamilyName, "email": p.Email, "status": p.Status,
		})
		if err != nil {
			return fmt.Errorf("seed person %s: %w", p.Email, err)
		}
		if n == 1 {
			*inserted++
		}
	}
	return nil
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
