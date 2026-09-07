# reset · integration-toolkit

- **Status:** closeout
- **Session:** start
- **Project:** go-core, go-web-sdk, go-web-sdk-template, go-web-service, standards-lab
- **Branch:** integration-toolkit

## Disposition

- **Integrated:** `v1.data.sql.tasks.toolkit`. The reference service's integration harness
  moved to the layers that own what it exercises, its API as built, in eleven stages with
  three mid-session releases in dependency order. go-core v0.4.0 ships `process/processtest`:
  `Main` builds one main package per suite run with the race detector, `Launch` runs it as a
  subprocess with a composed environment and captured output, `Process.Await` waits on a
  condition a client observes and fails with the output if the process exits first,
  `Process.Stop` interrupts and returns the exit code, `FreePort` and `WaitFor` beside them,
  and `Forwarder`, the loopback relay a test severs; hermetic, proved on the unit tier against
  its own `internal/testprog`. go-web-sdk v0.7.0 ships `webtest`: `Client` and `Response`
  read whole over one connection, `Problem` as `web` writes one, `Decode`, `IfMatch`, `Live`
  on its own short-timeout client, and `Probe` from the retired `internal/webtest`.
  template/v0.7.0 gains the engine-free tier: an `integration` package whose `Service` embeds
  the toolkit's process, the tagged suite asserting boot, both probes with the `lifecycle`
  check, the drain to exit 0, and two instances side by side, `mise run integration`, `vet`
  and `lint` carrying the tag, and the CI job on merge to main and `workflow_dispatch` in both
  workflow copies; a direct-to-main fix then pointed setup-go's module cache at
  `template/go.sum`, a warning that predated the step, and the dual-copy rule names
  `cache-dependency-path` beside `working-directory`. The service pins both releases and
  keeps only its compose defaults, the `Seed` and `Database` options, `Ready` on the probe,
  and admin-mount state control; `client.go` and `proxy.go` are gone with their tests, the
  schema-walk test stays as `state_test.go`, and the suite calls the toolkit directly.
  Validation: every module's vet, unit tier, lint, and tidy green; `GOWORK=off` builds of the
  service and the template against the tags; the service suite's seven cases green twice on
  one compose project, fresh and populated; the template's tier green locally and in its
  post-merge CI run.
- **Integrated:** the promotion criterion. The Forwarder moved on fit, not on a second
  consumer: `design/service-organization.md`'s promotion bullet now states that a piece
  promotes when it is expressed in the lower layer's terms and depends on nothing above it,
  and a second consumer confirms the shape rather than licensing the move. go-web-service
  `concepts/integration-tier.md` lost its toolkit section (the code expresses it) and its
  per-domain helpers section is reworded to the criterion: `GuardedCommand` and
  `CollectionRead` belong in `webtest`, extracted at `v1.data.tasks.people`;
  `concepts/data-layer.md`'s generic seed helper is a fit question for the evaluation, no
  longer waiting on a second service.
- **Integrated:** `design/testing-hierarchy.md` states the library-toolkit convention as
  built (sqlate/sqltest, go-core/process/processtest, go-web-sdk/webtest, the template's
  wiring) in place of the planned paragraph, and its docs rule lists the toolkit's pages.
- **Promoted:** nothing.
- **Culled:** nothing.
- **Retained:** for the docs pass (`v1.data.sql.tasks.docs`): tests-and-docs and
  release-and-ci as before, and now the go-core, go-web-sdk, and template index pages and the
  template's task table, which the toolkit moved out from under; the roadmap's `docs` summary
  names them, and the docs-pass inventory note folds them in when that session runs. The
  harness rules in the testing hierarchy stand unchanged; the toolkit inherited them. The
  compose-variable drift item from the retrospective still stands.
- **Cross-repo:** at the coordinator, `roadmap.toml` deletes `v1.data.sql.tasks.toolkit`,
  advances `next` to `v1.data.sql.tasks.states`, corrects the `hardening` summary's claim that
  the tier's scaffolding is the service's package (it is the toolkit and the package over it),
  and adds the toolkit's pages to the `docs` summary; `context/README.md`'s testing-hierarchy
  entry dates the toolkit. At go-core, go-web-sdk, and go-web-sdk-template, each
  `context/README.md` capability map records its package as built, the landing-zone page due
  in the docs pass.

## Next-focus

`v1.data.sql.tasks.states`, a `start` session spanning go-database and go-web-service in that
order: named seed sets declared as data, a transition in go-database's admin service that
resets the schema and applies one, exposed through the admin mount and a startup switch, with
`mise run db-state <name>` for the developer and the suite's `Reset` becoming one call to the
same endpoint; the library half is go-database's, the sets the service's. The direction is
go-web-service `context/concepts/integration-tier.md`; the transition composes the existing
migrate and seed mechanisms, and the session settles the declaration format. go-database
releases mid-session and the service pins it, as this step did. After it,
`v1.data.sql.integration.listener`.
