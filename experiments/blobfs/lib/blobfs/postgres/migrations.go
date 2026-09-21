package postgres

import (
	"embed"
	"fmt"

	"github.com/standards-lab/sqlate/migrate"

	"github.com/standards-lab/org/experiments/blobfs/lib/migrator"
)

const (
	// Source is the migration set's name and the prefix of every object
	// the set owns.
	Source = "blobfs"

	// Table is the history table the set's migrations are recorded in. It
	// is distinct from a consumer's own history table, so the two sets keep
	// separate version lines.
	Table = "blobfs_schema_version"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrations returns blobfs's migration set for Postgres, whole: the name
// Source, the history table Table, and the migrations in version order,
// read from the embedded migrations directory. A consumer declares it
// ahead of its own set in its migrator.
func Migrations() (migrator.Set, error) {
	files, err := migrate.Files(migrationFiles, "migrations")
	if err != nil {
		return migrator.Set{}, fmt.Errorf("blobfs/postgres: %w", err)
	}
	return migrator.Set{Name: Source, Table: Table, Migrations: files}, nil
}
