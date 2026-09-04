# reset · database-v0.4

- **Status:** closeout
- **Session:** start
- **Project:** sqlate, go-database, standards-lab
- **Branch:** database-v0.4

## Disposition

- **Integrated:** `v1.data.sql.integration.database`. go-database v0.4.0 is built and validated:
  the `ast`, `operation`, `exec`, and `seed` packages, the `Session`, `Tx`, and `Dialect` types,
  the `Provider` constant, and the constraint classes are gone; `New` is `New(conn, cfg)`; the
  `postgres` sub-module keeps the driver, the DSN, and pool construction and supplies no
  dialect; the `admin` package holds the database admin service over sqlate, generic over a
  prebuilt migrator, a `Seeder` returning a count map, a `Registry`, the catalog, and the pool;
  CI and the mise build task build each module with `GOWORK=off`. The unit tier is green (20
  tests in `admin` over `sqlate/sqltest`), and the behavior check against PostgreSQL 18.4 drove
  verify, apply, seed, the verbs, and `Diagnose` end to end. The tags `v0.4.0` and
  `postgres/v0.3.0` are cut on merge, bottom-up: the base first, then the provider's `require`
  bumped and its transient `replace` dropped. The task is deleted from the roadmap and `next`
  advanced.
- **Integrated:** `go-database/context/design/layers.md` decayed into
  `design/infrastructure-service.md`: the four-layer ontology, the guard contract, and the
  dialect material are sqlate's; the new note holds the boundaries between go-database, sqlate,
  and the application, the composition a root writes, and the wiring rule. The capability map
  names `database`, `admin`, and `postgres`.
- **Culled:** `go-database/context/concepts/sql-architecture.md` (its record is sqlate's
  documentation and `experiments/sql-dsl/REVIEW.md`) and `concepts/v0.4-findings.md` (every
  item consumed; the one deferral, `Config.Password` redaction, is cited on the `listener` task).
- **Retained:** `docs/context/concepts/dsl-docs-pass.md` line 41 still lists the go-database
  index page as rewritten around `query`, `migrate`, and `seed`; its header paragraph corrects
  it and the docs pass consumes the note. go-web-service's context describes its code at the
  pinned v0.3.0 until the `service` task rewrites it. The landing zone's `layers.md`,
  `dialect.md`, and `index.md` under go-database describe v0.3.0; the docs pass inventories them.
- **Cross-repo:** sqlate `postgres/v0.1.1`, merged and tagged in-session: `Dialect.ServerVersion`
  as a capability rather than a member of `sqlate.Dialect`. At the coordinator:
  `design/dsl-driven-services.md` §4.1, §5, §9, and History state v0.4.0 as built and drop the
  claim that a library release waits for the service change; `context/README.md` and
  `design/testing-hierarchy.md` say the same; the roadmap's two integration criteria are
  corrected to match, the `listener` task cites the password-redaction defect, and `next` is
  `websdk`. The `goals.v1` criterion "a library change and the service change that proves it
  release together" is left for the architect's ruling.

## Next-focus

`v1.data.sql.integration.websdk`, a `start` session at go-web-sdk: `IfMatch` and
`PreconditionError` promoted from the service's `sdk` staging, the `ErrorWriter` option to carry
the error text on chosen statuses (the admin handler's `reject` retires on it), the strict body
decode and the respond plumbing every handler repeats, and the operator-syntax decision for the
query parser. The template and the service pin the release.
