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
// for its dialect. It is a method rather than a constructor because the
// catalog's type belongs to the query library, which only database.go
// names; the field is reached without naming it.
func (s *Store) compileLibrary() error {
	lib, err := data.New(s.catalog, s.db.Dialect())
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
		return result, &StopError{Step: StepInsert, Path: req.Path, File: f}
	}
	obj, err := st.Put(ctx, f.Key, req.Body, req.ContentType, req.Size)
	if err != nil {
		return result, fmt.Errorf("files: put %s: %w (the row stays pending; a put of the same path retries)", req.Path, err)
	}
	if req.StopAfter == StepWrite {
		return result, &StopError{Step: StepWrite, Path: req.Path, File: f}
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
