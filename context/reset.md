# reset · harness-hardening

- **Status:** closeout
- **Session:** start
- **Project:** claude-plugins, sqlate, standards-lab
- **Branch:** harness-hardening

## Disposition

- **Cross-repo:** sqlate's `sqlint` gained the guarded-statement check (`sqlint/v0.1.1`,
  released) — closes the lint gap `dsl-driven-services.md` §9 flagged.
- **Cross-repo:** claude-plugins released `marathon v0.11.0` (delegation to a project-declared
  agent, the sufficiency question at SETTLE, a stage's check running the repository's own
  tooling) and `marathon-roadmap v0.1.5` (retargeted at marathon 0.11 in the same pass, so this
  workspace's next session resolves both without a version mismatch).
- **Promoted:** the architect's personal model-routing convention
  (`~/.claude/behavior/model-routing.md`, the `fable`/`opus` agents) into a marathon-native
  mechanism — `[agents]` in `marathon.toml`, a `delegation` field per agent, no fixed roles —
  and declared for this workspace under `[workspace.agents]` here.
- **Integrated:** `context/design/dsl-driven-services.md` §9 — the guarded-statement and
  grammar's-page open questions removed, now that the code expresses both; §8's "what remains"
  updated now that hardening is closed.
- **Integrated:** `context/roadmap.toml` — `v1.harness.tasks.hardening` deleted; the backlog
  items associated with harness programming (`marathon-sitrep`, `marathon-extraction`,
  `marathon-references`, `harness-tooling`, `harness-testing`) adopted as `v1.harness`'s own
  tasks, none yet in `next`; `next` advances to `v1.auth.strategy`.
- **Retained:** `v1.harness`'s five adopted tasks — still open, none scheduled.

## Next-focus

`v1.auth.strategy` is next: a `plan` session in standards-lab producing the auth strategy
record, the counterpart of `design/dsl-driven-services.md` — authentication over OAuth 2.0 and
OIDC with Keycloak as the declared provider; the authorization model analyzed across RBAC,
ReBAC, and ABAC and one chosen with its reasoning; the request-identity carrier; the
authorization predicate on sqlate's one-row read path; the organization-lineage decision; and
the composition-root seams (the infrastructure-backed middleware home, the per-domain deps
constructor). The domains and every later layer build to its contract; the build of go-auth is
the goal's remainder.
