# Authentication and authorization

The strategy for authentication and authorization across the reference architecture: OAuth 2.0 and
OpenID Connect with Keycloak as the declared provider, a relationship-derived authorization model
evaluated as SQL inside each service's own database, the subject and identity carrier every domain
method builds against, and the rule that lets services compose across a runtime boundary without a
shared authorization store. It is the counterpart of `design/dsl-driven-services.md`: every capability
goal beneath `goals.v1.auth` and every domain past this point builds to the contract this record states;
the build of go-auth itself is `goals.v1.auth`'s remainder.

This is a strategy record. It contains the principles, the reasoning that produced them, and the shape
of the result. Implementation detail lives elsewhere: go-auth's own `doc.go`, README, and CHANGELOG for
what it ships and at which version; each consuming service's `context/design/domain-architecture.md` for
how its own domains apply this contract; `standards-lab/context/design/service-organization.md` for the
runtime cross-service rules this record depends on and contributes to.

## 1. Authentication

The standard tier is OAuth 2.0, OpenID Connect, and JWT; Keycloak is the declared provider, Entra the
anticipated second (`design/service-organization.md`). Every service in the reference architecture is a
resource server only — never an OAuth client — with one narrow exception: a service calling another on
an end user's behalf is a client for the RFC 8693 token-exchange grant alone, never
authorization-code, refresh, or session management (§7).

On every request, go-auth verifies: the bearer token from `Authorization: Bearer` and nowhere else; the
signature against the issuer's JWKS, with key-rotation handling (cache the key set, refresh on an
unknown `kid`, rate-limit the refresh); `iss` against the configured issuer; `aud` against the configured
audience; `exp`/`nbf`/`iat` with a bounded, configured clock skew; an algorithm allowlist pinned to the
issuer's asymmetric algorithms, rejecting `none` and any symmetric algorithm; and `typ` where the
provider sets it.

The service reads exactly two claims: `iss` and `sub`, the link table's key (§3). `email`,
`preferred_username`, and `name` may be recorded for the linking UX and for audit, never used as
identity — they are mutable and not guaranteed unique. The service reads no role, group, permission, or
scope claim; authorization is decided entirely from the service's own grant model (§2), never from a
token.

Cryptography is sourced, not hand-rolled (`design/dependency-sourcing.md`): `github.com/coreos/go-oidc/v3`
is go-auth's verification library. go-auth's own README states its admitted dependency line (specification
surface, threat model, cryptography) as the enhancement that rule requires.

Auth's swap-cost class is interchangeable with review. Token verification is fully interchangeable:
discovery, JWKS, and the registered-claim checks are provider-neutral, and a provider swap is a
configuration change (issuer, audience, algorithm list). The review a Keycloak-to-Entra move requires is
narrow and named: `aud` semantics (app-ID-URI versus client-ID audiences across Entra's v1/v2 token
formats), `iss` shape (tenant-scoped, versioned endpoints), and the RFC 8693 versus on-behalf-of grant
type for cross-service calls (§7). None of these touch authorization, because authorization reads no
claims — that separation is what keeps the class this narrow rather than sliding toward schema-bound.

The one part of auth that is not interchangeable: the `(issuer, subject)` identity rows are provider-bound
by nature, since a subject claim is stable only within its issuing provider. A provider swap is a
re-linking migration of these rows, not a configuration change, and they are the one artifact on the
port list this capability contributes.

## 2. The authorization model

Authorization is relationship-derived, evaluated as SQL inside each service's own database, with a role
vocabulary on the grant carrying the verbs. RBAC survives only as that verb vocabulary — the mapping from
a role to the capabilities it grants — because the actual authorization questions this architecture asks
are relations, not role membership: whether a subject holds custody of a record, and whether a subject
holds a grant on a unit that is an ancestor-or-self of the record's unit. ABAC and every externalized
relationship-authorization product (Zanzibar, SpiceDB, OpenFGA, Ory Keto, Keycloak's native Authorization
Services) are rejected; §10 carries the full reasoning for each.

