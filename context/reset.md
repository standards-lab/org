# reset · service-showcase

- **Status:** closeout
- **Session:** plan
- **Project:** go-web-service, standards-lab
- **Branch:** service-showcase

## Disposition

- **Cross-repo:** `standards-lab/context/roadmap.toml` — restructured the flat
  `backlog.service-showcase` entry into a root goal, `goals.slab`, sibling to `v1` rather than
  nested under it: nesting under `v1` would bind it into `v1.0`'s own closing criteria,
  contradicting its standing claim of not being a v1 layer dependency. Two tasks match the two
  `start` sessions settled here — `tasks.mechanism` and `tasks.surface`. `next` updated to
  `slab.mechanism`, `slab.surface` ahead of `v1.middleware`, same priority and reasoning as the
  entry it replaces.

Sharpened `go-web-service/context/concepts/service-showcase.md`: settled `slab`, a `cobra`-based
CLI in its own module at `go-web-service/tools/slab` — the `sqlate/sqlint` shape, kept out of
`cmd/server`'s own `go.mod` rather than a dependency of it — against the two directions the note
had left open (a narrated `mise run demo` script, an OpenAPI-plus-explorer-UI). Rejected along
the way: a `demo:<capability>:<scenario>` `mise` task per repo (no singular engagement point, and
`mise` isn't shaped for a process that doesn't return); a standalone new repository (unneeded
release/CI surface, when `go-web-service` already converges everything worth demoing). The
settled mechanism — a scenario as an ordered sequence of intent/action/observation steps — covers
all three signal shapes the note flagged (request/response, the observability side-channel, a
long-running background action for live Grafana traffic) without a redesign once `v1.messaging`
lands, since a reactor signal is only a fourth observation channel. `cobra` is adopted for the
command tree, evaluated against `dependency-sourcing.md`'s markers rather than hand-rolled.

## Next-focus

A `start` session in `go-web-service`, advancing `slab.mechanism`: stand up `tools/slab` as its
own `cobra`-based module and build one scenario per novel shape — `sqlate:compile` (a library
import, no database), one `go-web-service` request paired with its trace (the observability
side-channel), and the traffic generator (a long-running background action). Settle, as part of
that build, whether `sqlate` needs a small exported inspection hook for the compile demo's most
interesting intermediate artifact (the post-splice, pre-placeholder-rewrite SQL body behind the
unexported `Catalog.expand`, `sqlate/query/patterns.go:346`), or whether `slab` narrates from the
already-public surface instead. `goals.slab.tasks.surface` (full organization CRUD, the admin
migrate/verify/seed/state walk) follows once this proves out.
