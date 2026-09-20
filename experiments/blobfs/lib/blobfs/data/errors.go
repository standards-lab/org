package data

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/standards-lab/sqlate"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// writeMapping is what a constraint means on a write: the violation class
// the constraint reports and the sentinel that class means there.
type writeMapping struct {
	class    error
	sentinel error
}

// writeSentinels maps the constraints an insert or an update can violate
// to the sentinel each one means there: a unique constraint on a name is a
// name already held, the root's partial unique index is a second root, and
// a foreign key to a directory is a parent or directory that does not
// exist. The mapping is the write's view of the constraint. A delete
// violates the same foreign keys with the opposite meaning (the row still
// has children), so the delete operations of a later stage carry their own
// table.
var writeSentinels = map[string]writeMapping{
	blobfs.ConstraintUniqueDirectoryRoot:       {sqlate.ErrUniqueViolation, blobfs.ErrRootDirectory},
	blobfs.ConstraintUniqueDirectoryParentName: {sqlate.ErrUniqueViolation, blobfs.ErrNameTaken},
	blobfs.ConstraintUniqueFileDirectoryName:   {sqlate.ErrUniqueViolation, blobfs.ErrNameTaken},
	blobfs.ConstraintForeignKeyDirectoryParent: {sqlate.ErrForeignKeyViolation, blobfs.ErrNotFound},
	blobfs.ConstraintForeignKeyFileDirectory:   {sqlate.ErrForeignKeyViolation, blobfs.ErrNotFound},
}

// classifyWrite maps a constraint violation from an insert or an update to
// blobfs's sentinel when the violated constraint is one blobfs owns and
// writeSentinels lists under the class reported, keeping the
// sqlate.ConstraintError reachable through errors.As. Any other error, a
// violation of a consumer's constraint included, is returned as it came.
func classifyWrite(err error) error {
	var ce *sqlate.ConstraintError
	if !errors.As(err, &ce) {
		return err
	}
	m, ok := writeSentinels[ce.Constraint]
	if !ok || !errors.Is(ce.Class, m.class) {
		return err
	}
	return fmt.Errorf("%w: %w", m.sentinel, err)
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
