// Command blobfs is the experiment's command-line file system. This file
// is process entry alone: it derives the root context from the interrupt
// signal, hands the process's streams to the composition root, and exits
// with the code the run returns. It imports only internal/app.
//
// The signal context comes from the standard library rather than
// go-core/process, which the workspace's other tools use, so go-core stays
// out of the experiment's dependency line.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/standards-lab/org/experiments/blobfs/internal/app"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return app.New(os.Stdout, os.Stderr).Run(ctx)
}
