package app

import (
	"errors"
	"os"

	"github.com/spf13/pflag"
)

// Config is the root command's persistent flags, resolved once cobra parses
// them during execution. The tree is built before that, so a layer that
// depends on a value reads it through a closure when a subcommand runs,
// never at construction.
type Config struct {
	// DSN is the --dsn flag as parsed: the database's connection string.
	// Empty when the flag was not given; dsn falls back to BLOBFS_DSN then.
	DSN string
}

// errNoDSN reports a run that names no database by either route.
var errNoDSN = errors.New("no database: set --dsn or BLOBFS_DSN")

// storageEnvPrefix is the prefix the object store's configuration reads
// its environment under: BLOBFS_STORAGE_ENDPOINT, BLOBFS_STORAGE_CONTAINER,
// BLOBFS_STORAGE_ACCOUNT, and BLOBFS_STORAGE_KEY, and the optional limits
// the storage package documents. The store has no flag: it is opened by
// the first file command that needs it, and the directory commands never
// read these variables.
const storageEnvPrefix = "blobfs"

// bind registers the persistent flags on f. The DSN's default is left empty
// here rather than read from the environment, so New reads nothing outside
// the process; dsn consults BLOBFS_DSN when a constructor runs.
func (c *Config) bind(f *pflag.FlagSet) {
	f.StringVar(&c.DSN, "dsn", "", "the database's connection string (env BLOBFS_DSN)")
}

// dsn returns the connection string to open: --dsn when given, otherwise
// BLOBFS_DSN, and errNoDSN when both are empty. It is called from a
// constructor during execution, after cobra has parsed the flags.
func (c *Config) dsn() (string, error) {
	if c.DSN != "" {
		return c.DSN, nil
	}
	if v := os.Getenv("BLOBFS_DSN"); v != "" {
		return v, nil
	}
	return "", errNoDSN
}
