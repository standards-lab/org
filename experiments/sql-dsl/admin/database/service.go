package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/standards-lab/go-core/lifecycle"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/migrate"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

// Stage is the lifecycle stage the schema correction runs in: after the
// pool (0), before the domains verify their statements (2).
const Stage = 1

// Service is the database admin service. Every operation is a trigger over
// the migrator or the session; Start runs the same functions the endpoints
// do. Ready reports a clean, complete schema and follows every operation.
type Service struct {
	db       *sqldb.DB
	migrator *migrate.Migrator
	logger   *slog.Logger
	ready    atomic.Bool
}

// New builds the service over db and the embedded migration set.
func New(db *sqldb.DB, logger *slog.Logger) (*Service, error) {
	m, err := migrate.New(db, Migrations(), migrate.Options{Logger: logger})
	if err != nil {
		return nil, fmt.Errorf("admin/database: %w", err)
	}
	return &Service{db: db, migrator: m, logger: logger}, nil
}

// Register declares the schema stage on lc: Start corrects the schema and
// Ready gates readiness on it.
func (s *Service) Register(lc *lifecycle.Coordinator) {
	lc.Add(lifecycle.Service{
		Name:  "schema",
		Stage: Stage,
		Start: s.Start,
		Check: s,
	})
}

// Ready reports whether the history is the embedded set's clean head, as of
// the last operation.
func (s *Service) Ready() bool { return s.ready.Load() }

// Start attempts to bring the schema to the embedded set's head: a pending
// history is applied under the lock; a clean, complete one passes. A state
// the mechanism cannot correct — a dirty row, a history the set does not
// carry — fails startup; an operator resolves it through the admin
// endpoints (force, then up) on a process started against a corrected
// database, or from another replica.
func (s *Service) Start(ctx context.Context) error {
	err := s.migrator.Verify(ctx)
	if pending, ok := errors.AsType[*migrate.PendingError](err); ok {
		s.logger.Info("schema pending; applying", "versions", pending.Versions)
		if err := s.migrator.Up(ctx); err != nil {
			return fmt.Errorf("apply: %w", err)
		}
		err = s.migrator.Verify(ctx)
	}
	if err != nil {
		return err // the lifecycle prefixes the service name
	}
	s.ready.Store(true)
	v, err := s.migrator.Version(ctx)
	if err != nil {
		return err
	}
	s.logger.Info("schema current", "version", v.Version)
	return nil
}

// Diagnose reads the database's health. The version query is the one
// native-tier read here; every engine spells it differently.
func (s *Service) Diagnose(ctx context.Context) (Diagnostics, error) {
	base := s.db.Base()
	d := Diagnostics{Dialect: base.Dialect().Name()}
	start := time.Now()
	if err := base.Ping(ctx); err != nil {
		return d, fmt.Errorf("ping: %w", err)
	}
	d.Ping = time.Since(start)
	rows, err := s.db.QueryContext(ctx, "SELECT version()")
	if err != nil {
		return d, fmt.Errorf("server version: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		if err := rows.Scan(&d.ServerVersion); err != nil {
			return d, s.db.MapError(err)
		}
	}
	if err := rows.Err(); err != nil {
		return d, s.db.MapError(err)
	}
	st := base.Conn().Stats()
	d.Pool = Pool{
		Open: st.OpenConnections, InUse: st.InUse, Idle: st.Idle, MaxOpen: st.MaxOpenConnections,
		WaitCount: st.WaitCount, WaitDuration: st.WaitDuration,
	}
	return d, nil
}

// Status reads the schema's state and refreshes Ready from it.
func (s *Service) Status(ctx context.Context) (Status, error) {
	v, err := s.migrator.Version(ctx)
	if err != nil {
		return Status{}, err
	}
	st := Status{Version: v.Version, Dirty: v.Dirty, Pending: []int{}}
	verr := s.migrator.Verify(ctx)
	if pending, ok := errors.AsType[*migrate.PendingError](verr); ok {
		st.Pending = pending.Versions
	}
	st.Ready = verr == nil
	s.ready.Store(st.Ready)
	for _, m := range s.migrator.Migrations() {
		st.Migrations = append(st.Migrations, MigrationInfo{
			Version: m.Version, Name: m.Name, Transactional: m.Transactional,
			Applied: m.Version < v.Version || (m.Version == v.Version && !v.Dirty),
		})
	}
	return st, nil
}

// Verify checks the history is the set's clean head; the error names what
// is wrong. Ready follows the result.
func (s *Service) Verify(ctx context.Context) error {
	err := s.migrator.Verify(ctx)
	s.ready.Store(err == nil)
	return err
}

// Up applies every pending migration and returns the resulting state.
func (s *Service) Up(ctx context.Context) (Status, error) {
	return s.after(ctx, s.migrator.Up(ctx))
}

// Down reverts the n most recent migrations; n must be positive.
func (s *Service) Down(ctx context.Context, n int) (Status, error) {
	if n <= 0 {
		return Status{}, fmt.Errorf("%w: steps must be positive", ErrValidation)
	}
	return s.after(ctx, s.migrator.Down(ctx, n))
}

// Steps applies n pending (n > 0) or reverts -n applied (n < 0).
func (s *Service) Steps(ctx context.Context, n int) (Status, error) {
	if n == 0 {
		return Status{}, fmt.Errorf("%w: steps must be non-zero", ErrValidation)
	}
	return s.after(ctx, s.migrator.Steps(ctx, n))
}

// Force sets the history to version, clearing dirty state; 0 empties it. It
// never touches the schema: it exists to clear a dirty row after the
// operator has repaired the schema by hand, and it can just as well
// manufacture one — a forced-down history re-applies files against objects
// that still exist.
func (s *Service) Force(ctx context.Context, version int) (Status, error) {
	if version < 0 {
		return Status{}, fmt.Errorf("%w: version must not be negative", ErrValidation)
	}
	return s.after(ctx, s.migrator.Force(ctx, version))
}

// after returns the state following a mutating operation, or its error.
func (s *Service) after(ctx context.Context, err error) (Status, error) {
	if err != nil {
		if st, serr := s.Status(ctx); serr == nil {
			_ = st // refreshes Ready; the operation's error is the answer
		}
		return Status{}, err
	}
	return s.Status(ctx)
}
