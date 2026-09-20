package files

import (
	"context"
	"database/sql"
	"fmt"
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

// splitParent splits the path of a directory to create into its parent's
// path, the name of the new directory, and the new directory's depth (1
// for a top-level directory). The root itself is blobfs.ErrRootDirectory,
// a relative path or one ending with a slash blobfs.ErrInvalidPath. The
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
