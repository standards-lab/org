// Package app is the composition root: App assembles the layers into a
// command tree and runs the process. The package is one file per layer, so
// its file list is the application's layer list: config.go declares the
// root's persistent flags and the resolved values, infrastructure.go opens
// the database and builds the logger and the output, domain.go the domain
// stores and their commands, admin.go the admin clients and their commands,
// and commands.go is the list of mounts. Extending the application means
// editing a layer file's body; the signatures, cmd/blobfs, and App.Run stay
// untouched.
//
// New is the cold start, with no I/O: it builds the root command with its
// persistent flags, the infrastructure over the config, the domain and
// admin layers over the infrastructure, and mounts each layer's commands
// on the root.
// The values the layers depend on, the DSN and the variant name, are
// persistent flags with environment variables behind them, and cobra parses
// flags during execution, after the tree is built. So the infrastructure
// opens the database on demand rather than at New, and each layer closes a
// client constructor over it that a subcommand's RunE calls when it runs.
// Nothing below the root reads a flag or the environment before then, and a
// constructor that finds no DSN, or a variant name it does not know, fails,
// with the error reaching the RunE that called it. The variant is the
// domain layer's choice alone: domain.go maps the name to the option
// files.New takes, and is the one application file that names the Postgres
// variant.
//
// Run is the hot start: it executes the tree under ctx, closes what the
// infrastructure opened, renders the error a command returns, and returns
// the process exit code.
package app
