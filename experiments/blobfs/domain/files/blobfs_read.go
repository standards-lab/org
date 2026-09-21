package files

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"

	"github.com/standards-lab/sqlate"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// List returns the contents of the directory at path under l: the
// directories under it through blobfs's directory listing and its files
// through blobfs's file listing, each one page of l's size with its total
// when l asks for one. It is the path form of ListDirectory: the path is
// resolved and the directory's contents read by id, all in one read-only
// repeatable-read transaction, so the resolution and the two halves see
// the same snapshot and agree with each other.
//
// Each half continues from its own cursor in l.After when one is given,
// and each page says in More whether rows remain and carries the cursor
// of the next page in Next.
//
// A unit in l is the directory-grain ownership rehearsal: the scope is
// derived from the path and checked once, at the depth-one ancestor of
// path, before the path is resolved further, and a unit that does not own
// that ancestor is refused with ErrNotOwned. At the root the unit's scope
// is the set of top-level directories it owns, so the directory half
// comes from the owner read model filtered by the unit, and the file half
// is empty: a file in the root has no depth-one ancestor and belongs to
// no unit. That read model pages by number only, so a cursor there is
// ErrNoCursorAtRoot, before any I/O.
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
			return s.contents(ctx, tx, path, dir.ID, l)
		}
		if path == "/" {
			return s.topLevel(ctx, tx, l.Unit, l)
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
		owned, err := s.ownedByUnit(ctx, tx, ancestor.ID, l.Unit)
		if err != nil {
			return Contents{}, err
		}
		if !owned {
			return Contents{}, fmt.Errorf("ls %s as unit %s: %w", path, l.Unit, ErrNotOwned)
		}
		dir := ancestor
		if ancestorPath != path {
			if dir, err = s.blobfs.ResolveDirectory(ctx, tx, path); err != nil {
				return Contents{}, err
			}
		}
		return s.contents(ctx, tx, path, dir.ID, l)
	}, sqlate.ReadOnly(), sqlate.Isolation(sql.LevelRepeatableRead))
}

// ListDirectory returns the contents of the directory with id under l, as
// List does for a path: both halves, each one page with its total when l
// asks for one, from their own cursors, in one read-only repeatable-read
// transaction. The directory is read once so that an id no directory
// holds is blobfs.ErrNotFound, and the contents' Path is empty: no path
// is computed for a listing by id.
//
// The scope is the ownership check by id, InScope, run inside the same
// transaction before the directory is read: with a unit and a scope
// directory, the unit must own the scope directory and the listed
// directory must lie within it, or the listing is ErrNotOwned. A Listing
// whose Unit is set names that unit as the scope's when the scope names
// none, and is refused when the two differ. At the root, the unit's scope
// is the set of top-level directories it owns, read through the owner
// read model as List does at /, and the scope's directory is not
// consulted; that read model takes no cursor (ErrNoCursorAtRoot).
func (s *Store) ListDirectory(ctx context.Context, id string, l Listing, scope Scope) (Contents, error) {
	if l.Unit != "" && scope.Unit == "" {
		scope.Unit = l.Unit
	}
	if l.Unit != "" && l.Unit != scope.Unit {
		return Contents{}, fmt.Errorf("files: ls directory %s: the listing names unit %s and the scope unit %s: %w", id, l.Unit, scope.Unit, ErrNotOwned)
	}
	if id == blobfs.RootID && scope.Unit != "" {
		if l.After != (After{}) {
			return Contents{}, fmt.Errorf("ls / as unit %s: %w", scope.Unit, ErrNoCursorAtRoot)
		}
		return s.db.Transact(ctx, func(tx *sqlate.Tx) (Contents, error) {
			return s.topLevel(ctx, tx, scope.Unit, l)
		}, sqlate.ReadOnly(), sqlate.Isolation(sql.LevelRepeatableRead))
	}
	c, err := s.db.Transact(ctx, func(tx *sqlate.Tx) (Contents, error) {
		if err := s.inScope(ctx, tx, scope, id); err != nil {
			return Contents{}, err
		}
		if _, err := s.blobfs.Directory(ctx, tx, id); err != nil {
			return Contents{}, err
		}
		return s.contents(ctx, tx, "", id, l)
	}, sqlate.ReadOnly(), sqlate.Isolation(sql.LevelRepeatableRead))
	if err != nil {
		return Contents{}, fmt.Errorf("files: ls directory %s: %w", id, err)
	}
	return c, nil
}

// contents reads the two halves of the directory with id through sess:
// the directory half under the sort terms naming a directory field, the
// file half under every term, each from its own cursor when l carries
// one. path is what the result reports as listed.
func (s *Store) contents(ctx context.Context, sess sqlate.Session, path, id string, l Listing) (Contents, error) {
	dirs, err := s.blobfs.Children(ctx, sess, id, lower(l, directoryFields, l.After.Directories))
	if err != nil {
		return Contents{}, err
	}
	files, err := s.blobfs.ListFiles(ctx, sess, id, lower(l, nil, l.After.Files))
	if err != nil {
		return Contents{}, err
	}
	return Contents{Path: path, Directories: page(dirs), Files: page(files)}, nil
}

