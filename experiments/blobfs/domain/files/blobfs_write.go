package files

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	return s.finishPut(ctx, st, "put", req.Path, result, req)
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
	return s.finishPut(ctx, st, "put", label, result, req)
}

// Copy is cp <src> <dst>: the available file at src copied to a new file
// at dst with the same bytes and the same content type, through the
// two-phase write Put runs. dst is read as mv reads it: an existing
// directory receives the copy under the source's name, and any other path
// is the copy's path, whose parent must exist and whose last segment is
// the copy's name. The first transaction resolves the source, refuses one
// that is not an available file, reads the destination, and runs blobfs's
// begin-or-resume step with the source's content type, so a pending row
// an earlier stopped cp left at the destination is taken up again
// (Resumed); it commits before any byte moves. The source's object is
// then read from the store and written under the new row's key, streamed
// through this process, and the completion on the pool records the size
// and the entity tag the store reports for the new object. The stops,
// StopAfter insert and write, and the retry are Put's: a stop leaves the
// copy's row pending, and a cp of the same paths resumes it and streams
// the source again.
//
// The source is checked before the destination is read. cp copies files,
// so a file at src wins over a directory of the same name, and src
// naming a directory and no file is ErrNotAFile; a pending or a deleting
// file is ErrNotAvailable with the status in the message. A destination
// name an available or a deleting file holds is blobfs.ErrNameTaken, so
// nothing is overwritten, and a copy onto the source itself is refused
// that way, since the source holds its own name. A copy may cross
// top-level directories, and neither a bookmark nor an owner row follows
// it. The root as src is blobfs.ErrRootDirectory and a relative dst is
// blobfs.ErrInvalidPath, both before any I/O; a source that does not
// exist, or a destination parent that does not, is blobfs.ErrNotFound.
// The object store is opened before the first step, as in Put.
func (s *Store) Copy(ctx context.Context, req CopyRequest) (CopyResult, error) {
	srcParent, srcName, _, err := splitParent(req.Source)
	if err != nil {
		return CopyResult{}, fmt.Errorf("files: cp %s: %w", req.Source, err)
	}
	if !strings.HasPrefix(req.Destination, "/") {
		return CopyResult{}, fmt.Errorf("files: cp %s %s: %w: %q does not start with /", req.Source, req.Destination, blobfs.ErrInvalidPath, req.Destination)
	}
	label := req.Source + " " + req.Destination
	st, err := s.objects(ctx)
	if err != nil {
		return CopyResult{}, fmt.Errorf("files: cp %s: %w", label, err)
	}
	var src blobfs.File
	var to string
	var result PutResult
	result.File, err = s.db.Transact(ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
		dir, err := s.blobfs.ResolveDirectory(ctx, tx, srcParent)
		if err != nil {
			return blobfs.File{}, err
		}
		src, err = s.blobfs.FileByName(ctx, tx, dir.ID, srcName)
		if errors.Is(err, blobfs.ErrNotFound) {
			if _, dirErr := s.blobfs.ResolveDirectory(ctx, tx, req.Source); dirErr == nil {
				return blobfs.File{}, ErrNotAFile
			}
		}
		if err != nil {
			return blobfs.File{}, err
		}
		if err := copyable(src); err != nil {
			return blobfs.File{}, err
		}
		parent, parentPath, name, err := s.destination(ctx, tx, req.Destination, src.Name)
		if err != nil {
			return blobfs.File{}, err
		}
		f, err := s.beginPut(ctx, tx, st, parent.ID, name, src.ContentType, &result.Resumed)
		if err != nil {
			return blobfs.File{}, err
		}
		to = strings.TrimSuffix(parentPath, "/") + "/" + f.Name
		return f, nil
	})
	if err != nil {
		return CopyResult{}, fmt.Errorf("files: cp %s: %w", label, err)
	}
	result, err = s.finishCopy(ctx, st, label, src, result, req.StopAfter)
	return CopyResult{From: req.Source, To: to, File: result.File, Resumed: result.Resumed}, err
}

