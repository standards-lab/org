package schema

import (
	"embed"
	"fmt"
	"log/slog"

	"github.com/standards-lab/org/experiments/sql-dsl/lib/migrate"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

//go:embed migrations/*.sql
var files embed.FS

// Migrations is the embedded set. A layout defect is a wiring defect and
// panics at cold start.
func Migrations() []migrate.Migration {
	ms, err := migrate.Files(files, "migrations")
	if err != nil {
		panic(fmt.Sprintf("schema: %v", err))
	}
	return ms
}

// NewMigrator builds the migrator startup and the -schema mode share.
func NewMigrator(db *sqldb.DB, logger *slog.Logger) (*migrate.Migrator, error) {
	return migrate.New(db, Migrations(), migrate.Options{Logger: logger})
}
