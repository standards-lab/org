package organization

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/standards-lab/org/experiments/sql-dsl/lib/query"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

//go:embed sql/*.sql
var files embed.FS

// treeLock names the organization tree in the service's lock registry: one
// name per contended structure, <domain>.<structure>, so no two domains can
// collide on a number. Transfers take it so two concurrent transfers cannot
// each pass the cycle check and commit a cycle past the other.
const treeLock = "organization.tree"

// store is the domain's SQL client: the statements of sql/ bound once to
// their typed handles, and the operations as methods named for their
// commands — the file, the store method, the service method, and the route
// share one name. It is the package's sole importer of query.
type store struct {
	db            *sqldb.DB
	src           *query.Source
	view          query.Projection[Organization]
	createRows    query.Rows[Identity]
	inSubtree     query.Rows[int64]
	lockTree      query.Statement
	editGuard     query.Guard
	transferGuard query.Guard
	deleteGuard   query.Guard
}

func newStore(db *sqldb.DB) *store {
	src := query.MustLoad(files, "sql", db.Dialect())
	check := src.Statement("version")
	return &store{
		db:            db,
		src:           src,
		view:          query.Project(src.Statement("organization_view"), scanOrganization),
		createRows:    query.Scan(src.Statement("create"), scanIdentity),
		inSubtree:     query.Scan(src.Statement("in_subtree"), query.Scalar[int64]),
		lockTree:      src.Statement("lock_tree"),
		editGuard:     query.Guarded(src.Statement("edit"), check, "version"),
		transferGuard: query.Guarded(src.Statement("transfer"), check, "version"),
		deleteGuard:   query.Guarded(src.Statement("delete"), check, "version"),
	}
}

// Verify prepares every statement and the projection's field contract
// against the live schema.
func (s *store) Verify(ctx context.Context) error {
	return query.Verify(ctx, s.db, s.src, s.view)
}

// scanOrganization reads one projected row in the view's SELECT order.
func scanOrganization(rows *sql.Rows) (Organization, error) {
	var o Organization
	err := rows.Scan(&o.ID, &o.ParentID, &o.Code, &o.Name, &o.Version, &o.CreatedAt, &o.UpdatedAt, &o.Path)
	return o, err
}

// scanIdentity reads create's RETURNING row.
func scanIdentity(rows *sql.Rows) (Identity, error) {
	var i Identity
	err := rows.Scan(&i.ID, &i.Version)
	return i, err
}

func (s *store) list(ctx context.Context, d query.Directives) ([]Organization, int, error) {
	return s.view.List(ctx, s.db, d)
}

func (s *store) find(ctx context.Context, field, value string) (Organization, error) {
	return s.view.One(ctx, s.db, field, value)
}

func (s *store) create(ctx context.Context, c CreateOrganization) (Identity, error) {
	return s.createRows.One(ctx, s.db, query.Args{"parent_id": c.ParentID, "code": c.Code, "name": c.Name})
}

func (s *store) edit(ctx context.Context, id string, version int64, e EditOrganization) (Identity, error) {
	v, err := s.editGuard.Run(ctx, s.db, version, query.Args{"id": id, "code": e.Code, "name": e.Name})
	return Identity{ID: id, Version: v}, err
}

// transfer is the domain's first action: under the tree's lock, the cycle
// check walks the new parent's ancestor chain, then the guarded update
// moves parent_id. A nonexistent new parent falls through the walk to the
// foreign-key violation.
func (s *store) transfer(ctx context.Context, id string, version int64, t TransferOrganization) (Identity, error) {
	v, err := sqldb.Transact(ctx, s.db, func(tx *sqldb.Tx) (int64, error) {
		if _, err := s.lockTree.Exec(ctx, tx, query.Args{"name": treeLock}); err != nil {
			return 0, err
		}
		if t.ParentID != nil {
			n, err := s.inSubtree.One(ctx, tx, query.Args{"node": id, "candidate": *t.ParentID})
			if err != nil {
				return 0, err
			}
			if n > 0 {
				return 0, fmt.Errorf("%w: %s is in the subtree of %s", ErrCycle, *t.ParentID, id)
			}
		}
		return s.transferGuard.Run(ctx, tx, version, query.Args{"id": id, "parent_id": t.ParentID})
	})
	return Identity{ID: id, Version: v}, err
}

func (s *store) delete(ctx context.Context, id string, version int64) error {
	_, err := s.deleteGuard.Run(ctx, s.db, version, query.Args{"id": id})
	return err
}
