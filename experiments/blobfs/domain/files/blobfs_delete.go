package files

import (
	"context"
	"errors"
	"fmt"

	"github.com/standards-lab/sqlate"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

// Remove is the file delete as the consumer sequences it, rm <path>: the
// parent is resolved and the name looked up on the pool, whatever the
// row's status, and the row then goes through the three steps of
// deleteFile, as RemoveFile does by id without the lookup. A pending row
// (an abandoned write), an available row, and a row already deleting (an
// earlier rm that stopped) are treated alike: the steps are idempotent,
// so a rerun resumes where the earlier run stopped. A file that does not
// exist, or a parent that does not, is blobfs.ErrNotFound; after a
// finished rm the same path reports that, which is how a caller learns
// the delete is done. The row returned is the row as the begin step left
// it, deleting.
func (s *Store) Remove(ctx context.Context, path string, stopAfter Step) (blobfs.File, error) {
	parent, name, _, err := splitParent(path)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("files: rm %s: %w", path, err)
	}
	dir, err := s.blobfs.ResolveDirectory(ctx, s.db, parent)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("files: rm %s: %w", path, err)
	}
	f, err := s.blobfs.FileByName(ctx, s.db, dir.ID, name)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("files: rm %s: %w", path, err)
	}
	return s.deleteFile(ctx, path, f.ID, 0, stopAfter)
}

// RemoveFile is the file delete by id: the three steps of deleteFile for
// the file with id, with no read before them, so a caller that holds an
// id from a listing acts on it directly. A file that does not exist is
// blobfs.ErrNotFound from the begin step, and the messages name the file
// as "file <id>".
//
// version, when not 0, is the version the caller read: the begin
// transaction first holds the row at that version through
// blobfs.HoldFile, and a row that moved on is query.ErrVersionMismatch,
// with nothing begun. A row already deleting is held by nothing and is
// not refused: its version advanced when its delete began, and a rerun
// resumes that delete whatever version the caller holds, as Remove does.
// With 0 the begin step alone guards the row, by its status.
//
// With a scope, the row is read once on the pool, before the store is
// opened, so that its directory can be checked against the scope
// (InScope); a file outside the scope is ErrNotOwned with nothing begun.
func (s *Store) RemoveFile(ctx context.Context, id string, version int64, stopAfter Step, scope Scope) (blobfs.File, error) {
	label := "file " + id
	if scope != (Scope{}) {
		f, err := s.blobfs.File(ctx, s.db, id)
		if err != nil {
			return blobfs.File{}, fmt.Errorf("files: rm %s: %w", label, err)
		}
		if err := s.inScope(ctx, s.db, scope, f.DirectoryID); err != nil {
			return blobfs.File{}, fmt.Errorf("files: rm %s: %w", label, err)
		}
	}
	return s.deleteFile(ctx, label, id, version, stopAfter)
}

