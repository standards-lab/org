# Dependency sourcing

Settled 2026-08-31, at the retrospective, generalizing the middleware sourcing decision from
the go-web-sdk review. The rule governs every layer of the architecture, not only middleware:
when the organization builds a capability itself, when it sources one, and how a sourced
dependency stays compatible with the dependency-line principle.

## The rule

Hand-roll any capability that is generic to its layer and has no specification or security
surface — the failure mode is visible in a unit test you would think to write. Source from an
industry-standard library — or copy its implementation in with attribution — any capability
whose correctness depends on a specification with known corner cases, a threat model, or
cryptography. Never carry a dependency for something the standard library already provides.

The test for "trivial" is not line count. A request-ID middleware fails loudly; a CORS
middleware fails by letting the wrong origin through with right-looking headers. More
generally: when a capability's depth and maintenance scope exceed what the organization can
execute and maintain correctly, and an industry-standard solution exists, reach for the
solution — and evaluate every such adoption for the safety, stability, and flexibility of the
architecture.

## What makes a third-party library "standard"

Markers, in rough order of weight:

1. **Stdlib types at the boundary.** It accepts and returns the standard library's types —
   `http.Handler`, `context.Context`, `error` — not its own context or handler type. The
   strongest signal: the library can be removed without touching callers.
2. **Zero or near-zero transitive dependencies.** `go mod graph` is short. A library that
   pulls a logging or configuration framework to do one thing is not standard, whatever its
   stars.
3. **Adopted by the projects that define the ecosystem** — Kubernetes, Prometheus, Grafana,
   Docker, HashiCorp, Cloudflare — or referenced by the language team's own material. Those
   maintainers have already done the review.
4. **Stable major version with a long tail.** Long-lived majors, low churn, a CHANGELOG that
   is mostly fixes. A library rewriting its API yearly is a framework in waiting.
5. **Solves a specification, not a preference.** When the library's job is correctness against
   an external document (CORS, OIDC, OpenTelemetry, content negotiation), the maintainer's
   accumulated corner cases are the product. When its job is convenience (binding, rendering,
   validation DSLs), it is a preference, and preferences belong in-house.

**Copy with attribution** is a first-class option when the module line is the only objection
and the upstream change rate is low: the reviewed corner cases land in-house without a
`go.mod` entry, at the cost of tracking upstream fixes by hand. Prefer importing when upstream
moves with a spec. Copied code carries the license header and upstream commit in the file, and
a CHANGELOG line at each sync.

## Compatibility with the dependency line

The narrowing rule says a lower level enhances a principle it derives from and never loosens
it, and each repository already states its own dependency line as an enhancement — go-core
declares the standard library alone. Sourcing stays compatible the same way: a repository that
admits sourced dependencies **states its line explicitly** — which categories it admits (spec,
threat model, crypto) and under which markers — beside its landing-zone link, exactly as
go-core states its stricter line. The org principle ("a minimal, deliberate dependency
footprint") is then enhanced per repo in both directions: tighter where possible, admitting
sourced correctness where the alternative is hand-rolled incorrectness. A silent import that
no stated line covers is a defect.

## Obligations

- **Hand-rolled is owned for the life of the standard.** Each release of an owning repository
  re-checks its in-house implementations against the current language release notes — a
  scheduled release-checklist item, not an ad hoc one.
- **Sourced is pinned and re-evaluated.** Marker 2 (the transitive graph) is re-checked at
  each upgrade; a dependency that grows a framework underneath it is re-litigated, not
  ridden.
- **Placement follows the layer topology** (`service-organization.md`): a capability that
  collaborates with an infrastructure service lives in that service's library over stdlib
  types, never in an application SDK.
