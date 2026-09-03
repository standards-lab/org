package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"

	"github.com/standards-lab/go-database"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

// Guard is the optimistic-concurrency protocol over two authored
// statements: the command, whose WHERE names the key and the expected
// version and whose SET advances it, and the check, which reads the row's
// current version by key. The SQL is the consumer's; the guard is the
// function over it.
type Guard struct {
	command Statement
	check   Statement
	version string
}

// Guarded binds a command and its check; version names the parameter both
// statements bind the expected version to.
func Guarded(command, check Statement, version string) Guard {
	return Guard{command: command, check: check, version: version}
}

// Run executes the command with version bound under the guard's parameter
// name alongside args. A row affected is success and the new version,
// version+1, with no second round trip. No row affected runs the check with
// the same args: no row is sql.ErrNoRows, a row is database.ErrVersionMismatch
// carrying the expected and current versions.
func (g Guard) Run(ctx context.Context, s sqldb.Session, version int64, args Args) (int64, error) {
	bound := make(Args, len(args)+1)
	maps.Copy(bound, args)
	bound[g.version] = version
	n, err := g.command.Exec(ctx, s, bound)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		return version + 1, nil
	}
	current, err := Scan(g.check, Scalar[int64]).One(ctx, s, bound)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, sql.ErrNoRows
	}
	if err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("%w: expected %d, current %d", database.ErrVersionMismatch, version, current)
}
