package main

import (
	"io"
	"os"

	"github.com/standards-lab/go-core/process"

	"github.com/standards-lab/org/experiments/sql-dsl/internal/app"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/config"
)

const usage = `usage: server [-schema <verb>]

With no arguments the service starts and serves. -schema runs one schema
verb against the configured database and exits (server -schema help).
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	switch {
	case len(args) == 0:
		return serve(stdout, stderr)
	case args[0] == "-schema":
		return schemaCmd(args[1:], stdout, stderr)
	case args[0] == "-h" || args[0] == "-help" || args[0] == "--help" || args[0] == "help":
		return process.Usage(stdout, usage)
	default:
		return process.Usage(stderr, usage)
	}
}

func serve(stdout, stderr io.Writer) int {
	ctx, stop := process.SignalContext()
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return process.Fail(stderr, "config load failed", err)
	}

	a, err := app.New(cfg, stdout)
	if err != nil {
		return process.Fail(stderr, "app init failed", err)
	}

	return a.Run(ctx)
}
