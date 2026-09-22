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
	stmts   *query.Statements
	variant Variant

	directoryByID      query.Rows[blobfs.Directory]
	directoryChild     query.Rows[blobfs.Directory]
	directoryAncestors query.Rows[ancestor]
	fileByID           query.Rows[blobfs.File]
	fileByName         query.Rows[blobfs.File]
	holdFile           query.Statement
	holdFileAtVersion  query.Statement
	removeFile         query.Statement
	removeDirectory    query.Statement
	directoryIsWithin  query.Rows[int64]
	reparentDirectory  query.Guard
	moveFile           query.Guard
	files              listing[blobfs.File]
	children           listing[blobfs.Directory]
}

// ancestor is one row of directory_ancestors: a directory's parent and
// name on the chain up to the root, whose parent is nil and whose name
// is /.
type ancestor struct {
	ParentID *string `json:"parent_id"`
	Name     string  `json:"name"`
}

// New compiles blobfs's statements against catalog for dialect and binds
// them. The catalog must carry the blobfs namespace, registered from
// Patterns(), and the query library's own namespace from query.Patterns():
// the statements include patterns of the first, and the listing composer
// fills the clause patterns of the second. A catalog without either is
// refused before compiling, naming what is missing; any other compile
// failure is returned as the loader reports it. No I/O happens here.
//
// The options choose the variant the store forwards its variation points
// to (see Variant); without WithVariant the store runs the standard
// baseline, built over the same compiled statements.
func New(catalog *query.Catalog, dialect sqlate.Dialect, opts ...Option) (*Store, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
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
	variant := o.variant
	if variant == nil {
		variant = newStandard(stmts)
	}
	directory := query.Scanner[blobfs.Directory]()
	file := query.Scanner[blobfs.File]()
	files, err := newListing[blobfs.File](clauses, variant, stmts.Statement("files_in_directory"), stmts.Statement("files_in_directory_with_total"))
	if err != nil {
		return nil, err
	}
	children, err := newListing[blobfs.Directory](clauses, variant, stmts.Statement("children_of_directory"), stmts.Statement("children_of_directory_with_total"))
	if err != nil {
		return nil, err
	}
	return &Store{
		stmts:              stmts,
		variant:            variant,
		directoryByID:      stmts.Statement("directory_by_id").Scan(directory),
		directoryChild:     stmts.Statement("directory_child").Scan(directory),
		directoryAncestors: stmts.Statement("directory_ancestors").Scan(query.Scanner[ancestor]()),
		fileByID:           stmts.Statement("file_by_id").Scan(file),
		fileByName:         stmts.Statement("file_by_name").Scan(file),
		holdFile:           stmts.Statement("hold_file"),
		holdFileAtVersion:  stmts.Statement("hold_file_at_version"),
		removeFile:         stmts.Statement("remove_file"),
		removeDirectory:    stmts.Statement("remove_directory"),
		directoryIsWithin:  stmts.Statement("directory_is_within").Scan(query.Scalar[int64]),
		reparentDirectory:  stmts.Statement("reparent_directory").Guarded(stmts.Statement("directory_version"), "version"),
		moveFile:           stmts.Statement("move_file").Guarded(stmts.Statement("file_version"), "version"),
		files:              files,
		children:           children,
	}, nil
}

// Statements returns the compiled inventory in name order, for a consumer
// that lists or registers the SQL its program runs: the persistence
// package's own statements, the standard baseline's among them, followed
// by the variant's own when it compiled any.
func (s *Store) Statements() []query.Statement {
	out := s.stmts.Statements()
	if inv, ok := s.variant.(inventory); ok {
		out = append(out, inv.Statements()...)
	}
	return out
}

// Verify prepares every statement as authored and one canonical rendering
// of each listing statement (every declared field filtered and sorted,
// with the paging clause) against the schema the session reaches, so a
// statement the migrated schema no longer satisfies fails at startup and
// not at first use. A variant that can verify itself is verified in the
// same pass.
func (s *Store) Verify(ctx context.Context, sess sqlate.Session) error {
	vs := []query.Verifier{s.stmts, s.files, s.children}
	if v, ok := s.variant.(query.Verifier); ok {
		vs = append(vs, v)
	}
	return query.Verify(ctx, sess, vs...)
}
