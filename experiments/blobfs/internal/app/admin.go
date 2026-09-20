package app

import (
	"github.com/spf13/cobra"

	"github.com/standards-lab/org/experiments/blobfs/admin/schema"
	"github.com/standards-lab/org/experiments/blobfs/output"
)

// Admin composes the admin clients, one field per admin package. Each field
// is the constructor the package's commands call when a subcommand runs,
// not the client itself: the client binds to the database, which opens over
// a DSN that is parsed after the tree is built. A constructor returns an
// error when the run names no database, and the error reaches the RunE
// that called it.
type Admin struct {
	Schema func() (*schema.Client, error)
}

// newAdmin wires the admin layer over infra, each admin package's client
// constructor closed over the infrastructure's database and logger.
func newAdmin(infra *Infrastructure) *Admin {
	return &Admin{
		Schema: func() (*schema.Client, error) {
			db, err := infra.Database()
			if err != nil {
				return nil, err
			}
			return schema.NewClient(db, infra.Logger())
		},
	}
}

// mountAdmin builds the admin layer's commands, one per admin package, each
// handed its client constructor from adm and the output to render through.
// They mount directly on the root: a command-line tool that owns no service
// has no admin container the way a service has an /admin mount, so schema
// is a root subcommand.
func mountAdmin(adm *Admin, out *output.Output) []*cobra.Command {
	return []*cobra.Command{
		schema.Commands(adm.Schema, out),
	}
}
