package organization

import (
	"context"
	"errors"

	"github.com/standards-lab/go-core/lifecycle"
	"github.com/standards-lab/go-web-sdk"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/sdk"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

// ErrCycle reports a transfer whose new parent sits inside the
// organization's own subtree, the organization itself included.
var ErrCycle = errors.New("transfer would create a cycle")

// Stage is the lifecycle stage the domains verify their statements in:
// after the schema (1), before the root.
const Stage = 2

// Service is the organization domain service: the layer's public API, one
// method per endpoint, every operation delegated whole to the store.
// Queries return data; commands validate their input — each command type
// owns its rules — run guarded, and return Identity only.
type Service struct {
	store *store
}

// New constructs the service over the session it reads from. Construction
// loads and binds the statements and performs no I/O.
func New(db *sqldb.DB) *Service {
	return &Service{store: newStore(db)}
}

// Register declares the domain's startup verification on lc.
func (s *Service) Register(lc *lifecycle.Coordinator) {
	lc.Add(lifecycle.Service{Name: "organization", Stage: Stage, Start: s.Verify})
}

// Verify prepares every statement and the read contract against the
// migrated schema; startup runs it at the domains' stage.
func (s *Service) Verify(ctx context.Context) error { return s.store.Verify(ctx) }

// List returns one page of organizations and the total count, honoring the
// parsed query's directives and exact-match filters. An unknown sort or
// filter field, or a value the engine cannot read, unwraps to
// query.ErrDirectives.
func (s *Service) List(ctx context.Context, q web.Query) ([]Organization, int, error) {
	return s.store.list(ctx, sdk.Directives(q))
}

// Find returns the organization with the given id, or sql.ErrNoRows.
func (s *Service) Find(ctx context.Context, id string) (Organization, error) {
	return s.store.find(ctx, "id", id)
}

// FindByPath resolves a composed organization path ("/acme/engineering") to
// its node, or sql.ErrNoRows.
func (s *Service) FindByPath(ctx context.Context, path string) (Organization, error) {
	return s.store.find(ctx, "path", path)
}

// Create creates an organization under the stated parent — nil for a root —
// returning its engine-minted identity. A duplicate sibling code or a
// nonexistent parent surfaces as a constraint violation.
func (s *Service) Create(ctx context.Context, c CreateOrganization) (Identity, error) {
	if err := c.Validate(); err != nil {
		return Identity{}, err
	}
	return s.store.create(ctx, c)
}

// Edit rewrites the organization's descriptive fields under the version
// guard, returning the advanced identity.
func (s *Service) Edit(ctx context.Context, id string, version int64, e EditOrganization) (Identity, error) {
	if err := e.Validate(); err != nil {
		return Identity{}, err
	}
	return s.store.edit(ctx, id, version, e)
}

// Transfer is an action: it moves the organization under a new parent — nil
// for the root — under the version guard and the tree lock. A destination
// inside the organization's own subtree is ErrCycle.
func (s *Service) Transfer(ctx context.Context, id string, version int64, t TransferOrganization) (Identity, error) {
	if err := t.Validate(); err != nil {
		return Identity{}, err
	}
	return s.store.transfer(ctx, id, version, t)
}

// Delete removes the organization under the version guard; children block
// deletion.
func (s *Service) Delete(ctx context.Context, id string, version int64) error {
	return s.store.delete(ctx, id, version)
}
