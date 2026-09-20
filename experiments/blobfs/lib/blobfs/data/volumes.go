package data

import (
	"context"
	"fmt"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/query"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// CreateVolume creates a volume named name and its root directory, and
// returns both rows as the database holds them. The name is normalized and
// validated first; a refusal is a blobfs.NameError. The session must be a
// *sqlate.Tx, because the two inserts are one unit and each statement is
// headed transaction: required: any other session is refused with
// query.ErrTransactionRequired. A name another volume holds is
// blobfs.ErrNameTaken.
func (s *Store) CreateVolume(ctx context.Context, sess sqlate.Session, name string) (blobfs.Volume, blobfs.Directory, error) {
	name, err := validName(name)
	if err != nil {
		return blobfs.Volume{}, blobfs.Directory{}, fmt.Errorf("data: create volume: %w", err)
	}
	volumeID, rootID := blobfs.NewID(), blobfs.NewID()
	if _, err := s.createVolume.Exec(ctx, sess, query.Args{"id": volumeID, "name": name}); err != nil {
		return blobfs.Volume{}, blobfs.Directory{}, fmt.Errorf("data: create volume %q: %w", name, classifyWrite(err))
	}
	if _, err := s.createRootDirectory.Exec(ctx, sess, query.Args{"id": rootID, "volume_id": volumeID}); err != nil {
		return blobfs.Volume{}, blobfs.Directory{}, fmt.Errorf("data: create root of volume %q: %w", name, classifyWrite(err))
	}
	volume, err := s.volumeByID.One(ctx, sess, query.Args{"id": volumeID})
	if err != nil {
		return blobfs.Volume{}, blobfs.Directory{}, fmt.Errorf("data: read back volume %q: %w", name, err)
	}
	root, err := s.directoryByID.One(ctx, sess, query.Args{"id": rootID})
	if err != nil {
		return blobfs.Volume{}, blobfs.Directory{}, fmt.Errorf("data: read back root of volume %q: %w", name, err)
	}
	return volume, root, nil
}

// Volume returns the volume with id, or blobfs.ErrNotFound.
func (s *Store) Volume(ctx context.Context, sess sqlate.Session, id string) (blobfs.Volume, error) {
	v, err := s.volumeByID.One(ctx, sess, query.Args{"id": id})
	if err != nil {
		return blobfs.Volume{}, fmt.Errorf("data: volume %s: %w", id, notFound(err))
	}
	return v, nil
}

// VolumeByName returns the volume named name, compared after
// normalization, or blobfs.ErrNotFound.
func (s *Store) VolumeByName(ctx context.Context, sess sqlate.Session, name string) (blobfs.Volume, error) {
	name = blobfs.NormalizeName(name)
	v, err := s.volumeByName.One(ctx, sess, query.Args{"name": name})
	if err != nil {
		return blobfs.Volume{}, fmt.Errorf("data: volume %q: %w", name, notFound(err))
	}
	return v, nil
}

// Volumes lists volumes under d: one page, sorted and filtered by the
// declared fields id, name, version, created_at, and updated_at, and the
// total under the same filters. A directive naming another field is a
// query.UnknownFieldError.
func (s *Store) Volumes(ctx context.Context, sess sqlate.Session, d query.Directives) ([]blobfs.Volume, int, error) {
	return s.volumes.List(ctx, sess, d)
}

// RootDirectory returns the root directory of the volume with volumeID,
// found by the root's back-reference to its volume, or blobfs.ErrNotFound.
func (s *Store) RootDirectory(ctx context.Context, sess sqlate.Session, volumeID string) (blobfs.Directory, error) {
	root, err := s.rootDirectory.One(ctx, sess, query.Args{"volume_id": volumeID})
	if err != nil {
		return blobfs.Directory{}, fmt.Errorf("data: root of volume %s: %w", volumeID, notFound(err))
	}
	return root, nil
}

// RenameVolume renames the volume with id to name under the
// optimistic-concurrency guard and returns the new version. The name is
// normalized and validated first. A volume whose current version is not
// version is query.ErrVersionMismatch, a missing volume is
// blobfs.ErrNotFound, and a name another volume holds is
// blobfs.ErrNameTaken.
func (s *Store) RenameVolume(ctx context.Context, sess sqlate.Session, id string, version int64, name string) (int64, error) {
	name, err := validName(name)
	if err != nil {
		return 0, fmt.Errorf("data: rename volume: %w", err)
	}
	next, err := s.renameVolume.Run(ctx, sess, version, query.Args{"id": id, "name": name})
	if err != nil {
		return 0, fmt.Errorf("data: rename volume %s: %w", id, notFound(classifyWrite(err)))
	}
	return next, nil
}