// deleteFile runs the steps of a file delete for the file with id, named
// label in the messages, in three steps with two transaction boundaries,
// the mirror of Put. First, in one transaction on its own: with a version
// the row is held at it, then blobfs's begin step moves the row to
// deleting through the variant, then the bookmark table is read for the
// file, and the delete is refused with ErrBookmarked while any unit
// bookmarks it, which rolls the begin back, so a bookmarked file's object
// is never deleted; otherwise the transaction commits. Second, outside
// any transaction, the object is deleted under the row's key; the store
// treats a missing object as success, so the step repeats safely. Third,
// on the pool, blobfs's complete step removes the row.
//
// The begin runs before the check because of the library's
// reference-then-delete rule: AddBookmark holds the file's row through
// blobfs.HoldFile before it inserts, and the begin takes the same lock,
// so an add that holds first commits its bookmark before this check
// reads the table, and an add that arrives after this begin waits and
// then refuses the deleting row. Either way the two operations serialize
// on the row, and a bookmark cannot reach a file whose object is being
// deleted. The foreign key fk_bookmark_file still refuses the row's
// removal in the third step should a bookmark be inserted without the
// hold; that refusal is ErrBookmarked too, the row stays deleting with
// its bookmark, and an rm after the bookmark is removed converges.
//
// The object store is opened before the first step, so a store that
// cannot be reached fails the rm before the row is touched. A stop after
// the first or the second step (stopAfter, or a failure of the object
// delete) leaves the row deleting, where ls and stat show it, cat refuses
// it, and put refuses its name; the error says so, and an rm of the same
// path resumes: the begin returns the deleting row unchanged, the object
// delete finds nothing or the object, and the complete removes the row.
func (s *Store) deleteFile(ctx context.Context, label, id string, version int64, stopAfter Step) (blobfs.File, error) {
	st, err := s.objects(ctx)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("files: rm %s: %w", label, err)
	}
	f, err := s.db.Transact(ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
		if version != 0 {
			// The hold takes the row's lock at the caller's version, so a
			// row that moved on is refused before the begin. A deleting
			// row is not holdable at any version and is let through: the
			// begin returns it unchanged, and the rerun resumes.
			if err := s.blobfs.HoldFile(ctx, tx, id, data.AtVersion(version)); err != nil && !errors.Is(err, blobfs.ErrDeleting) {
				return blobfs.File{}, err
			}
		}
		f, err := s.blobfs.BeginFileDelete(ctx, tx, id)
		if err != nil {
			return blobfs.File{}, err
		}
		n, err := s.bookmarksOfFile(ctx, tx, id)
		if err != nil {
			return blobfs.File{}, err
		}
		if n > 0 {
			return blobfs.File{}, fmt.Errorf("%d unit(s) bookmark the file; remove the bookmarks and rerun rm: %w", n, ErrBookmarked)
		}
		return f, nil
	})
	if err != nil {
		return blobfs.File{}, fmt.Errorf("files: rm %s: %w", label, err)
	}
	if stopAfter == StepBegin {
		return f, &StopError{Command: "rm", Step: StepBegin, Path: label, File: f}
	}
	if err := st.Delete(ctx, f.Key); err != nil {
		return f, fmt.Errorf("files: rm %s: %w (the row stays deleting; rerun rm)", label, err)
	}
	if stopAfter == StepObject {
		return f, &StopError{Command: "rm", Step: StepObject, Path: label, File: f}
	}
	if err := s.blobfs.CompleteFileDelete(ctx, s.db, f.ID); err != nil {
		err = classifyFileDelete(err)
		if errors.Is(err, ErrBookmarked) {
			return f, fmt.Errorf("files: rm %s: a bookmark references the file, inserted without holding it; the row stays deleting and its object is gone; remove the bookmark and rerun rm: %w", label, err)
		}
		return f, fmt.Errorf("files: rm %s: %w", label, err)
	}
	return f, nil
}

// RemoveDirectory is rmdir <path>: the directory at path is resolved on
// the pool and removed, with its ownership row when it has one, in one
// transaction, so an owned top-level directory goes with its owner row
// and a refused removal keeps both. The root is blobfs.ErrRootDirectory
// before any I/O, a path that ends with a slash blobfs.ErrInvalidPath, a
// directory that does not exist blobfs.ErrNotFound, and one that still
// has child directories or files blobfs.ErrNotEmpty: there is no cascade,
// and rm -r is the recursive delete. The row returned is the directory as
// it was.
func (s *Store) RemoveDirectory(ctx context.Context, path string) (blobfs.Directory, error) {
	if _, _, _, err := splitParent(path); err != nil {
		return blobfs.Directory{}, fmt.Errorf("files: rmdir %s: %w", path, err)
	}
	dir, err := s.blobfs.ResolveDirectory(ctx, s.db, path)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("files: rmdir %s: %w", path, err)
	}
	if err := s.removeDirectory(ctx, dir.ID); err != nil {
		return blobfs.Directory{}, fmt.Errorf("files: rmdir %s: %w", path, err)
	}
	return dir, nil
}

// removeDirectory removes the directory with id and its ownership row, if
// any, in one transaction: the owner row first, then the directory
// through the library, whose refusal rolls the owner row back.
func (s *Store) removeDirectory(ctx context.Context, id string) error {
	_, err := s.db.Transact(ctx, func(tx *sqlate.Tx) (struct{}, error) {
		if err := s.deleteOwner(ctx, tx, id); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, s.blobfs.RemoveDirectory(ctx, tx, id)
	})
	return err
}

// removalPage is how many rows one pass of the recursive delete lists
// before it removes them and lists again.
const removalPage = 100

// maxEmptyRounds is how many times the recursive delete empties a
// directory whose removal was then refused because a row arrived in
// between, before it reports the refusal.
const maxEmptyRounds = 3

// maxStalledPasses is how many passes in a row over one directory's
// listing may leave its total no smaller than the pass before, which is
// a directory receiving rows at least as fast as the walk removes them,
// before the walk reports ErrTreeBusy. One stalled pass is a transient
// insert; three in a row is a sustained writer.
const maxStalledPasses = 3

