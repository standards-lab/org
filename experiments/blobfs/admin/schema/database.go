package schema

import (
	"context"
	"log/slog"

	"github.com/standards-lab/sqlate"

	blobfsmigrations "github.com/standards-lab/org/experiments/blobfs/lib/blobfs/migrations"
	"github.com/standards-lab/org/experiments/blobfs/lib/migrator"
	appmigrations "github.com/standards-lab/org/experiments/blobfs/migrations"
)

// ConsumerSet is the name of the consumer's migration set in the migrator
// and in its log lines. blobfs's set carries the name its source exports.
const ConsumerSet = "consumer"

// Sets returns the two migration sets in canonical order: blobfs's set
// first, under its own history table, and the consumer's set last, under
// sqlate's default table. A dialect blobfs ships no DDL for is refused with
// the source's ErrUnsupportedEngine.
func Sets(dialect sqlate.Dialect) ([]migrator.Set, error) {
	blobfsSet, err := blobfsmigrations.Migrations(dialect)
	if err != nil {
		return nil, err
	}
	consumerSet, err := appmigrations.Migrations()
	if err != nil {
		return nil, err
	}
	return []migrator.Set{
		{Name: blobfsmigrations.Source, Table: blobfsmigrations.Table, Migrations: blobfsSet},
		{Name: ConsumerSet, Migrations: consumerSet},
	}, nil
}

// Client runs the schema operations over the multi-set migrator.
type Client struct {
	migrator *migrator.Migrator
}

// NewClient builds the migrator over db for the sets Sets returns, logging
// each applied and reverted migration to logger; a nil logger is silent.
// It performs no I/O: the migrator validates the sets and opens nothing.
func NewClient(db *sqlate.DB, logger *slog.Logger) (*Client, error) {
	sets, err := Sets(db.Dialect())
	if err != nil {
		return nil, err
	}
	m, err := migrator.New(db, sets, migrator.Options{Logger: logger})
	if err != nil {
		return nil, err
	}
	return &Client{migrator: m}, nil
}

// Up applies every pending migration of both sets, blobfs's set first.
func (c *Client) Up(ctx context.Context) error {
	return c.migrator.Up(ctx)
}

// Down reverts every applied migration of both sets, the consumer's set
// first, so its foreign keys into blobfs's tables never block the revert.
func (c *Client) Down(ctx context.Context) error {
	return c.migrator.Down(ctx)
}
