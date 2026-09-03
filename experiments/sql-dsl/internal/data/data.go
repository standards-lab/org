package data

import (
	"github.com/standards-lab/org/experiments/sql-dsl/lib/query"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

// Database is the database as a domain sees it: the session, and the
// catalog of every pattern namespace the composition root registered. A
// domain compiles its statements against Catalog and runs them through
// DB; it defines no patterns of its own.
type Database struct {
	*sqldb.DB
	Catalog *query.Catalog
}

// New groups a session with the catalog its statements compile against.
func New(db *sqldb.DB, catalog *query.Catalog) *Database {
	return &Database{DB: db, Catalog: catalog}
}