// CopyFile copies the file with id into the directory with directoryID,
// as name or under its own name when name is empty: Copy with both
// resolutions replaced by ids. The first transaction reads the source
// row, runs the scope check when a scope is given, on the source's
// directory and on the destination as MoveEntry does, and runs blobfs's
// begin-or-resume step; the object read and write and the completion
// follow as in Copy, with the same stops, refusals, and messages, which
// name the copy as "file <id> into directory <id>". The result's From and
// To are empty: no path is computed for a copy by id. A source that does
// not exist, or a destination directory that does not, is
// blobfs.ErrNotFound, and a directory outside the scope is ErrNotOwned,
// in both cases before any row is inserted.
func (s *Store) CopyFile(ctx context.Context, id, directoryID, name string, stopAfter Step, scope Scope) (CopyResult, error) {
	label := "file " + id + " into directory " + directoryID
	st, err := s.objects(ctx)
	if err != nil {
		return CopyResult{}, fmt.Errorf("files: cp %s: %w", label, err)
	}
	var src blobfs.File
	var result PutResult
	result.File, err = s.db.Transact(ctx, func(tx *sqlate.Tx) (blobfs.File, error) {
		f, err := s.blobfs.File(ctx, tx, id)
		if err != nil {
			return blobfs.File{}, err
		}
		src = f
		if err := s.inScope(ctx, tx, scope, src.DirectoryID, directoryID); err != nil {
			return blobfs.File{}, err
		}
		if err := copyable(src); err != nil {
			return blobfs.File{}, err
		}
		as := name
		if as == "" {
			as = src.Name
		}
		return s.beginPut(ctx, tx, st, directoryID, as, src.ContentType, &result.Resumed)
	})
	if err != nil {
		return CopyResult{}, fmt.Errorf("files: cp %s: %w", label, err)
	}
	result, err = s.finishCopy(ctx, st, label, src, result, stopAfter)
	return CopyResult{File: result.File, Resumed: result.Resumed}, err
}

// copyable checks the source row src of a copy: only an available file
// has an object to copy, and a pending or a deleting one is
// ErrNotAvailable with the status named, before the destination is read.
func copyable(src blobfs.File) error {
	if src.Status != blobfs.StatusAvailable {
		return fmt.Errorf("the file is %s: %w", src.Status, ErrNotAvailable)
	}
	return nil
}

// finishCopy runs the second and third steps of a copy whose first step
// committed result.File as pending: the stop after the insert, the read
// of src's object from st, and then finishPut over that object as the
// body, with src's content type and size declared. The source's object
// is opened after the insert stop, so a stop there reaches the store for
// nothing, and a source object the store no longer holds leaves the row
// pending, as a failed object write does.
func (s *Store) finishCopy(ctx context.Context, st *Storage, label string, src blobfs.File, result PutResult, stopAfter Step) (PutResult, error) {
	if stopAfter == StepInsert {
		return result, &StopError{Command: "cp", Step: StepInsert, Path: label, File: result.File}
	}
	body, err := st.Get(ctx, src.Key)
	if err != nil {
		return result, fmt.Errorf("files: cp %s: %w (the row stays pending; a cp of the same path retries)", label, err)
	}
	defer func() { _ = body.Close() }()
	return s.finishPut(ctx, st, "cp", label, result, PutRequest{Body: body, ContentType: src.ContentType, Size: sizeOf(src), StopAfter: stopAfter})
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
// write to st under the row's key with req's body, content type, and
// size, the stop after the write, and the completion on the pool guarded
// by the row's version. command is the command the messages and the stop
// name, put or cp, and label is how they name the file; req.Path is not
// read.
func (s *Store) finishPut(ctx context.Context, st *Storage, command, label string, result PutResult, req PutRequest) (PutResult, error) {
	f := result.File
	if req.StopAfter == StepInsert {
		return result, &StopError{Command: command, Step: StepInsert, Path: label, File: f}
	}
	obj, err := st.Put(ctx, f.Key, req.Body, req.ContentType, req.Size)
	if err != nil {
		return result, fmt.Errorf("files: %s %s: %w (the row stays pending; a %s of the same path retries)", command, label, err, command)
	}
	if req.StopAfter == StepWrite {
		return result, &StopError{Command: command, Step: StepWrite, Path: label, File: f}
	}
	done, err := s.blobfs.CompleteFileWrite(ctx, s.db, f.ID, f.Version, obj)
	if err != nil {
		return result, fmt.Errorf("files: %s %s: %w", command, label, err)
	}
	result.File = done
	return result, nil
}
