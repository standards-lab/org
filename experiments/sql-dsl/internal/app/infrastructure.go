package app

import (
	"fmt"
	"io"
	"log/slog"

	"github.com/standards-lab/go-core/lifecycle"
	"github.com/standards-lab/go-core/logging"
	"github.com/standards-lab/go-database/postgres"
	"github.com/standards-lab/org/experiments/sql-dsl/admin/database"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/config"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/data"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/pgdialect"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/query"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

// Infrastructure holds the composed infrastructure services. SQL is the
// database as the domains see it: the session every statement runs
// through, over the pool whose lifecycle registers here, grouped with the
// pattern catalog the statements compile against. The session is one
// object at promotion; the spike's wrapper reaches the v0.3.0 lifecycle
// through Base. The layers receive its fields at their construction sites,
// never the struct.
type Infrastructure struct {
	Logger *slog.Logger
	SQL    *data.Database
}

// newInfrastructure constructs the infrastructure services in one place,
// each registering on lc where it's built, so a service can't exist without
// a startup, shutdown, or readiness declaration: the database registers at
// stage 0 with its readiness check, ahead of the root stage. Construction
// performs no I/O. The pattern catalog is registered here, once: the
// library's namespace, the application's from the admin domain, and — for
// a port — an engine's overlay; every domain compiles against it.
func newInfrastructure(
	w io.Writer,
	cfg *config.Config,
	lc *lifecycle.Coordinator,
) (*Infrastructure, error) {
	logger := logging.New(w, cfg.Log)

	db, err := postgres.New(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	lc.Add(lifecycle.Service{
		Name:     "database",
		Stage:    0,
		Start:    db.Start,
		Shutdown: db.Shutdown,
		Check:    db,
	})

	catalog, err := query.NewCatalog(query.Patterns(), database.Patterns())
	if err != nil {
		return nil, fmt.Errorf("patterns: %w", err)
	}
	return &Infrastructure{
		Logger: logger,
		SQL:    data.New(sqldb.Wrap(db, pgdialect.Wrap(db.Dialect())), catalog),
	}, nil
}
