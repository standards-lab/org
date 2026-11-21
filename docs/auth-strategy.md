# Authentication and authorization: design detail

The design detail behind the auth strategy note (`context/auth-strategy.md` in this repository):
what go-auth, the goal `v1.auth`, and each domain's auth sweep build to. The note holds the
decisions and their reasons; this page holds the specifics they need. Section numbers match the
note's. go-auth's own `doc.go`, README, and CHANGELOG will state what it ships and at which version
once it exists.

## 1. Authentication

### Token verification

On every request, go-auth verifies:

- the bearer token from `Authorization: Bearer` and nowhere else;
- the signature against the issuer's JWKS, with key-rotation handling: cache the key set, refresh on
  an unknown `kid`, rate-limit the refresh;
- `iss` against the configured issuer and `aud` against the configured audience;
- `exp`, `nbf`, and `iat` with a bounded, configured clock skew;
- an algorithm allowlist pinned to the issuer's asymmetric algorithms, rejecting `none` and any
  symmetric algorithm;
- `typ` where the provider sets it.

`email`, `preferred_username`, and `name` may be recorded for the linking UX and for audit, never
used as identity. go-auth's README states its admitted dependency line (specification surface,
threat model, cryptography) as the enhancement the sourcing rule requires.

The resource-server-only rule excludes authorization-code, refresh, and session management; the
token-exchange exception covers the RFC 8693 grant alone.

### The provider-swap review

Token verification is fully interchangeable: discovery, JWKS, and the registered-claim checks are
provider-neutral, and a provider swap is a configuration change (issuer, audience, algorithm list).
The review a Keycloak-to-Entra move requires is narrow and named:

- `aud` semantics: app-ID-URI versus client-ID audiences across Entra's v1 and v2 token formats;
- `iss` shape: tenant-scoped, versioned endpoints;
- the RFC 8693 grant versus Entra's on-behalf-of grant for cross-service calls.

None of these touch authorization, because authorization reads no claims.

## 2. The authorization model

### The scope pattern

The scope predicate is a published pattern under the `auth` namespace, composed by the read model
with `OR` alongside any other scope pattern a domain needs (a custody-derived permission, for
instance):

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

The scope predicate applies to List, One, and their count twin alike, and to a guarded command's
`WHERE`; a row outside the subject's scope is not there.

`unit_grant.unit_id` nullable means service-wide; a service-wide grant needs no root unit to hang
from (the tree permits multiple roots). Role-to-capability is structural reference data, so it is a
migration, not a seed, and it is what lets the pattern stay parameterized by capability rather than
needing one pattern per operation. Because capability names are globally unique and namespaced, a
grant minted anywhere names a capability in any service's vocabulary without collision.

### Who publishes which half

Only go-auth's sqlate-pinning sub-module publishes the `auth` namespace's universal half (the
subject-holds-capability test); it cannot publish the containment half, since the hierarchy being
contained is each service's own domain data. A read model composes both from its own statement. This
is the extension a domain applying the contract makes to `domain-architecture.md`'s rule that a
pattern is published by the application or the library: an infrastructure library is a third
publisher, narrowly, for exactly the universal half of its own capability's authorization.

The capability check's placement follows from `internal/app/doc.go` in go-web-service: a domain has
two entry points, routes and reactors.

### Tenancy

`unit_grant.unit_id`'s nullable-means-service-wide shape assumes a single tenant per deployment.

## 3. The subject anchor and identity carrier

### Schema

- `subject(id, kind)`, `kind IN ('human','machine')`.
- `person` shares its primary key with its subject row through a composite foreign key, `person(id,
  kind) REFERENCES subject(id, kind)`, with `CHECK (kind = 'human')`: the standard-tier
  supertype/subtype pattern.
- The identity link table holds issuer and subject claim, unique per identity, and references
  `subject_id`, not `person_id`; so does `unit_grant`.

Domain packages import the root `auth` package, never the reverse, and the schema mirrors that. This
is what makes the authorization schema portable if organization or grants are ever extracted into
their own service.

`person.id` is a UUID never reused, free of authentication meaning. It additionally is a
`subject.id`; the anchor adds authorization vocabulary to it, and the promise the data layer makes
about it is unchanged.

### Operators

A `human` subject holds a `person` row only when the service has a business record for that human;
`person` is optional, not implied by kind. An operator is linked through the identity table like
anyone else and holds ordinary `unit_grant` rows, typically service-wide, for administrative
capabilities, so it never leaks into staff listings or the activate and deactivate transitions built
for people the service employs. The first operator subject is seeded, never migrated, because which
humans operate a given deployment is not structural reference data.

### Identity and Subject

