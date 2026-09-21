package schema

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/standards-lab/org/experiments/blobfs/lib/migrator"
	"github.com/standards-lab/org/experiments/blobfs/output"
)

// ErrResetNotConfirmed reports a schema reset run without --yes.
var ErrResetNotConfirmed = errors.New("schema reset: reverts every set and drops the history tables; pass --yes to confirm")

// deps is what every leaf command needs: the client constructor and the
// output to render its result through. Bundled once so a leaf builder is a
// method taking neither.
type deps struct {
	newClient func() (*Client, error)
	out       *output.Output
}

// Commands builds the schema command with its status, up, down, and reset
// subcommands. Each leaf's RunE calls newClient when it runs, never when
// the tree is built: the composition root closes newClient over its
// persistent flags, which cobra parses during execution, so the DSN is
// unknown until then, and the constructor fails when no DSN is set. out is
// the output every subcommand renders its result through. A subcommand
// returns its error unrendered; the root writes it through out's Error and
// sets the exit code.
func Commands(newClient func() (*Client, error), out *output.Output) *cobra.Command {
	d := deps{newClient: newClient, out: out}
	schema := &cobra.Command{
		Use:   "schema",
		Short: "Report, apply, revert, and reset the two migration sets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	schema.AddCommand(
		d.status(),
		d.leaf("up", "Apply every pending migration, blobfs's set first and then the consumer's", (*Client).Up, "schema up: both sets at head"),
		d.leaf("down", "Revert every applied migration, the consumer's set first and then blobfs's; the history tables stay", (*Client).Down, "schema down: both sets reverted"),
		d.reset(),
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

// status builds the status subcommand: one row per set, in canonical
// order, with the set's history table, its head, its latest version, the
// pending migrations by number and name, and whether the head is dirty.
func (d deps) status() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show each set's head, latest version, pending migrations, and dirty mark",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := d.newClient()
			if err != nil {
				return err
			}
			sets, err := c.Status(cmd.Context())
			if err != nil {
				return err
			}
			d.out.Rows(statusHeader, statusRows(sets))
			return nil
		},
	}
}

// reset builds the reset subcommand. It is destructive, so it runs only
// under --yes and refuses, before constructing the client, without it.
func (d deps) reset() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Revert every set, the consumer's first, and drop the history tables; requires --yes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !yes {
				return ErrResetNotConfirmed
			}
			c, err := d.newClient()
			if err != nil {
				return err
			}
			if err := c.Reset(cmd.Context()); err != nil {
				return err
			}
			d.out.Line("schema reset: both sets reverted and their history tables dropped")
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm the reset: every set is reverted and the history tables are dropped")
	return cmd
}

var statusHeader = []string{"set", "table", "version", "latest", "pending", "dirty"}

// statusRows renders each set's status as one row under statusHeader. The
// pending column lists the pending migrations as NNNN name, or none.
func statusRows(sets []migrator.SetStatus) [][]string {
	rows := make([][]string, 0, len(sets))
	for _, s := range sets {
		pending := "none"
		if len(s.Pending) > 0 {
			names := make([]string, 0, len(s.Pending))
			for _, m := range s.Pending {
				names = append(names, strconv.Itoa(m.Version)+" "+m.Name)
			}
			pending = strings.Join(names, ", ")
		}
		rows = append(rows, []string{s.Name, s.Table, strconv.Itoa(s.Version), strconv.Itoa(s.Latest), pending, strconv.FormatBool(s.Dirty)})
	}
	return rows
}
