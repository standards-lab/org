# Authentication and authorization

The reference architecture's auth strategy, the counterpart of the Go Elemental [DSL-driven
services](https://github.com/standards-lab/architecture/blob/main/standards/go-elemental/principles/dsl-driven-services.md)
principle. go-auth is planned (`goals.v1.auth`): a domain built after it builds to this contract
directly, and one built ahead of it adopts the contract in a sweep task of its own. The [auth
strategy page](../docs/auth-strategy.md) holds the build's design detail and the full case against
each alternative; each consuming service's domain-architecture page (go-web-service's
`docs/domain-architecture.md`) states how its domains
apply the contract, and `service-organization.md` holds the runtime cross-service rules.

## 1. Authentication

Standard tier: OAuth 2.0, OpenID Connect, JWT; Keycloak is the declared provider, Entra the
anticipated second. Every service is a resource server only, with one exception: a service calling
another on an end user's behalf is a client for the RFC 8693 token-exchange grant alone (§7).
go-auth verifies tokens with `github.com/coreos/go-oidc/v3`, because cryptography is sourced, not
hand-rolled (`architecture/standards/go-elemental/principles/dependencies.md`, "Sourcing").

The service reads exactly two claims, `iss` and `sub`, the link table's key (§3). Profile claims are
never identity, being mutable and not unique, and no role, group, or scope claim is read:
authorization comes from the service's own grants (§2). That keeps auth's swap-cost class
interchangeable with review: a provider swap is configuration plus a narrow, named review. The
exception is the `(issuer, subject)` identity rows, stable only within their provider: a swap is a
re-linking migration, and they are the one artifact this capability adds to the port list.

## 2. The authorization model

Authorization is relationship-derived, evaluated as SQL inside each service's own database, with a
role vocabulary on the grant carrying the verbs. RBAC survives only as that vocabulary, because the
questions asked are relations: does the subject hold custody of a record, or a grant on a unit that
is an ancestor-or-self of the record's unit. Two enforcement points, two failure modes:

- **Capability**, in the domain service method before the statement runs: `403`, since the failure
  is about the caller. Only the service method guards both entry points, routes and reactors.
- **Scope**, a predicate inside the statement (reads, their counts, guarded commands): `404`, or the
  guard's no-row outcome, since a scope failure never confirms a row exists.

The scope predicate is a published pattern under the `auth` namespace, parameterized by capability
through a role-to-capability table that is a migration, not a seed. go-auth publishes its universal
half (the subject holds the capability), making an infrastructure library a narrow third pattern
publisher beyond `domain-architecture.md`'s two; each service composes the containment half over its
own hierarchy. Capability names are globally unique, `<service>.<domain>.<verb>`, so grants
correlate across services (§9). One deployment serves one tenant: a tenant column is a pervasive
shape no retrofit adds cheaply.

## 3. The subject anchor and identity carrier

A `subject(id, kind)` table, `human` or `machine`, is the model's root; `person` is a subtype
sharing its key, and the identity link table and `unit_grant` reference the subject. A subject may
carry several identities (a provider migration needs it); an identity maps to one subject. The
authorization schema depends on no domain table, mirroring the Go dependency, so extraction stays
open (§9). `person.id` stays the data layer's anchor. An operator is a `human` subject with no
`person` row, holding ordinary grants without appearing among the people a service employs; the
first is seeded, never migrated.

`auth.Identity` (go-auth, provider-neutral) and `auth.Subject` (each service's own root `auth`
package) are separate types because the authenticated identity and the authorized subject are facts
owned at different tiers. Group-scope middleware resolves the subject (`401` no valid token, `403`
no linked subject). `Subject` carries no unit list, unbounded under a service-wide grant. It is an
explicit parameter on every domain service method, never carried in the context past the middleware:
a forgotten context value compiles and silently authorizes nothing, while a missing parameter does
not compile.

## 4. The sqlate requirement

A sqlate projection base binds its own parameters, arity one, which is all the scope predicate
needs; a cross-service unit-id set (§7) binds as one array-typed parameter. A planned `sqlint`
`scope` check verifies every projection base includes a scope pattern, with per-glob exemptions for
reference data, closing the model's one silent failure, a forgotten scope include.

