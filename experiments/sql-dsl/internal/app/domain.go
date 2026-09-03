package app

import (
	"github.com/standards-lab/go-core/lifecycle"
	"github.com/standards-lab/go-web-sdk"
	"github.com/standards-lab/org/experiments/sql-dsl/domain/organization"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/config"
)

// Domain composes the application's domain services, one field per domain
// layer.
type Domain struct {
	Organization *organization.Service
}

// newDomain wires the domain layer over infra: each domain package's
// service is constructed here from the infrastructure fields it uses, never
// the Infrastructure struct itself, and registers its startup verification
// on lc at the domains' stage.
func newDomain(infra *Infrastructure, lc *lifecycle.Coordinator) *Domain {
	org := organization.New(infra.SQL)
	org.Register(lc)
	return &Domain{Organization: org}
}

// mountAPI builds the API mount, /api, with each domain layer's route group
// mounted into it and each handler handed its policy from cfg at the
// construction site.
func mountAPI(dom *Domain, cfg *config.Config) *web.Group {
	api := web.NewGroup("/api")
	api.Mount(organization.Routes(dom.Organization, cfg.Reads.Limits()))
	return api
}
