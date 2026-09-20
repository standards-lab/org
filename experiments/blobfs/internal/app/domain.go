package app

import (
	"github.com/spf13/cobra"

	"github.com/standards-lab/org/experiments/blobfs/domain/files"
	"github.com/standards-lab/org/experiments/blobfs/output"
)

// Domain composes the domain stores, one field per domain package. Each
// field is the constructor the package's commands call when a subcommand
// runs, not the store itself: the store binds to the database, which opens
// over a DSN that is parsed after the tree is built. A constructor returns
// an error when the run names no database or the statements do not
// compile, and the error reaches the RunE that called it.
type Domain struct {
	Files func() (*files.Store, error)
}

// newDomain wires the domain layer over infra, each domain package's store
// constructor closed over the infrastructure's database.
func newDomain(infra *Infrastructure) *Domain {
	return &Domain{
		Files: func() (*files.Store, error) {
			db, err := infra.Database()
			if err != nil {
				return nil, err
			}
			return files.New(db)
		},
	}
}

// mountDomain builds the domain layer's commands, one list per domain
// package, each handed its store constructor from dom and the output to
// render through. They mount directly on the root: the file system's
// commands are the tool's purpose, so they are root subcommands.
func mountDomain(dom *Domain, out *output.Output) []*cobra.Command {
	return files.Commands(dom.Files, out)
}
