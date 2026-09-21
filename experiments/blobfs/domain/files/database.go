package files

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

//go:embed statements/*.sql
var statementFiles embed.FS

// Store is the domain's client. It holds the program's one pattern
// catalog, the consumer's statements compiled against that catalog and
// bound to typed handles, blobfs's persistence compiled against the same
// catalog, and the object store, opened on the first file command that
// needs it. Its methods are the domain's operations. This file is the
// only one in the package that imports the query library: it converts a
// Listing to the library's listing and to the query library's directives,
// and it wraps each consumer statement in a typed method, so blobfs.go
// composes operations without naming the query library.
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

// bookmarkCount is the one row of file_bookmark_count: how many units
// bookmark a file.
type bookmarkCount struct {
	Bookmarks int64 `json:"bookmarks"`
}

// Option configures New beyond its required arguments.
type Option func(*options)

// options collects what the options set.
type options struct {
	variant VariantConstructor
}

// VariantConstructor builds the data.Variant blobfs's store forwards its
// variation points to, against the program's catalog and the database's
// dialect: the shape of pgnative.New and of data.NewStandard, wrapped to
// return the interface.
type VariantConstructor func(catalog *query.Catalog, dialect sqlate.Dialect) (data.Variant, error)

// WithVariant makes New build blobfs's store over the variant that build
// returns, compiled against the same catalog as the store's own
// statements, instead of the standard baseline. The composition root
// chooses the variant; the tests run the consumer over each.
func WithVariant(build VariantConstructor) Option {
	return func(o *options) { o.variant = build }
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

// bookmarkMapping is what a constraint of the bookmark table means on an
// insert: the violation class the constraint reports and the sentinel
// that class means there.
type bookmarkMapping struct {
	class    error
	sentinel error
}

// bookmarkSentinels maps the constraints a bookmark insert can violate to
// the sentinel each one means there: the primary key is a bookmark the
// unit holds already, the partial unique index is another active
// bookmark, and the foreign key is a file that no longer exists. The names
// are the consumer's own, so blobfs's write mapping never sees them and
// the consumer classifies its own constraints where it names them.
var bookmarkSentinels = map[string]bookmarkMapping{
	ConstraintPrimaryKeyBookmark:     {sqlate.ErrUniqueViolation, ErrAlreadyBookmarked},
	ConstraintUniqueBookmarkActive:   {sqlate.ErrUniqueViolation, ErrActiveBookmark},
	ConstraintForeignKeyBookmarkFile: {sqlate.ErrForeignKeyViolation, blobfs.ErrNotFound},
}

// classifyBookmark maps a constraint violation from a bookmark insert to
// the consumer's sentinel when the violated constraint is one
// bookmarkSentinels lists under the class reported, keeping the
// sqlate.ConstraintError reachable through errors.As. Any other error is
// returned as it came.
func classifyBookmark(err error) error {
	var ce *sqlate.ConstraintError
	if !errors.As(err, &ce) {
		return err
	}
	m, ok := bookmarkSentinels[ce.Constraint]
	if !ok || !errors.Is(ce.Class, m.class) {
		return err
	}
	return fmt.Errorf("%w: %w", m.sentinel, err)
}

// insertBookmark writes the bookmark of the file with fileID for the unit
// with unitID through sess, active or not. A violated constraint reaches
// the caller classified.
func (s *Store) insertBookmark(ctx context.Context, sess sqlate.Session, unitID, fileID string, active bool) error {
	_, err := s.createBookmark.Exec(ctx, sess, query.Args{"unit_id": unitID, "file_id": fileID, "active": active})
	if err != nil {
		return fmt.Errorf("files: bookmark file %s for unit %s: %w", fileID, unitID, classifyBookmark(err))
	}
	return nil
}

// fileDeleteSentinels maps the consumer's constraints a file's removal can
// violate to the sentinel each one means there: the bookmark table's
// foreign key means a unit still bookmarks the file. The library reports
// the violation as blobfs.ErrReferenced by class and leaves the name to
// the consumer; the same key means a missing file on a bookmark insert.
var fileDeleteSentinels = map[string]bookmarkMapping{
	ConstraintForeignKeyBookmarkFile: {sqlate.ErrForeignKeyViolation, ErrBookmarked},
}

// classifyFileDelete maps a refusal of a file's removal to the consumer's
// sentinel when the violated constraint is one fileDeleteSentinels lists
// under the class reported, keeping the sqlate.ConstraintError reachable
// through errors.As. Any other error is returned as it came.
func classifyFileDelete(err error) error {
	var ce *sqlate.ConstraintError
	if !errors.As(err, &ce) {
		return err
	}
	m, ok := fileDeleteSentinels[ce.Constraint]
	if !ok || !errors.Is(ce.Class, m.class) {
		return err
	}
	return fmt.Errorf("%w: %w", m.sentinel, err)
}

// bookmarksOfFile returns how many units bookmark the file with fileID,
// through sess.
func (s *Store) bookmarksOfFile(ctx context.Context, sess sqlate.Session, fileID string) (int64, error) {
	c, err := s.bookmarkCount.One(ctx, sess, query.Args{"file_id": fileID})
	if err != nil {
		return 0, fmt.Errorf("files: bookmarks of file %s: %w", fileID, err)
	}
	return c.Bookmarks, nil
}

// deleteBookmark removes the bookmark of the file with fileID for the
// unit with unitID through sess. No row affected is ErrNoBookmark.
func (s *Store) deleteBookmark(ctx context.Context, sess sqlate.Session, unitID, fileID string) error {
	n, err := s.removeBookmark.Exec(ctx, sess, query.Args{"unit_id": unitID, "file_id": fileID})
	if err != nil {
		return fmt.Errorf("files: remove bookmark of file %s for unit %s: %w", fileID, unitID, err)
	}
	if n == 0 {
		return fmt.Errorf("files: remove bookmark of file %s for unit %s: %w", fileID, unitID, ErrNoBookmark)
	}
	return nil
}

// bookmarksOf lists the bookmarks of the unit with unitID through sess:
// one page of the bookmark read model under l, filtered by unit_id,
// sorted by the caller's terms or by path when there are none, with
// file_id appended by the projection as the tie-breaker. The projection
// always runs its count statement, so under TotalNone the count is read
// and dropped and the page reports NoTotal.
func (s *Store) bookmarksOf(ctx context.Context, sess sqlate.Session, unitID string, l Listing) (Page[BookmarkedFile], error) {
	sort := sortTerms(l.Sort, nil)
	if len(sort) == 0 {
		sort = []query.Sort{{Field: "path"}}
	}
	d := query.Directives{
		Page:    query.Page{Number: l.Page, Size: l.Size},
		Sort:    sort,
		Filters: []query.Filter{{Field: "unit_id", Op: query.OpEq, Value: unitID}},
	}
	rows, total, err := s.bookmarks.List(ctx, sess, d)
	if err != nil {
		return Page[BookmarkedFile]{}, fmt.Errorf("files: bookmarks of %s: %w", unitID, err)
	}
	if l.Total == TotalNone {
		total = NoTotal
	}
	return Page[BookmarkedFile]{Rows: rows, Total: total}, nil
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

// deleteOwner removes the ownership row of a directory inside tx, if the
// directory has one; none affected is not an error. The statement
// requires a transaction, so a pool session is refused.
func (s *Store) deleteOwner(ctx context.Context, tx *sqlate.Tx, directoryID string) error {
	if _, err := s.removeOwner.Exec(ctx, tx, query.Args{"directory_id": directoryID}); err != nil {
		return fmt.Errorf("files: remove owner of %s: %w", directoryID, err)
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
