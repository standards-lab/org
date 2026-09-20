package data

import (
	"errors"
	"testing"

	"github.com/standards-lab/sqlate"

	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs"
)

// TestClassifyWrite is the truth table of the write mapping: each of
// blobfs's constraints maps to its sentinel under the class it reports,
// the root's partial index included, and the sqlate.ConstraintError stays
// reachable. A constraint blobfs does not own, a class the constraint does
// not report, and an error that is no violation pass through unchanged.
func TestClassifyWrite(t *testing.T) {
	cause := errors.New("driver")
	cases := []struct {
		constraint string
		class      error
		want       error
	}{
		{blobfs.ConstraintUniqueDirectoryRoot, sqlate.ErrUniqueViolation, blobfs.ErrRootDirectory},
		{blobfs.ConstraintUniqueDirectoryParentName, sqlate.ErrUniqueViolation, blobfs.ErrNameTaken},
		{blobfs.ConstraintUniqueFileDirectoryName, sqlate.ErrUniqueViolation, blobfs.ErrNameTaken},
		{blobfs.ConstraintForeignKeyDirectoryParent, sqlate.ErrForeignKeyViolation, blobfs.ErrNotFound},
		{blobfs.ConstraintForeignKeyFileDirectory, sqlate.ErrForeignKeyViolation, blobfs.ErrNotFound},
		{"uq_bookmark_active", sqlate.ErrUniqueViolation, nil},
		{blobfs.ConstraintUniqueDirectoryRoot, sqlate.ErrCheckViolation, nil},
	}
	for _, c := range cases {
		in := &sqlate.ConstraintError{Constraint: c.constraint, Class: c.class, Err: cause}
		got := classifyWrite(in)
		var ce *sqlate.ConstraintError
		if !errors.As(got, &ce) || ce != in {
			t.Errorf("%s: the ConstraintError is not reachable from %v", c.constraint, got)
		}
		if c.want == nil {
			if got != in {
				t.Errorf("%s under %v = %v, want the error unchanged", c.constraint, c.class, got)
			}
			continue
		}
		if !errors.Is(got, c.want) {
			t.Errorf("%s under %v = %v, want %v", c.constraint, c.class, got, c.want)
		}
	}
	if got := classifyWrite(cause); got != cause {
		t.Errorf("a plain error = %v, want it unchanged", got)
	}
}
