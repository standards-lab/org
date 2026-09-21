package app

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

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
	// Variant is the --variant flag as parsed: the name of the blobfs
	// variant the file commands run over. Empty when the flag was not
	// given; variant falls back to BLOBFS_VARIANT and then to the standard
	// baseline.
	Variant string
}

// errNoDSN reports a run that names no database by either route.
var errNoDSN = errors.New("no database: set --dsn or BLOBFS_DSN")

// The names --variant and BLOBFS_VARIANT accept. variantStandard is the
// default: the standard-tier baseline every engine runs. variantPGNative is
// the Postgres variant, lib/blobfs/data/pgnative.
const (
	variantStandard = "standard"
	variantPGNative = "pgnative"
)

// variantNames lists the accepted names, in the order the help and the
// refusal print them.
var variantNames = []string{variantStandard, variantPGNative}

// storageEnvPrefix is the prefix the object store's configuration reads
// its environment under: BLOBFS_STORAGE_ENDPOINT, BLOBFS_STORAGE_CONTAINER,
// BLOBFS_STORAGE_ACCOUNT, and BLOBFS_STORAGE_KEY, and the optional limits
// the storage package documents. The store has no flag: it is opened by
// the first file command that needs it, and the directory commands never
// read these variables.
const storageEnvPrefix = "blobfs"

// bind registers the persistent flags on f. The defaults are left empty
// here rather than read from the environment, so New reads nothing outside
// the process; dsn and variant consult BLOBFS_DSN and BLOBFS_VARIANT when a
// constructor runs.
func (c *Config) bind(f *pflag.FlagSet) {
	f.StringVar(&c.DSN, "dsn", "", "the database's connection string (env BLOBFS_DSN)")
	f.StringVar(&c.Variant, "variant", "", "the blobfs variant the file commands run over: "+strings.Join(variantNames, " or ")+" (env BLOBFS_VARIANT; default "+variantStandard+")")
}

// variant returns the name of the variant to build the file store over:
// --variant when given, otherwise BLOBFS_VARIANT, otherwise the standard
// baseline. A name that is neither accepted one is refused with an error
// that lists both, before any I/O. It is called from the domain's store
// constructor during execution, after cobra has parsed the flags; the
// schema commands never ask, because the migrations are the same on every
// variant.
func (c *Config) variant() (string, error) {
	name := c.Variant
	if name == "" {
		name = os.Getenv("BLOBFS_VARIANT")
	}
	if name == "" {
		name = variantStandard
	}
	if !slices.Contains(variantNames, name) {
		return "", fmt.Errorf("unknown variant %q: --variant or BLOBFS_VARIANT is %s", name, strings.Join(variantNames, " or "))
	}
	return name, nil
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
