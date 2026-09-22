package app

import (
	"github.com/spf13/cobra"

	"github.com/standards-lab/org/experiments/blobfs/domain/files"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/postgres"
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
// constructor closed over the infrastructure's database and its object
// store's opener, which the store calls when a file command first needs
// an object. The files store is built over the variant the run chose,
// resolved before the database opens so an unknown name is refused before
// any I/O.
func newDomain(infra *Infrastructure) *Domain {
	return &Domain{
		Files: func() (*files.Store, error) {
			opts, err := variantOptions(infra.cfg)
			if err != nil {
				return nil, err
			}
			db, err := infra.Database()
			if err != nil {
				return nil, err
			}
			return files.New(db, infra.Storage, opts...)
		},
	}
}

// variantOptions returns the options that make files.New build blobfs's
// store over the variant cfg resolves. The standard baseline is what
// files.New builds without an option, compiled once with the store's own
// statements, so it maps to none; the Postgres variant maps to
// files.WithVariant over postgres.New. This is the one file of the
// application that names the Postgres engine package: the variant is a
// composition choice, and no domain or admin package makes it.
func variantOptions(cfg *Config) ([]files.Option, error) {
	name, err := cfg.variant()
	if err != nil {
		return nil, err
	}
	if name == variantPostgres {
		return []files.Option{files.WithVariant(postgres.New)}, nil
	}
	return nil, nil
}

// mountDomain builds the domain layer's commands, one list per domain
// package, each handed its store constructor from dom and the output to
// render through. They mount directly on the root: the file system's
// commands are the tool's purpose, so they are root subcommands.
func mountDomain(dom *Domain, out *output.Output) []*cobra.Command {
	return files.Commands(dom.Files, out)
}
