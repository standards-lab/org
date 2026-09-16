# reset · slab-mechanism

- **Status:** handoff
- **Session:** start
- **Project:** go-web-service
- **Branch:** slab-mechanism

## Disposition

- **Retained, corrected:** `go-web-service/context/concepts/service-showcase.md` — kept (the
  traffic generator and the admin walk are still ahead of it), but its naming
  (`slab run <scenario>` / `<capability>:<scenario>`) and its observability-link description (a
  Tempo/Grafana deep link keyed to `datasources.yaml`'s fixed uids) were stale against what this
  session actually built, and its "staged across two sessions" split no longer matches: `demo
  domain` already covers the full CRUD surface the split had reserved for session 2. Rewrote all
  three, and dropped the assumption about depending on fixed datasource uids, since nothing
  builds a deep link anymore.
- **Cross-repo:** `standards-lab/context/roadmap.toml` — `goals.slab.tasks.mechanism`'s summary
  and proof rewritten to name what's built (`demo sqlate`, `demo domain`) and what remains (the
  traffic generator), and to record the settled sqlate-export answer (no export).
  `goals.slab.tasks.surface` narrowed from "full organization-domain CRUD and the admin walk" to
  just the admin `/admin/database` walk, since `demo domain` already did the CRUD half. Both
  edits committed directly to `standards-lab`'s `main`, not a feature branch — the coordinator's
  reset file and roadmap are always current there.

## Next-focus

A `start` session in `go-web-service`, continuing `slab.tasks.mechanism` on the open
`slab-mechanism` branch (already commits `a5a6ad3`, `4dfeadb`, `2bd3033`, `aadbcc7`, `9d7c257` —
all reviewed and merged into that branch's history, nothing awaiting review). Two pieces of that
task remain before it closes:

1. **Design `demo observability`, the traffic generator** — the third and last novel shape
   (`sqlate`: a library import; `domain`: request/response and the observability side-channel;
   `observability`: a long-running background action). Start with a SETTLE discussion, not code:
   the architect asked to talk through the scenario's precise layout before building it, the same
   way `demo domain`'s shape was settled turn by turn this session rather than delegated whole.
   Carry forward from this session: the tight, request/response-forward narration density (`demo
   sqlate` and `demo domain` are the density bar — description before the call, minimal prose,
   80-column wrapped `Note`s); the reset-first pattern (`demo domain`'s step 1) as a candidate for
   giving the generator a known starting point too; and the fact that **`internal/grafana`, the
   deep-link builder, is gone** — the original stage-5 sketch (in this session's now-superseded
   plan file) assumed a range-scoped Grafana deep link on exit, which no longer fits the terse
   `Reporter.Trace`/`Observability`-block convention `demo domain` established. Whether the
   generator points at Grafana at all when it stops, and in what form, is open and belongs in the
   SETTLE discussion, not assumed.
2. **Stage 6, once `demo observability` is built and approved**: `tools/slab/README.md`, a line
   in the root `README.md`, a `CHANGELOG.md` entry, and a final `mise run vet`/`test`/`lint` pass
   across both modules.

After both, the session closes: `context/concepts/service-showcase.md` gets its own
`observability` section (mirroring the sqlate/domain treatment added this reset), and
`goals.slab.tasks.mechanism` is deleted from the roadmap with `next` advanced past it, per the
usual closeout hook.

Verification for the next session to lean on: `mise run db-up && mise run otel-up`, `mise run
serve` backgrounded, then `mise run slab -- list` / `mise run slab -- demo <name>` against the
real stack — every scenario built so far has been verified live, not just unit-tested, and
`demo observability` should be no exception.
