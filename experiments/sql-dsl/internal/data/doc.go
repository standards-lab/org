// Package data is the service's database infrastructure: what the domains
// take from the database, and the content the admin service administers.
// Database groups the session every statement runs through with the
// pattern catalog the statements compile against, the dialect reachable
// through the session; neither library may own the grouping — the query
// library must not know the pool, the provider library must not know
// patterns — and the composition root cannot, since the domains would
// import it. The content is the schema (migrations/), the application's
// pattern namespace (patterns/), and the seed files with their statements
// (seeds/, statements/), each behind a function the admin service triggers:
// Migrations, Patterns, and a Seeder. Domains and the admin service are
// peers over this package; nothing here imports either. The template
// scaffolds it.
package data
