# Service organization

How the organization builds out its infrastructure services and the reference architecture that
composes them. The settled principles migrated to the architecture repository — the
[service tiers](https://github.com/standards-lab/architecture/blob/main/principles/service-tiers.md) and
[repository topology](https://github.com/standards-lab/architecture/blob/main/principles/repository-topology.md)
principles, and the Go Elemental pages — and this note keeps the planning direction the
architecture repository does not document: which providers each service is expected to gain, and how the tiers
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
- **SQL** — one provider per engine. Built: `sqlate` with its `postgres` sub-module, and
  go-database as the infrastructure service over it (`design/dsl-driven-services.md`). The
  service is schema-bound: a second engine is a second provider, and for an application a
  port, never a switch.

## Tier topology

Application SDKs and infrastructure libraries are peers on the Core SDK. An application SDK
never imports an infrastructure library, neither its base module nor a provider. Cross-tier
composition happens in the application: the SDK exposes an extension point, and the
application declares the policy. The web SDK's error writing is the worked example. The SDK
defines the error-returning handler adapter and its writer; the application supplies, at its
composition root, the matchers that map the database library's error types to HTTP statuses.
The adapter (settled at the 2026-08-31 retrospective, built in go-web-sdk v0.6.0) moves the
mechanics into the SDK without moving the vocabulary, so matcher policy stays the consumer's. When an
infrastructure library contributes to an SDK-defined surface, as the management listener will
(`v1.admin-listener`), the dependency points from the infrastructure library to
the SDK, never the reverse. The cost is a small adapter per service; the return is independent
releases and SDKs that accumulate no infrastructure vocabulary. Settled at the service's
first write layer.

## Runtime composition across services

Distinct from the tier topology above, which governs how a library composes with an application at
build time: how a *deployed* service reaches another service's data at request time, once the
reference architecture grows past one deployed service. Stated now, while there is one, as planning
direction rather than a promoted principle — it promotes to the architecture repository once a second
deployed service exists to confirm the shape.

A service reaches another service's data only through that service's own API. It does not connect to
another service's database, does not replicate its tables to read them, and does not re-derive its
rules. A consuming service declares the question it needs answered; the owning service answers it from
its own data, under its own rules — the cross-domain rule `go-web-service/context/concepts/data-layer.md`
states for domains inside one service, applied at the process boundary: configuration stands in for the
composition root, an HTTP contract for the injected interface. A client to another service is a
capability-named translation file, the same shape as any other infrastructure integration.

Composing data across that boundary follows one further rule: what crosses a service boundary is an
input to the next query — a key or a predicate — never rows to be merged, filtered, or re-sorted after
the fact. A stage is one call, answered entirely from one service's own database; nothing about how a
service authorizes or joins within its own database changes because of this rule, which concerns only
what happens when the next fact a caller needs lives in a different service's database. A composer that
fetches a page from one service and discards rows against a fact from another holds a page whose total
is already wrong — the failure this rule exists to prevent by construction. A remote sort key, or a
filter on a record-cardinality attribute another service owns per record, cannot compose as a predicate
at any scale; that is reporting and search, served off the live path by a materialized, event-driven
read model, never by a query spanning two databases.

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
  drifts from the reference service. The criterion is fit, not a count of consumers: a piece
  promotes when it is expressed in the lower layer's terms and depends on nothing above it. A
  second consumer confirms the shape; it is not the license. Because the tiers co-evolve, a
  library change and the service change that proves it release as a coordinated snapshot.
- A release in a member repository prompts a coordinator-side sweep in the session that follows
  it: the profiles and the references catalog are checked against what the organization now
  ships. Presentation states shipped-versus-planned without pinning versions; each repository's
  releases page records the exact versions.
