package files

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/standards-lab/sqlate"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

// This file is the translation over the library under test: it constructs
// blobfs's persistence against the consumer's catalog and composes the
// consumer's operations from blobfs's methods and the consumer's own
// statements. The consumer uses blobfs.Directory and blobfs.File as the
// library defines them.

// compileLibrary compiles blobfs's statements against the store's catalog
// for its dialect, over the variant build returns when one was given and
// over the standard baseline otherwise. It is a method rather than a
// constructor because the catalog's type belongs to the query library,
// which only database.go names; the field is reached without naming it.
func (s *Store) compileLibrary(build VariantConstructor) error {
	var opts []data.Option
	if build != nil {
		v, err := build(s.catalog, s.db.Dialect())
		if err != nil {
			return fmt.Errorf("files: variant: %w", err)
		}
		opts = append(opts, data.WithVariant(v))
	}
	lib, err := data.New(s.catalog, s.db.Dialect(), opts...)
	if err != nil {
		return fmt.Errorf("files: %w", err)
	}
	s.blobfs = lib
	return nil
}

// Mkdir creates the directory at path under its parent, which must exist,
// and returns the row. With a non-empty unit the path must be at depth one
// (ErrUnitDepth otherwise, before any I/O), and the directory and its
// ownership row are written in one transaction, so a directory the
// consumer created with a unit never exists without its owner. There is
// no -p: a parent that does not exist is blobfs.ErrNotFound. The root
// cannot be created (blobfs.ErrRootDirectory), a path that ends with a
// slash is blobfs.ErrInvalidPath, and a name already held under the
// parent blobfs.ErrNameTaken.
func (s *Store) Mkdir(ctx context.Context, path, unit string) (blobfs.Directory, error) {
	parent, name, depth, err := splitParent(path)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("files: mkdir %s: %w", path, err)
	}
	if unit == "" {
		dir, err := s.blobfs.ResolveDirectory(ctx, s.db, parent)
		if err != nil {
			return blobfs.Directory{}, err
		}
		return s.blobfs.Mkdir(ctx, s.db, dir.ID, name)
	}
	if depth != 1 {
		return blobfs.Directory{}, fmt.Errorf("mkdir %s: %w", path, ErrUnitDepth)
	}
	return s.db.Transact(ctx, func(tx *sqlate.Tx) (blobfs.Directory, error) {
		dir, err := s.blobfs.ResolveDirectory(ctx, tx, parent)
		if err != nil {
			return blobfs.Directory{}, err
		}
		made, err := s.blobfs.Mkdir(ctx, tx, dir.ID, name)
		if err != nil {
			return blobfs.Directory{}, err
		}
		if err := s.insertOwner(ctx, tx, made.ID, unit); err != nil {
			return blobfs.Directory{}, err
		}
		return made, nil
	})
}

// List returns the contents of the directory at path under l: the
// directories under it through blobfs's directory listing and its files
// through blobfs's file listing, each one page of l's size with its total
// when l asks for one. The address resolution and both halves run in one
// read-only repeatable-read transaction, so the two halves see the same
// snapshot and agree with each other.
//
// Each half continues from its own cursor in l.After when one is given,
// and each page carries the cursor of the next page in Next.
//
// A unit in l is the directory-grain ownership rehearsal: the scope is
// checked once, at the depth-one ancestor of path, before the path is
// resolved further, and a unit that does not own that ancestor is refused
// with ErrNotOwned. At the root the unit's scope is the set of top-level
// directories it owns, so the directory half comes from the owner read
// model filtered by the unit, and the file half is empty: a file in the
// root has no depth-one ancestor and belongs to no unit. That read model
// pages by number only, so a cursor there is ErrNoCursorAtRoot, before
// any I/O.
func (s *Store) List(ctx context.Context, path string, l Listing) (Contents, error) {
	if path == "/" && l.Unit != "" && l.After != (After{}) {
		return Contents{}, fmt.Errorf("ls / as unit %s: %w", l.Unit, ErrNoCursorAtRoot)
	}
	return s.db.Transact(ctx, func(tx *sqlate.Tx) (Contents, error) {
		if l.Unit == "" {
			dir, err := s.blobfs.ResolveDirectory(ctx, tx, path)
			if err != nil {
				return Contents{}, err
			}
			return s.contents(ctx, tx, path, dir, l)
		}
		if path == "/" {
			return s.topLevel(ctx, tx, l)
		}
		ancestorPath := path
		if rest, ok := strings.CutPrefix(path, "/"); ok {
			first, _, _ := strings.Cut(rest, "/")
			ancestorPath = "/" + first
		}
		ancestor, err := s.blobfs.ResolveDirectory(ctx, tx, ancestorPath)
		if err != nil {
			return Contents{}, err
		}
		o, owned, err := s.owner(ctx, tx, ancestor.ID)
		if err != nil {
			return Contents{}, err
		}
		if !owned || o.UnitID != l.Unit {
			return Contents{}, fmt.Errorf("ls %s as unit %s: %w", path, l.Unit, ErrNotOwned)
		}
		dir := ancestor
		if ancestorPath != path {
			if dir, err = s.blobfs.ResolveDirectory(ctx, tx, path); err != nil {
				return Contents{}, err
			}
		}
		return s.contents(ctx, tx, path, dir, l)
	}, sqlate.ReadOnly(), sqlate.Isolation(sql.LevelRepeatableRead))
}

