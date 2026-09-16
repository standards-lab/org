# reset · slab-mechanism

- **Status:** closeout
- **Session:** start
- **Project:** go-web-service
- **Branch:** slab-mechanism

## Disposition

- **Promoted:** `go-web-service/context/concepts/service-showcase.md` →
  `context/design/slab-conventions.md`, rewritten as objective conventions (the module/dependency
  rule, the package layout, naming, the scenario mechanism) rather than narrative design
  reasoning — the mechanism is now proven by three real scenarios and the module-boundary
  decision was independently re-examined and reconfirmed this session. Dropped the unbuilt
  "long-running background action" signal shape and the session-by-session history narrative;
  added `problems`.
- **Cross-repo:** `standards-lab/context/roadmap.toml` — `goals.slab.tasks.mechanism` deleted
  (finished: `demo problems`, the third scenario and the service's problem-response tour, closes
  the task in place of the originally planned traffic generator). `goals.slab.tasks.surface`
  redefined from "extend to the admin walk" to "scriptable domain subcommands" (`admin`, `org`)
  that invoke the service's real endpoints directly in place of curl, distinct from the narrated
  `demo` scenarios. `goals.slab`'s own summary corrected to drop the stale traffic-generator
  claim and name the problem-response contract and the planned subcommands. Both tasks' context
  citations moved to the new `design/` path. `next` advanced past `slab.mechanism`. All edits
  committed directly to `standards-lab`'s `main`, not a feature branch.

## Next-focus

A `start` session in `go-web-service`, taking up `slab.tasks.surface` as redefined this session:
add `admin` and `org` subcommands to `slab` that invoke the running service's real endpoints
directly and return their result — a scriptable client replacing ad-hoc curl for interacting with
the service by hand, distinct in kind from the narrated `demo` scenarios already built (a
subcommand runs one real call, not a tour). `context/design/slab-conventions.md` states slab's
current package/naming/mechanism conventions — read it first, and update it as this task's build
settles new ones (a subcommand's own shape, how it differs structurally from a narrated scenario's
step).

Open design questions for that session's own SETTLE, not assumed here: how `org`/`admin` map onto
sub-subcommands per domain operation (one per HTTP route? per domain verb?), how a caller supplies
a request body and flags for each operation, the output format (raw JSON? something
narration-adjacent?), and whether a codegen helper that scaffolds a new command subcommand as the
domain surface grows — the architect's own suggestion — belongs in this task or its own.
