// Package admin is the administrative layer: the admin domains, one per
// infrastructure service that needs administering, each an admin service
// over the library mechanisms it triggers and a route group the admin mount
// serves beside the API mount. The vocabulary, proposed by the
// v1.data.sql.prototype experiment:
//
//   - admin mount — the /admin route group, the administrative counterpart
//     of the /api mount;
//   - admin domain — a package under admin/ named for the infrastructure
//     service it administers (database today);
//   - admin service — the domain's service: its operations are triggers over
//     library functions and the infrastructure layer's own (for the
//     database, lib/migrate and internal/data's migration set, seeder, and
//     catalog), never mechanisms of their own; it administers the
//     infrastructure the data layer owns, and startup calls the same
//     functions its endpoints do;
//   - admin handler — the domain's route group, mounted into the admin mount.
//
// The composition root (internal/app) wires the admin domains and mounts
// them. The admin mount's isolation — its own listener, authentication,
// audit — is the strategy's production constraint and is not built in the
// spike.
package admin
