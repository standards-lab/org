package files

import (
	"context"
	"fmt"

	"github.com/standards-lab/sqlate"
)

// InScope checks, on the pool, that the directory with directoryID lies
// within scope: nil when it does, ErrNotOwned when the unit does not own
// the scope directory or the directory is not within it. It is the
// directory-grain ownership check by id, the one every id-keyed operation
// runs when it is given a scope, exposed so a caller can check a
// client-named scope on its own. A zero scope checks nothing and returns
// nil.
//
// The scope's directory id is an input to check, not to trust: the owner
// row of the scope directory is read first, and only a directory the unit
// owns is a scope at all, so a unit that names a directory it does not
// own learns nothing about what lies under it; the containment check,
// the library's IsWithin, runs second and costs the depth of the target.
// The root has no owner row, so naming it as the scope is refused.
func (s *Store) InScope(ctx context.Context, directoryID string, scope Scope) error {
	return s.inScope(ctx, s.db, scope, directoryID)
}

// inScope is the check through sess for the directories with ids: one
// owner read of the scope directory, then one IsWithin per id, in order,
// stopping at the first refusal. A zero scope checks nothing.
func (s *Store) inScope(ctx context.Context, sess sqlate.Session, scope Scope, ids ...string) error {
	if scope == (Scope{}) {
		return nil
	}
	if scope.Unit == "" || scope.DirectoryID == "" {
		return fmt.Errorf("files: scope (unit %q, directory %q) names a unit or a directory but not both: %w", scope.Unit, scope.DirectoryID, ErrNotOwned)
	}
	owned, err := s.ownedByUnit(ctx, sess, scope.DirectoryID, scope.Unit)
	if err != nil {
		return err
	}
	if !owned {
		return fmt.Errorf("files: unit %s does not own the scope directory %s: %w", scope.Unit, scope.DirectoryID, ErrNotOwned)
	}
	for _, id := range ids {
		within, err := s.blobfs.IsWithin(ctx, sess, id, scope.DirectoryID)
		if err != nil {
			return err
		}
		if !within {
			return fmt.Errorf("files: directory %s is not within the scope %s of unit %s: %w", id, scope.DirectoryID, scope.Unit, ErrNotOwned)
		}
	}
	return nil
}
