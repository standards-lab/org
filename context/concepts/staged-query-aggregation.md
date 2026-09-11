# Staged query aggregation

An unscheduled investigation, gated on two things that don't exist yet: the bounded, staged query
strategy (`design/auth-strategy.md` §7) actually built, and practical experience with how bounded
queries behave in production once it is.

`design/service-organization.md`'s runtime-composition rule composes data across services one stage at a
time, presentationally: a client (or a future composer) issues each stage's call itself, one authorized,
paged, and sorted result per service, per hop. That covers drill-down navigation and the authorization
case (a predicate crossing a boundary) completely. It does not cover a single logical request that wants
a live, on-demand composed view across several services at once — an analysis or reporting need that
would otherwise reach for the same rule's other named answer, a periodically-rebuilt materialized read
model, but wants current data rather than a stale snapshot.

The direction worth investigating once the gates above are cleared: an orchestration layer that composes
the staged hops of one logical request into a single response graph, with each hop still independently
authorized, filtered, and sorted within its own service — nothing about the authorization boundary at
each layer changes; only the composition of the results is centralized into one round trip for the
caller. This is a live-query composition mechanism, distinct from both presentational staging (many
client-issued calls) and the materialized-view answer (a stale, precomputed aggregate) that
`design/service-organization.md` already names for search and reporting.

Not designed here. What would need working out, once there's something to work it out against: how a
response graph's shape is declared, what "authorized, filtered, and sorted at each layer" means for a
result that spans several independently-paged collections, and whether this composes with or replaces
the materialized-view answer for the cases that need one.
