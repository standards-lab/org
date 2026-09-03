# reset · sql-dsl

- **Status:** closeout
- **Session:** experiment
- **Project:** standards-lab, go-database
- **Branch:** sql-dsl

## Disposition

- **Promoted:** the experiment's verdict, `experiments/sql-dsl/REVIEW.md`, into
  `context/concepts/sqlate.md` — the SQL templating library's name and rationale, grammar,
  layout, and placement — the concept `v1.data.sql.integration.sqlate` reads.
- **Promoted:** `design/dsl-driven-services.md` — §3 gains PRQL, considered and ruled out;
  §4.1 is the settled split (sqlate below go-database v0.4.0, the single-module plan
  superseded); §4.2 states the `{{name}}` grammar and `Statements`; §6 the catalog as built;
  §7 the artifact table across six repositories; §8 the integration sequence; §12 the
  prototype adjustments with the vocabulary's home. `context/README.md`'s capability map
  names sqlate.
- **Promoted:** the roadmap — `v1.data.sql`'s claim and criteria re-read against the split;
  `v1.data.sql.integration` with six tasks (`sqlate`, `database`, `websdk`, `template`,
  `service`, `listener`); `docs`, `hardening`, and `suite` re-read; `backlog.sql-meta-language`
  narrowed to schema typing and portability by construction.
- **Culled:** `v1.data.sql.prototype`, finished, and the pre-split tasks `query`, `migrate`,
  `organization`, `startup`, superseded by the integration goal; the signature cache, judged
  unnecessary; the `<prefix>-<language>` naming rule, superseded by sqlate's.
- **Retained:** the spike under `experiments/sql-dsl` as the durable record — the code the
  `sqlate` task moves, `NOTES.md` with the Ontology, `REVIEW.md`. Stable context cites the
  concept and the design note, and the two review files only from the roadmap's `context`
  entries.
- **Retained:** the member-repository adjustments `REVIEW.md` §5 lists for go-web-service
  (the entity roles, the admin layer, `internal/data`, the composition root as files, the
  multi-entity tension closed by the amended definition), go-web-sdk (the detail option, the
  precondition parse), go-web-sdk-template (the capability map), and the docs landing zone
  (the sqlate pages, the grammar's page, the meta-language reframe, the architecture
  definition's amendment): each is the first act of its integration task or the docs pass.
- **Retained:** `backlog.marathon-stage-review` and `backlog.marathon-workspace-experiments`,
  unchanged, first in `next`.
- **Cross-repo:** go-database `concepts/sql-architecture.md` — the outcome banner pointing at
  the review and the concept; decisions 2 and 3 marked superseded (the grammar, the generic
  methods); question 1's verdict; the Home line corrected to the coordinator. A commit on
  go-database's `sql-dsl` branch with its own pull request.

## Next-focus

`backlog.marathon-stage-review` and `backlog.marathon-workspace-experiments`, one `start`
session in claude-plugins: the review gate the experiment ran under (report with the
working tree uncommitted, iterate until approved, commit only on approval, the architect
states whether a reset follows) becomes the skill's, and an experiment's home is the
coordinator's `experiments/`. Then `v1.data.sql.integration.sqlate` at standards-lab: create
the repository from `experiments/sql-dsl/lib` and `cmd/sqlint` per `concepts/sqlate.md`,
insert it into the workspace order beside go-core, move the concept into its context.

The compose stack at close: `sql-dsl-postgres` up on 127.0.0.1:5433, database `app` at
schema version 3, seeded; `mise run db-down` in `experiments/sql-dsl` stops it.
