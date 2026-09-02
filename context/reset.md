# reset · sql-dsl

- **Status:** handoff
- **Session:** experiment
- **Project:** standards-lab
- **Branch:** sql-dsl

## Disposition

- **Added:** roadmap `backlog.marathon-stage-review` — the per-stage review gate marathon's
  staged execution lacks (commit before review; no gate at all for `experiment`), scoped to
  claude-plugins.
- **Added:** roadmap `backlog.marathon-workspace-experiments` — in a workspace, the
  coordinator's `experiments/` is the default home for every experiment, so a spike never
  collides with a development repository's tooling; scoped to claude-plugins.
- **Integrated:** roadmap `next` — the two marathon items follow the prototype as one
  claude-plugins session (the architect's sequencing, 2026-09-02); `next` is otherwise
  unchanged, a handoff does not advance it.
- **Retained:** go-database `concepts/sql-architecture.md` — its "Home" line still says
  `go-database/experiments/sql-dsl`; the experiment lives at the coordinator under the new
  convention (every experiment in `standards-lab/experiments/`). Corrected at close, with the
  task's `repos`.
- **Retained:** roadmap `v1.data.sql.prototype` — `repos = ["go-database"]` is stale for the
  same reason; a handoff does not advance the manifest.
- **Retained:** `standards-lab/context/design/dsl-driven-services.md` §3 — PRQL, considered
  and ruled out this session, is recorded in the experiment's notes and lands in §3 at close.

The session's own record, decisions, and transcripts are `experiments/sql-dsl/NOTES.md`
(Position, decisions log, Q3 deviations ledger, Q4 protocol and transcripts, Q5 destinations).

## Next-focus

`v1.data.sql.prototype`, resumed as `experiment` on branch `sql-dsl` of this repository; the
spike is `experiments/sql-dsl`, a nested Go module. Read `experiments/sql-dsl/NOTES.md`
Position first.

**Review gate (this session's rule, binding until the skill carries it):** a stage is not
complete until the architect has reviewed its diff; report with the working tree uncommitted,
iterate until approved, commit only on approval, and the architect states whether a reset
follows. Open each stage with a summary of what is in scope for that review.

**Position:** stages 1–3 of 12 are built and committed, one commit each, plus the stage 1
review correction (`fb2a8b0`):

- Stage 1 · **approved** — scaffold, database up, diagnostics; after review: the admin layer
  (`admin/`, `admin/database` owning the schema), the `/admin` mount, the composition root
  collapsed to `internal/config` and `internal/app`.
- Stage 2 · **committed, awaiting review** (`bee5923`) — `lib/sqldb` (session wrapper),
  `lib/pgdialect` (lock capability), `lib/drivertest` (driver fake).
- Stage 3 · **committed, awaiting review** (`8139456`) — `lib/migrate`; the migration set and
  the schema verbs now live in `admin/database` after the stage 1 correction.

**Exact next move:** open the stage 2 review with its summary (the three `lib/` packages,
their sketch deltas under NOTES Q3); iterate; then stage 3 (`lib/migrate`'s protocol as built,
NOTES Q4). Then stage 4: the live-engine acceptance proofs as `//go:build compose` tests in
`lib/migrate/live_test.go` — non-transactional DDL, dirty state and force, concurrent starters
in-process and process-level, the unlocked negative, the cancelled-context run — transcribed
under Q4. Stages 5–12 as planned: seed (the ~25-line loader under the admin domain, per
domain's JSON), `query` core, projection and guard, organization baseline, people and the
lint, load-time stitching, build-time generation with the judging table, iterate until the
architect judges the strategy definitive, then `close`.

**Settled this session, not yet in stable context:** every experiment lives at the
coordinator; the admin layer vocabulary (admin mount, admin domain, admin service, admin
handler); startup verifies and corrects the schema and fails only on dirty or unknown state,
readiness gated on it; the ErrorWriter and `force` findings. All in NOTES decisions log.

**Running it:** in `experiments/sql-dsl`: `mise trust && mise install`, `mise run db-up`
(postgres:18 on 127.0.0.1:5433), `mise run serve` (127.0.0.1:8081; startup applies pending
migrations), `mise run test`, `mise run lint`, `mise run split-check`. Compose state at
handoff: `sql-dsl-postgres` up, database `app` at schema version 3, clean, no data.
