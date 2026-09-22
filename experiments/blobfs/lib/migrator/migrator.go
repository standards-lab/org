package migrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/migrate"
)

// defaultTable is the history table sqlate's migrate package uses when
// Options.Table is empty. migrate does not export the name, so the shim
// repeats it to check that two sets never share a table and to report the
// table in Status.
const defaultTable = "schema_version"

// defaultLockName is the outer lock's name when Options.LockName is empty,
// in sqlate's <owner>.<structure> form.
const defaultLockName = "migrator.sets"

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
	// LockName names the outer lock a run holds; default "migrator.sets".
	LockName string
	// Unlocked allows runs on a dialect without the lock capability, or with
	// it, without taking the outer lock; concurrent starters are then
	// unsafe. The inner migrators never lock, whatever this says.
	Unlocked bool
}

// SetStatus is one set's state as Status reads it: the set's name and
// history table, the highest applied version (zero when nothing is
// applied), the set's latest version, the migrations the history has not
// applied, and whether the head is dirty.
type SetStatus struct {
	Name    string
	Table   string
	Version int
	Latest  int
	Pending []migrate.Migration
	Dirty   bool
}

// SetError is an error from one set's run or check, naming the set. It
// unwraps to the inner migrator's error, so errors.Is classifies it
// (migrate.ErrDirty, migrate.ErrUnknownVersion) and errors.As reaches the
// inner error's version (*migrate.DirtyError).
type SetError struct {
	Set string
	Err error
}

func (e *SetError) Error() string { return fmt.Sprintf("migrator: set %q: %v", e.Set, e.Err) }

func (e *SetError) Unwrap() error { return e.Err }

// Migrator runs the declared sets in order under one lock.
type Migrator struct {
	db     *sqlate.DB
	sets   []runner
	opts   Options
	locker sqlate.Locker
}

// runner is one set with the migrate.Migrator built for it.
type runner struct {
	name     string
	table    string
	migrator *migrate.Migrator
}

// New validates the sets and builds one unlocked migrate.Migrator per set
// in the declared order. Every set has a name, no two sets share a name,
// and no two sets share a history table; migrate.New validates each set's
// migrations. The lock capability is taken from the dialect when it has
// it. New performs no I/O.
func New(db *sqlate.DB, sets []Set, opts Options) (*Migrator, error) {
	if db == nil {
		return nil, errors.New("migrator: nil db")
	}
	if len(sets) == 0 {
		return nil, errors.New("migrator: no sets")
	}
	if opts.LockName == "" {
		opts.LockName = defaultLockName
	}
	names := map[string]bool{}
	tables := map[string]string{}
	m := &Migrator{db: db, sets: make([]runner, 0, len(sets)), opts: opts}
	m.locker, _ = db.Dialect().(sqlate.Locker)
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
		inner, err := migrate.New(db, set.Migrations, migrate.Options{Table: set.Table, Unlocked: true, Logger: opts.Logger})
		if err != nil {
			return nil, fmt.Errorf("migrator: set %q: %w", set.Name, err)
		}
		m.sets = append(m.sets, runner{name: set.Name, table: table, migrator: inner})
	}
	return m, nil
}

// Up applies every pending migration of every set, sets in declared order,
// under the outer lock. Every set is checked first: a dirty set or a
// history that does not match its set refuses the run before any set
// runs. The first failure stops the run; the sets before it stay applied.
func (m *Migrator) Up(ctx context.Context) error {
	return m.locked(ctx, func(ctx context.Context, _ *sql.Conn) error {
		if err := m.check(ctx); err != nil {
			return err
		}
		for _, r := range m.sets {
			if err := r.migrator.Up(ctx); err != nil {
				return &SetError{Set: r.name, Err: err}
			}
		}
		return nil
	})
}

// Down reverts every applied migration of every set, sets in reverse
// declared order, under the outer lock, so the consumer's set is reverted
// before the sets it references. Every set is checked first, as in Up.
// The history tables stay, empty. The first failure stops the run.
func (m *Migrator) Down(ctx context.Context) error {
	return m.locked(ctx, func(ctx context.Context, _ *sql.Conn) error {
		if err := m.check(ctx); err != nil {
			return err
		}
		return m.revertAll(ctx, nil)
	})
}

// Reset returns the database to its state before the first Up: it reverts
// every set as Down does and drops each set's history table once the set
// is reverted, in the same reverse order. Every set is checked first, as
// in Up, so a dirty set refuses the reset; Force repairs the set's history
// once its schema is fixed by hand. A later Up replays every set from
// zero.
func (m *Migrator) Reset(ctx context.Context) error {
	return m.locked(ctx, func(ctx context.Context, conn *sql.Conn) error {
		if err := m.check(ctx); err != nil {
			return err
		}
		return m.revertAll(ctx, func(r runner) error {
			// migrate has no operation that removes its history table, so
			// the shim drops it on the pinned connection.
			if _, err := conn.ExecContext(ctx, "DROP TABLE "+r.table); err != nil {
				return m.db.MapError(err)
			}
			m.log("history table dropped", "set", r.name, "table", r.table)
			return nil
		})
	})
}

