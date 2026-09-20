package data

import (
	"context"
	"fmt"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// File returns the file with id, or blobfs.ErrNotFound.
func (s *Store) File(ctx context.Context, sess sqlate.Session, id string) (blobfs.File, error) {
	f, err := s.fileByID.One(ctx, sess, query.Args{"id": id})
	if err != nil {
		return blobfs.File{}, fmt.Errorf("data: file %s: %w", id, notFound(err))
	}
	return f, nil
}
