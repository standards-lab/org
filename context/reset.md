# reset · admin-listener

- **Status:** closeout
- **Session:** plan
- **Project:** standards-lab
- **Branch:** admin-listener

## Disposition

- **Integrated:** the management listener re-homed. The session opened as `start` on the
  listener and the architect ruled it premature: its token, its authentication, and its audit
  record are choices the auth and observability layers make properly later, and a reference
  architecture should not document an interim scheme it intends to replace. The listener is now
  `goals.v1.admin-listener`, a goal at claim resolution sequenced behind both layers.
  `design/dsl-driven-services.md` §6.3, §8, §9, §10, and §11 and `design/service-organization.md`
  cite the goal; `design/testing-hierarchy.md`'s sqlint claim is dated. `concepts/admin-listener.md`
  carries the exploration across go-core, go-database, go-web-sdk, go-web-service, and the
  template, so the goal's session does not repeat it: the per-block env segment, the singular
  composition root in the service and the template, the ungated destructive verbs and their
  seam, the one-client harness, the absent redaction contract and go-database's plain-string
  password, and the three posture questions.
- **Integrated:** the v1 execution sequence, settled with the architect and recorded as `next`:
  `v1.alignment.review`, `v1.alignment.docs`, `v1.harness.hardening`, `v1.harness.sitrep`,
  `v1.auth.strategy`, `v1.web`, `v1.observability`, `v1.storage`, `v1.auth`,
  `v1.admin-listener`, `v1.messaging`, `v1.data`, `v1.ai`, `v1.client`, `v1.deployment`. The
  reasoning: the written context aligned before anything builds on it; the harness next; the
  auth strategy (a plan session) before any layer or domain builds to its contract; the web
  SDK's handler contract and then observability so each later layer instruments as it lands;
  the domains once the integration standards they enhance exist. `goals.v1.auth` analyzes the
  authorization model across RBAC, ReBAC, and ABAC rather than assuming ABAC.
  `goals.v1.observability` names the LGTM stack (Loki, Grafana, Tempo, Mimir) as the
  development-side stack, run natively as a container stack.
- **Integrated:** the roadmap restructured. `goals.v1.data.sql.integration` closed (its releases
  and the service rewrite landed; the listener left it) and `goals.v1.data.sql` closed with it
  (its docs criterion moved). `goals.v1.alignment` holds `review` (every repository's context
  against its code, absorbing the context stratification pass) and `docs` (the docs pass,
  gaining the validation-first, rolling-currency, and tooling-principles pages).
  `goals.v1.harness` holds `hardening` (moved) and `sitrep` (promoted from the backlog): the
  organization's situational-awareness tool over a GitHub Pages dev blog, with sitreps and
  briefs as its first entry categories and `briefs/` and `interview.md` transitioning into it.
  `goals.v1` records the extraction ruling: a layer goal closes on its extracted record wherever
  the layer was proven; v1.0 keeps the running composition, with adoption a sweep task per
  layer. `v1.data.evaluation` carries the validation-first enforcement pass and rules from the
  extracted record if the extension has landed.
- **Promoted:** nothing.
- **Culled:** `backlog.context-stratification`, `backlog.validation-first`,
  `backlog.rolling-currency`, and `backlog.marathon-sitrep`, each folded into a goal task as
  above. `backlog.marathon-extraction` and `backlog.harness-tooling` were added.
- **Retained:** `concepts/marathon-extraction.md` and `concepts/tooling-principles.md`, filed
  this session as candidate direction for `claude-plugins` and the docs pass. The member-repo
  citations of the retired listener slug, left for `v1.alignment.review` rather than four
  one-line pull requests: `go-web-sdk/context/concepts/error-handling.md` item 6,
  `go-database/context/design/infrastructure-service.md`,
  `go-web-service/context/concepts/retrospective-findings.md` (which also cites a
  `backlog.workspace-sweep` the manifest does not hold), the service's three code comments
  (`internal/app/admin.go`, `admin/database/doc.go`, `admin/database/handler.go`), and its
  README's admin section. A manifest constraint found while editing: a goal slug cannot be a
  field name (`context`, `name`, `summary`, `criteria`, `repos`, `proof`, `tasks`), since TOML
  reads `[goals.v1.context]` as the parent's `context` field; the alignment goal was renamed for
  it, and the manifest reference should say so when `v1.harness.hardening` runs.

## Next-focus

`v1.alignment.review`, a `review` session over the whole workspace, the coordinator first and
then the repositories in the coordinator's `order`: each `context/` against its code, the
coordinator's notes, the profiles and the references catalog against what the organization now
ships, the known stale citations above fixed, and every restated detail collapsed to a link to
its single home. Then `v1.alignment.docs`.
