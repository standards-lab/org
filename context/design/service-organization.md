# Service organization

How the organization builds out its infrastructure services and the reference architecture that
composes them. The settled principles migrated to the docs landing zone — the
[service tiers](https://github.com/standards-lab/docs/blob/main/principles/service-tiers.md) and
[repository topology](https://github.com/standards-lab/docs/blob/main/principles/repository-topology.md)
principles, and the Go Elemental pages — and this note keeps the planning direction the landing
zone does not document: which providers each service is expected to gain, and how the tiers
co-evolve.

## Anticipated services and their providers

Each infrastructure library declares its swap class when it is built; the anticipated lineup:

- **Auth** — a Keycloak provider and an Entra provider, each usable locally or managed. The
  standard tier is OAuth 2.0, OpenID Connect, and JWT; token verification is interchangeable,
  and what a token's claims contain is interchangeable with review. Provider-specific directory
  features are native.
- **Object storage** — an Azure Blob provider (azurite ↔ Azure Blob) and an S3 provider
  (minio ↔ S3). No formal standard exists, so the organization establishes the standard tier as
  the minimal operation set common to both target APIs; those operations are interchangeable,
  and consistency is interchangeable with review.
- **SQL** — one provider per engine; built (`go-database`, `postgres`). The service is
  schema-bound: a second engine is a second provider, and for an application a port, never a
  switch.

## Tier topology

Application SDKs and infrastructure libraries are peers on the Core SDK; an application SDK
never imports an infrastructure library, base module or provider. Cross-tier composition
happens in the application: the SDK exposes the extension point and the application declares
the policy — the web SDK's error-writing seam is the worked example: the SDK defines the
error-returning handler adapter and its writer, and the application supplies the matchers that
map go-database's error sentinels to HTTP statuses at its composition root. The adapter
(settled at the 2026-08-31 retrospective, `goals.v1.web`) moves the mechanics into the SDK
without moving the vocabulary: matcher policy stays the consumer's. When an infrastructure
library later contributes to an SDK-defined surface — the management-surface direction,
`v1.data.sql.startup` — the
dependency points infrastructure → SDK, never the reverse. The cost is a small adapter per
service; the return is independent releases and SDKs that accrete no infrastructure
vocabularies. Settled during `v1.data.writes.web`.

## Co-evolution

The abstractions live in the infrastructure libraries and the reference service consumes them;
the dependency runs one way, but they are built together. The reference service proves the
declared stack's provider of each service, local and managed, and the abstractions co-evolve
with it. The template is the first consumer of the Core SDK and the application SDK; the
reference service is the first consumer of the infrastructure libraries.

A second provider is proven elsewhere — by its own tests, a focused reference, or a real
consumer, one session each. It is what shows a service's provider contracts hold for more than
one implementation and validates any organization-established standard tier. The reference
service never grows a second provider of the same service; the bound is services × one provider,
never services × providers, and the .NET mirror is bounded the same way.

## Refinements and releases

The reference architecture is marathon-managed, so every change is a session.

- The documented layer is the unit of change: a refinement moves a layer's code, its doc
  section, and its tests together, and a doc section that no longer matches the code is a
  defect fixed in the same change.
- A refinement that proves a better pattern promotes outward — into the SDKs and the
  infrastructure libraries, the template, and the standard — so the seeded baseline never
  drifts from the reference service. Because the tiers co-evolve, a library change and the
  service change that proves it release as a coordinated snapshot.
- A release in a member repository prompts a coordinator-side sweep in the session that follows
  it: the profiles and the references catalog are checked against what the organization now
  ships. Presentation states shipped-versus-planned without pinning versions; each repository's
  releases page records the exact versions.