`auth.Identity` carries issuer, subject claim, audience, expiry, and a raw-claims accessor, with no
application vocabulary. go-auth resolves it through its own middleware, a `func(http.Handler)
http.Handler` over stdlib types: structurally a `web.Middleware` with no go-web-sdk import.

`auth.Subject` carries the subject id, kind, and a capability set, with the person id available only
when a person row exists. A middleware at the API module's group scope resolves it with one query
through the link table and one for the capability set, failing the request at `401` (no valid token)
or `403` (a valid token with no linked subject) before any handler runs. The scope predicate tests
containment against an index rather than enumerating units.

Every domain service method takes the subject explicitly: `Find(ctx, subj auth.Subject, id string)`,
and so on. The context carries it for exactly the one hop with no signature to put it on, middleware
to handler, and nowhere else.

## 4. The sqlate requirement

### Binding

Base arguments occupy the leading placeholder positions, filter values and paging follow, and
`count` binds identically to the page so a total always matches it. Composing a subject and a
capability into a base needs no parameter expansion, since each binds as a single scalar value;
sqlate's parameter rewrite rebinds repeated occurrences of one name to its first bound position, so
two pattern includes naming the same parameter cost one bind. The cross-service unit-id set binds as
a single array-typed parameter with an `= ANY(...)` membership test, one placeholder with one value,
still arity one, and native tier only for that one conditional case.

### The planned sqlint scope check

A `scope` check in `sqlint`'s `statements` role verifies every projection base under its configured
globs includes at least one configured scope pattern, with per-glob exemptions for reference data no
subject's grant should ever need to reach (a catalog table, for instance). It is the same
declaration-plus-lint discipline the DSL-driven services principle chose for the dialect axis
("Which artifact is portable"), closing the one silent failure the model has: a read model whose
author forgot its scope include, unauthorized and passing every test written about it.

## 5. Organization lineage

### Closure maintenance

`path` is composed by the existing recursive CTE in go-web-service
(`domain/organization/statements/organization_view.sql`). Closure maintenance is standard SQL inside
the transaction the guarded transfer command already runs, under the advisory lock that already
serializes it:

- create inserts the self row plus a copy of the parent's ancestor rows at depth+1;
- transfer deletes the moved subtree's former ancestor links and inserts the cross product of the
  new parent's ancestors-and-self with the moved subtree's descendants-and-self;
- delete needs nothing beyond the node's own closure rows, since the schema already blocks deleting
  a node with children.

The subtree cycle check (`in_subtree.sql`) reduces to a single closure lookup. Composing `path` from
the closure table would retire the per-read tree walk.

## 6. Composition-root seams

### The auth layer file

Router-scope middleware wraps the entire dispatch, including the health and readiness probes, while
group-scope middleware does not; the group seam already exists in go-web-sdk. `internal/app` gains
`auth.go` as a layer file, parallel to `admin.go` and `domain.go`. Its define function defines the
verifier, the resolver, and the evaluator as graph nodes. Each node's constructor is a method on
`*Nodes`, passed to Define as a method value, that builds the node from the infrastructure nodes it
uses, and the node's Define line declares any order or group with `graph.After` and `graph.In`.
`mountAPI` reads the three from `Nodes` with `Use`, as it reads every other node. Every route under
the API module requires a valid, linked subject, unconditionally.

### Domain Deps

```go
type Deps struct {
	DB    *data.Database
	Authz auth.Evaluator
}

func New(d Deps) (*Service, error)
```

The error return pays back the positional constructor's compile-time arity check by validating
required fields at construction and naming what's missing. `Deps` holds no logger, no tracer, and no
cross-domain interface until a layer that needs one exists. A cross-domain invariant that would run
upward is an interface the consuming domain declares and injects at the composition root, and the
domain's own `Deps` struct is that injection point.

## 7. Cross-service authorization and staged resolution

### The boundary

The consuming service declares the question it needs answered; the owning service answers it from
its own data, under its own rules, with configuration in place of the composition root and an HTTP
contract in place of an injected interface. A client to another service is a capability-named
translation file, the same shape as any other infrastructure integration. A call with no end user (a
reactor, a scheduled job) carries the calling service's own machine subject, holding ordinary grant
rows in the callee exactly like a human subject.

### Staged resolution

A stage is one call, answered entirely from one service's own database; nothing about how a service
authorizes its own multi-table joins changes. Within one service, the single-statement scope
predicate is simply correct, with no boundary to stage across; the rule concerns only what happens
when the next fact a caller needs lives in a different service's database.

- A **key** becomes a parameter of the next call: a selected organization id filtering the next call
  for its people.
