# reset · v1-resequencing

- **Status:** closeout
- **Session:** plan
- **Project:** standards-lab
- **Branch:** v1-resequencing

## Disposition

- **Cross-repo:** `standards-lab/context/roadmap.toml` — `next` resequenced so the infrastructure
  libraries (storage, messaging, AI) prove out before auth: `v1.auth` and `v1.admin-listener` move
  from immediately after `v1.storage` to immediately before `v1.client`, and `v1.data` moves ahead
  of `v1.ai`. New order: `slab.surface, v1.middleware, v1.storage, v1.messaging, v1.ai, v1.data,
  v1.auth, v1.admin-listener, v1.client, v1.deployment, v1.repository-docs`. The header rationale
  comment rewritten to match. `goals.v1.data`'s summary dropped its claimed dependency on "the
  auth strategy's contract" and now states the sequencing choice directly: the domain layer proves
  out before auth constrains it, and each domain adopts the auth contract in a sweep task of its
  own once go-auth lands.
- **Retained:** `context/design/auth-strategy.md` — its opening paragraph asserted "every domain
  past this point builds to the contract this record states," which assumed domains always sit
  after `goals.v1.auth` in the sequence. Corrected to state the contract applies directly only to
  domains built after go-auth lands, with the sweep-task adoption path for domains built ahead of
  it (the record's substance is otherwise unchanged and stays retained as the auth strategy of
  record).

## Next-focus

A `start` session in `go-web-service`, taking up `slab.tasks.surface` as redefined in the prior
session: add `admin` and `org` subcommands to `slab` that invoke the running service's real
endpoints directly and return their result — a scriptable client replacing ad-hoc curl for
interacting with the service by hand, distinct in kind from the narrated `demo` scenarios already
built (a subcommand runs one real call, not a tour). `context/design/slab-conventions.md` states
slab's current package/naming/mechanism conventions — read it first, and update it as this task's
build settles new ones (a subcommand's own shape, how it differs structurally from a narrated
scenario's step).

While in this area of the code, also extend the `sqlate` demo scenario
(`go-web-service/tools/slab/internal/demo/compile.go`): step `[3/4]` (`registerCatalog`) narrates
`query.NewCatalog`/`query.Publish`/`catalog.Compile` but never shows how `fsys` itself gets built —
currently `s.fsys` comes from `repo.FS(ctx)` (`os.DirFS` over the real repo root), not `go:embed`.
Add narration showing a `//go:embed` directive and an `embed.FS` variable as the idiomatic way to
source `fsys`, matching sqlate's own documented convention (`sqlate/query/patterns.go`'s
`//go:embed patterns/*.sql`, and the same idiom shown in `sqlate/docs/concepts.md` and
`sqlate/docs/quick-start.md`). Open for that session's own SETTLE: whether this is a new narration
line inside the existing step or a small step of its own, and whether the demo's actual `s.fsys`
should switch to a real `embed.FS` or stay illustrative while the mechanism stays on-disk (the
`sqlate`, `domain`, and `problems` scenarios may depend on reading files not embedded).

Open design questions for that session's own SETTLE, not assumed here: how `org`/`admin` map onto
sub-subcommands per domain operation (one per HTTP route? per domain verb?), how a caller supplies
a request body and flags for each operation, the output format (raw JSON? something
narration-adjacent?), and whether a codegen helper that scaffolds a new command subcommand as the
domain surface grows — the architect's own suggestion — belongs in this task or its own.
