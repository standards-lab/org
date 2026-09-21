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

// TestClassifyDelete is the truth table of the delete mapping: blobfs's
// two foreign keys into blobfs_directory mean not empty, a foreign key
// blobfs does not own (a consumer's, whatever its name) means referenced,
// and in both the sqlate.ConstraintError stays reachable. A unique or
// check violation, blobfs's own name under a class it does not report
// there, and an error that is no violation pass through unchanged.
func TestClassifyDelete(t *testing.T) {
	cause := errors.New("driver")
	cases := []struct {
		constraint string
		class      error
		want       error
	}{
		{blobfs.ConstraintForeignKeyDirectoryParent, sqlate.ErrForeignKeyViolation, blobfs.ErrNotEmpty},
		{blobfs.ConstraintForeignKeyFileDirectory, sqlate.ErrForeignKeyViolation, blobfs.ErrNotEmpty},
		{"fk_bookmark_file", sqlate.ErrForeignKeyViolation, blobfs.ErrReferenced},
		{"fk_directory_owner_directory", sqlate.ErrForeignKeyViolation, blobfs.ErrReferenced},
		{"", sqlate.ErrForeignKeyViolation, blobfs.ErrReferenced},
		{blobfs.ConstraintForeignKeyDirectoryParent, sqlate.ErrUniqueViolation, nil},
		{blobfs.ConstraintUniqueFileDirectoryName, sqlate.ErrUniqueViolation, nil},
		{"cc_something", sqlate.ErrCheckViolation, nil},
	}
	for _, c := range cases {
		in := &sqlate.ConstraintError{Constraint: c.constraint, Class: c.class, Err: cause}
		got := classifyDelete(in)
		var ce *sqlate.ConstraintError
		if !errors.As(got, &ce) || ce != in {
			t.Errorf("%q: the ConstraintError is not reachable from %v", c.constraint, got)
		}
		if c.want == nil {
			if got != in {
				t.Errorf("%q under %v = %v, want the error unchanged", c.constraint, c.class, got)
			}
			continue
		}
		if !errors.Is(got, c.want) {
			t.Errorf("%q under %v = %v, want %v", c.constraint, c.class, got, c.want)
		}
		if c.want == blobfs.ErrReferenced && errors.Is(got, blobfs.ErrNotEmpty) {
			t.Errorf("%q: a consumer's key classified as not empty", c.constraint)
		}
	}
	if got := classifyDelete(cause); got != cause {
		t.Errorf("a plain error = %v, want it unchanged", got)
	}
}
