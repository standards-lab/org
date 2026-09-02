// Package database is the admin domain of the database service: schema
// state, verification, correction, seeding, and diagnostics as an admin
// service over lib/migrate and the session, exposed as a route group under
// the admin mount and run once at startup. It owns the embedded migration
// set and the seed files.
package database
