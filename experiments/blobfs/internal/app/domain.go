package app

import (
	"github.com/spf13/cobra"

	"github.com/standards-lab/org/experiments/blobfs/output"
)

// Domain composes the domain stores, one field per domain package. Each
// field is the constructor the package's commands call when a subcommand
// runs, not the store itself: the store binds to the database, which opens
// over a DSN that is parsed after the tree is built. A constructor returns
// an error when the run names no database or the statements do not
// compile, and the error reaches the RunE that called it.
//
// The volume-based domain package was removed with the single-root
// schema, and the file-system domain over one root is the next stage's
// unit, so the struct has no field today.
type Domain struct{}

// newDomain wires the domain layer over infra, each domain package's store
// constructor closed over the infrastructure's database.
func newDomain(_ *Infrastructure) *Domain {
	return &Domain{}
}

// mountDomain builds the domain layer's commands, one list per domain
// package, each handed its store constructor from dom and the output to
// render through. They mount directly on the root: the file system's
// commands are the tool's purpose, so they are root subcommands.
func mountDomain(_ *Domain, _ *output.Output) []*cobra.Command {
	return nil
}
