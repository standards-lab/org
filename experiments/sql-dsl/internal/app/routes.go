package app

import (
	"github.com/standards-lab/go-web-sdk"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/config"
)

// routes composes the two modules: the API mount and the admin mount.
func routes(dom *Domain, adm *Admin, cfg *config.Config) []*web.Module {
	return []*web.Module{
		web.NewModule(mountAPI(dom, cfg)),
		web.NewModule(mountAdmin(adm)),
	}
}
