# reset · alignment-review

- **Status:** closeout
- **Session:** review
- **Project:** standards-lab (org, .github, .github-private), claude-plugins, go-core, sqlate, go-database, go-web-sdk, go-web-sdk-template, go-web-service, docs
- **Branch:** alignment-review

## Disposition

- **Integrated:** four design notes decayed. `standards-lab/design/elemental-architecture.md`
  (its two pointers live at `goals.v1.messaging` and go-web-service's `concepts/data-layer.md`);
  `claude-plugins/design/skill-is-the-source-of-truth.md` (`scripts/check.sh` enforces it;
  CLAUDE.md and the context README keep the one-line rule);
  `go-database/design/infrastructure-service.md` (the ownership boundary, wiring rule,
  lifecycle stage, and `Versioner` capability are the `database` and `admin` package comments;
  the one-set-per-service assumption moved to the capability map);
  `go-web-service/design/composition-root.md` (`internal/app/doc.go`, the `admin` package
  comment, and the architecture repository's composition-root principle express it; why the base layers
  sit at the module root moved to `design/domain-architecture.md`).
- **Integrated:** restated detail collapsed to its home. `design/dsl-driven-services.md` §4–§8
  became one paragraph each pointing at sqlate's guide, the module doc comments, and the
  CHANGELOGs, the release ledger removed and §9–§11 renumbered to §8–§10;
  `design/testing-hierarchy.md`'s toolkit inventory points at the packages' `doc.go`;
  `design/standards.md` links the downward-dependencies principle; `references.md` states
  purpose and points at each README for the packages; `CLAUDE.md` links the routing rule;
  `concepts/admin-listener.md`'s requirement is its §6.3 link; `concepts/marathon-extraction.md`
  defers sequencing to the roadmap. In the members: claude-plugins' host and marathon READMEs
  link the extensions and staged-execution references; the go-core, sqlate, go-database,
  go-web-sdk, and template capability maps point at their READMEs and doc comments;
  go-web-sdk's `middleware-sourcing.md` cites the coordinator's rule, markers, and obligations;
  go-web-service's `domain-architecture.md` defers the detail policy and read grammar to the
  SDK, `documented-layers.md` the release discipline to the `goals.v1` criteria and the
  principle pages, and CLAUDE.md the test tiers to the README; docs' context README defers
  the tier rule to CLAUDE.md, the tree to README.md, and hosting to `backlog.docs-site`.
- **Integrated:** stale claims fixed. Forward citations under retired slugs: the listener is
  `v1.admin-listener` in go-web-sdk's `error-handling.md` item 6, go-web-service's
  `retrospective-findings.md`, `internal/app/admin.go`, and `admin/database/doc.go`;
  `backlog.context-stratification` → `v1.alignment.review`; `backlog.marathon-sitrep` →
  `v1.harness.sitrep`; the docs-pass slug → `v1.alignment.docs` in testing-hierarchy, the
  go-database and go-web-sdk maps, and both docs concepts; `backlog.workspace-sweep`,
  `backlog.validation-first`, `v1.data.writes.web`, and `backlog.go-elemental-rename` dropped;
  `v1.data.writes.operations` → `v1.data.evaluation`; `v1.data.tasks.people` →
  `v1.data.people`. Versions and names: go-database v0.5.0 and the `Seeder` as named states,
  sqlate v0.1.1, go-web-sdk and the template v0.7.0 (pins dropped from the maps per
  `design/service-organization.md`); `dotnet-elemental`; the three-repository layout;
  go-core's staged lifecycle and five packages in its README and CLAUDE.md; the template's
  task list and gitignore delta; marathon-roadmap's floor at 0.10; go-web-sdk's
  dependency-line statement (the admitted categories are stated when the first sourced
  middleware lands) and httpsnoop dropped for the SDK's own recorder; the reference service's
  `retrospective-findings.md` signatures and its closed startup findings; `RecursivePath` in
  `organization-lineage.md`; `sql-meta-language.md`'s body agreeing with its banners;
  `entrypoint-composition-split.md`'s path; the brief's current focus and layer order;
  sqlate added to both profiles, the brief, and the interview.
- **Promoted:** nothing.
- **Culled:** `standards-lab/concepts/external-providers.md` (the provider seam is sqlate's;
  the remainder is `backlog.second-providers` and `backlog.dotnet-mirror`);
  `claude-plugins/concepts/marathon-functions.md` (the architect ruled a divergence from
  strategy is talked through and pivoted, not handled by a functions tier);
  `go-web-sdk/concepts/direction.md` and `service-middleware.md` (the readiness type hook
  joined `error-handling.md` item 2; the placement reasoning joined `middleware-sourcing.md`);
  `claude-plugins/concepts/marathon-sitrep.md`'s output-formats section (the publish target
  is `v1.harness.sitrep`'s); go-web-service's `retrospective-findings.md` reduced to its auth
  section, and `data-layer.md`, `organization-lineage.md`, and `integration-tier.md` to what
  remains.
- **Retained:** the harness rules in `design/testing-hierarchy.md` and the Idiom section in
  go-web-sdk's `error-handling.md`, design content no code states; `concepts/docs-site.md`,
  the direction `backlog.docs-site` cites (its dispatch-triggered site repository and the
  roadmap summary's sitrep-blog starting point differ, both at claim resolution); the README
  Standard sections of the template and go-database, the declaration mechanism
  `design/standards.md` names; `concepts/tooling-principles.md` and
  `concepts/rolling-currency.md` as inputs to `v1.alignment.docs`;
  `concepts/elemental-runtime-layers.md`, gated on `v1.deployment`; go-web-sdk's `web`
  bullet inventory. Debt on record: go-database's `go.mod` pins go-core v0.3.0 and sqlate
  v0.1.0 and `postgres/go.mod` pins the base at v0.4.0, rolling-currency debt for a release
  session; `experiments/sql-dsl/REVIEW.md` keeps the retired listener slug as an archive; the
  plugin README changes ride the next tag of each plugin.
- **Cross-repo:** the sqlate line in `public/profile/README.md` and
  `private/profile/README.md`, one identical hunk; the docs-drift inventory appended to
  `docs/context/concepts/dsl-docs-pass.md` for `v1.alignment.docs` (go-database's five pages,
  go-web-sdk's four, the template's two, `architecture.md`'s Domain Service sentence,
  tests-and-docs and release-and-ci, go-core's index, the Go Elemental index's `dotnet-minimal`,
  and the absent sqlate, go-web-service, integration-tier, and four principle pages); the
  roadmap: `goals.v1.alignment.tasks.review` deleted and `next` advanced.

## Next-focus

`v1.alignment.docs`, a `docs` session in the architecture repository (`docs`, with standards-lab for the
catalog adjacents): the work list is `docs/context/concepts/dsl-docs-pass.md`, its inventory
plus the drift inventory this review appended, and the task's summary in `roadmap.toml`. Then
`v1.harness.hardening`.
