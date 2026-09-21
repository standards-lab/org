package files

import (
	"context"
	"embed"
	"fmt"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

//go:embed statements/*.sql
var statementFiles embed.FS

// Store is the domain's client. It holds the program's one pattern
// catalog, the consumer's statements compiled against that catalog and
// bound to typed handles, blobfs's persistence compiled against the same
// catalog, and the object store, opened on the first file command that
// needs it. Its methods are the domain's operations. Only database.go and
// the database_<concern>.go files import the query library: database.go
// converts a Listing to the library's listing and to the query library's
// directives, and the other files wrap each consumer statement in a typed
// method, so the blobfs_<concern>.go files compose operations without
// naming the query library.
type Store struct {
	db      *sqlate.DB
	catalog *query.Catalog
	stmts   *query.Statements

	openStorage func(context.Context) (*Storage, error)
	storage     *Storage

	createOwner      query.Statement
	ownerOfDirectory query.Rows[DirectoryOwner]
	ownedDirectories query.Projection[OwnedDirectory]
	removeOwner      query.Statement
	createBookmark   query.Statement
	removeBookmark   query.Statement
	bookmarks        query.Projection[BookmarkedFile]
	bookmarkCount    query.Rows[bookmarkCount]

	blobfs *data.Store
}

// Option configures New beyond its required arguments.
type Option func(*options)

// options collects what the options set.
type options struct {
	variant VariantConstructor
}

// VariantConstructor builds the data.Variant blobfs's store forwards its
// variation points to, against the program's catalog and the database's
// dialect: the shape of postgres.New and of data.NewStandard, with the
// result as the interface.
type VariantConstructor func(catalog *query.Catalog, dialect sqlate.Dialect) (data.Variant, error)

// WithVariant makes New build blobfs's store over the variant that build
// returns, compiled against the same catalog as the store's own
// statements, instead of the standard baseline. build is any constructor
// whose result implements data.Variant, so postgres.New passes as it is:
// Go does not convert a function that returns *postgres.Variant to one
// that returns the interface, and the type parameter does that wrapping
// here, where the query library's types are named already. The
// composition root chooses the variant; the tests run the consumer over
// each.
func WithVariant[V data.Variant](build func(catalog *query.Catalog, dialect sqlate.Dialect) (V, error)) Option {
	return func(o *options) {
		o.variant = func(catalog *query.Catalog, dialect sqlate.Dialect) (data.Variant, error) {
			v, err := build(catalog, dialect)
			if err != nil {
				return nil, err
			}
			return v, nil
		}
	}
}

// New builds the catalog from the query library's patterns and blobfs's
// published namespace, compiles blobfs's statements and then the consumer's
// against it for db's dialect, and binds the handles. A compile failure is
// returned as the loader reports it; no I/O happens here. The database is
// verified separately, by Verify, when a command first uses the store.
//
// openStorage opens the object store, and the store calls it once, on the
// first operation that needs an object: the directory commands never open
// it, so they run without the store's configuration and without the
// store reachable. A nil openStorage builds a store with no object store,
// whose file operations fail with ErrNoStorage; the hermetic tests of the
// directory commands do that. The options choose blobfs's variant; without
// WithVariant the store runs the standard baseline.
func New(db *sqlate.DB, openStorage func(context.Context) (*Storage, error), opts ...Option) (*Store, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	catalog, err := query.NewCatalog(query.Patterns(), data.Patterns())
	if err != nil {
		return nil, fmt.Errorf("files: %w", err)
	}
	s := &Store{db: db, catalog: catalog, openStorage: openStorage}
	if err := s.compileLibrary(o.variant); err != nil {
		return nil, err
	}
	stmts, err := catalog.Compile(statementFiles, "statements", db.Dialect())
	if err != nil {
		return nil, fmt.Errorf("files: %w", err)
	}
	s.stmts = stmts
	s.createOwner = stmts.Statement("create_directory_owner")
	s.ownerOfDirectory = stmts.Statement("owner_of_directory").Scan(query.Scanner[DirectoryOwner]())
	s.ownedDirectories = stmts.Statement("owned_directories").Project(query.Scanner[OwnedDirectory]())
	s.removeOwner = stmts.Statement("remove_directory_owner")
	s.createBookmark = stmts.Statement("create_bookmark")
	s.removeBookmark = stmts.Statement("remove_bookmark")
	s.bookmarks = stmts.Statement("bookmarks").Project(query.Scanner[BookmarkedFile]())
	s.bookmarkCount = stmts.Statement("file_bookmark_count").Scan(query.Scanner[bookmarkCount]())
	return s, nil
}

// Verify prepares every statement and every listing's field contract, the
// consumer's and blobfs's, against the database, so a schema that is not
// applied or no longer matches the statements fails before a command does
// any work. A failure wraps ErrVerify and the joined causes.
func (s *Store) Verify(ctx context.Context) error {
	if err := query.Verify(ctx, s.db, s.stmts, s.ownedDirectories, s.bookmarks, s.blobfs); err != nil {
		return fmt.Errorf("%w: %w", ErrVerify, err)
	}
	return nil
}

// directoryFields are the fields both of the library's listings declare,
// and the owner read model declares beside its unit, so a sort term naming
// one of them applies to the directory half of ls as well as to the file
// half. The file half takes every term and refuses one it does not
// declare; a term naming a field only files have (size, status, and so on)
// sorts the files and leaves the directories in name order. parent_id is
// left out: the directory listing declares it, but every row of one
// listing shares it, so a sort by it orders nothing.
var directoryFields = map[string]bool{
	"id": true, "name": true, "version": true, "created_at": true, "updated_at": true,
}

// sortTerms lowers the sort terms to the query library's, keeping only
// those whose field the set allows; a nil set keeps every term.
func sortTerms(terms []Sort, allowed map[string]bool) []query.Sort {
	var out []query.Sort
	for _, t := range terms {
		if allowed != nil && !allowed[t.Field] {
			continue
		}
		out = append(out, query.Sort{Field: t.Field, Descending: t.Descending})
	}
	return out
}

// lower lowers a Listing to the library's listing for one half of ls: the
// page, the total mode, the half's cursor, and the sort terms, all of
// them for the file half and those naming a directory field for the
// directory half.
func lower(l Listing, allowed map[string]bool, after string) data.Listing {
	out := data.Listing{Page: l.Page, Size: l.Size, Sort: sortTerms(l.Sort, allowed), After: after}
	if l.Total == TotalNone {
		out.Total = data.TotalNone
	}
	return out
}

// page translates a library page to the consumer's: the rows as they are,
// the total, NoTotal when the library reports none, whether more rows
// remain, and the next cursor.
func page[T any](p data.Page[T]) Page[T] {
	total := p.Total
	if total == data.NoTotal {
		total = NoTotal
	}
	return Page[T]{Rows: p.Rows, Total: total, More: p.More, Next: p.Next}
}

// more reports whether rows remain after a projection's page: the page
// number and size place the page's last row against the count, which the
// projection always runs. It is how the consumer's read models derive
// More, since they page by number only and fetch exactly their size.
func more(l Listing, listed, total int) bool {
	return (l.Page-1)*l.Size+listed < total
}
