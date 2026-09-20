// Package migrations is the consumer's own migration set: the
// directory_owner table, which binds a blobfs directory to the unit that
// owns it and stands in for the join table an application's scope
// predicate filters at the directory grain, and the bookmark table, which
// joins a unit to one of the files it may reach, with one active bookmark
// per unit. The consumer's migrations reference the blobfs tables freely,
// so the set runs after blobfs's set and under sqlate's default history
// table.
//
// Table, constraint, and index names carry the workspace's prefixes (pk_,
// fk_, uq_, cc_, ix_) and never blobfs_, so a consumer's object is
// distinguishable from one blobfs owns, and a violation of a consumer
// constraint from one of blobfs's own.
package migrations

import (
	"embed"

	"github.com/standards-lab/sqlate/migrate"
)

//go:embed postgres/*.sql
var files embed.FS

// Migrations returns the consumer's Postgres set in version order.
func Migrations() ([]migrate.Migration, error) {
	return migrate.Files(files, "postgres")
}
