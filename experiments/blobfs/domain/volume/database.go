package volume

import (
	"context"
	"embed"
	"errors"
	"fmt"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

//go:embed statements/*.sql
var files embed.FS

// ErrVerify reports a database the consumer's or blobfs's statements do not
// prepare against. The usual cause is a schema that is not applied, so the
// message says which command applies it.
var ErrVerify = errors.New("volume: the database does not satisfy the statements; if the schema is not applied, run blobfs schema up")

// Store is the domain's SQL client: the one pattern catalog of the program,
// the consumer's statements of statements/ compiled against it and bound to
// their typed handles, blobfs's persistence compiled against the same
// catalog, and the operations as methods. This file is the package's sole
// importer of the query library: it lowers a Listing to the library's
// directives and wraps each consumer statement in a typed method, so the
// translation file over blobfs, blobfs.go, composes operations without
// naming the library.
type Store struct {
	db      *sqlate.DB
	catalog *query.Catalog
	stmts   *query.Statements

	volumeView    query.Projection[VolumeEntry]
	fileView      query.Projection[FileEntry]
	createOwner   query.Statement
	ownerByVolume query.Rows[Owner]

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
		return nil, fmt.Errorf("volume: %w", err)
	}
	s := &Store{db: db, catalog: catalog}
	if err := s.compileLibrary(); err != nil {
		return nil, err
	}
	stmts, err := catalog.Compile(files, "statements", db.Dialect())
	if err != nil {
		return nil, fmt.Errorf("volume: %w", err)
	}
	s.stmts = stmts
	s.volumeView = stmts.Statement("volume_view").Project(query.Scanner[VolumeEntry]())
	s.fileView = stmts.Statement("file_view").Project(query.Scanner[FileEntry]())
	s.createOwner = stmts.Statement("create_owner")
	s.ownerByVolume = stmts.Statement("owner_by_volume").Scan(query.Scanner[Owner]())
	return s, nil
}

// Verify prepares every statement and every projection's field contract,
// the consumer's and blobfs's, against the database, so a schema that is
// not applied or no longer matches the statements fails before a command
// does any work. A failure wraps ErrVerify and the joined causes.
func (s *Store) Verify(ctx context.Context) error {
	if err := query.Verify(ctx, s.db, s.stmts, s.volumeView, s.fileView, s.blobfs); err != nil {
		return fmt.Errorf("%w: %w", ErrVerify, err)
	}
	return nil
}

// Volumes lists the volumes with their owners under l: one page of
// volume_view, sorted by its declared fields, filtered by unit when l names
// one, and the total under the same filter.
func (s *Store) Volumes(ctx context.Context, l Listing) ([]VolumeEntry, int, error) {
	return s.volumeView.List(ctx, s.db, directives(l))
}

// insertOwner writes the ownership row of a volume inside tx. The statement
// requires a transaction, so a pool session is refused.
func (s *Store) insertOwner(ctx context.Context, tx *sqlate.Tx, volumeID, unitID string) error {
	_, err := s.createOwner.Exec(ctx, tx, query.Args{"volume_id": volumeID, "unit_id": unitID})
	if err != nil {
		return fmt.Errorf("volume: create owner of %s: %w", volumeID, err)
	}
	return nil
}

// owner reads the ownership row of the volume with volumeID, or
// sql.ErrNoRows when the volume has none.
func (s *Store) owner(ctx context.Context, volumeID string) (Owner, error) {
	return s.ownerByVolume.One(ctx, s.db, query.Args{"volume_id": volumeID})
}

// directives lowers a Listing to the query library's directives: the page,
// the sort terms in order, and an equality filter on unit_id when the
// listing names a unit. The filter is the shape of the auth strategy's
// scope predicate over the consumer's join table, stated as a directive
// because a projection base binds no parameters of its own.
func directives(l Listing) query.Directives {
	d := query.Directives{Page: query.Page{Number: l.Page, Size: l.Size}}
	for _, s := range l.Sort {
		d.Sort = append(d.Sort, query.Sort{Field: s.Field, Descending: s.Descending})
	}
	if l.Unit != "" {
		d.Filters = []query.Filter{{Field: "unit_id", Op: query.OpEq, Value: l.Unit}}
	}
	return d
}
