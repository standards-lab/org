# reset · auth-strategy

- **Status:** closeout
- **Session:** plan
- **Project:** standards-lab, go-web-service, claude-plugins
- **Branch:** auth-strategy

## Disposition

- **Integrated:** `context/design/auth-strategy.md` authored — the counterpart of
  `design/dsl-driven-services.md`: authentication, the relationship-derived authorization model
  (rejecting ABAC and every externalized relationship store, including a bespoke centralized one,
  with the technical reasoning recorded), the subject anchor and identity carrier, the sqlate
  requirement, organization lineage, composition-root seams, cross-service authorization and staged
  resolution, and object storage. Every later layer and domain in go-web-service builds to it; the
  build of go-auth itself is `goals.v1.auth`'s remainder.
- **Integrated:** `design/service-organization.md` gains the runtime cross-service composition rule
  (API-only access; compose predicates, never pages), stated as workspace-scoped planning direction
  pending a second deployed service to confirm it — not yet promoted to the architecture repository.
- **Cross-repo:** go-web-service's `concepts/identity-linking.md` and `concepts/organization-lineage.md`
  rewritten as current fact against the settled strategy, reasoning kept once in `auth-strategy.md`
  rather than restated; `context/README.md`'s Auth capability-map entry updated to match.
- **Integrated:** go-web-service's `concepts/retrospective-findings.md` decayed — its only remaining
  content (the auth layer's five gaps) is fully consumed by the new record.
- **Cross-repo:** a dangling reference to the decayed `retrospective-findings.md` fixed in
  `concepts/admin-listener.md`, redirected to `auth-strategy.md` §6.
- **Culled:** `goals.v1.auth.tasks.strategy` deleted (done); `goals.v1.auth`'s summary and context
  rewritten to point at the settled record instead of the task and the decayed findings note.
- **Cross-repo:** a new concept, `standards-lab/context/concepts/staged-query-aggregation.md`, captures
  a live-orchestration alternative to the materialized-view answer for cross-service reporting, raised
  by the architect and deliberately not designed — gated on the bounded query strategy being built and
  proven in practice.
- **Cross-repo:** claude-plugins' `context/concepts/marathon-updates.md` gains a fourth finding, found
  live during this session's staged execution: `references/staged-execution.md`'s delegation call is
  silently skippable, with a proposed stage-loop checkpoint to make it a stated decision. Not yet
  committed in claude-plugins — the file was already untracked when this session found it, and the
  addition rides whatever session next touches it.
- **Integrated:** `context/roadmap.toml` — `v1.harness.tasks.marathon-updates` added, covering all four
  `marathon-updates.md` findings; `next` advances to it, ahead of `v1.web`.

## Next-focus

`v1.harness.marathon-updates` is next: a session in claude-plugins assessing and applying, where they
hold, the four findings `context/concepts/marathon-updates.md` carries — extracting the architecture
layer into an optional `marathon-architecture` extension; a pre-write check for context curation before
anything lands in `context/`; whether authoritative context should wait for a session past the one that
designed the shape it documents; and the stage-loop delegation checkpoint. `v1.web` (the web SDK's
handler contract) follows once this closes.