// topLevel is the listing of the root as a unit: the unit's top-level
// directories through the owner read model, and no files.
func (s *Store) topLevel(ctx context.Context, sess sqlate.Session, unit string, l Listing) (Contents, error) {
	owned, err := s.ownedBy(ctx, sess, unit, l)
	if err != nil {
		return Contents{}, err
	}
	dirs := Page[blobfs.Directory]{Total: owned.Total, More: owned.More}
	for _, o := range owned.Rows {
		dirs.Rows = append(dirs.Rows, o.Directory())
	}
	files := Page[blobfs.File]{Total: 0}
	if l.Total == TotalNone {
		files.Total = NoTotal
	}
	return Contents{Path: "/", Directories: dirs, Files: files}, nil
}

// Stat returns the row of the file at path, whatever its status: the
// parent directory is resolved and the last segment looked up among its
// files, on the pool. The lookup by name is the last step of the
// resolution and returns the row itself, so no read by id follows it. A
// file that does not exist, or a parent that does not, is
// blobfs.ErrNotFound. The object store is not consulted.
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

// StatFile returns the row of the file with id, whatever its status, on
// the pool: one read by id. A file that does not exist is
// blobfs.ErrNotFound. With a scope, the file's directory must lie within
// it (InScope), checked after the read since the check needs the
// directory id, and a file outside it is ErrNotOwned. The object store is
// not consulted.
func (s *Store) StatFile(ctx context.Context, id string, scope Scope) (blobfs.File, error) {
	f, err := s.blobfs.File(ctx, s.db, id)
	if err != nil {
		return blobfs.File{}, fmt.Errorf("files: stat file %s: %w", id, err)
	}
	if err := s.inScope(ctx, s.db, scope, f.DirectoryID); err != nil {
		return blobfs.File{}, fmt.Errorf("files: stat file %s: %w", id, err)
	}
	return f, nil
}

// Resolve returns the row of the directory at path, on the pool: the
// library's path resolution, one child lookup per segment from the root.
// It is the path form of StatDirectory. The root is the seeded root row.
// A segment that does not name a directory is blobfs.ErrNotFound, and a
// relative path is blobfs.ErrInvalidPath.
func (s *Store) Resolve(ctx context.Context, path string) (blobfs.Directory, error) {
	d, err := s.blobfs.ResolveDirectory(ctx, s.db, path)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("files: stat %s: %w", path, err)
	}
	return d, nil
}

// StatDirectory returns the row of the directory with id, on the pool:
// one read by id. A directory that does not exist is blobfs.ErrNotFound.
// With a scope, the directory must lie within it (InScope), and one
// outside it is ErrNotOwned.
func (s *Store) StatDirectory(ctx context.Context, id string, scope Scope) (blobfs.Directory, error) {
	if err := s.inScope(ctx, s.db, scope, id); err != nil {
		return blobfs.Directory{}, fmt.Errorf("files: stat directory %s: %w", id, err)
	}
	d, err := s.blobfs.Directory(ctx, s.db, id)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("files: stat directory %s: %w", id, err)
	}
	return d, nil
}

// Open opens the content of the file at path for reading and returns the
// row with it; the caller closes the reader. It is Stat followed by the
// read of the row's object, as OpenFile does by id. Only an available
// file has content to read: a pending file's object has not been written
// and a deleting file's is being removed, and either is ErrNotAvailable,
// with the status in the message, before the store is asked. An available
// row whose object the store does not hold is ErrObjectMissing.
func (s *Store) Open(ctx context.Context, path string) (io.ReadCloser, blobfs.File, error) {
	f, err := s.Stat(ctx, path)
	if err != nil {
		return nil, blobfs.File{}, err
	}
	return s.open(ctx, path, f)
}

// OpenFile opens the content of the file with id for reading and returns
// the row with it; the caller closes the reader. It is StatFile, with its
// scope check, followed by the read of the row's object, with Open's
// refusals: a file that is not available is ErrNotAvailable before the
// store is asked, and an available row with no object is
// ErrObjectMissing.
func (s *Store) OpenFile(ctx context.Context, id string, scope Scope) (io.ReadCloser, blobfs.File, error) {
	f, err := s.StatFile(ctx, id, scope)
	if err != nil {
		return nil, blobfs.File{}, err
	}
	return s.open(ctx, "file "+id, f)
}

// open reads the object of the row f from the store, for the file the
// messages name as label: the status is checked first, then the store is
// opened and asked for the object under the row's key.
func (s *Store) open(ctx context.Context, label string, f blobfs.File) (io.ReadCloser, blobfs.File, error) {
	if f.Status != blobfs.StatusAvailable {
		return nil, blobfs.File{}, fmt.Errorf("files: cat %s: the file is %s: %w", label, f.Status, ErrNotAvailable)
	}
	st, err := s.objects(ctx)
	if err != nil {
		return nil, blobfs.File{}, fmt.Errorf("files: cat %s: %w", label, err)
	}
	body, err := st.Get(ctx, f.Key)
	if err != nil {
		return nil, blobfs.File{}, fmt.Errorf("files: cat %s: %w", label, err)
	}
	return body, f, nil
}