// revertAll reverts every set in reverse declared order and runs after,
// when not nil, on each set once it is reverted.
func (m *Migrator) revertAll(ctx context.Context, after func(runner) error) error {
	for _, r := range slices.Backward(m.sets) {
		// Steps tolerates a count larger than the applied prefix, so the
		// whole set is the count to revert everything.
		if err := r.migrator.Down(ctx, len(r.migrator.Migrations())); err != nil {
			return &SetError{Set: r.name, Err: err}
		}
		if after != nil {
			if err := after(r); err != nil {
				return &SetError{Set: r.name, Err: err}
			}
		}
	}
	return nil
}

// Status reads every set's state in declared order. It takes no lock and
// runs on the pool, so it never waits on a running Up, and a report read
// while a run is in progress can show a set mid-run. A history row the set
// does not contain is a SetError wrapping migrate.ErrUnknownVersion.
func (m *Migrator) Status(ctx context.Context) ([]SetStatus, error) {
	out := make([]SetStatus, 0, len(m.sets))
	for _, r := range m.sets {
		head, err := r.migrator.Version(ctx)
		if err != nil {
			return nil, &SetError{Set: r.name, Err: err}
		}
		// Verify reports pending migrations and a dirty head as errors of
		// their own classes; both are state Status reports, not failures.
		if err := r.migrator.Verify(ctx); err != nil && !errors.Is(err, migrate.ErrPending) && !errors.Is(err, migrate.ErrDirty) {
			return nil, &SetError{Set: r.name, Err: err}
		}
		s := SetStatus{Name: r.name, Table: r.table, Version: head.Version, Dirty: head.Dirty}
		for _, mig := range r.migrator.Migrations() {
			s.Latest = mig.Version
			if mig.Version > head.Version {
				s.Pending = append(s.Pending, mig)
			}
		}
		out = append(out, s)
	}
	return out, nil
}

// Force sets one set's history to version as an operator override, under
// the outer lock: rows above it are deleted, its row is inserted if absent
// and marked clean, and version 0 empties the history. Nothing runs against
// the schema itself. It is the repair for a dirty set, once the failed
// migration's objects are fixed by hand: Force to the version that is
// applied, then Up or Reset. A set name the migrator does not hold is an
// error.
func (m *Migrator) Force(ctx context.Context, set string, version int) error {
	for _, r := range m.sets {
		if r.name != set {
			continue
		}
		return m.locked(ctx, func(ctx context.Context, _ *sql.Conn) error {
			if err := r.migrator.Force(ctx, version); err != nil {
				return &SetError{Set: r.name, Err: err}
			}
			return nil
		})
	}
	return fmt.Errorf("migrator: no set %q", set)
}

// check verifies every set's history before a run: a dirty set or a
// history that does not match its set is a SetError; pending migrations
// are not an error.
func (m *Migrator) check(ctx context.Context) error {
	for _, r := range m.sets {
		err := r.migrator.Verify(ctx)
		if err == nil || errors.Is(err, migrate.ErrPending) {
			continue
		}
		return &SetError{Set: r.name, Err: err}
	}
	return nil
}

// locked pins a connection, takes the outer lock on it when the dialect
// has the capability, runs fn, and releases the lock under a context that
// survives cancellation before the connection returns to the pool. Once
// ctx has ended the driver discards the connection and the session's end
// releases the lock, so an unlock failure then is not reported. A dialect
// without the capability is migrate.ErrNoLocker unless Options.Unlocked.
func (m *Migrator) locked(ctx context.Context, fn func(context.Context, *sql.Conn) error) (err error) {
	if m.locker == nil && !m.opts.Unlocked {
		return migrate.ErrNoLocker
	}
	conn, err := m.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil && err == nil {
			err = m.db.MapError(cerr)
		}
	}()
	if m.locker != nil && !m.opts.Unlocked {
		if err := m.locker.Lock(ctx, conn, m.opts.LockName); err != nil {
			return m.db.MapError(err)
		}
		defer func() {
			if uerr := m.locker.Unlock(context.WithoutCancel(ctx), conn, m.opts.LockName); uerr != nil && ctx.Err() == nil {
				err = errors.Join(err, m.db.MapError(uerr))
			}
		}()
	}
	return fn(ctx, conn)
}

func (m *Migrator) log(msg string, args ...any) {
	if m.opts.Logger != nil {
		m.opts.Logger.Info(msg, args...)
	}
}
