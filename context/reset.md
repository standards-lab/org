# reset · states

- **Status:** closeout
- **Session:** start
- **Project:** go-database, go-web-service, go-core, standards-lab
- **Branch:** states

## Disposition

- **Integrated:** `v1.data.sql.tasks.states`. Named database states, in seven stages across
  go-database and go-web-service with one mid-session release, plus a go-core fix the session
  found. go-database v0.5.0: `admin.Seeder` declares its sets (`States`, `Seed(ctx, state)`),
  `Options.Seed` names the set that applies at every startup, `Service.Seed` takes a name
  (empty for the configured set), `Service.Reset` is the transition to a named state (every
  migration reverted, the set applied, the state's set seeded, composing the migrator's `Down`
  and `Up` with the seeder, answering a `Transition`), `ErrUnknownState` refuses an undeclared
  name before I/O. The service: one file per state under `data/seeds/`, keyed by table
  (`default`, the reference tree; `empty`, no rows), decoded strictly; `admin.seed`
  (`APP_ADMIN_SEED`) is a state name, the local overlay naming `default`; `GET
  /admin/database/states`, `POST /admin/database/state`, and an optional `{"state"}` body on
  `POST /admin/database/seed`; `mise run db-state <state>` through mise's `usage` field; the
  suite's `Reset` is one call and walks seeded to empty, a named seed over it, and back,
  reading the organizations after each. Validation: both modules' vet, unit tier, lint, and
  tidy green; `GOWORK=off` builds of the service against the tags; `mise run integration`
  green over the bridge, over the pin, and twice on one compose project; the run-and-verify
  against the local stack through the task and the mount, drain logged.
- **Integrated:** the premise correction. The architect ruled that a set is not development
  and test tooling: it is how a new deployment initializes its data. Applying a set is
  therefore a production operation, idempotent and ungated beyond a name; only the reset is
  destructive, and it joins `down` and `force` in the class the management listener's
  confirmation token gates. There is no reset-at-boot switch. The framing left every doc
  comment, README sentence, and design note the step touched: go-database
  `design/infrastructure-service.md` states the set's role and `Reset`'s class; the coordinator's
  `design/dsl-driven-services.md` §6.2 restates the Seed concern and §6.3 adds the states and
  the gated `state` verb.
- **Integrated:** go-core v0.4.1. `processtest.Main` resolved the module root with `go list
  -m`, which under a multi-module `go.work` is every module, so the service's suite could not
  build under the sibling-development convention; it now asks for the suite package's own
  module. Proved by the service's integration tier under a two-module `go.work`, a session-time
  acceptance proof. The service pins it; the `go.work` convention in `concepts/data-layer.md`
  stands as written.
- **Integrated:** go-web-service `concepts/integration-tier.md` lost its named-states section
  (the code and README express it); `context/README.md` and `design/composition-root.md` name
  the configured seed set and the states.
- **Promoted:** nothing.
- **Culled:** the "development and test tooling" framing of seeding, everywhere the step
  touched it.
- **Retained:** the per-domain protocol helpers in `concepts/integration-tier.md`, for
  `v1.data.tasks.people`. go-web-sdk-template pins go-core v0.4.0; the fix does not affect a
  single-module suite, and the template picks up v0.4.1 at its next release. The stage list in
  the plan ordered `internal/app` before `admin/database`, which it imports; the session
  swapped them, a reminder that the dependency order is between packages, not layers. The docs
  pass (`v1.data.sql.tasks.docs`) carries the states.
- **Cross-repo:** at the coordinator, `roadmap.toml` deletes `v1.data.sql.tasks.states`,
  advances `next` to `v1.data.sql.integration.tasks.listener`, names the state verb among the
  ones the listener's token gates, and adds the states to the `docs` summary;
  `context/README.md`'s DSL entry cites go-database v0.5.0. At go-database, `context/README.md`
  recorded the admin capability as built (rode the release) and
  `design/infrastructure-service.md` is tended on its own `states` branch.

## Next-focus

`v1.data.sql.integration.tasks.listener`, a `start` session in go-web-service first, with
go-web-sdk and go-web-sdk-template as the step settles them: the admin mount on its own
listener, disabled unless configured, the confirmation token gating `down`, `force`, and
`state`, auth staged as the strategy's §10 posture questions decide, config rendering waiting
on a go-core redaction contract, and go-web-sdk's config env segment made per-block. The
direction is standards-lab `design/dsl-driven-services.md` §6.3 and §10. After it, the domain
tasks under `v1.data.tasks` or the docs pass, as the architect weighs them.
