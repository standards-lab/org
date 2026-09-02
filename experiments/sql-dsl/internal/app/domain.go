package app

import (
	"github.com/standards-lab/go-web-sdk"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/config"
)

// Domain composes the application's domain services, one field per domain
// layer. The two domains land with their stages.
type Domain struct{}

// newDomain wires the domain layer over infra: each domain package's
// service is constructed here from the infrastructure fields it uses, never
// the Infrastructure struct itself.
func newDomain(infra *Infrastructure) *Domain {
	return &Domain{}
}

// mountAPI builds the API mount, /api, with each domain layer's route group
// mounted into it and each handler handed its policy from cfg at the
// construction site.
func mountAPI(dom *Domain, cfg *config.Config) *web.Group {
	api := web.NewGroup("/api")
	return api
}
