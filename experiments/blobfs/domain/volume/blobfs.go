package volume

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/standards-lab/sqlate"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
)

// This file is the translation over the library under test: it constructs
// blobfs's persistence against the consumer's catalog and composes the
// consumer's operations from blobfs's methods and the consumer's own
// statements. The consumer uses blobfs.Volume and blobfs.Directory as the
// library defines them.

// compileLibrary compiles blobfs's statements against the store's catalog
// for its dialect. It is a method rather than a constructor because the
// catalog's type belongs to the query library, which only database.go
// names; the field is reached without naming it.
func (s *Store) compileLibrary() error {
	lib, err := data.New(s.catalog, s.db.Dialect())
	if err != nil {
		return fmt.Errorf("volume: %w", err)
	}
	s.blobfs = lib
	return nil
}

// CreateVolume creates the volume named name, owned by the unit with
// unitID, and returns it as the read model shows it. The three rows, the
// volume and its root through blobfs and the consumer's owner row, are
// written in one transaction: a name another volume holds
// (blobfs.ErrNameTaken), a name ValidateName refuses (blobfs.ErrInvalidName),
// or a unit id the engine cannot read leave nothing behind.
func (s *Store) CreateVolume(ctx context.Context, name, unitID string) (VolumeEntry, error) {
	return s.db.Transact(ctx, func(tx *sqlate.Tx) (VolumeEntry, error) {
		v, _, err := s.blobfs.CreateVolume(ctx, tx, name)
		if err != nil {
			return VolumeEntry{}, err
		}
		if err := s.insertOwner(ctx, tx, v.ID, unitID); err != nil {
			return VolumeEntry{}, err
		}
		return VolumeEntry{ID: v.ID, Name: v.Name, Version: v.Version, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, UnitID: unitID}, nil
	})
}

// VolumeByName returns the volume named name, or blobfs.ErrNotFound.
func (s *Store) VolumeByName(ctx context.Context, name string) (blobfs.Volume, error) {
	return s.blobfs.VolumeByName(ctx, s.db, name)
}

// RenameVolume renames the volume named name to newName under blobfs's
// version guard, at the version the lookup saw, and returns the volume as
// renamed. A rename that lands between the lookup and the update is
// query.ErrVersionMismatch; the command reports it and the operator runs
// the rename again. A missing volume is blobfs.ErrNotFound and a name
// another volume holds blobfs.ErrNameTaken.
func (s *Store) RenameVolume(ctx context.Context, name, newName string) (blobfs.Volume, error) {
	v, err := s.blobfs.VolumeByName(ctx, s.db, name)
	if err != nil {
		return blobfs.Volume{}, err
	}
	if _, err := s.blobfs.RenameVolume(ctx, s.db, v.ID, v.Version, newName); err != nil {
		return blobfs.Volume{}, err
	}
	return s.blobfs.Volume(ctx, s.db, v.ID)
}

// Resolve returns the volume and the directory an address names: the
// volume by name, then the path resolved inside it by the library. A volume
// or a segment that does not exist is blobfs.ErrNotFound; a path the library
// refuses is blobfs.ErrInvalidPath.
func (s *Store) Resolve(ctx context.Context, addr Address) (blobfs.Volume, blobfs.Directory, error) {
	v, err := s.blobfs.VolumeByName(ctx, s.db, addr.Volume)
	if err != nil {
		return blobfs.Volume{}, blobfs.Directory{}, err
	}
	d, err := s.blobfs.ResolveDirectory(ctx, s.db, v.ID, addr.Path)
	if err != nil {
		return blobfs.Volume{}, blobfs.Directory{}, err
	}
	return v, d, nil
}

// Mkdir creates the directory an address names under its parent, which
// must exist, and returns the row. The root of a volume cannot be created
// (ErrInvalidAddress); a parent that does not exist is blobfs.ErrNotFound,
// and a name already held under the parent blobfs.ErrNameTaken.
func (s *Store) Mkdir(ctx context.Context, addr Address) (blobfs.Directory, error) {
	parent, name, err := addr.Split()
	if err != nil {
		return blobfs.Directory{}, err
	}
	_, dir, err := s.Resolve(ctx, parent)
	if err != nil {
		return blobfs.Directory{}, err
	}
	return s.blobfs.Mkdir(ctx, s.db, dir.ID, name)
}

// List returns the contents of the directory an address names under l:
// the directories under it through blobfs's own listing, sorted by name,
// and its files through the consumer's file_view scoped to the directory
// with ListIn, sorted by l's terms. Both are one page of l's size with a
// total.
//
// A unit in l scopes the two halves by different means. The files carry an
// equality filter on unit_id, the read model's own column, as a directive.
// blobfs's directory listing declares no unit_id, because the library
// knows nothing about owners, so the directory half is scoped by the
// consumer's owner row instead: a volume the unit does not own lists no
// directories, and a total of zero, as its files do under the filter.
func (s *Store) List(ctx context.Context, addr Address, l Listing) (Contents, error) {
	v, dir, err := s.Resolve(ctx, addr)
	if err != nil {
		return Contents{}, err
	}
	var c Contents
	inScope := true
	if l.Unit != "" {
		o, err := s.owner(ctx, v.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Contents{}, fmt.Errorf("volume: owner of %s: %w", addr.Volume, err)
		}
		inScope = err == nil && o.UnitID == l.Unit
	}
	if inScope {
		byName := Listing{Page: l.Page, Size: l.Size, Sort: []Sort{{Field: "name"}}}
		c.Directories, c.DirectoryTotal, err = s.blobfs.Children(ctx, s.db, dir.ID, directives(byName))
		if err != nil {
			return Contents{}, err
		}
	}
	c.Files, c.FileTotal, err = data.ListIn(ctx, s.db, s.fileView, "directory_id", dir.ID, directives(l))
	if err != nil {
		return Contents{}, err
	}
	return c, nil
}
