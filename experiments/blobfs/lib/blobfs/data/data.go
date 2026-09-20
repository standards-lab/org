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

// Patterns is blobfs's published pattern source: the column lists of the
// directory and file rows, standard-tier fragments a consumer's statements
// include. The consumer registers it in its one catalog beside
// query.Patterns() and its own sources. Every pattern is parameter-free
// and includes no other pattern: a pattern cannot include one, and a
// pattern's slots would become parameters of the including statement,
// which a projection base rejects.
func Patterns() query.Source {
	return query.Publish(Namespace, patternFiles, "patterns")
}

// Store is blobfs's persistence: the embedded statements compiled once
// against the consumer's catalog and bound to their typed handles, the two
// listings, and the operations as methods. It holds no session; every
// method takes one.
type Store struct {
	stmts *query.Statements

	createDirectory    query.Statement
	directoryByID      query.Rows[blobfs.Directory]
	directoryChild     query.Rows[blobfs.Directory]
	directoryAncestors query.Rows[ancestor]
	files              listing[blobfs.File]
	children           listing[blobfs.Directory]
}

// ancestor is one row of directory_ancestors: a directory's parent and
// name on the chain up to the root, the root's both nil.
type ancestor struct {
	ParentID *string `json:"parent_id"`
	Name     *string `json:"name"`
}

// New compiles blobfs's statements against catalog for dialect and binds
// them. The catalog must carry the blobfs namespace, registered from
// Patterns(), and the query library's own namespace from query.Patterns():
// the statements include patterns of the first, and the listing composer
// fills the clause patterns of the second. A catalog without either is
// refused before compiling, naming what is missing; any other compile
// failure is returned as the loader reports it. No I/O happens here.
func New(catalog *query.Catalog, dialect sqlate.Dialect) (*Store, error) {
	if !slices.Contains(catalog.Namespaces(), Namespace) {
		return nil, fmt.Errorf("data: the catalog has no %q namespace; register data.Patterns() in it", Namespace)
	}
	clauses, err := newClauses(catalog, dialect)
	if err != nil {
		return nil, err
	}
	stmts, err := catalog.Compile(statementFiles, "statements", dialect)
	if err != nil {
		return nil, fmt.Errorf("data: %w", err)
	}
	directory := query.Scanner[blobfs.Directory]()
	files, err := newListing[blobfs.File](clauses, stmts.Statement("files_in_directory"), stmts.Statement("files_in_directory_with_total"))
	if err != nil {
		return nil, err
	}
	children, err := newListing[blobfs.Directory](clauses, stmts.Statement("children_of_directory"), stmts.Statement("children_of_directory_with_total"))
	if err != nil {
		return nil, err
	}
	return &Store{
		stmts:              stmts,
		createDirectory:    stmts.Statement("create_directory"),
		directoryByID:      stmts.Statement("directory_by_id").Scan(directory),
		directoryChild:     stmts.Statement("directory_child").Scan(directory),
		directoryAncestors: stmts.Statement("directory_ancestors").Scan(query.Scanner[ancestor]()),
		files:              files,
		children:           children,
	}, nil
}

// Statements returns the compiled inventory in name order, for a consumer
// that lists or registers the SQL its program runs.
func (s *Store) Statements() []query.Statement {
	return s.stmts.Statements()
}

// Verify prepares every statement as authored and one canonical rendering
// of each listing statement (every declared field filtered and sorted,
// with the paging clause) against the schema the session reaches, so a
// statement the migrated schema no longer satisfies fails at startup and
// not at first use.
func (s *Store) Verify(ctx context.Context, sess sqlate.Session) error {
	return query.Verify(ctx, sess, s.stmts, s.files, s.children)
}
