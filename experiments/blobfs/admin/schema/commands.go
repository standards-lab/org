package schema

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/standards-lab/org/experiments/blobfs/output"
)

// deps is what every leaf command needs: the client constructor and the
// output to render its result through. Bundled once so a leaf builder is a
// method taking neither.
type deps struct {
	newClient func() (*Client, error)
	out       *output.Output
}

// Commands builds the schema command with its up and down subcommands.
// Each leaf's RunE calls newClient when it runs, never when the tree is
// built: the composition root closes newClient over its persistent flags,
// which cobra parses during execution, so the DSN is unknown until then,
// and the constructor fails when no DSN is set. out is the output every
// subcommand renders its result through. A subcommand returns its error
// unrendered; the root writes it through out's Error and sets the exit
// code.
func Commands(newClient func() (*Client, error), out *output.Output) *cobra.Command {
	d := deps{newClient: newClient, out: out}
	schema := &cobra.Command{
		Use:   "schema",
		Short: "Apply and revert the two migration sets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	schema.AddCommand(
		d.leaf("up", "Apply every pending migration, blobfs's set first and then the consumer's", (*Client).Up, "schema up: both sets at head"),
		d.leaf("down", "Revert every applied migration, the consumer's set first and then blobfs's", (*Client).Down, "schema down: both sets reverted"),
	)
	return schema
}

// leaf builds one subcommand over a Client method that takes no input: it
// constructs the client, runs op under the command's context, and prints
// result on success.
func (d deps) leaf(use, short string, op func(*Client, context.Context) error, result string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := d.newClient()
			if err != nil {
				return err
			}
			if err := op(c, cmd.Context()); err != nil {
				return err
			}
			d.out.Line(result)
			return nil
		},
	}
}
