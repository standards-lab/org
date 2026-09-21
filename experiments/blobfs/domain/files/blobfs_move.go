package files

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/standards-lab/sqlate"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// Move is mv <src> <dst>: the directory or file at src moved to dst, in
// one transaction. dst is read the way Unix reads it: when it names an
// existing directory the source moves into it under its own name, and
// otherwise dst is the new path, whose parent must exist and whose last
// segment is the new name, so a move to a new name under the same parent
// is a rename. src is resolved as a directory first and as a file when no
// directory is at the path; a directory and a file may share a name, and
// the directory wins.
//
// A directory moves through the library's MoveDirectory, which takes the
// tree lock, runs the cycle check, and updates the row, all inside this
// transaction, so the lock covers the resolutions' snapshot as well; a
// move into the directory itself or one of its descendants is
// blobfs.ErrCycle. A file moves through MoveFile, which needs no lock. The
// directory's contents and the file's object follow by id: no key
// encodes a path, so nothing moves in the store, and a bookmark of a
// moved file keeps pointing at it, with its listed path recomputed. A
// deleting file is blobfs.ErrDeleting; a pending one moves.
//
// The move stays under one top-level directory (ErrMoveAcrossScopes
// otherwise): the top-level directory that contains the source must be
// the one that contains the destination, where an entry at the top level
// counts as contained by the root. So a top-level directory may be
// renamed but not moved below another, nothing moves up to the top level
// or across two top-level directories, and an owner row keeps binding a
// directory at depth one. The rule is checked after both paths resolve
// and before anything changes.
//
// The root is blobfs.ErrRootDirectory before any I/O. A source that does
// not exist, or a destination whose parent does not, is
// blobfs.ErrNotFound; a name already held in the destination by an entry
// of the same kind is blobfs.ErrNameTaken.
func (s *Store) Move(ctx context.Context, src, dst string) (MoveResult, error) {
	srcParent, srcName, _, err := splitParent(src)
	if err != nil {
		return MoveResult{}, fmt.Errorf("files: mv %s: %w", src, err)
	}
	if !strings.HasPrefix(dst, "/") {
		return MoveResult{}, fmt.Errorf("files: mv %s %s: %w: %q does not start with /", src, dst, blobfs.ErrInvalidPath, dst)
	}
	res, err := s.db.Transact(ctx, func(tx *sqlate.Tx) (MoveResult, error) {
		parent, parentPath, name, err := s.destination(ctx, tx, dst, srcName)
		if err != nil {
			return MoveResult{}, err
		}
		to := strings.TrimSuffix(parentPath, "/") + "/" + name
		if scopeOf(src) != scopeOf(to) {
			return MoveResult{}, fmt.Errorf("%s is under %s and %s under %s: %w", src, scopePath(src), to, scopePath(to), ErrMoveAcrossScopes)
		}
		dir, err := s.blobfs.ResolveDirectory(ctx, tx, src)
		switch {
		case err == nil:
			moved, err := s.blobfs.MoveDirectory(ctx, tx, dir.ID, parent.ID, name, dir.Version)
			if err != nil {
				return MoveResult{}, err
			}
			return MoveResult{Kind: EntryDirectory, ID: moved.ID, From: src, To: strings.TrimSuffix(parentPath, "/") + "/" + *moved.Name}, nil
		case !errors.Is(err, blobfs.ErrNotFound):
			return MoveResult{}, err
		}
		srcDir, err := s.blobfs.ResolveDirectory(ctx, tx, srcParent)
		if err != nil {
			return MoveResult{}, err
		}
		f, err := s.blobfs.FileByName(ctx, tx, srcDir.ID, srcName)
		if err != nil {
			return MoveResult{}, err
		}
		moved, err := s.blobfs.MoveFile(ctx, tx, f.ID, parent.ID, name, f.Version)
		if err != nil {
			return MoveResult{}, err
		}
		return MoveResult{Kind: EntryFile, ID: moved.ID, From: src, To: strings.TrimSuffix(parentPath, "/") + "/" + moved.Name}, nil
	})
	if err != nil {
		return MoveResult{}, fmt.Errorf("files: mv %s %s: %w", src, dst, err)
	}
	return res, nil
}

// destination reads mv's destination through sess: the directory the
// source moves into, that directory's path, and the name the source
// takes there. dst names an existing directory, in which case the name
// is the source's own, or a new path, in which case the parent must exist
// and the last segment is the name.
func (s *Store) destination(ctx context.Context, sess sqlate.Session, dst, srcName string) (blobfs.Directory, string, string, error) {
	dir, err := s.blobfs.ResolveDirectory(ctx, sess, dst)
	switch {
	case err == nil:
		return dir, dst, srcName, nil
	case !errors.Is(err, blobfs.ErrNotFound):
		return blobfs.Directory{}, "", "", err
	}
	parentPath, name, _, err := splitParent(dst)
	if err != nil {
		return blobfs.Directory{}, "", "", err
	}
	parent, err := s.blobfs.ResolveDirectory(ctx, sess, parentPath)
	if err != nil {
		return blobfs.Directory{}, "", "", err
	}
	return parent, parentPath, name, nil
}

// scopeOf returns the normalized name of the top-level directory that
// contains the entry at path, or the empty string when the entry is
// itself at the top level, so that the root contains it.
func scopeOf(path string) string {
	first, _, below := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	if !below {
		return ""
	}
	return blobfs.NormalizeName(first)
}

// scopePath renders scopeOf(path) for a message: the top-level
// directory's path, or / for the root.
func scopePath(path string) string {
	if scope := scopeOf(path); scope != "" {
		return "/" + scope
	}
	return "/"
}
