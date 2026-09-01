# The blueprint organization

Standards Lab's repositories are not only worked examples of a standard's tiers — the
organization itself is the worked example of a standards-based, agentic organizational
architecture strategy. The vision already claims each repository as a worked example for
others to follow; this extends the claim one level up: how an organization defines architectures,
standards, and principles, binds them with the narrowing rule, organizes their repositories by
the topology principle, documents them in a landing zone, and develops them with the harness —
that whole arrangement is the deliverable, demonstrated by being lived. Agentic is a first-class
dimension of the strategy, not tooling beside it: the harness level's programming standards and
workflow integrations are part of the architecture itself, even though the harness stands
outside the software's repository tiers and governs how agent tooling is built rather than what
the software does. The thesis binding the two: clear architectural principles and standards,
broken down into appropriately layered boundaries, optimize the utility of agentic workflows in
creating well-structured software — the structure that keeps a system legible to people is what
lets agents build it well.

Settled 2026-08-31, promoted from the concept captured during the workspace-sweep session; the
Go Elemental rename (`backlog.go-elemental-rename`, recorded in the session that landed this
note) was its first consequence.

## The three roles

- **Blueprint.** Standards Lab incubates architectures and standards. Its repositories carry
  single-concern names with a language prefix inside the one organization, and they are not
  renamed as the strategy matures — the blueprint's job is to stay legible as the reference for
  how such an effort is organized.
- **Boundary.** An architecture or standard intended for production use is encapsulated at an
  organizational boundary: it graduates to its own organization, whose name carries the
  standard's identity and whose repositories keep short single-concern names. The module
  namespace (`go-elemental/core`) arrives as a consequence of the boundary, never as a refactor
  of the blueprint — which is why the blueprint's verbose repository names
  (`go-web-sdk-template`) are not a problem to solve.
- **Catalog.** The docs landing zone documents the blueprint's one definitive architecture and
  its standards in depth, and references every other implementation — graduated organizations
  and external implementations built by others — rather than hosting their documentation. The
  catalog section is a placeholder until the Elemental Architecture completes and graduates to
  its own organization; the `references.toml` External references grouping is the same
  separation at the catalog level.

## Structural consequences, resolved

- **One definitive architecture, three levels of resolution.** The organizational boundary
  absorbs the burden of scaling many architectures, so the blueprint builds out exactly one —
  the Elemental Architecture — and the documented hierarchy is architecture → standard →
  module. The former organizational-principles tier dissolved into the architecture's
  principles; the docs landing zone holds the architecture at its root (`architecture.md`,
  `principles/`), each standard under `standards/<key>/`, and each module's pages beneath its
  standard. The module level carries three classes — library, template, app — refined by the
  software tier vocabulary (core SDK, application SDKs, infrastructure libraries, templates,
  references).
- **Informative, not prescriptive.** The blueprint's arrangement may serve any discipline that
  benefits from an agentic development workflow, and it never prescribes how an external effort
  structures its own architectures and standards — it is a worked example, not a schema.
- **The minimal-dependency principle belongs to the architecture.** The Elemental
  Architecture's purpose is optimizing supply-chain boundaries — mitigating what dependencies
  bring into an architecture, and establishing the deliberate maintenance boundaries
  (`design/dependency-sourcing.md`) that make the pinned-and-current posture tractable. The
  organizational minimal-footprint principle derives from it; a standard's dependency line is
  its enhancement of an architectural principle, not a private posture. This is what the Go
  Elemental rename asserts: a framework-heavy Go standard would not be a competing
  implementation of Elemental — it would not implement Elemental at all.

## Relations to standing context

- The narrowing rule and the definitions (architecture / standard / principle) are unchanged;
  this note adds the organizational lifecycle around them.
- `design/workspace-docs.md` defines the docs tier within the workspace; the catalog role
  extends its reach to implementations outside the workspace without changing the
  within-workspace convention.
- The charter's governance objective ("which these organizational repositories can eventually
  serve themselves") is this strategy's endgame: the blueprint governing the organizations it
  graduates.

## Open questions

- Graduation timing relative to v1.0: graduating at or before the v1.0 cut means no production
  consumer ever pins the blueprint's module paths.
- Graduation mechanics: what transfers — repositories moved, or a clean re-rooting with the
  blueprint retained as the incubator's record — and who owns the graduated organization.