// contents reads the two halves of dir through sess: the directory half
// under the sort terms naming a directory field, the file half under every
// term, each from its own cursor when l carries one.
func (s *Store) contents(ctx context.Context, sess sqlate.Session, path string, dir blobfs.Directory, l Listing) (Contents, error) {
	dirs, err := s.blobfs.Children(ctx, sess, dir.ID, lower(l, directoryFields, l.After.Directories))
	if err != nil {
		return Contents{}, err
	}
	files, err := s.blobfs.ListFiles(ctx, sess, dir.ID, lower(l, nil, l.After.Files))
	if err != nil {
		return Contents{}, err
	}
	return Contents{Path: path, Directories: page(dirs), Files: page(files)}, nil
}

// topLevel is ls / --unit: the unit's top-level directories through the
// owner read model, and no files.
func (s *Store) topLevel(ctx context.Context, sess sqlate.Session, l Listing) (Contents, error) {
	owned, err := s.ownedBy(ctx, sess, l.Unit, l)
	if err != nil {
		return Contents{}, err
	}
	dirs := Page[blobfs.Directory]{Total: owned.Total}
	for _, o := range owned.Rows {
		dirs.Rows = append(dirs.Rows, o.Directory())
	}
	files := Page[blobfs.File]{Total: 0}
	if l.Total == TotalNone {
		files.Total = NoTotal
	}
	return Contents{Path: "/", Directories: dirs, Files: files}, nil
}

// Put is the two-phase write as the consumer sequences it, in three steps
// with two transaction boundaries. First, in one transaction on its own:
// the parent directory is resolved, the name is looked up, and either the
// pending row is inserted through blobfs's begin step or, when a pending
// row already holds the name, that row is taken up again (Resumed); the
// transaction commits, so the pending row is durable before any byte
// reaches the store, and it is where a consumer would write its own rows
// beside the pending row. Second, outside any transaction, the object is
// stored under the row's key with the declared content type. Third, on the
// pool, blobfs's complete step moves the row to available with what the
// store reported, guarded by the version read in the first step.
//
// A stop after the first or the second step (StopAfter, or a failure of
// the object write) leaves the row pending, where ls and stat show it;
// the error says so, and a put of the same path resumes the row: it
// stores the object again, which replaces one an earlier attempt left,
// and completes. A name held by an available or a deleting row is
// blobfs.ErrNameTaken, since the consumer has no content replacement, and
// a parent that does not exist is blobfs.ErrNotFound. The object store is
// opened before the first step, so a store that cannot be reached fails
// the put before any row is inserted; the key is validated against it in
// the begin step, before the insert.
func (s *Store) Put(ctx context.Context, req PutRequest) (PutResult, error) {
	parent, name, _, err := splitParent(req.Path)
	if err != nil {
		return PutResult{}, fmt.Errorf("files: put %s: %w", req.Path, err)
	}
	st, err := s.objects(ctx)
	if err != nil {
		return PutResult{}, fmt.Errorf("files: put %s: %w", req.Path, err)
	}
	resumed := false
	f, err := s.db.Transact(ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
		dir, err := s.blobfs.ResolveDirectory(ctx, tx, parent)
		if err != nil {
			return blobfs.File{}, err
		}
		existing, err := s.blobfs.FileByName(ctx, tx, dir.ID, name)
		switch {
		case err == nil && existing.Status == blobfs.StatusPending:
			resumed = true
			return existing, nil
		case err == nil:
			return blobfs.File{}, fmt.Errorf("a file named %q is %s: %w", existing.Name, existing.Status, blobfs.ErrNameTaken)
		case !errors.Is(err, blobfs.ErrNotFound):
			return blobfs.File{}, err
		}
		return s.blobfs.BeginFileWrite(ctx, tx, st, dir.ID, name, req.ContentType)
	})
	if err != nil {
		return PutResult{}, fmt.Errorf("files: put %s: %w", req.Path, err)
	}
	result := PutResult{File: f, Resumed: resumed}
	if req.StopAfter == StepInsert {
		return result, &StopError{Command: "put", Step: StepInsert, Path: req.Path, File: f}
	}
	obj, err := st.Put(ctx, f.Key, req.Body, req.ContentType, req.Size)
	if err != nil {
		return result, fmt.Errorf("files: put %s: %w (the row stays pending; a put of the same path retries)", req.Path, err)
	}
	if req.StopAfter == StepWrite {
		return result, &StopError{Command: "put", Step: StepWrite, Path: req.Path, File: f}
	}
	done, err := s.blobfs.CompleteFileWrite(ctx, s.db, f.ID, f.Version, obj)
	if err != nil {
		return result, fmt.Errorf("files: put %s: %w", req.Path, err)
	}
	result.File = done
	return result, nil
}

