package app

import (
	"github.com/standards-lab/go-web-sdk"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/config"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/domain"
)

// routes composes the API module: the one /api group, which an application
// built from the template mounts its domain-service route groups into, each
// constructor drawing its dependencies from dom and each handler handed its
// policy from cfg at the construction site. The template ships the group
// initialized and empty.
func routes(dom *domain.Domain, cfg *config.Config) []*web.Module {
	api := web.NewGroup("/api")
	return []*web.Module{web.NewModule(api)}
}
