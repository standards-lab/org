package schema

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"time"

	"github.com/standards-lab/go-database"
)

// Diagnostics is one read of the database's health: the dialect the pool
// speaks, the round trip of a ping, the engine's version banner, and the
// pool's counters.
type Diagnostics struct {
	Dialect       string
	Ping          time.Duration
	ServerVersion string
	Pool          sql.DBStats
}

// Diagnose reads Diagnostics from a started db. The version query is the
// one native-tier read here; every engine spells it differently.
func Diagnose(ctx context.Context, db *database.DB) (Diagnostics, error) {
	d := Diagnostics{Dialect: db.Dialect().Name()}

	start := time.Now()
	if err := db.Ping(ctx); err != nil {
		return d, fmt.Errorf("ping: %w", err)
	}
	d.Ping = time.Since(start)

	if err := db.QueryRowContext(ctx, "SELECT version()").Scan(&d.ServerVersion); err != nil {
		return d, fmt.Errorf("server version: %w", err)
	}

	d.Pool = db.Conn().Stats()
	return d, nil
}

// Write renders the diagnostics as key/value lines.
func (d Diagnostics) Write(w io.Writer) {
	_, _ = fmt.Fprintf(w, "dialect:        %s\n", d.Dialect)
	_, _ = fmt.Fprintf(w, "ping:           %s\n", d.Ping)
	_, _ = fmt.Fprintf(w, "server version: %s\n", d.ServerVersion)
	_, _ = fmt.Fprintf(w, "pool:           open=%d in_use=%d idle=%d max_open=%d wait_count=%d wait=%s\n",
		d.Pool.OpenConnections, d.Pool.InUse, d.Pool.Idle, d.Pool.MaxOpenConnections,
		d.Pool.WaitCount, d.Pool.WaitDuration)
}
