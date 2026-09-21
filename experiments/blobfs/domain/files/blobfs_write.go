package files

import (
	"context"
	"fmt"

	"github.com/standards-lab/sqlate"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/data"
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
// the parent directory is resolved and blobfs's begin-or-resume step
// either inserts the pending row or, when a pending row already holds the
// name, returns that row to be taken up again (Resumed); the transaction
// commits, so the pending row is durable before any byte reaches the
// store, and it is where a consumer would write its own rows beside the
// pending row. Second, outside any transaction, the object is stored
// under the row's key with the declared content type. Third, on the pool,
// blobfs's complete step moves the row to available with what the store
// reported, guarded by the version read in the first step. It is the path
// form of PutFile: the resolution of the parent is the one step PutFile
// does not run, and the two share the rest.
//
// A stop after the first or the second step (StopAfter, or a failure of
// the object write) leaves the row pending, where ls and stat show it;
// the error says so, and a put of the same path resumes the row: it
// stores the object again, which replaces one an earlier attempt left,
// and completes. A name held by an available or a deleting row, which the
// begin-or-resume step reports as data.WriteExists, is
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
	var result PutResult
	result.File, err = s.db.Transact(ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
		dir, err := s.blobfs.ResolveDirectory(ctx, tx, parent)
		if err != nil {
			return blobfs.File{}, err
		}
		return s.beginPut(ctx, tx, st, dir.ID, name, req.ContentType, &result.Resumed)
	})
	if err != nil {
		return PutResult{}, fmt.Errorf("files: put %s: %w", req.Path, err)
	}
	return s.finishPut(ctx, st, req.Path, result, req)
}

// PutFile is the two-phase write of the file named name into the
// directory with directoryID: Put with the parent's resolution replaced
// by the id. The first transaction holds the scope check, when a scope is
// given, and blobfs's begin-or-resume step; the object write and the
// completion follow as in Put, with the same stops, refusals, and
// messages, which name the file as "<name> in directory <id>". req.Path
// is not read; the file is addressed by directoryID and name. A directory
// that does not exist is blobfs.ErrNotFound, and one outside the scope is
// ErrNotOwned, in both cases before any row is inserted.
func (s *Store) PutFile(ctx context.Context, directoryID, name string, req PutRequest, scope Scope) (PutResult, error) {
	label := name + " in directory " + directoryID
	st, err := s.objects(ctx)
	if err != nil {
		return PutResult{}, fmt.Errorf("files: put %s: %w", label, err)
	}
	var result PutResult
	result.File, err = s.db.Transact(ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
		if err := s.inScope(ctx, tx, scope, directoryID); err != nil {
			return blobfs.File{}, err
		}
		return s.beginPut(ctx, tx, st, directoryID, name, req.ContentType, &result.Resumed)
	})
	if err != nil {
		return PutResult{}, fmt.Errorf("files: put %s: %w", label, err)
	}
	return s.finishPut(ctx, st, label, result, req)
}

// beginPut is the first step of a put inside tx: blobfs's begin-or-resume
// step for name in the directory with directoryID, with the key validated
// against st. A resumed pending row sets *resumed; a name an available or
// a deleting row holds is blobfs.ErrNameTaken with the status named.
func (s *Store) beginPut(ctx context.Context, tx *sqlate.Tx, st *Storage, directoryID, name, contentType string, resumed *bool) (blobfs.File, error) {
	f, outcome, err := s.blobfs.BeginOrResumeFileWrite(ctx, tx, st, directoryID, name, contentType)
	if err != nil {
		return blobfs.File{}, err
	}
	switch outcome {
	case data.WriteResumed:
		*resumed = true
	case data.WriteExists:
		return blobfs.File{}, fmt.Errorf("a file named %q is %s: %w", f.Name, f.Status, blobfs.ErrNameTaken)
	}
	return f, nil
}

// finishPut runs the second and third steps of a put whose first step
// committed result.File as pending: the stop after the insert, the object
// write to st under the row's key, the stop after the write, and the
// completion on the pool guarded by the row's version. label is how the
// messages name the file.
func (s *Store) finishPut(ctx context.Context, st *Storage, label string, result PutResult, req PutRequest) (PutResult, error) {
	f := result.File
	if req.StopAfter == StepInsert {
		return result, &StopError{Command: "put", Step: StepInsert, Path: label, File: f}
	}
	obj, err := st.Put(ctx, f.Key, req.Body, req.ContentType, req.Size)
	if err != nil {
		return result, fmt.Errorf("files: put %s: %w (the row stays pending; a put of the same path retries)", label, err)
	}
	if req.StopAfter == StepWrite {
		return result, &StopError{Command: "put", Step: StepWrite, Path: label, File: f}
	}
	done, err := s.blobfs.CompleteFileWrite(ctx, s.db, f.ID, f.Version, obj)
	if err != nil {
		return result, fmt.Errorf("files: put %s: %w", label, err)
	}
	result.File = done
	return result, nil
}
