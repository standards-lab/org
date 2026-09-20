package app

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"github.com/standards-lab/org/experiments/blobfs/output"
)

// App is the application: the command tree assembled over the
// infrastructure and the admin layer, the output the commands render
// through, and the infrastructure that owns what a run opens.
type App struct {
	root  *cobra.Command
	out   *output.Output
	infra *Infrastructure
}

// New is the cold start: it composes the layers in dependency order and
// performs no I/O. The tree writes its output to stdout and its errors to
// stderr: cobra's own output (help, usage) through the root's writers, the
// logger's lines through stderr, and every command's result through the
// one output.Output built over the same two streams.
func New(stdout, stderr io.Writer) *App {
	cfg := &Config{}
	root := newRoot(cfg)
	root.SetOut(stdout)
	root.SetErr(stderr)

	out := output.New(stdout, stderr)
	infra := newInfrastructure(cfg, stderr)
	adm := newAdmin(infra)

	root.AddCommand(commands(adm, out)...)

	return &App{root: root, out: out, infra: infra}
}

// Run is the hot start: it executes the tree under ctx, which cancels the
// running command when the process is signalled, then closes what the
// infrastructure opened during the run. The tree silences cobra's own
// reporting, so the error a command returns is rendered here, through the
// output's Error, and sets the exit code. A close error is rendered the
// same way when the command itself succeeded.
func (a *App) Run(ctx context.Context) int {
	err := a.root.ExecuteContext(ctx)
	if cerr := a.infra.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		a.out.Error(err)
		return 1
	}
	return 0
}

// newRoot builds the root command with cfg's persistent flags bound. Run
// without a subcommand, it prints its help.
func newRoot(cfg *Config) *cobra.Command {
	root := &cobra.Command{
		Use:   "blobfs",
		Short: "A command-line file system over blobfs",
		Long: "blobfs is the experiment's command-line file system: a SQL-backed directory\n" +
			"tree over Postgres with file content in an object store. schema applies and\n" +
			"reverts the database schema; the file commands arrive in later stages.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cfg.bind(root.PersistentFlags())
	return root
}
