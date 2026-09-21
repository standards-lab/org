package files

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"
)

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

// ownedByUnit reports whether the unit with unitID owns the directory with
// directoryID, through sess: one owner read. A directory with no owner
// row, or one another unit owns, is not owned. It is the owner-row half
// of the ownership check, shared by the path form, which runs it at the
// depth-one ancestor of the path, and by the scope check by id.
func (s *Store) ownedByUnit(ctx context.Context, sess sqlate.Session, directoryID, unitID string) (bool, error) {
	o, owned, err := s.owner(ctx, sess, directoryID)
	if err != nil {
		return false, err
	}
	return owned && o.UnitID == unitID, nil
}

// ownedBy lists the directories the unit with unitID owns through sess:
// one page of owned_directories under l, sorted by the terms the
// directory half takes, filtered by unit_id. The projection always runs
// its count statement, so under TotalNone the count is read and dropped;
// the page then reports NoTotal like the library's listings do. More is
// derived from that count in both modes, since the projection fetches
// exactly its page size and no row beyond it.
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
	p := Page[OwnedDirectory]{Rows: rows, Total: total, More: more(l, len(rows), total)}
	if l.Total == TotalNone {
		p.Total = NoTotal
	}
	return p, nil
}