Two enforcement points, with two distinct failure modes:

- **Capability**, before the statement runs, in the domain service method: does the subject hold any
  grant whose role permits this operation. `403` on failure — a capability failure is about the caller
  and says so.
- **Scope**, inside the statement itself: a predicate in the read model's own SQL (List, One, and their
  count twin alike), and in a guarded command's `WHERE`. A row outside the subject's scope is not there.
  `404` on a read, the guard's existing no-row outcome on a command — a scope failure never confirms a
  row exists.

The scope predicate is a published pattern under the `auth` namespace, composed by the read model with
`OR` alongside any other scope pattern a domain needs (a custody-derived permission, for instance):

```sql
--| tier: standard
EXISTS (SELECT 1 FROM unit_grant g
        JOIN role_capability rc ON rc.role = g.role
        WHERE g.subject_id = {{subject:uuid}}
          AND rc.capability = {{capability}}
          AND (g.unit_id IS NULL
               OR EXISTS (SELECT 1 FROM organization_closure c
                          WHERE c.ancestor_id = g.unit_id
                            AND c.descendant_id = o.unit_id)))
```

`unit_grant.unit_id` nullable means service-wide; a service-wide grant needs no root unit to hang from
(the tree permits multiple roots). Role-to-capability is structural reference data, so it is a migration,
not a seed, and it is what lets the pattern stay parameterized by capability rather than needing one
pattern per operation. Capability names are globally unique and namespaced
(`<service>.<domain>.<verb>`), so a grant minted anywhere names a capability in any service's vocabulary
without collision — this is what a future enterprise-wide view of grants (§9) correlates on.

Only go-auth's sqlate-pinning sub-module publishes the `auth` namespace's universal half (the
subject-holds-capability test); it cannot publish the containment half, since the hierarchy being
contained is each service's own domain data. A read model composes both from its own statement. This is
the extension a domain applying this contract makes to `domain-architecture.md`'s existing rule that a
pattern is published by the application or the library: an infrastructure library is a third publisher,
narrowly, for exactly the universal half of its own capability's authorization.

The capability check lives in the domain service method, never in middleware or the handler: a domain has
two entry points, routes and reactors (`internal/app/doc.go`), and only the service method guards both by
construction.

Multi-tenancy: one deployment per tenant. `unit_grant.unit_id`'s nullable-means-service-wide shape assumes
a single tenant per deployment; a tenant column is a pervasive schema shape a later retrofit cannot add
cheaply, so this is stated as the position rather than left implicit.

## 3. The subject anchor and identity carrier

