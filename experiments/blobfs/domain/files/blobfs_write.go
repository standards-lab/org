package files

import (
	"context"
	"errors"
	"fmt"

	"github.com/standards-lab/sqlate"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

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
