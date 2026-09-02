package query

import (
	"errors"
	"fmt"
)

// ErrTransactionRequired reports a statement headed "-- transaction:
// required" run on a session that is not a transaction: the one silent
// hazard, a transaction-scoped lock outside one, made loud.
var ErrTransactionRequired = errors.New("query: statement requires a transaction")

// ArgumentError reports a parameter the Args did not carry. It is a
// programming error in the caller, not request input, and matches no
// request sentinel.
type ArgumentError struct {
	Statement string
	Name      string
}

func (e *ArgumentError) Error() string {
	return fmt.Sprintf("query: %s: missing argument %q", e.Statement, e.Name)
}
