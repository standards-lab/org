// Package data is what a domain needs from the database, grouped: the
// session every statement runs through and the pattern catalog its
// statements compile against, the dialect reachable through the session.
// Neither library may own the grouping — the query library must not know
// the pool, the database library must not know patterns — and the
// composition root cannot, since the domains would import it; so the
// service holds it here, builds it in newInfrastructure, and hands it to
// every domain's and admin service's New. The template scaffolds it.
package data
