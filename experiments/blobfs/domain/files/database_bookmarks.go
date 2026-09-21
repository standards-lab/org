package files

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// bookmarkCount is the one row of file_bookmark_count: how many units
// bookmark a file.
type bookmarkCount struct {
	Bookmarks int64 `json:"bookmarks"`
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
// bookmarkSentinels lists under the class reported. The result is a
// blobfs.ViolationError, the library's own wrapper, whose message names
// the sentinel and the constraint and which keeps the
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
	return &blobfs.ViolationError{Sentinel: m.sentinel, Constraint: ce.Constraint, Err: err}
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
// under the class reported. The result is a blobfs.ViolationError over
// the library's error, so blobfs.ErrReferenced and the
// sqlate.ConstraintError stay reachable through errors.Is and errors.As
// while the message names the consumer's sentinel and the constraint.
// Any other error is returned as it came.
func classifyFileDelete(err error) error {
	var ce *sqlate.ConstraintError
	if !errors.As(err, &ce) {
		return err
	}
	m, ok := fileDeleteSentinels[ce.Constraint]
	if !ok || !errors.Is(ce.Class, m.class) {
		return err
	}
	return &blobfs.ViolationError{Sentinel: m.sentinel, Constraint: ce.Constraint, Err: err}
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

// AddBookmark records that the unit bookmarks the file at path and
// returns the file's row. With active, the bookmark becomes the unit's one
// active bookmark, and the add is refused with ErrActiveBookmark while
// another bookmark of the unit is active; the other one is left as it is.
// The resolution of the path and the insert run in one transaction, the
// consumer's pattern for a write that follows a read, and the transaction
// is where a service would insert the bookmark beside the pending row of
// its own upload.
//
// A file that does not exist, or a parent that does not, is
// blobfs.ErrNotFound, and so is a file removed between its resolution and
// the insert, which the foreign key reports. A pending file can be
// bookmarked: a bookmark written beside a pending row is the shape a
// service uses, and the listing shows the status. A deleting file is
// refused with ErrNotAvailable, because its delete is under way and a new
// bookmark would hold it. A file the unit has bookmarked already is
// ErrAlreadyBookmarked, active or not.
func (s *Store) AddBookmark(ctx context.Context, path, unit string, active bool) (blobfs.File, error) {
	parent, name, _, err := splitParent(path)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("files: bookmark add %s: %w", path, err)
	}
	f, err := s.db.Transact(ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
		dir, err := s.blobfs.ResolveDirectory(ctx, tx, parent)
		if err != nil {
			return blobfs.File{}, err
		}
		f, err := s.blobfs.FileByName(ctx, tx, dir.ID, name)
		if err != nil {
			return blobfs.File{}, err
		}
		if f.Status == blobfs.StatusDeleting {
			return blobfs.File{}, fmt.Errorf("the file is %s: %w", f.Status, ErrNotAvailable)
		}
		if err := s.insertBookmark(ctx, tx, unit, f.ID, active); err != nil {
			return blobfs.File{}, err
		}
		return f, nil
	})
	if err != nil {
		return blobfs.File{}, fmt.Errorf("files: bookmark add %s as unit %s: %w", path, unit, err)
	}
	return f, nil
}

// RemoveBookmark removes the unit's bookmark of the file at path, active
// or not, and returns the file's row. The path is resolved and the row
// deleted on the pool: nothing needs the two to share a snapshot, since
// the delete is keyed by the unit and the file's id. A file that does not
// exist is blobfs.ErrNotFound; a file the unit has not bookmarked is
// ErrNoBookmark. Removing the active bookmark is allowed: the one-active
// rule bounds how many bookmarks are active, and removing one leaves the
// unit with none, which any later add with active may fill.
func (s *Store) RemoveBookmark(ctx context.Context, path, unit string) (blobfs.File, error) {
	f, err := s.Stat(ctx, path)
	if err != nil {
		return blobfs.File{}, err
	}
	if err := s.deleteBookmark(ctx, s.db, unit, f.ID); err != nil {
		return blobfs.File{}, fmt.Errorf("files: bookmark rm %s as unit %s: %w", path, unit, err)
	}
	return f, nil
}

// ListBookmarks returns one page of the unit's bookmarks under l, each
// with its file's path, through the consumer's bookmark read model
// filtered by the unit. The read model pages by number only, so l.After
// is ignored. Its total comes from a count statement separate from the
// page, so the two run in one read-only repeatable-read transaction and
// agree with each other. Without a sort term the page is in path order.
func (s *Store) ListBookmarks(ctx context.Context, unit string, l Listing) (Page[BookmarkedFile], error) {
	return s.db.Transact(ctx, func(tx *sqlate.Tx) (Page[BookmarkedFile], error) {
		return s.bookmarksOf(ctx, tx, unit, l)
	}, sqlate.ReadOnly(), sqlate.Isolation(sql.LevelRepeatableRead))
}