A `subject` anchor table is the authorization model's root: `subject(id, kind)`, `kind IN
('human','machine')`. `person` is one kind, sharing its primary key with its subject row through a
composite foreign key (`person(id, kind) REFERENCES subject(id, kind)`, `CHECK (kind = 'human')`) — the
standard-tier supertype/subtype pattern. The identity link table (issuer, subject claim, unique per
identity) references `subject_id`, not `person_id`; so does `unit_grant`. This keeps the authorization
schema dependent on nothing in any domain's own tables, matching the Go package dependency it mirrors —
domain packages import the root `auth` package, never the reverse — and it is what makes the authorization
schema portable if organization or grants are ever extracted into their own service (§9).

`person.id` stays the stable anchor `identity-linking.md` states: a UUID never reused, free of
authentication meaning. It additionally is a `subject.id`; the anchor adds authorization vocabulary to
it, not authentication vocabulary, and the promise the data layer makes about it is unchanged.

A `human` subject holds a `person` row only when the service has a business record for that human;
`person` is optional, not implied by kind. This is what lets an operator hold ordinary authorization
without leaking into staff listings or activate/deactivate transitions built for people the service
employs: an operator is a `human` subject with no `person` row, linked through the identity table like
anyone else, holding ordinary `unit_grant` rows (typically service-wide) for administrative capabilities.
The first operator subject is deployment data, seeded — never migrated, since which humans operate a
given deployment is not structural reference data.

Two types carry identity, in two places, because the authenticated identity and the authorized subject
are different facts owned at different tiers. `auth.Identity` — issuer, subject claim, audience, expiry,
a raw-claims accessor — lives in go-auth, provider-neutral, carrying no application vocabulary; go-auth
resolves it via its own middleware, which is `func(http.Handler) http.Handler` over stdlib types and is
structurally a `web.Middleware` with no go-web-sdk import. `auth.Subject` — subject id, kind, and a
capability set, with the person id available only when a person row exists — lives in each service's own
root `auth` package, resolved through that service's link and grant tables. A middleware at the API
module's group scope performs the resolution (one query through the link table, one for the capability
set), failing the request at `401` (no valid token) or `403` (a valid token with no linked subject) before
any handler runs. `Subject` deliberately carries no unit list: the set is unbounded under a service-wide
grant, and the scope predicate tests containment against an index rather than enumerating it.

`Subject` is an explicit parameter on every domain service method — `Find(ctx, subj auth.Subject, id
string)`, and so on — never threaded through the context past the middleware boundary. A method that
forgot a context-carried subject would compile, run, and silently authorize nothing; a method that cannot
be called without a `Subject` parameter cannot make that mistake. The context carries it for exactly the
one hop with no signature to put it on — middleware to handler — and nowhere else.

## 4. The sqlate requirement

A sqlate projection base may bind its own parameters, arity one. This is what lets the scope predicate
reach a read model's base statement at all — today a projection base may declare none. The requirement
is exactly this lift, nothing more: base arguments occupy the leading placeholder positions, filter
values and paging follow, and `count` binds identically to the page so a total always matches it.
Composing a subject and a capability into a base needs no parameter expansion, since both bind as a
single scalar value each; sqlate's own parameter rewrite already rebinds repeated occurrences of one
name to its first bound position, so two pattern includes naming the same parameter cost one bind. The
one case that does need an expanded value — a cross-service unit-id set (§7) — binds as a single
array-typed parameter with an `= ANY(...)` membership test, one placeholder with one value, still arity
one, and native tier only for that one conditional case.

The companion requirement on `sqlint`: a `scope` check in the `statements` role, verifying every
projection base under its configured globs includes at least one configured scope pattern, with per-glob
exemptions for reference data no subject's grant should ever need to reach (a catalog table, for
instance). This is the same declaration-plus-lint discipline `dsl-driven-services.md` §2.3 chose for the
dialect axis, closing the one silent failure the model has: a read model whose author forgot its scope
include, unauthorized and passing every test written about it.

## 5. Organization lineage

The organization tree's lineage is materialized in a closure table, `organization_closure(ancestor_id,
descendant_id, depth)`. `path` stays a read-time projection composed by its existing recursive CTE
(`domain/organization/statements/organization_view.sql`); the closure table's only role is the ancestor
test the scope predicate in §2 needs, as an indexed join rather than a per-request tree walk. Maintenance
is standard SQL inside the transaction the guarded transfer command already runs, under the advisory lock
that already serializes it: create inserts the self row plus a copy of the parent's ancestor rows at
depth+1; transfer deletes the moved subtree's former ancestor links and inserts the cross product of the
new parent's ancestors-and-self with the moved subtree's descendants-and-self; delete needs nothing
beyond the node's own closure rows, since the schema already blocks deleting a node with children. The
subtree cycle check (`in_subtree.sql`) reduces to a single closure lookup.

Composing `path` from the closure table instead of the recursive CTE — retiring the per-read tree walk —
is an available follow-on, not part of this decision.

## 6. Composition-root seams

Authentication middleware mounts at the API module's group scope, never at router scope: router-scope
middleware wraps the entire dispatch including the health and readiness probes, where a token must never
be required, while group-scope middleware does not. No SDK change is needed; the seam already exists.
`internal/app` gains `auth.go` as a layer file, parallel to `admin.go` and `domain.go`, composing an
`Auth` struct (the verifier, the resolver, the evaluator) from `infra`; `routes` and `mountAPI` take the
composed `Auth` layer, matching how every other layer file is threaded rather than passing `infra`
itself. No API route is ever public: every route under the API module requires a valid, linked subject,
unconditionally, so the middleware is unconditional on the group with no per-route opt-out.

Every domain gains a `Deps` struct and an error-returning constructor: `type Deps struct { DB
*data.Database; Authz auth.Evaluator }`, `func New(d Deps) (*Service, error)`. The struct makes a future
dependency addition additive rather than a breaking signature change everywhere; the error return pays
back what the struct gives up (the positional constructor's compile-time arity check) by validating
required fields at construction and naming what's missing, rather than silently zero-valuing an omitted
`Authz`. `Deps` is sized to `DB` and `Authz` only — no logger, no tracer, no cross-domain interface —
until a layer that needs one exists. It is per-domain, not shared: a cross-domain invariant that would
run upward is an interface the consuming domain declares and injects at the composition root
(`context/concepts/data-layer.md`), and a domain's own `Deps` struct is that injection point.

## 7. Cross-service authorization and staged resolution

A service reaches another service's data only through that service's own API — never its database, never
a replica of its tables, never a re-derivation of its rules. The consuming service declares the question
it needs answered; the owning service answers it from its own data, under its own rules — the same
cross-domain rule `context/concepts/data-layer.md` states for domains inside one service, applied at the
process boundary with configuration in place of the composition root and an HTTP contract in place of an
injected interface. A client to another service is a capability-named translation file, the same shape as
any other infrastructure integration.

A cross-service call carries the end user's identity as a token the owning service verifies for itself;
the calling service never asserts a subject on its own authority, which would make it a confused deputy.
Where audiences must stay separate, RFC 8693 token exchange is the mechanism (§1). A call with no end
user — a reactor, a scheduled job — carries the calling service's own machine subject, holding ordinary
grant rows in the callee exactly like a human subject; no "trusted internal service" bypass is ever
needed, because a calling service is an ordinary subject like any other.

**Staged resolution** governs how a request composes data across that boundary: what crosses a service
boundary is an input to the next query — a key or a predicate — never rows to be merged, filtered, or
re-sorted after the fact. A stage is one call, answered entirely from one service's own database, with
nothing about how that service authorizes its own multi-table joins changed by the rule; within one
service, §2's single-statement scope predicate is simply correct, and there is no boundary to stage
across. The rule concerns only what happens when the next fact a caller needs lives in a different
service's database.

Two things may cross a boundary. A **key**, becoming a parameter of the next call — a selected
organization id filtering the next call for its people. A **predicate set**, becoming a `WHERE` term of
the next call — the mechanism that resolves an authorization question spanning services (below). What
may never cross is a result set to be narrowed after the fact: a composer that fetches a page and then
discards rows against a fact from another call holds a page whose total is already wrong.

Applied to a subtree-scoped authorization question once organization and a consuming domain are separate
services: when a specific unit is already selected, resolving whether it lies within the subject's grant
is one cacheable containment call (cached per `(granted unit, requested unit)`, invalidated only by a
transfer) — caching a decision the owning service made, not caching its tables, and so staying inside the
API-only rule above. When no unit is selected, the consuming service asks the owning service to expand
the subject's granted units to their descendant set and binds the result as one array-typed parameter
(§4) in its own read model — a call bounded by the tree's node count, not by record count, for the same
structural reason the closure table is cheap: a service-wide grant, the case that would otherwise expand
to the whole tree, needs no set at all, since it drops the predicate entirely.

What staged resolution does not reach: a remote sort key, or a filter on a record-cardinality attribute
another service owns per record (not organizational containment). Neither composes as a predicate at any
scale, and both are reporting and search, not live authorization — served, when a real need for them
exists, by a materialized, event-driven read model off the authorization path entirely, advisory and
stale-tolerant, with any action on a row it surfaces re-authorized by the owning service at the point of
action. Nothing in the workspace currently asks for this; it is recorded as the mechanism, not built.

## 8. Object storage

The rule for authorizing access to an object: authorize the record, then reach the technology. Every
object needs a SQL row regardless of authorization — content type, size, soft delete, audit, lifecycle,
since an object store cannot itself be queried — so an attachment table
(`id, owner_kind, owner_id, unit_id, storage_key, ...`) carries the same scope predicate as any other
read model, at no additional cost. The object store itself never authorizes: no per-object ACLs, no
bucket policies keyed to end users. The service holds one credential per environment and is the sole
authority, because S3's and Azure Blob's own per-object authorization models share nothing standard tier
could express without putting the security-critical decision on the port list, in a second dialect, for
the second provider to re-derive.

Storage keys stay opaque, never authorization-bearing: an organizational path must never be encoded into
a key and authorized by prefix match, the same mistake `ltree` and Keycloak's group-path policies both
make (§10) — object stores have no rename, so a unit transfer would mean copying and deleting every
object beneath it. Containment is tested against an index, never a string prefix.

Bytes are proxied through the service, not served by a presigned-URL redirect: every read stays
authorized on every request, at the cost of service bandwidth, acceptable for the small, bounded objects
this reference service serves. A presigned URL is a bearer credential the service cannot revoke before
expiry. If large files become a real need, the strategy stays proxying, extended with compression and
chunked streaming rather than a redirect; live media streaming (audio or video signals, standard files or
live) is a separate future concern, out of scope here. Uploads go through the service the same way, for
the same reason — symmetric with reads, and the service validates content before it ever reaches storage.

## 9. What's deferred, and their triggers

**A relationship store** (evaluate OpenFGA first, per §10) — fired only by per-object sharing appearing
as a product feature: a person granting a named individual access to one record outside their unit,
which breaks the containment assumption §2's model depends on. Nothing else on this architecture's
product shape reaches this trigger; §7's mechanism handles the authorization-staging case that would
otherwise be the strongest candidate.

**A replica of the containment table** — fired only if the expansion call in §7 proves unacceptable on
latency or availability grounds, with measurements behind the call rather than as a first answer.
PostgreSQL logical replication is evaluated before an application-level outbox, since the replicated data
(a tree and a grant set) is small, low-churn, and snapshot-consistent — a full periodic refresh is
self-consistent regardless of what was missed, unlike a relationship stream.

**Extracting organization and its grants into their own service** — held, staying local to whichever
service builds it first. The schema in §3 is shaped so extraction later is additive: the only statements
touching authorization tables are the two published patterns and the resolving middleware's queries, so
extraction rewrites two pattern files per service rather than every read model that uses them.

**The administration control plane** — a registry of active services, an aggregated read view built from
grant-change events, and delegated grant mutation through token exchange, never acting on its own
authority. Deferred until a second deployed service exists to administer; nothing beyond the identity-link
and grant-change event obligation below is committed now.

**Grant writes and identity-link writes emit events.** Committed now, implemented when `v1.messaging`
lands: this is what answers an enterprise-wide "what can this person do anywhere" question by
aggregation rather than a synchronous authorization dependency, and it is the population path for a
replica or a relationship store if either trigger above ever fires. Identity-link events, not only grant
events, are what let a future aggregate view correlate one person across services on the one globally
stable key available — the `(issuer, subject claim)` pair.

**A cross-unit search or reporting view** — deferred until the product asks for one; §7 names the
mechanism (a materialized, event-driven read model) so building it later is a known answer, not a new
design. A live-orchestrated alternative to that mechanism — composing the staged hops of a single request
into one response graph, each hop still independently authorized — is a separate, later question,
captured in `concepts/staged-query-aggregation.md`.

## 10. Alternatives considered

**ABAC** (Cedar, Rego over subject/resource/action/environment attributes). Decides about a resource
already in hand; the dominant read here is a paged collection, where the question is which of many rows a
subject may see, not a decision about one. Answering that with ABAC means either fetching everything and
filtering in the application, wrong at any size and incompatible with a correct paging total, or partial
evaluation of the policy into a database filter — making a policy engine responsible for generating SQL
this architecture deliberately authors by hand. It is also a second DSL-driven service (`dsl-driven-services.md`
§2.1 names Rego and Cedar as exactly this category): a second language with expressive content the host
cannot type-check, its own runtime, its own release cadence, its own vulnerability history, on the
request path of every endpoint. Revisit only for a genuinely attribute-shaped, resource-in-hand
requirement — time-boxed access, break-glass, IP restriction — each expressible as a condition column on
a grant row, needing no policy engine.

**An externalized relationship store** (Zanzibar's model: SpiceDB, OpenFGA, Ory Keto). The relations this
service authorizes against are database-enforced invariants — custody's partial unique index guaranteeing
one open row per instance, the organization tree's sibling-scoped unique codes and version column — that
no relationship store expresses; adopting one means mirroring them, and a mirror is a permanent dual
write (an outbox, a strictly serialized relay, a reconciliation job) in every service that adopts it,
forever, whether or not that service has a cross-service relation. The decisive technical objection is
independent of that cost: every candidate documents a hard ceiling on filtering a paged collection by
permission — SpiceDB's `LookupResources` degrades past roughly 10,000 permitted resources, OpenFGA's
`ListObjects` past roughly 1,000 — exactly where scale would motivate adopting one, and the fix (SpiceDB's
Materialize) is commercial-only. The swap class across candidates is schema-bound: SpiceDB's, OpenFGA's,
and Ory's schema languages are three incompatible languages, so committing to one on the information one
service has about a fundamentally cross-service problem is the expensive mistake this architecture avoids
elsewhere by keeping SQL the one portable artifact (`dsl-driven-services.md` §2.3). If this ever becomes
the right tool, OpenFGA is the better candidate of the two: per-store isolated, immutable versioned
schemas callers pin explicitly, versus SpiceDB's one mutable global schema per cluster with no
per-caller pinning.

**Keycloak's native Authorization Services** (the UMA 2.0 resource-server model). Rejected on a ground
internal to the architecture alone: it is Keycloak's own API, unambiguously native tier, and Entra — the
named second provider — has no equivalent; building authorization on it would put the entire
authorization model, not one predicate, on the port list. It also cannot reach live per-transaction data
(a custody fact that changes on every issue and return) without an undocumented internal SPI requiring a
server rebuild per policy change, and its group-policy containment mechanism is a string prefix match on
the group path — the same failure as a materialized text path (§5). Most decisively: Keycloak built the
partial-evaluation mechanism that would let it filter a permitted-resource collection, and reserved it for
its own admin console, never exposing it to application resource servers — its own standards effort
(AuthZEN) lists implementing the search API as an explicit non-goal.

**PostgreSQL row-level security.** Attractive on the surface — policies on the tables, impossible to
forget, zero sqlate change — but it makes the entire authorization model engine-bound rather than one
predicate, worse than the `ltree` case it parallels; pooled connections need `SET LOCAL` inside a
transaction while reads do not currently run in one; the policy lives in DDL, invisible at the review of
a domain operation; and the admin service and seeder both need bypass paths.

**Fetch-then-check on a single-row read** (read unfiltered, evaluate in Go, map to 404). No sqlate
change, but it does not generalize to the list path, which must filter in SQL regardless — leaving two
authorization mechanisms with two chances to disagree, instead of one pattern applied identically on both
paths.

**`ltree` and a materialized text path**, for organization lineage (§5). `ltree` is a Postgres extension
(native tier, entering the port list for the single hottest, most security-critical predicate in the
service), couples the organization's code charset permanently to an index's label grammar, and produces a
separator mismatch against the HTTP contract's `/`-delimited path. A materialized text path loses exactly
where it must win: its prefix-scan optimization needs a plan-time-known prefix, and the authorization
predicate's prefix comes from a joined grant row at runtime, so the subtree test degrades to a scan.

## Record

This strategy was settled through a single `plan` session, working the model, its multi-service
implications, cross-service composition, and the subject and identity shape through with the architect
before any context changed. The reasoning trace — every alternative's full technical evaluation, the
rounds that stress-tested and revised the model as scope widened — lives in this session's own record;
nothing here restates it a second time.
