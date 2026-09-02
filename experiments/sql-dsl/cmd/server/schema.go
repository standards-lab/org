package main

import (
	"context"
	"fmt"
	"io"

	"github.com/standards-lab/go-core/process"

	"github.com/standards-lab/org/experiments/sql-dsl/internal/config"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/infrastructure"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/schema"
)

// The -schema mode is the surviving fragment of a database CLI: a one-shot
// invocation of the service binary that shares its composition root and
// calls the same functions startup and the management surface call.

const schemaUsage = `usage: server -schema <verb>

verbs:
	diag     connection diagnostics: dialect, ping latency, server version, pool counters
`

func schemaCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return process.Usage(stderr, schemaUsage)
	}
	switch args[0] {
	case "-h", "-help", "--help", "help":
		return process.Usage(stdout, schemaUsage)
	case "diag":
		return withDatabase(stdout, stderr, func(ctx context.Context, infra *infrastructure.Infrastructure) int {
			d, err := schema.Diagnose(ctx, infra.DB)
			if err != nil {
				return process.Fail(stderr, "diagnostics failed", err)
			}
			d.Write(stdout)
			return process.ExitOK
		})
	default:
		return process.Usage(stderr, fmt.Sprintf("server -schema: unknown verb %q\n%s", args[0], schemaUsage))
	}
}

// withDatabase composes the infrastructure without a lifecycle, starts the
// database, runs fn, and shuts the database down.
func withDatabase(
	stdout, stderr io.Writer,
	fn func(ctx context.Context, infra *infrastructure.Infrastructure) int,
) int {
	ctx, stop := process.SignalContext()
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return process.Fail(stderr, "config load failed", err)
	}

	infra, err := infrastructure.New(stdout, cfg, nil)
	if err != nil {
		return process.Fail(stderr, "infrastructure init failed", err)
	}

	if err := infra.DB.Start(ctx); err != nil {
		return process.Fail(stderr, "database start failed", err)
	}
	defer func() {
		if err := infra.DB.Shutdown(context.Background()); err != nil {
			_, _ = fmt.Fprintln(stderr, "database shutdown:", err)
		}
	}()

	return fn(ctx, infra)
}
