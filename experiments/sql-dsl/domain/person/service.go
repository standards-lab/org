package person

import (
	"context"

	"github.com/standards-lab/go-core/lifecycle"
	"github.com/standards-lab/go-web-sdk"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/data"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/sdk"
)

// Stage is the lifecycle stage the domains verify their statements in.
const Stage = 2

// Service is the person domain service: the layer's public API, one method
// per endpoint, every operation delegated whole to the store.
type Service struct {
	store *store
}

// New constructs the service over the session; construction loads and
// binds the statements and performs no I/O.
func New(db *data.Database) *Service { return &Service{store: newStore(db)} }

// Register declares the domain's startup verification on lc.
func (s *Service) Register(lc *lifecycle.Coordinator) {
	lc.Add(lifecycle.Service{Name: "person", Stage: Stage, Start: s.Verify})
}

// Verify prepares every statement and the read contract against the
// migrated schema.
func (s *Service) Verify(ctx context.Context) error { return s.store.Verify(ctx) }

// List returns one page of people and the total count under the parsed
// query's declarations.
func (s *Service) List(ctx context.Context, q web.Query) ([]Person, int, error) {
	return s.store.list(ctx, sdk.Directives(q))
}

// FindMany returns the people among ids that exist: a batch fetch by key,
// not a collection read, so no paging and no total beyond the result.
func (s *Service) FindMany(ctx context.Context, ids []string) ([]Person, error) {
	return s.store.findMany(ctx, ids)
}

// Find returns the person with the given id, or sql.ErrNoRows.
func (s *Service) Find(ctx context.Context, id string) (Person, error) {
	return s.store.find(ctx, id)
}

// Create creates a pending person in the stated unit, returning the
// engine-minted identity. A duplicate email or a nonexistent unit surfaces
// as a constraint violation.
func (s *Service) Create(ctx context.Context, c CreatePerson) (Identity, error) {
	if err := c.Validate(); err != nil {
		return Identity{}, err
	}
	return s.store.create(ctx, c)
}

// Edit rewrites the descriptive fields under the version guard.
func (s *Service) Edit(ctx context.Context, id string, version int64, e EditPerson) (Identity, error) {
	if err := e.Validate(); err != nil {
		return Identity{}, err
	}
	return s.store.edit(ctx, id, version, e)
}

// Delete removes the person under the version guard.
func (s *Service) Delete(ctx context.Context, id string, version int64) error {
	return s.store.delete(ctx, id, version)
}

// Activate is an action: pending or inactive → active.
func (s *Service) Activate(ctx context.Context, id string, version int64) (Identity, error) {
	return s.store.activate(ctx, id, version)
}

// Deactivate is an action: active → inactive.
func (s *Service) Deactivate(ctx context.Context, id string, version int64) (Identity, error) {
	return s.store.deactivate(ctx, id, version)
}

// TransferUnit is an action: the person moves to another unit, in any
// status.
func (s *Service) TransferUnit(ctx context.Context, id string, version int64, t TransferUnit) (Identity, error) {
	if err := t.Validate(); err != nil {
		return Identity{}, err
	}
	return s.store.transferUnit(ctx, id, version, t)
}
