// Package database is the admin domain of the database service: schema
// state, verification, correction, seeding, and diagnostics as an admin
// service over lib/migrate, the session, and the data layer's functions
// (internal/data: the migration set, the seeder, the catalog), exposed as a
// route group under the admin mount and run once at startup. It owns the
// operations and their policy — when the seed runs — and none of the
// content it administers.
package database
