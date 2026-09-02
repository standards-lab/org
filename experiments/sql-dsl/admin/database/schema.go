package database

import (
	"embed"
	"fmt"

	"github.com/standards-lab/org/experiments/sql-dsl/lib/migrate"
)

//go:embed migrations/*.sql
var files embed.FS

// Migrations is the embedded set. A layout defect is a wiring defect and
// panics at cold start.
func Migrations() []migrate.Migration {
	ms, err := migrate.Files(files, "migrations")
	if err != nil {
		panic(fmt.Sprintf("admin/database: %v", err))
	}
	return ms
}
