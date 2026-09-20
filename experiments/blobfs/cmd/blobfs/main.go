// Command blobfs is the experiment's command-line file system. This stage
// wires the schema commands only: schema up applies blobfs's migration set
// and then the consumer's, and schema down reverts both in reverse order.
// The later stages add the file commands and the schema status and reset
// commands.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
)

const usage = `usage: blobfs schema up|down

BLOBFS_DSN names the database.`

// errUsage reports a command line the program does not understand; main
// prints the usage text for it.
var errUsage = errors.New("unknown command")

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, errUsage) {
			fmt.Fprintln(os.Stderr, usage)
		} else {
			fmt.Fprintln(os.Stderr, "blobfs:", err)
		}
		os.Exit(1)
	}
}

// run dispatches the command line: the command, its subcommand, and the
// DSN from the environment.
func run(args []string) error {
	if len(args) != 2 || args[0] != "schema" {
		return errUsage
	}
	dsn := os.Getenv("BLOBFS_DSN")
	if dsn == "" {
		return errors.New("BLOBFS_DSN is not set")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	db, err := openDatabase(dsn)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	schema, err := newSchema(db.DB, logger)
	if err != nil {
		return err
	}
	switch args[1] {
	case "up":
		return schema.Up(ctx)
	case "down":
		return schema.Down(ctx)
	}
	return errUsage
}
