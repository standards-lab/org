// Package migrator runs several migration sets against one database, each
// set under its own history table, in the order the consumer declares them.
// A set is a migration source's name, its history table, and its
// migrations; a consumer declares blobfs's set first and its own set last,
// so blobfs's schema is at its head before the consumer's migrations
// reference it, and reverting runs the sets in reverse, so a consumer's
// foreign key into a blobfs table never blocks blobfs's down.
//
// This is the first form of the shim over published sqlate v0.1.1: one
// migrate.Migrator per set, each with sqlate's own per-table lock. The
// stage that finishes the shim replaces the per-set locks with one outer
// lock over the whole run and adds Status, Reset, and Verify. The package
// imports only sqlate and the standard library, so its code can move into
// sqlate.
package migrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/migrate"
)

// defaultTable is the history table sqlate's migrate package uses when
// Options.Table is empty. migrate does not export the name, so the shim
// repeats it to check that two sets never share a table.
const defaultTable = "schema_version"

// Set is one migration source: its name, the history table its migrations
// are recorded in (empty for sqlate's default), and its migrations in
// version order.
type Set struct {
	Name       string
	Table      string
	Migrations []migrate.Migration
}

// Options configures a Migrator.
type Options struct {
	// Logger records each applied and reverted migration; nil is silent.
	Logger *slog.Logger
}

// Migrator runs the declared sets in order.
type Migrator struct {
	sets []runner
}

// runner is one set with the migrate.Migrator built for it.
type runner struct {
	name     string
	migrator *migrate.Migrator
}

// New validates the sets and builds one migrate.Migrator per set in the
// declared order. Every set has a name, no two sets share a name, and no
// two sets share a history table; migrate.New validates each set's
// migrations. It performs no I/O.
func New(db *sqlate.DB, sets []Set, opts Options) (*Migrator, error) {
	if len(sets) == 0 {
		return nil, errors.New("migrator: no sets")
	}
	names := map[string]bool{}
	tables := map[string]string{}
	m := &Migrator{sets: make([]runner, 0, len(sets))}
	for _, set := range sets {
		if set.Name == "" {
			return nil, errors.New("migrator: a set has no name")
		}
		if names[set.Name] {
			return nil, fmt.Errorf("migrator: set %q is declared twice", set.Name)
		}
		names[set.Name] = true
		table := set.Table
		if table == "" {
			table = defaultTable
		}
		if other, ok := tables[table]; ok {
			return nil, fmt.Errorf("migrator: sets %q and %q share the history table %q", other, set.Name, table)
		}
		tables[table] = set.Name
		// Each inner migrator takes sqlate's default lock, migrate.<table>,
		// on its own pinned connection, so the sets of one run are locked
		// one after another and never together. The finished shim takes one
		// outer lock for the whole run and sets Unlocked on every inner
		// migrator.
		inner, err := migrate.New(db, set.Migrations, migrate.Options{Table: set.Table, Logger: opts.Logger})
		if err != nil {
			return nil, fmt.Errorf("migrator: set %q: %w", set.Name, err)
		}
		m.sets = append(m.sets, runner{name: set.Name, migrator: inner})
	}
	return m, nil
}

// Up applies every pending migration of every set, sets in declared order.
// The first failure stops the run; the sets before it stay applied.
func (m *Migrator) Up(ctx context.Context) error {
	for _, r := range m.sets {
		if err := r.migrator.Up(ctx); err != nil {
			return fmt.Errorf("migrator: set %q: %w", r.name, err)
		}
	}
	return nil
}

// Down reverts every applied migration of every set, sets in reverse
// declared order, so the consumer's set is reverted before the sets it
// references. The first failure stops the run.
func (m *Migrator) Down(ctx context.Context) error {
	for i := len(m.sets) - 1; i >= 0; i-- {
		r := m.sets[i]
		// Steps tolerates a count larger than the applied prefix, so the
		// whole set is the count to revert everything.
		if err := r.migrator.Down(ctx, len(r.migrator.Migrations())); err != nil {
			return fmt.Errorf("migrator: set %q: %w", r.name, err)
		}
	}
	return nil
}