## 5. Organization lineage

A planned closure table, `organization_closure(ancestor_id, descendant_id, depth)`, serves only the
scope predicate's ancestor test, as an indexed join. `path` stays a read-time projection from its
recursive CTE; deriving it from the closure table is a possible follow-on.

## 6. Composition-root seams

Authentication middleware mounts at the API module's group scope, never router scope, so health and
readiness probes never need a token; no API route is public, so it has no per-route opt-out. Each
domain gains a `Deps` struct (`DB` and `Authz` only, until a layer needs more) and an
error-returning constructor: a later dependency stays additive, and a missing field fails
construction instead of zero-valuing `Authz`. `Deps` is per-domain, the injection point for a
cross-domain interface (`go-web-service/context/data-layer.md`).

## 7. Cross-service authorization and staged resolution

A service reaches another's data only through that service's own API, never its database, a replica,
or a re-derivation of its rules: the cross-domain rule of `go-web-service/context/data-layer.md`, at
the process boundary. A call carries the end user's token for the callee to verify (token exchange
where audiences must stay separate) or, with no end user, the caller's own machine subject, holding
ordinary grants. No caller asserts a subject on its own authority (a confused deputy), and no
trusted-internal bypass exists.

**Staged resolution**: what crosses a service boundary is an input to the next query, a key or a
predicate set, never rows to filter or re-sort afterward, which leaves a wrong page total. Across
services, a selected unit is one cacheable containment call; otherwise the owner expands the
subject's grants to a descendant set, bounded by tree size and bound as one array parameter (§4),
and a service-wide grant needs no set. A remote sort key or a filter on another service's per-record
attribute composes as neither: that is reporting and search, served if ever needed by a
materialized, event-driven read model off the authorization path (recorded, not built).

## 8. Object storage

Authorize the record, then reach the technology. Every object has a row in the virtual-directory
library's tables, which hold no owner or unit; the consuming domain's join table carries `unit_id`
and the scope predicate, or at directory grain an owner row binds a top-level directory to a unit
(`blobfs-composition.md`). The object store never authorizes: one credential per environment, since
S3 and Azure Blob share no authorization model standard tier could express. Keys stay opaque, never
authorized by prefix (§10). Bytes are proxied both ways, never redirected to a presigned URL, a
bearer credential the service cannot revoke.

## 9. What's deferred, and their triggers

- **A relationship store**, OpenFGA first (§10): only when per-object sharing with a named
  individual outside their unit becomes a product feature, breaking §2's containment assumption.
- **A replica of the containment table**: only if §7's expansion call measurably fails on latency or
  availability.
- **Extracting organization and its grants into a service**: held; §3's schema keeps it additive.
- **The administration control plane**: until a second deployed service exists to administer.
- **A cross-unit search or reporting view**: until the product asks; §7 names the mechanism, and
  `goals.staged-query-aggregation` holds a live-orchestrated alternative.

**Grant writes and identity-link writes emit events**, a commitment on whichever goal builds those
writes, through go-messaging's outbox. They answer "what can this person do anywhere" by
aggregation, populate a replica or relationship store if one is triggered, and correlate a person
across services on the `(issuer, subject claim)` pair.

## 10. Alternatives considered

- **ABAC** (Cedar, Rego): decides about a resource in hand while the dominant read is a paged
  collection, and is a second DSL-driven service on every request path.
- **An externalized relationship store** (SpiceDB, OpenFGA, Ory Keto): must mirror invariants only
  the database enforces, a permanent dual write, and every candidate caps filtering a paged
  collection by permission.
- **Keycloak's native Authorization Services**: native tier with no Entra equivalent, putting the
  whole model on the port list.
- **PostgreSQL row-level security**: engine-binds the whole model.
- **Fetch-then-check on a single-row read**: the list path filters in SQL anyway, leaving two
  mechanisms that can disagree.
- **`ltree` or a materialized text path** for lineage: native tier on the hottest predicate, or a
  prefix known only at runtime, so the subtree test scans.
