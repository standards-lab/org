package files

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

//go:embed statements/*.sql
var statementFiles embed.FS

// Store is the domain's SQL client. It holds the program's one pattern
// catalog, the consumer's statements compiled against that catalog and
// bound to typed handles, and blobfs's persistence compiled against the
// same catalog. Its methods are the domain's operations. This file is the
// only one in the package that imports the query library: it converts a
// Listing to the library's listing and to the query library's directives,
// and it wraps each consumer statement in a typed method, so blobfs.go
// composes operations without naming the query library.
type Store struct {
	db      *sqlate.DB
	catalog *query.Catalog
	stmts   *query.Statements

	createOwner      query.Statement
	ownerOfDirectory query.Rows[DirectoryOwner]
	ownedDirectories query.Projection[OwnedDirectory]

	blobfs *data.Store
}

// New builds the catalog from the query library's patterns and blobfs's
// published namespace, compiles blobfs's statements and then the consumer's
// against it for db's dialect, and binds the handles. A compile failure is
// returned as the loader reports it; no I/O happens here. The database is
// verified separately, by Verify, when a command first uses the store.
func New(db *sqlate.DB) (*Store, error) {
	catalog, err := query.NewCatalog(query.Patterns(), data.Patterns())
	if err != nil {
		return nil, fmt.Errorf("files: %w", err)
	}
	s := &Store{db: db, catalog: catalog}
	if err := s.compileLibrary(); err != nil {
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
	return s, nil
}

// Verify prepares every statement and every listing's field contract, the
// consumer's and blobfs's, against the database, so a schema that is not
// applied or no longer matches the statements fails before a command does
// any work. A failure wraps ErrVerify and the joined causes.
func (s *Store) Verify(ctx context.Context) error {
	if err := query.Verify(ctx, s.db, s.stmts, s.ownedDirectories, s.blobfs); err != nil {
		return fmt.Errorf("%w: %w", ErrVerify, err)
	}
	return nil
}

// insertOwner writes the ownership row of a directory inside tx. The
// statement requires a transaction, so a pool session is refused.
func (s *Store) insertOwner(ctx context.Context, tx *sqlate.Tx, directoryID, unitID string) error {
	_, err := s.createOwner.Exec(ctx, tx, query.Args{"directory_id": directoryID, "unit_id": unitID})
	if err != nil {
		return fmt.Errorf("files: create owner of %s: %w", directoryID, err)
	}
	return nil
}

// owner reads the ownership row of the directory with directoryID through
// sess. The bool reports whether the directory has one.
func (s *Store) owner(ctx context.Context, sess sqlate.Session, directoryID string) (DirectoryOwner, bool, error) {
	o, err := s.ownerOfDirectory.One(ctx, sess, query.Args{"directory_id": directoryID})
	if errors.Is(err, sql.ErrNoRows) {
		return DirectoryOwner{}, false, nil
	}
	if err != nil {
		return DirectoryOwner{}, false, fmt.Errorf("files: owner of %s: %w", directoryID, err)
	}
	return o, true, nil
}

// ownedBy lists the directories the unit with unitID owns through sess:
// one page of owned_directories under l, sorted by the terms the
// directory half takes, filtered by unit_id. The projection always runs
// its count statement, so under TotalNone the count is read and dropped;
// the page then reports NoTotal like the library's listings do.
func (s *Store) ownedBy(ctx context.Context, sess sqlate.Session, unitID string, l Listing) (Page[OwnedDirectory], error) {
	d := query.Directives{
		Page:    query.Page{Number: l.Page, Size: l.Size},
		Sort:    sortTerms(l.Sort, directoryFields),
		Filters: []query.Filter{{Field: "unit_id", Op: query.OpEq, Value: unitID}},
	}
	rows, total, err := s.ownedDirectories.List(ctx, sess, d)
	if err != nil {
		return Page[OwnedDirectory]{}, fmt.Errorf("files: directories owned by %s: %w", unitID, err)
	}
	if l.Total == TotalNone {
		total = NoTotal
	}
	return Page[OwnedDirectory]{Rows: rows, Total: total}, nil
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
// the total, NoTotal when the library reports none, and the next cursor.
func page[T any](p data.Page[T]) Page[T] {
	total := p.Total
	if total == data.NoTotal {
		total = NoTotal
	}
	return Page[T]{Rows: p.Rows, Total: total, Next: p.Next}
}
