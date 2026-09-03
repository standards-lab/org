package person

import (
	"context"
	"embed"
	"fmt"

	"github.com/standards-lab/go-database"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/data"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/query"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

//go:embed statements/*.sql
var files embed.FS

// store is the domain's SQL client: the statements of sql/ bound once to
// their typed handles, and the operations as methods named for their
// commands and actions. The entities' tags are the scan and binding
// contract. It is the package's sole importer of query.
type store struct {
	db                *sqldb.DB
	stmts             *query.Statements
	view              query.Projection[Person]
	createRows        query.Rows[Identity]
	stateRows         query.Rows[state]
	editGuard         query.Guard
	deleteGuard       query.Guard
	activateGuard     query.Guard
	deactivateGuard   query.Guard
	transferUnitGuard query.Guard
}

func newStore(db *data.Database) *store {
	stmts := db.Catalog.MustCompile(files, "statements", db.Dialect())
	check := stmts.Statement("version")
	guard := func(name string) query.Guard { return query.Guarded(stmts.Statement(name), check, "version") }
	return &store{
		db:                db.DB,
		stmts:             stmts,
		view:              query.Project(stmts.Statement("person_view"), query.Scanner[Person]()),
		createRows:        query.Scan(stmts.Statement("create"), query.Scanner[Identity]()),
		stateRows:         query.Scan(stmts.Statement("state"), query.Scanner[state]()),
		editGuard:         guard("edit"),
		deleteGuard:       guard("delete"),
		activateGuard:     guard("activate"),
		deactivateGuard:   guard("deactivate"),
		transferUnitGuard: guard("transfer_unit"),
	}
}

// Verify prepares every statement and the read contract against the live
// schema.
func (s *store) Verify(ctx context.Context) error {
	return query.Verify(ctx, s.db, s.stmts, s.view)
}

func (s *store) list(ctx context.Context, d query.Directives) ([]Person, int, error) {
	return s.view.List(ctx, s.db, d)
}

func (s *store) find(ctx context.Context, id string) (Person, error) {
	return s.view.One(ctx, s.db, "id", id)
}

func (s *store) create(ctx context.Context, c CreatePerson) (Identity, error) {
	return s.createRows.One(ctx, s.db, query.ArgsOf(c))
}

func (s *store) edit(ctx context.Context, id string, version int64, e EditPerson) (Identity, error) {
	v, err := s.editGuard.Run(ctx, s.db, version, query.ArgsOf(e).With("id", id))
	return Identity{ID: id, Version: v}, err
}

func (s *store) delete(ctx context.Context, id string, version int64) error {
	_, err := s.deleteGuard.Run(ctx, s.db, version, query.Args{"id": id})
	return err
}

// The actions: each reads the record's state inside the transaction,
// checks the client's version against it — a stale view is the primary
// fact, answered before any rule — applies the transition rule, and runs
// its guarded update. The guard then only catches a change between the
// read and the update, which keeps the read-then-write race-safe.

func (s *store) activate(ctx context.Context, id string, version int64) (Identity, error) {
	return s.transition(ctx, id, version, state.canActivate, s.activateGuard, query.Args{"id": id})
}

func (s *store) deactivate(ctx context.Context, id string, version int64) (Identity, error) {
	return s.transition(ctx, id, version, state.canDeactivate, s.deactivateGuard, query.Args{"id": id})
}

func (s *store) transferUnit(ctx context.Context, id string, version int64, t TransferUnit) (Identity, error) {
	allowAny := func(state) error { return nil }
	return s.transition(ctx, id, version, allowAny, s.transferUnitGuard, query.ArgsOf(t).With("id", id))
}

// transition is the action protocol: read state, check the version, check
// the rule, run the guard. A missing record is sql.ErrNoRows from the read.
func (s *store) transition(ctx context.Context, id string, version int64, rule func(state) error, g query.Guard, args query.Args) (Identity, error) {
	v, err := sqldb.Transact(ctx, s.db, func(tx *sqldb.Tx) (int64, error) {
		st, err := s.stateRows.One(ctx, tx, query.Args{"id": id})
		if err != nil {
			return 0, err
		}
		if st.Version != version {
			return 0, fmt.Errorf("%w: expected %d, current %d", database.ErrVersionMismatch, version, st.Version)
		}
		if err := rule(st); err != nil {
			return 0, err
		}
		return g.Run(ctx, tx, version, args)
	})
	return Identity{ID: id, Version: v}, err
}
