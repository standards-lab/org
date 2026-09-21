// Package migrations is the third layer of blobfs: the DDL of the two
// tables, embedded and exported as a migration source. A consumer adds the
// source to its migrator ahead of its own set, under the history table
// Table, and blobfs's schema is at its head before the consumer's
// migrations reference it.
//
// The set runs in the order directory, file, file_created_index. The
// third migration adds the index a listing sorted by creation time uses,
// and it is the upgrade rehearsal: a version added over an installed
// schema. The directory migration seeds
// the one root directory, the row with no parent and no name and the id
// blobfs.RootID, and a partial unique index allows no second row without a
// parent. A check constraint states that a directory has no name exactly
// when it has no parent, so a root with a name or a non-root without one
// is refused.
//
// The source owns every object it creates, and every object's name starts
// with the source name and an underscore: the tables blobfs_directory and
// blobfs_file, their constraints and indexes, and the history table
// blobfs_schema_version. Constraint and index names are public API,
// because a violation reaches a consumer as
// sqlate.ConstraintError.Constraint, and the persistence layer maps
// blobfs's own constraints to its sentinel errors. The scheme is
// blobfs_<kind>_<table>_<detail>, where kind is pk, fk, uq, cc, or ix (a
// plain index), table is the table name without its blobfs_ prefix, and
// detail names the referenced relation, the indexed columns, or the
// checked rule: blobfs_fk_directory_parent,
// blobfs_uq_directory_parent_name, blobfs_cc_directory_root_name,
// blobfs_ix_file_directory_created.
//
// The DDL is Postgres at v1 and lives in the postgres directory. A second
// engine adds a directory with the same file names, and Migrations selects
// the directory by the dialect's name. A released file never changes in
// text or name; the golden-hash test in this package pins each one.
package migrations

import (
	"embed"
	"errors"
	"fmt"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/migrate"
)

const (
	// Source is the migration source's name and the prefix of every object
	// the source owns.
	Source = "blobfs"

	// Table is the history table the source's migrations are recorded in.
	// It is distinct from a consumer's own history table, so the two sets
	// keep separate version lines.
	Table = "blobfs_schema_version"
)

// ErrUnsupportedEngine reports a dialect the source ships no DDL for.
var ErrUnsupportedEngine = errors.New("blobfs/migrations: no migrations for engine")

//go:embed postgres/*.sql
var files embed.FS

// Migrations returns the source's migration set for dialect, in version
// order, read from the embedded directory named after the engine. A dialect
// the source has no directory for is an ErrUnsupportedEngine naming the
// engine.
func Migrations(dialect sqlate.Dialect) ([]migrate.Migration, error) {
	name := dialect.Name()
	switch name {
	case "postgres":
		return migrate.Files(files, name)
	}
	return nil, fmt.Errorf("%w %q", ErrUnsupportedEngine, name)
}
