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
