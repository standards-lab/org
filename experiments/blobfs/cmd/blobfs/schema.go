package main

import (
	"database/sql"
	"log/slog"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/postgres"

	consumer "github.com/standards-lab/org/experiments/blobfs/consumer/migrations"
	"github.com/standards-lab/org/experiments/blobfs/lib/blobfs/migrations"
	"github.com/standards-lab/org/experiments/blobfs/lib/migrator"
)

// Database is the program's database layer: the session over the pool.
type Database struct {
	DB   *sqlate.DB
	pool *sql.DB
}

// openDatabase opens the pool with the pgx driver and wraps it with the
// Postgres dialect.
func openDatabase(dsn string) (*Database, error) {
	pool, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	return &Database{DB: sqlate.Wrap(pool, postgres.Dialect{}), pool: pool}, nil
}

// Close closes the pool.
func (d *Database) Close() error { return d.pool.Close() }

// newSchema builds the migrator over the two sets in canonical order:
// blobfs's set under its own history table first, the consumer's set under
// sqlate's default table last.
func newSchema(db *sqlate.DB, logger *slog.Logger) (*migrator.Migrator, error) {
	blobfsSet, err := migrations.Migrations(db.Dialect())
	if err != nil {
		return nil, err
	}
	consumerSet, err := consumer.Migrations()
	if err != nil {
		return nil, err
	}
	return migrator.New(db, []migrator.Set{
		{Name: migrations.Source, Table: migrations.Table, Migrations: blobfsSet},
		{Name: "consumer", Migrations: consumerSet},
	}, migrator.Options{Logger: logger})
}
