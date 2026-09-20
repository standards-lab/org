package data

import (
	"context"
	"fmt"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// Mkdir creates a directory named name under the directory with parentID
// and returns the row as the database holds it. The name is normalized and
// validated first; a refusal is a blobfs.NameError. A name already held by
// a directory under the same parent is blobfs.ErrNameTaken, and a parent
// that does not exist is blobfs.ErrNotFound. One row is written, so the
// session may be the pool or a transaction.
func (s *Store) Mkdir(ctx context.Context, sess sqlate.Session, parentID, name string) (blobfs.Directory, error) {
	name, err := validName(name)
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: mkdir: %w", err)
	}
	id := blobfs.NewID()
	if _, err := s.createDirectory.Exec(ctx, sess, query.Args{"id": id, "parent_id": parentID, "name": name}); err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: mkdir %q under %s: %w", name, parentID, classifyWrite(err))
	}
	d, err := s.directoryByID.One(ctx, sess, query.Args{"id": id})
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: read back directory %q: %w", name, err)
	}
	return d, nil
}

// Directory returns the directory with id, or blobfs.ErrNotFound.
func (s *Store) Directory(ctx context.Context, sess sqlate.Session, id string) (blobfs.Directory, error) {
	d, err := s.directoryByID.One(ctx, sess, query.Args{"id": id})
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: directory %s: %w", id, notFound(err))
	}
	return d, nil
}

// Children lists the directories whose parent is parentID under d: one
// page, sorted and filtered by the declared fields id, parent_id, name,
// version, created_at, and updated_at, and the total under the same
// filters. The parent filter is appended to the caller's directives
// through ListIn, so the listing is always scoped to one parent. A parent
// that does not exist lists no rows and a total of zero, since the base is
// the directory table alone.
func (s *Store) Children(ctx context.Context, sess sqlate.Session, parentID string, d query.Directives) ([]blobfs.Directory, int, error) {
	return ListIn(ctx, sess, s.directories, "parent_id", parentID, d)
}
