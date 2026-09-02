package infrastructure

import (
	"fmt"
	"io"
	"log/slog"

	"github.com/standards-lab/go-core/lifecycle"
	"github.com/standards-lab/go-core/logging"
	"github.com/standards-lab/go-database"
	"github.com/standards-lab/go-database/postgres"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/config"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/schema"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/pgdialect"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

// Infrastructure holds the composed infrastructure services. DB is the
// lifecycle wrapper (start, shutdown, readiness); SQL is the session over it
// that every statement runs through, with the postgres lock capability.
type Infrastructure struct {
	Logger *slog.Logger
	DB     *database.DB
	SQL    *sqldb.DB
}

// New constructs the infrastructure services in one place, each registering
// on lc where it's built, so a service can't exist without a startup,
// shutdown, or readiness declaration: the database registers at stage 0 with
// its readiness check, and the schema stage at 1, ahead of the root stage.
// Construction performs no I/O. A nil lc composes the services without a lifecycle, for the one-shot
// schema mode that drives the database's start and shutdown itself.
func New(
	w io.Writer,
	cfg *config.Config,
	lc *lifecycle.Coordinator,
) (*Infrastructure, error) {
	logger := logging.New(w, cfg.Log)

	db, err := postgres.New(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}

	sqlDB := sqldb.Wrap(db, pgdialect.Wrap(db.Dialect()))

	if lc != nil {
		lc.Add(lifecycle.Service{
			Name:     "database",
			Stage:    0,
			Start:    db.Start,
			Shutdown: db.Shutdown,
			Check:    db,
		})
		schema.Register(lc, sqlDB, cfg.Schema, logger)
	}

	return &Infrastructure{
		Logger: logger,
		DB:     db,
		SQL:    sqlDB,
	}, nil
}
