package app

import (
	"database/sql"
	"io"
	"log/slog"

	_ "github.com/jackc/pgx/v5/stdlib" // the driver sql.Open resolves "pgx" to
	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/postgres"
)

// Infrastructure holds what the commands are composed on: the database and
// the logger. The database binds to the DSN, and that is a persistent flag
// cobra parses during execution, after the tree is built, so the struct
// holds the config and opens the pool on demand through Database, once per
// run, and closes it in Close. The struct stops at the composition root:
// the layer files close their own client constructors over it, and a
// package receives the constructor as a parameter, never the struct itself.
// This is the one file that names the pgx driver and the Postgres dialect.
type Infrastructure struct {
	cfg    *Config
	logger *slog.Logger
	pool   *sql.DB
	db     *sqlate.DB
}

// newInfrastructure constructs the infrastructure over cfg, logging to
// stderr. Construction opens nothing and reads no flag: cfg is populated
// later, when cobra parses.
func newInfrastructure(cfg *Config, stderr io.Writer) *Infrastructure {
	return &Infrastructure{
		cfg:    cfg,
		logger: slog.New(slog.NewTextHandler(stderr, nil)),
	}
}

// Database returns the session over the pool, opening the pool on the
// first call with the DSN as resolved: the pgx driver under the Postgres
// dialect. sql.Open connects to nothing, so a wrong DSN surfaces at the
// first statement, while a missing one fails here. It is called from a
// subcommand's RunE through a layer's constructor, never at construction.
func (i *Infrastructure) Database() (*sqlate.DB, error) {
	if i.db != nil {
		return i.db, nil
	}
	dsn, err := i.cfg.dsn()
	if err != nil {
		return nil, err
	}
	pool, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	i.pool = pool
	i.db = sqlate.Wrap(pool, postgres.Dialect{})
	return i.db, nil
}

// Logger returns the logger the layers record through: text lines on
// stderr.
func (i *Infrastructure) Logger() *slog.Logger {
	return i.logger
}

// Close closes the pool when Database opened one. It is safe to call when
// nothing was opened, so App.Run calls it unconditionally.
func (i *Infrastructure) Close() error {
	if i.pool == nil {
		return nil
	}
	pool := i.pool
	i.pool, i.db = nil, nil
	return pool.Close()
}
