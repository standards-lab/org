package app

import (
	"github.com/standards-lab/go-core/lifecycle"
	"github.com/standards-lab/go-web-sdk"
	"github.com/standards-lab/org/experiments/sql-dsl/admin/database"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/config"
)

// Admin composes the administrative services, one field per admin domain.
type Admin struct {
	Database *database.Service
}

// newAdmin wires the admin layer over infra, each admin service handed its
// switches from cfg at the construction site. It takes lc because an admin
// service owns a lifecycle stage: the database admin service verifies and
// corrects the schema at stage 1, ahead of the domains that verify their
// statements.
func newAdmin(infra *Infrastructure, cfg *config.Config, lc *lifecycle.Coordinator) (*Admin, error) {
	db, err := database.New(infra.Pool, infra.SQL, infra.Logger, database.Options{Seed: cfg.Admin.SeedEnabled()})
	if err != nil {
		return nil, err
	}
	db.Register(lc)
	return &Admin{Database: db}, nil
}

// mountAdmin builds the admin mount, /admin, with each admin domain's route
// group mounted into it. Its isolation — own listener, authentication,
// audit — is the production constraint the spike leaves to the startup task.
func mountAdmin(adm *Admin) *web.Group {
	admin := web.NewGroup("/admin")
	admin.Mount(database.Routes(adm.Database))
	return admin
}