- A **predicate set** becomes a `WHERE` term of the next call: the mechanism that resolves an
  authorization question spanning services.
- A result set to be narrowed after the fact never crosses: a composer that fetches a page and then
  discards rows against a fact from another call holds a page whose total is already wrong.

Applied to a subtree-scoped question once organization and a consuming domain are separate services:

- A selected unit resolves through one containment call, cached per `(granted unit, requested unit)`
  and invalidated only by a transfer. This caches a decision the owning service made, not its
  tables, so it stays inside the API-only rule.
- With no unit selected, the consuming service asks the owning service to expand the subject's
  granted units to their descendant set and binds the result as one array-typed parameter in its own
  read model. The call is bounded by the tree's node count, not by record count, for the same
  structural reason the closure table is cheap: a service-wide grant, the case that would otherwise
  expand to the whole tree, drops the predicate entirely.

### Beyond staged resolution

A remote sort key, or a filter on a record-cardinality attribute another service owns per record
(not organizational containment), composes as a predicate at no scale. Both are reporting and
search, not live authorization: served, when a real need exists, by a materialized, event-driven
read model off the authorization path, advisory and stale-tolerant, with any action on a row it
surfaces re-authorized by the owning service at the point of action.

## 8. Object storage

### The file grain

Every object needs a SQL row regardless of authorization (content type, size, soft delete, audit,
lifecycle), since an object store cannot itself be queried. The row lives in the virtual-directory
library's tables, which hold no owner and no unit (`blobfs`'s `docs/concepts.md`). The consuming
domain's join table references the file's row and carries the domain's `unit_id`, so it carries the
same scope predicate as any other read model at no additional cost; at the file grain the library's
tables are joined detail and the join table anchors the authorized read.

### The directory grain

