package drivertest

import (
	"errors"
	"strconv"
)

// Dialect is the stub dialect: $N placeholders and a MapError that wraps
// every non-nil error in a *MappedError, so a test proves an error crossed
// the mapping boundary with one errors.As.
type Dialect struct{}

func (Dialect) Name() string { return "test" }

func (Dialect) Placeholder(n int) string { return "$" + strconv.Itoa(n) }

// MapError wraps err in a *MappedError; nil stays nil. An already mapped
// error is returned as is, so double mapping is visible as absence of
// nesting rather than a crash.
func (Dialect) MapError(err error) error {
	if err == nil {
		return nil
	}
	var m *MappedError
	if errors.As(err, &m) {
		return err
	}
	return &MappedError{Err: err}
}

// MappedError marks an error that passed through Dialect.MapError.
type MappedError struct{ Err error }

func (e *MappedError) Error() string { return "mapped: " + e.Err.Error() }

func (e *MappedError) Unwrap() error { return e.Err }
