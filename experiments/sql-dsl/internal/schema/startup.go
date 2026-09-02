package schema

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/standards-lab/go-core/lifecycle"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/config"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

// Stage is the lifecycle stage the schema service runs in: after the
// database pool (0), before the domains verify their statements (2).
const Stage = 1

// Register declares the schema stage on lc under cfg's mode. It has a Start
// and no Shutdown or readiness: a schema is current or startup fails.
func Register(lc *lifecycle.Coordinator, db *sqldb.DB, cfg config.SchemaConfig, logger *slog.Logger) {
	lc.Add(lifecycle.Service{
		Name:  "schema",
		Stage: Stage,
		Start: func(ctx context.Context) error {
			return Start(ctx, db, cfg, logger)
		},
	})
}

// Start runs the configured mode: verify checks the history is the set's
// clean head; apply brings it there under the lock, then verifies; none
// skips. The management surface and the -schema mode call the same
// migrator functions.
func Start(ctx context.Context, db *sqldb.DB, cfg config.SchemaConfig, logger *slog.Logger) error {
	if cfg.Mode == config.SchemaNone {
		logger.Info("schema stage skipped", "mode", cfg.Mode)
		return nil
	}
	m, err := NewMigrator(db, logger)
	if err != nil {
		return err
	}
	if cfg.Mode == config.SchemaApply {
		if err := m.Up(ctx); err != nil {
			return fmt.Errorf("apply: %w", err)
		}
	}
	if err := m.Verify(ctx); err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	v, err := m.Version(ctx)
	if err != nil {
		return err
	}
	logger.Info("schema current", "mode", cfg.Mode, "version", v.Version)
	return nil
}