// Stat returns the row of the file at path, whatever its status: the
// parent directory is resolved and the last segment looked up among its
// files, on the pool. A file that does not exist, or a parent that does
// not, is blobfs.ErrNotFound. The object store is not consulted.
func (s *Store) Stat(ctx context.Context, path string) (blobfs.File, error) {
	parent, name, _, err := splitParent(path)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("files: stat %s: %w", path, err)
	}
	dir, err := s.blobfs.ResolveDirectory(ctx, s.db, parent)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("files: stat %s: %w", path, err)
	}
	f, err := s.blobfs.FileByName(ctx, s.db, dir.ID, name)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("files: stat %s: %w", path, err)
	}
	return f, nil
}

// Open opens the content of the file at path for reading and returns the
// row with it; the caller closes the reader. Only an available file has
// content to read: a pending file's object has not been written and a
// deleting file's is being removed, and either is ErrNotAvailable, with
// the status in the message, before the store is asked. An available row
// whose object the store does not hold is ErrObjectMissing.
func (s *Store) Open(ctx context.Context, path string) (io.ReadCloser, blobfs.File, error) {
	f, err := s.Stat(ctx, path)
	if err != nil {
		return nil, blobfs.File{}, err
	}
	if f.Status != blobfs.StatusAvailable {
		return nil, blobfs.File{}, fmt.Errorf("files: cat %s: the file is %s: %w", path, f.Status, ErrNotAvailable)
	}
	st, err := s.objects(ctx)
	if err != nil {
		return nil, blobfs.File{}, fmt.Errorf("files: cat %s: %w", path, err)
	}
	body, err := st.Get(ctx, f.Key)
	if err != nil {
		return nil, blobfs.File{}, fmt.Errorf("files: cat %s: %w", path, err)
	}
	return body, f, nil
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

// Remove is the file delete as the consumer sequences it, rm <path>: the
// parent is resolved and the name looked up on the pool, whatever the
// row's status, and the row then goes through the three steps of
// deleteFile. A pending row (an abandoned write), an available row, and a
// row already deleting (an earlier rm that stopped) are treated alike:
// the steps are idempotent, so a rerun resumes where the earlier run
// stopped. A file that does not exist, or a parent that does not, is
// blobfs.ErrNotFound; after a finished rm the same path reports that,
// which is how a caller learns the delete is done. The row returned is
// the row as the begin step left it, deleting.
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
	return s.deleteFile(ctx, path, f.ID, stopAfter)
}

