# reset · template-v0.6

- **Status:** closeout
- **Session:** start
- **Project:** go-web-sdk-template, standards-lab, go-database, go-web-service
- **Branch:** template-v0.6

## Disposition

- **Integrated:** `v1.data.sql.integration.template`, reduced at SETTLE. go-web-sdk-template
  template/v0.6.0 is built and validated in three stages: the pin to go-web-sdk v0.6.0 with the
  `reads` policy block (`ReadsConfig`, defaults 20 and 100, `APP_READS_DEFAULT_SIZE` and
  `APP_READS_MAX_SIZE`, validated to the invariant `web.ParseQuery` panics on, its env names
  recorded like the log and server blocks); the composition root as one file per layer under
  `internal/app` (`infrastructure.go`, `admin.go`, `domain.go`, `reactors.go`, each
  constructing its layer and owning its mount, `routes.go` the list of mounts) with the three
  packages deleted and the empty admin layer added as shape with its `/admin` group; and the
  starter README, the root README, and the changelog. Vet, race tests, lint, gofmt, and tidy
  are clean; the run-and-verify pass served the probes, answered 404 on both empty mounts, and
  drained on SIGINT to exit 0. The tag `template/v0.6.0` is cut on merge.
- **Integrated:** the template's `context/README.md` capability map says the layer files and
  the reads block; its Next bullet is gone, and a candidate-direction bullet says the admin
  layer's content and the database infrastructure patterns standardize from the reference
  service after `v1.data.sql.integration.service`.
- **Culled:** the template's data scaffolding as an item. The architect settled at SETTLE that
  the template stays engine-free: scaffolding `internal/data` and `admin/database` means
  declaring an engine, and database infrastructure setup and management are
  reference-architecture patterns go-web-service proves and the docs pass documents. The
  `internal/data`, `admin/database`, seeder, directives lowering, `sqlint.toml`, compose stack,
  mise tasks, and entity-conventions items moved to the `service` task.
- **Retained:** the landing-zone pages template/v0.6.0 moved out from under, for the docs
  pass: `docs/standards/go-elemental/go-web-sdk-template/index.md` (engine-free principle,
  five build points), `baseline.md` (four packages), and `elements.md` (`internal/reactors`,
  `internal/domain` as packages), plus the topology-and-naming principle's sentence naming
  the three packages. The `/admin` group serves on the API listener with no route; the
  starter README tells a generated service to settle the mount's isolation before mounting
  the first admin service, and the listener task decides the standard's posture.
- **Cross-repo:** at the coordinator, `roadmap.toml` deletes the `template` task, rewords the
  `integration` goal's summary and second criterion, moves the scaffolding items into
  `service` with the data-layer note linked, names the template's three pages and the
  database-infrastructure pattern pages in `docs`, corrects `hardening`'s last sentence, and
  advances `next` to `v1.data.sql.integration.service`; `design/dsl-driven-services.md` §5
  and §7 say the reference service builds the HTTP half and the `app` namespace comes from
  the service's `data` package, §8 marks the template done and moves the scaffolding to the
  service row, and §11 records 2026-09-06. At go-database,
  `design/infrastructure-service.md` and `context/README.md` no longer say the template
  scaffolds the HTTP half. At go-web-service, `design/composition-root.md`'s superseded
  paragraph and `concepts/data-layer.md` record the settled service layout: a root-level
  `data` package (the grouping with the registry, migrations, patterns, seeds behind the
  seeder, and the directives lowering) beside `domain/` and `admin/`, since domain packages
  import it and the topology-and-naming principle forbids a root-level package importing
  `internal/*`; `concepts/retrospective-findings.md` no longer cites the template task for
  the listener's reshape.

## Next-focus

`v1.data.sql.integration.service`, a `start` session at go-web-service: the coordinated
rewrite onto sqlate v0.1.0, go-database v0.4.0 with postgres/v0.3.0, and go-web-sdk v0.6.0,
on the layout `concepts/data-layer.md` now records — the root-level `data` package, the
organization domain on authored SQL as the spike built it, `admin/database` and the admin
mount, the composition root as files as template/v0.6.0 ships it, `cmd/db` and golang-migrate
deleted, `sqlint.toml` with the compose stack and mise tasks, and the entity conventions in the
domain architecture. SETTLE opens with the size question: the task lists three repositories,
and the go-database and go-web-sdk shares are context only unless the rewrite finds a library
defect. Nothing else remains in front of it.
