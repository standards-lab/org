package query

import (
	"context"
	"database/sql"

	"github.com/standards-lab/org/experiments/sql-dsl/lib/sqldb"
)

// Tier is a statement's declared portability.
type Tier string

const (
	// TierStandard is standard SQL: it runs on every engine the standard
	// covers with no port.
	TierStandard Tier = "standard"
	// TierNative uses one engine's reach; the native note names the reach
	// and the port.
	TierNative Tier = "native"
)

// Field is one entry of a projection base's field contract: the name a
// request may filter or sort by, and the SQL type a request value is cast
// to when it does, written as the engine reads it.
type Field struct {
	Name string
	Type string
}

// Args binds a statement's named parameters. A missing name is an
// ArgumentError; an extra name is ignored, so one map serves a guard's
// command and its narrower check.
type Args map[string]any

// Statement is one authored file, loaded: its text with the dialect's
// placeholders, its parameters in position order, and its header. It is a
// value, fetched by name once at wiring and held in a typed handle.
type Statement struct {
	name       string
	text       string
	tier       Tier
	native     string
	txRequired bool
	params     []string
	key        string
	fields     []Field
}

// Name is the file's base name without the .sql extension.
func (st Statement) Name() string { return st.name }

// Text is the SQL as the engine receives it: the file's body, parameters
// rewritten to the dialect's placeholders; the header is not sent.
func (st Statement) Text() string { return st.text }

// Tier is the declared tier.
func (st Statement) Tier() Tier { return st.tier }

// Native is the native note: the reach used and the port, for a native
// statement; empty for a standard one.
func (st Statement) Native() string { return st.native }

// TransactionRequired reports the "-- transaction: required" header.
func (st Statement) TransactionRequired() bool { return st.txRequired }

// Params returns the parameter names in position order.
func (st Statement) Params() []string {
	out := make([]string, len(st.params))
	copy(out, st.params)
	return out
}

// Key is the declared key of a projection base; empty otherwise.
func (st Statement) Key() string { return st.key }

// Fields returns the declared field contract, in header order.
func (st Statement) Fields() []Field {
	out := make([]Field, len(st.fields))
	copy(out, st.fields)
	return out
}

// Exec runs the statement and returns the rows affected.
func (st Statement) Exec(ctx context.Context, s sqldb.Session, args Args) (int64, error) {
	values, err := st.bind(s, args)
	if err != nil {
		return 0, err
	}
	res, err := s.ExecContext(ctx, st.text, values...)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return n, mapErr(s, err)
}

// query runs the statement for its rows.
func (st Statement) query(ctx context.Context, s sqldb.Session, args Args) (*sql.Rows, error) {
	values, err := st.bind(s, args)
	if err != nil {
		return nil, err
	}
	return s.QueryContext(ctx, st.text, values...)
}

// bind orders args by the statement's parameters and checks the session
// against the transaction requirement.
func (st Statement) bind(s sqldb.Session, args Args) ([]any, error) {
	if st.txRequired {
		if _, ok := s.(*sqldb.Tx); !ok {
			return nil, ErrTransactionRequired
		}
	}
	values := make([]any, len(st.params))
	for i, name := range st.params {
		v, ok := args[name]
		if !ok {
			return nil, &ArgumentError{Statement: st.name, Name: name}
		}
		values[i] = v
	}
	return values, nil
}

// mapErr routes an error that arose after a call returned — rows.Err, Scan,
// RowsAffected — through the session's mapper, the path the seam cannot
// cover itself.
func mapErr(s sqldb.Session, err error) error {
	if err == nil {
		return nil
	}
	if m, ok := s.(sqldb.ErrorMapper); ok {
		return m.MapError(err)
	}
	return err
}