// deleteFile runs the steps of a file delete for the file with id at
// path, in three steps with two transaction boundaries, the mirror of
// Put. First, in one transaction on its own: the bookmark table is read
// for the file, and the delete is refused with ErrBookmarked while any
// unit bookmarks it, so a bookmarked file's object is never deleted; then
// blobfs's begin step moves the row to deleting through the variant, and
// the transaction commits. Second, outside any transaction, the object is
// deleted under the row's key; the store treats a missing object as
// success, so the step repeats safely. Third, on the pool, blobfs's
// complete step removes the row.
//
// The object store is opened before the first step, so a store that
// cannot be reached fails the rm before the row is touched. A stop after
// the first or the second step (stopAfter, or a failure of the object
// delete) leaves the row deleting, where ls and stat show it, cat refuses
// it, and put refuses its name; the error says so, and an rm of the same
// path resumes: the begin returns the deleting row unchanged, the object
// delete finds nothing or the object, and the complete removes the row.
//
// The bookmark check and the complete step are the two ends of one race:
// a bookmark added between them (the add's own status check saw the row
// before the begin committed) makes the foreign key fk_bookmark_file
// refuse the row's removal after the object is gone. The refusal is
// ErrBookmarked, the row stays deleting with its bookmark, bookmark ls
// shows the status, and an rm after the bookmark is removed converges.
// Closing the race would need the add and the rm to serialize on the
// file row, which standard SQL cannot state without serializable
// isolation on both.
func (s *Store) deleteFile(ctx context.Context, path, id string, stopAfter Step) (blobfs.File, error) {
	st, err := s.objects(ctx)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("files: rm %s: %w", path, err)
	}
	f, err := s.db.Transact(ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
		n, err := s.bookmarksOfFile(ctx, tx, id)
		if err != nil {
			return blobfs.File{}, err
		}
		if n > 0 {
			return blobfs.File{}, fmt.Errorf("%d unit(s) bookmark the file; remove the bookmarks and rerun rm: %w", n, ErrBookmarked)
		}
		return s.blobfs.BeginFileDelete(ctx, tx, id)
	})
	if err != nil {
		return blobfs.File{}, fmt.Errorf("files: rm %s: %w", path, err)
	}
	if stopAfter == StepBegin {
		return f, &StopError{Command: "rm", Step: StepBegin, Path: path, File: f}
	}
	if err := st.Delete(ctx, f.Key); err != nil {
		return f, fmt.Errorf("files: rm %s: %w (the row stays deleting; rerun rm)", path, err)
	}
	if stopAfter == StepObject {
		return f, &StopError{Command: "rm", Step: StepObject, Path: path, File: f}
	}
	if err := s.blobfs.CompleteFileDelete(ctx, s.db, f.ID); err != nil {
		err = classifyFileDelete(err)
		if errors.Is(err, ErrBookmarked) {
			return f, fmt.Errorf("files: rm %s: a bookmark was added after the delete began; the row stays deleting and its object is gone; remove the bookmark and rerun rm: %w", path, err)
		}
		return f, fmt.Errorf("files: rm %s: %w", path, err)
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
	for round := 0; round < maxEmptyRounds; round++ {
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
			if err := w.removeTree(ctx, d.ID, path+"/"+*d.Name); err != nil {
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
			if _, err := w.store.deleteFile(ctx, filePath, f.ID, ""); err != nil {
				return err
			}
			w.result.Files++
			w.observe(RemovalEvent{Kind: RemovedFile, Path: filePath, ID: f.ID})
		}
	}
}

// splitParent splits the path of a directory or file to create into its
// parent's path, the name of the new row, and the new row's depth (1 for
// a top-level directory). The root itself is blobfs.ErrRootDirectory, a
// relative path or one ending with a slash blobfs.ErrInvalidPath. The
// parent's segments are validated when the parent is resolved and the
// name when the row is written.
func splitParent(path string) (parent, name string, depth int, err error) {
	if !strings.HasPrefix(path, "/") {
		return "", "", 0, fmt.Errorf("%w: %q does not start with /", blobfs.ErrInvalidPath, path)
	}
	if path == "/" {
		return "", "", 0, blobfs.ErrRootDirectory
	}
	at := strings.LastIndex(path, "/")
	parent, name = path[:at], path[at+1:]
	if name == "" {
		return "", "", 0, fmt.Errorf("%w: %q ends with a slash", blobfs.ErrInvalidPath, path)
	}
	if parent == "" {
		parent = "/"
	}
	return parent, name, strings.Count(path, "/"), nil
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
