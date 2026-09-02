package main

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/standards-lab/go-core/process"

	"github.com/standards-lab/org/experiments/sql-dsl/internal/config"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/infrastructure"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/schema"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/migrate"
)

// The -schema mode is the surviving fragment of a database CLI: a one-shot
// invocation of the service binary that shares its composition root and
// calls the same functions startup and the management surface call.

const schemaUsage = `usage: server -schema <verb> [arg]

verbs:
	diag         connection diagnostics: dialect, ping latency, server version, pool counters
	version      the applied schema version and whether it is dirty
	verify       check the history is the embedded set's clean head (exit 1 otherwise)
	up           apply every pending migration
	down [n]     revert the n most recent migrations (default 1)
	steps <n>    apply n pending (n > 0) or revert -n applied (n < 0)
	force <v>    set the history to version v, clearing dirty state; 0 empties it
`

func schemaCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return process.Usage(stderr, schemaUsage)
	}
	verb, rest := args[0], args[1:]
	switch verb {
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
	case "version", "verify", "up", "down", "steps", "force":
		n, err := verbArg(verb, rest)
		if err != nil {
			return process.Usage(stderr, fmt.Sprintf("server -schema %s: %v\n%s", verb, err, schemaUsage))
		}
		return withMigrator(stdout, stderr, func(ctx context.Context, m *migrate.Migrator) int {
			return runVerb(ctx, m, verb, n, stdout, stderr)
		})
	default:
		return process.Usage(stderr, fmt.Sprintf("server -schema: unknown verb %q\n%s", verb, schemaUsage))
	}
}

// verbArg parses the verb's integer argument: required for steps and
// force, optional for down (default 1), absent otherwise.
func verbArg(verb string, rest []string) (int, error) {
	switch verb {
	case "steps", "force":
		if len(rest) != 1 {
			return 0, fmt.Errorf("takes one integer argument")
		}
	case "down":
		if len(rest) == 0 {
			return 1, nil
		}
		if len(rest) > 1 {
			return 0, fmt.Errorf("takes at most one integer argument")
		}
	default:
		if len(rest) != 0 {
			return 0, fmt.Errorf("takes no arguments")
		}
		return 0, nil
	}
	n, err := strconv.Atoi(rest[0])
	if err != nil {
		return 0, fmt.Errorf("argument %q is not an integer", rest[0])
	}
	return n, nil
}

func runVerb(ctx context.Context, m *migrate.Migrator, verb string, n int, stdout, stderr io.Writer) int {
	var err error
	switch verb {
	case "version":
		var v migrate.Version
		if v, err = m.Version(ctx); err == nil {
			_, _ = fmt.Fprintf(stdout, "version: %d dirty: %t\n", v.Version, v.Dirty)
			return process.ExitOK
		}
	case "verify":
		err = m.Verify(ctx)
	case "up":
		err = m.Up(ctx)
	case "down":
		err = m.Down(ctx, n)
	case "steps":
		err = m.Steps(ctx, n)
	case "force":
		err = m.Force(ctx, n)
	}
	if err != nil {
		return process.Fail(stderr, "schema "+verb+" failed", err)
	}
	_, _ = fmt.Fprintf(stdout, "schema %s: ok\n", verb)
	return process.ExitOK
}

func withMigrator(stdout, stderr io.Writer, fn func(context.Context, *migrate.Migrator) int) int {
	return withDatabase(stdout, stderr, func(ctx context.Context, infra *infrastructure.Infrastructure) int {
		m, err := schema.NewMigrator(infra.SQL, infra.Logger)
		if err != nil {
			return process.Fail(stderr, "migrator init failed", err)
		}
		return fn(ctx, m)
	})
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