// RemoveTree is rm -r <path>: the directory at path and everything under
// it, files through the full delete steps of Remove and directories
// through those of RemoveDirectory, children before their parent and the
// target last. The root is blobfs.ErrRootDirectory before any I/O; the
// recursive delete never empties the root. The walk is not atomic and
// takes no lock: it lists each directory one page at a time and removes
// what the page holds until a page comes back empty, then removes the
// directory. A row inserted into a directory during the walk is either
// listed by a later page and removed, or, when it arrives after the empty
// page, refuses the directory's removal through the library's foreign
// key, and the walk empties the directory again, up to maxEmptyRounds
// times, after which the refusal is reported as blobfs.ErrNotEmpty. A
// directory that receives rows at least as fast as the walk removes them
// (maxStalledPasses passes in a row that leave the listing's total no
// smaller) is ErrTreeBusy. A bookmarked file stops the walk with
// ErrBookmarked. Every refusal leaves
// the tree consistent, with the counts of what was removed, and a rerun
// continues from there.
//
// observe, when not nil, is told of each file and directory removed and
// of each directory found empty, as it happens; the command prints a
// line per removal through it.
func (s *Store) RemoveTree(ctx context.Context, path string, observe func(RemovalEvent)) (TreeRemoval, error) {
	if _, _, _, err := splitParent(path); err != nil {
		return TreeRemoval{}, fmt.Errorf("files: rm -r %s: %w", path, err)
	}
	if observe == nil {
		observe = func(RemovalEvent) {}
	}
	dir, err := s.blobfs.ResolveDirectory(ctx, s.db, path)
	if err != nil {
		return TreeRemoval{}, fmt.Errorf("files: rm -r %s: %w", path, err)
	}
	w := &walker{store: s, observe: observe}
	if err := w.removeTree(ctx, dir.ID, path); err != nil {
		return w.result, fmt.Errorf("files: rm -r %s: %w", path, err)
	}
	return w.result, nil
}

// walker is one recursive delete in progress: the store it runs on, the
// observer it reports to, and the counts so far.
type walker struct {
	store   *Store
	observe func(RemovalEvent)
	result  TreeRemoval
}

// removeTree empties the directory with id at path and removes it,
// emptying it again when a row arrived between the empty listing and
// the removal, up to maxEmptyRounds times.
func (w *walker) removeTree(ctx context.Context, id, path string) error {
	var err error
	for range maxEmptyRounds {
		if err = w.empty(ctx, id, path); err != nil {
			return err
		}
		w.observe(RemovalEvent{Kind: DirectoryEmptied, Path: path})
		err = w.store.removeDirectory(ctx, id)
		if errors.Is(err, blobfs.ErrNotEmpty) {
			continue
		}
		if err != nil {
			return err
		}
		w.result.Directories++
		w.observe(RemovalEvent{Kind: RemovedDirectory, Path: path, ID: id})
		return nil
	}
	return fmt.Errorf("%s still receives rows after %d passes: %w", path, maxEmptyRounds, err)
}

// empty removes the contents of the directory with id at path: its child
// directories, each through removeTree, then its files, each through the
// delete steps. Each half is listed one page at a time until a page comes
// back empty, and maxStalledPasses passes in a row that leave the half's
// total no smaller than the pass before are ErrTreeBusy.
func (w *walker) empty(ctx context.Context, id, path string) error {
	l := data.Listing{Page: 1, Size: removalPage}
	var progress stall
	for {
		page, err := w.store.blobfs.Children(ctx, w.store.db, id, l)
		if err != nil {
			return err
		}
		if len(page.Rows) == 0 {
			break
		}
		if err := progress.pass(page.Total); err != nil {
			return fmt.Errorf("%s: the directory total %w", path, err)
		}
		for _, d := range page.Rows {
			if err := w.removeTree(ctx, d.ID, path+"/"+d.Name); err != nil {
				return err
			}
		}
	}
	progress = stall{}
	for {
		page, err := w.store.blobfs.ListFiles(ctx, w.store.db, id, l)
		if err != nil {
			return err
		}
		if len(page.Rows) == 0 {
			return nil
		}
		if err := progress.pass(page.Total); err != nil {
			return fmt.Errorf("%s: the file total %w", path, err)
		}
		for _, f := range page.Rows {
			filePath := path + "/" + f.Name
			if _, err := w.store.deleteFile(ctx, filePath, f.ID, 0, ""); err != nil {
				return err
			}
			w.result.Files++
			w.observe(RemovalEvent{Kind: RemovedFile, Path: filePath, ID: f.ID})
		}
	}
}

// stall tracks whether the passes over one listing make progress: the
// total the last pass saw, and how many passes in a row saw no smaller
// total than the one before.
type stall struct {
	prev    int
	seen    bool
	stalled int
}

// pass records one pass's total and reports ErrTreeBusy once
// maxStalledPasses passes in a row have made no progress.
func (s *stall) pass(total int) error {
	if s.seen && total >= s.prev {
		s.stalled++
		if s.stalled >= maxStalledPasses {
			return fmt.Errorf("stayed at %d or more over %d passes: %w", s.prev, s.stalled, ErrTreeBusy)
		}
	} else {
		s.stalled = 0
	}
	s.prev, s.seen = total, true
	return nil
}
