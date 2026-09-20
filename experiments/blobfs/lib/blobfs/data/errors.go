package data

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/standards-lab/sqlate"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// writeSentinels maps the constraints an insert or an update can violate
// to the sentinel each one means there: a unique constraint on a name is a
// name already held, and a foreign key to a directory is a parent or
// directory that does not exist. The mapping is the write's view of the
// constraint. A delete violates the same foreign keys with the opposite
// meaning (the row still has children), so the delete operations of a
// later stage carry their own table.
var writeSentinels = map[string]error{
	blobfs.ConstraintUniqueVolumeName:          blobfs.ErrNameTaken,
	blobfs.ConstraintUniqueDirectoryParentName: blobfs.ErrNameTaken,
	blobfs.ConstraintUniqueFileDirectoryName:   blobfs.ErrNameTaken,
	blobfs.ConstraintForeignKeyDirectoryParent: blobfs.ErrNotFound,
	blobfs.ConstraintForeignKeyFileDirectory:   blobfs.ErrNotFound,
}

// classifyWrite maps a constraint violation from an insert or an update to
// blobfs's sentinel when the violated constraint is one blobfs owns and
// writeSentinels lists, keeping the sqlate.ConstraintError reachable through
// errors.As. Any other error, a violation of a consumer's constraint
// included, is returned as it came.
func classifyWrite(err error) error {
	var ce *sqlate.ConstraintError
	if !errors.As(err, &ce) {
		return err
	}
	sentinel, ok := writeSentinels[ce.Constraint]
	if !ok {
		return err
	}
	switch {
	case sentinel == blobfs.ErrNameTaken && errors.Is(ce.Class, sqlate.ErrUniqueViolation),
		sentinel == blobfs.ErrNotFound && errors.Is(ce.Class, sqlate.ErrForeignKeyViolation):
		return fmt.Errorf("%w: %w", sentinel, err)
	}
	return err
}

// notFound maps sql.ErrNoRows, which the typed handles return unmapped, to
// blobfs.ErrNotFound and leaves every other error as it came.
func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return blobfs.ErrNotFound
	}
	return err
}

// validName normalizes name and validates the normalized form, returning
// the form to store. A refusal is the root package's NameError.
func validName(name string) (string, error) {
	name = blobfs.NormalizeName(name)
	if err := blobfs.ValidateName(name); err != nil {
		return "", err
	}
	return name, nil
}
