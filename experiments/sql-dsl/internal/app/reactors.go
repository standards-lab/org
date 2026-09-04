package app

import (
	"github.com/standards-lab/go-core/lifecycle"
)

// Reactors composes the application's event-driven entry points; none yet.
type Reactors struct{}

// newReactors constructs the reactors and registers each on lc. It takes
// infra for the transport connections a reactor owns and dom for the domain
// calls it dispatches to — the two halves a reactor joins.
func newReactors(
	infra *Infrastructure,
	dom *Domain,
	lc *lifecycle.Coordinator,
) (*Reactors, error) {
	return &Reactors{}, nil
}
