package data

import (
	"context"
	"embed"
	"fmt"
	"slices"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

//go:embed patterns/*.sql
var patternFiles embed.FS

//go:embed statements/*.sql
var statementFiles embed.FS

// Namespace is the pattern namespace blobfs publishes: a consumer's
// statement includes one of its patterns as {{> blobfs.name}}.
const Namespace = "blobfs"

// Patterns is blobfs's published pattern source, the standard-tier
// fragments a consumer's base statements include. The consumer registers it
// in its one catalog beside query.Patterns() and its own sources. Every
// pattern is parameter-free and includes no other pattern: a pattern cannot
// include one, and a pattern's slots would become parameters of the
// including statement, which a projection base rejects.
func Patterns() query.Source {
	return query.Publish(Namespace, patternFiles, "patterns")
}

// Store is blobfs's persistence: the embedded statements compiled once
// against the consumer's catalog and bound to their typed handles, and the
// operations as methods. It holds no session; every method takes one.
type Store struct {
	stmts *query.Statements

	createVolume        query.Statement
	createRootDirectory query.Statement
	volumeByID          query.Rows[blobfs.Volume]
	volumeByName        query.Rows[blobfs.Volume]
	volumes             query.Projection[blobfs.Volume]
	renameVolume        query.Guard
	rootDirectory       query.Rows[blobfs.Directory]

	createDirectory query.Statement
	directoryByID   query.Rows[blobfs.Directory]
	directoryChild  query.Rows[blobfs.Directory]
	directories     query.Projection[blobfs.Directory]
}

// New compiles blobfs's statements against catalog for dialect and binds
// them. The catalog must carry the blobfs namespace, registered from
// Patterns(), and the library's own namespace from query.Patterns(): the
// statements include patterns of both. A catalog without the blobfs
// namespace is refused before compiling, naming what is missing; any other
// compile failure is returned as the loader reports it. No I/O happens
// here.
func New(catalog *query.Catalog, dialect sqlate.Dialect) (*Store, error) {
	if !slices.Contains(catalog.Namespaces(), Namespace) {
		return nil, fmt.Errorf("data: the catalog has no %q namespace; register data.Patterns() in it", Namespace)
	}
	stmts, err := catalog.Compile(statementFiles, "statements", dialect)
	if err != nil {
		return nil, fmt.Errorf("data: %w", err)
	}
	volume := query.Scanner[blobfs.Volume]()
	directory := query.Scanner[blobfs.Directory]()
	return &Store{
		stmts:               stmts,
		createVolume:        stmts.Statement("create_volume"),
		createRootDirectory: stmts.Statement("create_root_directory"),
		volumeByID:          stmts.Statement("volume_by_id").Scan(volume),
		volumeByName:        stmts.Statement("volume_by_name").Scan(volume),
		volumes:             stmts.Statement("volumes").Project(volume),
		renameVolume:        stmts.Statement("rename_volume").Guarded(stmts.Statement("volume_version"), "version"),
		rootDirectory:       stmts.Statement("root_directory").Scan(directory),
		createDirectory:     stmts.Statement("create_directory"),
		directoryByID:       stmts.Statement("directory_by_id").Scan(directory),
		directoryChild:      stmts.Statement("directory_child").Scan(directory),
		directories:         stmts.Statement("directories").Project(directory),
	}, nil
}

// Statements returns the compiled inventory in name order, for a consumer
// that lists or registers the SQL its program runs.
func (s *Store) Statements() []query.Statement {
	return s.stmts.Statements()
}

// Verify prepares every statement and both projections' field contracts
// against the schema the session reaches, so a statement the migrated
// schema no longer satisfies fails at startup and not at first use.
func (s *Store) Verify(ctx context.Context, sess sqlate.Session) error {
	return query.Verify(ctx, sess, s.stmts, s.volumes, s.directories)
}