An owner row binds one top-level directory to a unit, and a scoped listing checks that row once, at
the top-level ancestor of the path being read, before the path resolves further; the library's own
directory-anchored listings run unchanged beneath that check and carry no ownership predicate of
their own (go-web-service's storage domain, `domain/organization/storage.go`). Because the owner row is read only at that one
depth, carrying an entry across two top-level directories would move it out of one unit's scope and
into another's without either unit's say, so a move never crosses them. A unit's right to move
within its own scope, and whether that right needs a check beyond the structural one, is
authorization, proven in the storage domain's auth sweep rather than by the virtual-directory
library.

### The store and the bytes

The object store holds no per-object ACLs and no bucket policies keyed to end users. The service
holds one credential per environment and is the sole authority, because S3's and Azure Blob's
per-object authorization models share nothing standard tier could express without putting the
security-critical decision on the port list, in a second dialect, for the second provider to
re-derive.

An organizational path must never be encoded into a key and authorized by prefix match, the mistake
`ltree` and Keycloak's group-path policies both make: object stores have no rename, so a unit
transfer would mean copying and deleting every object beneath it. Containment is tested against an
index, never a string prefix.

Proxying bytes costs service bandwidth, acceptable for the small, bounded objects the reference
service serves. If large files become a real need, the strategy stays proxying, extended with
compression and chunked streaming rather than a redirect; live media streaming (audio or video
signals, standard files or live) is a separate concern, out of scope. Uploads go through the service
for the same reason, symmetric with reads, and the service validates content before it reaches
storage.

## 9. Deferred work

- **A relationship store.** The trigger is a person granting a named individual access to one record
  outside their unit. Nothing else on this architecture's product shape reaches it; staged
  resolution handles the authorization-staging case that would otherwise be the strongest candidate.
- **A replica of the containment table.** Measurements back the call rather than it being a first
  answer. PostgreSQL logical replication is evaluated before an application-level outbox, since the
  replicated data (a tree and a grant set) is small, low-churn, and snapshot-consistent: a full
  periodic refresh is self-consistent regardless of what was missed, unlike a relationship stream.
- **Extracting organization and its grants.** It stays local to whichever service builds it first.
  The only statements touching authorization tables are the two published patterns and the resolving
  middleware's queries, so extraction rewrites two pattern files per service rather than every read
  model that uses them.
- **The administration control plane.** A registry of active services, an aggregated read view built
  from grant-change events, and delegated grant mutation through token exchange, never acting on its
  own authority. Nothing beyond the identity-link and grant-change event obligation is committed.
- **Live orchestration of staged hops.** Composing the staged hops of a single request into one
  response graph, each hop still independently authorized, is a separate question from the
  materialized read model, held in `staged-query-aggregation`.

Identity-link events, not only grant events, are what let a future aggregate view correlate one
person across services on the one globally stable key available, the `(issuer, subject claim)` pair.

## 10. Alternatives considered

### ABAC

Cedar, or Rego over subject, resource, action, and environment attributes. It decides about a
resource already in hand; the dominant read here is a paged collection, where the question is which
of many rows a subject may see. Answering that with ABAC means either fetching everything and
filtering in the application, wrong at any size and incompatible with a correct paging total, or
partial evaluation of the policy into a database filter, making a policy engine responsible for
generating SQL this architecture deliberately authors by hand. It is also a second DSL-driven
service (the DSL-driven services principle's "The two categories of infrastructure service" names
Rego and Cedar as exactly this category): a second language with expressive content the host cannot
type-check, its own runtime, release cadence, and vulnerability history, on the request path of
every endpoint. Revisit only for a genuinely attribute-shaped, resource-in-hand requirement
(time-boxed access, break-glass, IP restriction), each expressible as a condition column on a grant
row, needing no policy engine.

### An externalized relationship store

Zanzibar's model: SpiceDB, OpenFGA, Ory Keto. The relations this service authorizes against are
database-enforced invariants (custody's partial unique index guaranteeing one open row per instance,
the organization tree's sibling-scoped unique codes and version column) that no relationship store
expresses. Adopting one means mirroring them, and a mirror is a permanent dual write (an outbox, a
strictly serialized relay, a reconciliation job) in every service that adopts it, whether or not
that service has a cross-service relation. The decisive technical objection is independent of that
cost: every candidate documents a hard ceiling on filtering a paged collection by permission
(SpiceDB's `LookupResources` degrades past roughly 10,000 permitted resources, OpenFGA's
`ListObjects` past roughly 1,000) exactly where scale would motivate adopting one, and the fix
(SpiceDB's Materialize) is commercial-only. The swap class across candidates is schema-bound:
SpiceDB's, OpenFGA's, and Ory's schema languages are three incompatible languages, so committing to
one on the information one service has about a fundamentally cross-service problem is the expensive
mistake this architecture avoids elsewhere by keeping SQL the one portable artifact (the DSL-driven
services principle, "Which artifact is portable"). If this ever becomes the right tool, OpenFGA is
the better candidate: per-store isolated, immutable versioned schemas callers pin explicitly, versus
SpiceDB's one mutable global schema per cluster with no per-caller pinning.

### Keycloak's native Authorization Services

The UMA 2.0 resource-server model. It is Keycloak's own API, unambiguously native tier, and Entra,
the named second provider, has no equivalent; building authorization on it would put the entire
authorization model, not one predicate, on the port list. It also cannot reach live per-transaction
data (a custody fact that changes on every issue and return) without an undocumented internal SPI
requiring a server rebuild per policy change, and its group-policy containment mechanism is a string
prefix match on the group path, the same failure as a materialized text path. Most decisively,
Keycloak built the partial-evaluation mechanism that would let it filter a permitted-resource
collection and reserved it for its own admin console, never exposing it to application resource
servers; its own standards effort (AuthZEN) lists implementing the search API as an explicit
non-goal.

### PostgreSQL row-level security

Attractive on the surface (policies on the tables, impossible to forget, zero sqlate change), but it
makes the entire authorization model engine-bound rather than one predicate, worse than the `ltree`
case it parallels. Pooled connections need `SET LOCAL` inside a transaction while reads do not run
in one; the policy lives in DDL, invisible at the review of a domain operation; and the admin
service and seeder both need bypass paths.

### Fetch-then-check on a single-row read

Read unfiltered, evaluate in Go, map to 404. No sqlate change, but it does not generalize to the
list path, which must filter in SQL regardless, leaving two authorization mechanisms with two
chances to disagree instead of one pattern applied identically on both paths.

### ltree and a materialized text path

`ltree` is a Postgres extension (native tier, entering the port list for the single hottest, most
security-critical predicate in the service), couples the organization's code charset permanently to
an index's label grammar, and produces a separator mismatch against the HTTP contract's
`/`-delimited path. A materialized text path loses exactly where it must win: its prefix-scan
optimization needs a plan-time-known prefix, and the authorization predicate's prefix comes from a
joined grant row at runtime, so the subtree test degrades to a scan.

## v1.auth

- The admin listener (`v1.admin-listener`) consumes the authentication this goal settles.
- The storage domain's auth sweep proves what the blobfs experiment cannot: the consumer's join
  table driving a blobfs listing under the scope predicate.
- `v1.client.reference` lands first, on the open API, so this goal's client sweep task puts
  the reference client behind authentication and decides whether an image is fetched as an `<img>`
  with a cookie or with a bearer token.

## v1.admin-listener

- It waits on auth and observability because each choice it would make (the confirmation token's
  mechanism, the listener's authentication, the audit record) is one those layers make.
