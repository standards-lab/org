# Object storage

The strategy for object storage across the reference architecture: the `go-storage`
infrastructure library, its standard-tier interface derived from what Azure Blob and S3 share,
the `azureblob` provider, and the write-path guarantee every consumer builds against. Every
capability goal beneath `goals.v1.storage` builds to the contract this record states.

This is a strategy record. It contains the principles, the reasoning that produced them, and the
shape of the result. Implementation detail lives elsewhere: `go-storage`'s own `doc.go`, README,
and CHANGELOG for what it ships and at which version; `auth-strategy.md` §8 for the authorization
posture every object read and write already obeys (authorize the record, proxy the bytes, never a
presigned-URL redirect); `standards-lab/context/concepts/blobfs.md` for the virtual-directory
metadata layer this record deliberately does not design — `go-storage` stays the object protocol
alone, and the SQL-backed hierarchy above it is a separate capability's concern.

## 1. Scope and shape

No formal standard exists for object storage, so `service-tiers.md` has the organization
establish the common interface itself: derived from what the target APIs share, kept minimal, and
validated against more than one provider before it is called standard. That validation is
deliberately incomplete at v1. `service-organization.md` names the anticipated pair — an Azure
Blob provider (azurite ↔ Azure Blob) and an S3 provider (minio ↔ S3) — but `backlog.second-providers`
holds the S3 provider as a post-1.0 candidate, proven by its own focused reference once a real
consumer earns it, the same bound `service-organization.md`'s co-evolution section states for
every capability: the reference service never grows a second provider of the same service. Azure
Blob is the sole v1 provider, azurite the local instance. The standard-tier interface below is
derived from both target APIs now, so the eventual S3 provider is an adapter rather than a
redesign, but until that provider exists the interface is the organization's proposed standard
tier, not yet a validated one, and this record states that plainly rather than overclaiming.

An interface is also the least reversible artifact this goal produces: every future provider
implements it, and widening it later is a breaking change for each one. The operation set below
was kept deliberately narrow for that reason.

## 2. The standard tier

`dsl-driven-services.md` §2.1 already classifies object storage as protocol-driven: the
consumer calls operations with typed arguments, and the provider boundary is an interface over
those operations. Nothing here is DSL-driven; there is no text artifact and no dialect axis.

The base module, `github.com/standards-lab/go-storage`, package `storage`:

```go
type Object struct {
    Key         string
    Size        int64
    ContentType string
    ETag        string // an HTTP entity tag, the same string on every call
    ModifiedAt  time.Time
}

// Blob is an open read: the object's metadata and its byte stream.
type Blob struct {
    Object
    Body io.ReadCloser
}

type PutOptions struct {
    ContentType string
    Size        int64 // the body's length when known; 0 means unknown; when set it must match the body
}

// GetOptions is reserved for options on Get. It is empty on purpose: a byte
// range is the anticipated first field, and the parameter exists now so
// adding it later changes no signature.
type GetOptions struct{}

type ListOptions struct {
    Prefix string
    Token  string // an opaque continuation token
    Limit  int
}

type Page struct {
    Objects []Object
    Next    string
}

// Client is the standard tier: the operations common to Azure Blob and S3.
// A provider sub-module implements it over its own SDK.
type Client interface {
    Put(ctx context.Context, key string, body io.Reader, opts PutOptions) (Object, error)
    Get(ctx context.Context, key string, opts GetOptions) (Blob, error)
    Stat(ctx context.Context, key string) (Object, error)
    Delete(ctx context.Context, key string) error
    List(ctx context.Context, opts ListOptions) (Page, error)
    EnsureContainer(ctx context.Context) error
    Probe(ctx context.Context) error
    Capabilities() Capabilities
}
```

Three narrowings carry the reasoning.

**No conditional writes.** S3 gained `If-None-Match`/`If-Match` on `PutObject` only in 2024, and
coverage across S3-compatible stores is uneven; `service-tiers.md` requires validating an
operation against more than one provider before calling it standard, and this one is not
validated. Nothing in the reference service needs it: a stored object's own concurrency question
is answered by the owning SQL row's version column, never by the object store's ETag.

**No object metadata on the interface.** Azure blob metadata keys must be valid C# identifiers
and normalize case on the round trip; S3 user metadata takes an `x-amz-meta-` prefix and
lowercases keys. The two do not round-trip identically, and nothing needs them to: every object
already needs a SQL row regardless of authorization (`auth-strategy.md` §8), and that row is the
metadata authority. Only `ContentType` stays, carried as an HTTP header with identical semantics
on both target APIs.

**`Probe` and `EnsureContainer` are not object operations.** `Probe` answers whether the
configured credential and container are reachable — Azure's `GetProperties` on the container,
S3's `HeadBucket`. `EnsureContainer` creates the configured container and succeeds when it
already exists — Azure's container `Create`, S3's `CreateBucket`, both of which report an
existing container distinguishably. Both exist for `Store`'s lifecycle wiring below and for the
admin domain, not for anything else a consumer calls. `EnsureContainer` is standard tier because
provisioning is the first operation an empty backing store needs, and widening `Client` after a
provider exists breaks that provider.

`PutOptions.Size` is explicit because the two SDKs disagree about needing it: Azure's upload
chunks an unknown-length reader without help, while the AWS SDK needs either a seekable body or a
known length to sign the request. Passing the request's `Content-Length` straight through when
it's known, and leaving it zero otherwise, keeps the common path free of buffering while letting
a provider buffer only when it must.

`Put` is all or nothing. On any error nothing is written at the key and an existing object is
unchanged, and on success the object holds exactly the bytes the body yielded through EOF. A
declared `Size` greater than 0 must equal the body's length, and a mismatch is an error that
stores nothing. Azure's `Put Block List` and S3's `CompleteMultipartUpload` make an upload
visible only at commit, so a provider over either can keep the contract, and the `storagetest`
conformance suite proves that each does. `ETag` is an HTTP entity tag, so a service can send it
as an `ETag` header, and `Put`, `Get`, `Stat`, and `List` report one string for one version of an
object; Azure leaves the tag unquoted in a listing's XML, so the adapter quotes it.

### `Store`, the lifecycle wrapper

```go
func New(c Client, cfg Config) *Store
func (s *Store) Start(ctx context.Context) error   // EnsureContainer, Probe, then mark started
func (s *Store) Shutdown(ctx context.Context) error
func (s *Store) Ready() bool
func (s *Store) EnsureContainer(ctx context.Context) error
func (s *Store) Container() string
```

`Store` implements `Client` itself, delegating to the provider after a not-ready check and after
enforcing `Config.MaxObjectSize` on a `Put` body. This differs from `database.DB`, which wraps
the pool but runs no statements, because `sqlate` is `go-database`'s call surface and object
storage has no equivalent above it — `Store` is the only call surface `go-storage` has. `Start`
ensures the container exists and then probes, both bounded by `Config.RequestTimeout`, and fails
startup when the store is unreachable, matching `DB.Start`. The container is required
configuration, so an empty store starts cleanly. `Store.EnsureContainer` repeats the step on
demand, without a readiness check, so a container deleted while the process runs can be
recovered; `Store.Container` names the configured container. `Ready` is a live probe bounded by
`Config.RequestTimeout`, so readiness recovers after an outage; it satisfies
`lifecycle.ReadinessChecker` structurally and registers at stage 0 beside the database.
`Shutdown` closes the provider once when it implements `io.Closer`.

The size bound applies to every `Put` body. A declared `PutOptions.Size` over the bound is
rejected before the provider is called, and any other body is read through a reader that fails on
the first byte past the bound, because a declared size is the caller's claim and the bound exists
for untrusted bodies. `io.LimitedReader` cannot do this: it reports `io.EOF` at the limit, which
makes an oversize body look like a complete short one. A declared `Size` is enforced through a
second reader that ends a short body in `io.ErrUnexpectedEOF` and fails a long one on the first
byte past `Size`, so a mismatch reaches the provider as a read failure before it can commit. The
bound is the inner reader, so a body past a `Size` equal to the bound still reports
`ErrTooLarge`. `Store` cannot undo a commit, so beyond that check it relies on the provider's
atomicity.

### `Capabilities`

A provider declares a small set of facts about its target API rather than the consumer assuming
them, the same shape `go-database`'s `admin` package already uses for a non-SQL provider fact
(the server-version read is "a dialect capability the `admin` package declares and
`sqlate/postgres` implements"). This is deliberately not called a dialect — that term stays
reserved for SQL rendering across SQL engines (`dsl-driven-services.md` §2.1) — and it is
structured to grow, not scoped to one fact:

```go
// Capabilities is what a provider's target API requires of a key, and room
// for further per-provider facts (chunking thresholds, and so on) as they
// earn a place here.
type Capabilities struct {
    MaxKeyLength int
    ValidateKey  func(key string) error
}
```

Every provider declares its `Capabilities` as a `Client` method, and a consumer reaches them
through `Store.Capabilities()`. A consumer that constructs keys from a variable, human-supplied
segment — `blobfs`'s filename suffix is the worked case — calls `ValidateKey` against the active
provider's `Capabilities` before ever reaching `Put`, so a key a future S3 provider would reject
fails at construction, not at the object store. `MaxKeyLength` states no unit, because the target
APIs may measure a key differently; each provider's `doc.go` states its own.

### Errors

Mirroring `go-database/errors.go`'s dual-wrap form, `fmt.Errorf("%w: %w", sentinel, err)`:

- `ErrNotFound` — no object at the key.
- `ErrTooLarge` — a body exceeding `Config.MaxObjectSize`, rejected by `Store` before it reaches
  the provider.
- `ErrNotReady` — a call before `Start` or after `Shutdown`.
- `ErrUnavailable` — the store is unreachable.

Classification is the provider adapter's job, and it is the most load-bearing thing the adapter
does: Azure raises `bloberror.BlobNotFound` on a missing key, while S3's `DeleteObject` answers
204 on one — normalizing both into `ErrNotFound` (get, stat) and a no-op success (delete) in one
place is what makes the capability's swap-cost class hold (§6).

### Config

On go-core's Merge-and-Finalize contract, the same shape as `database.Config`:

```go
type Config struct {
    Endpoint       string
    Container      string
    Account        string
    Key            string
    Options        map[string]string
    MaxObjectSize  int64
    ListPageSize   int
    RequestTimeout *config.Duration
}
```

`Container`, not `Bucket` — "bucket" is S3's own coinage, and the standard tier takes neither
provider's product vocabulary; the eventual S3 provider maps it to the bucket name and states so
in its own `doc.go`. `Key` rides the secrets layer of `config.Load`, as the database password
does. Credentials are shared-key at v1, which azurite supports directly; `azidentity` and managed
identity are deferred to the deployment goal, keeping MSAL out of `azureblob`'s graph for a
capability nothing uses yet.

`MaxObjectSize` and `ListPageSize` have no default. `baseline-standards.md` says a library ships no
policy numbers, so the application supplies both, and each is 0 when unset: unbounded for
`MaxObjectSize`, the provider's own page size for `ListPageSize`. `RequestTimeout` is the one
default, 10 seconds, because it bounds only the calls `Store` makes on its own behalf in `Start` and
`Ready`; in `Start` it covers the container check and the probe together. The object operations take
their timeouts from the caller's context and the provider's transport. `Container` is the one
required field.

## 3. Standard versus native, and the port list

Native tier is everything reached through the provider sub-module's own handle: leases, access
tiers, snapshots and blob index tags, server-side copy, and — explicitly, regardless of how
convenient it looks — presigned URLs and SAS tokens. `service-tiers.md` is direct about this
last one: "a feature that several providers happen to share but no standard defines is still
native," and `auth-strategy.md` §8 forbids it on the request path outright, so nothing in the
reference service reaches for it.

The projected port list at v1 is empty, the same state OpenTelemetry's section of `stack.md`
holds: the service uses the standard tier throughout, and the azurite compose service is
operational configuration for a chosen tool, not a native-tier artifact.

## 4. Module layout and the dependency line

```
github.com/standards-lab/go-storage            base module, packages storage and storagetest
    standard library + go-core
github.com/standards-lab/go-storage/azureblob   provider sub-module, package azureblob
    + Azure SDK for Go's storage/azblob
```

`topology-and-naming.md` states the shape for every infrastructure library — one base module
presenting the standard tier, each provider its own nested sub-module — and
`dependency-sourcing.md`'s obligation on isolating sourced weight names exactly this: the base
module never imports the provider's SDK. The provider sub-module is named `azureblob`, for the
target API, never the driver it wraps (`azblob`) or the platform alone (`azure`, which would be
the wrong name the day a second Azure storage API entered this repository).

The Azure SDK clears `dependency-sourcing.md`'s markers: stdlib types at most of its boundary
(`context.Context`, `io.Reader`/`io.ReadCloser`, with its own response structs the adapter absorbs),
a transitive graph that stays inside the provider sub-module (shared-key credentials exclude
`azidentity`, and the rest of the graph is the SDK's own to choose), maintenance by Microsoft as the
API's own vendor, a stable major version since 2022 with a changelog that is mostly fixes, and
correctness against an external specification — shared-key HMAC canonicalization and retry against
server-side throttling — that a hand-rolled implementation would get wrong in the CORS-shaped way
`dependency-sourcing.md` warns about.

### Container provisioning and diagnostics

Azurite starts with no container, and creating one is not an object operation, so it lives on
`Client` as `EnsureContainer` (§2). `Store.Start` calls it, so the required container exists
before the store reports ready, and `Store.EnsureContainer` recovers one deleted while the
process runs. There is no `go-storage/admin` package: the operations hold no policy, only a
forwarded call and reads of `Ready`, `Capabilities`, and `Container`. The admin domain is
`go-web-service/admin/storage`, a route group and its request and response types over `Store`,
mounted into the `/admin` mount the way `admin/database` is. It gives the harness the state
control path `testing-hierarchy.md` already requires: "State control goes through the
operator's surface; a case starts from a state it made." Whether the serving role should hold
permission to create the container is a posture question `admin-listener.md` holds.

## 5. Swap-cost class: interchangeable with review

`service-tiers.md` lists object storage's common operations under Interchangeable and object-store
consistency under Interchangeable with review. Read as naming an operation set and a behavior set
separately — the operations swap by configuration, but the behavior beneath them is not fully
specified by the operation set — a capability declares by their union, and object storage's is
**interchangeable with review**, the class auth and observability each declared for their own
capability. Three named review items:

- **Read-after-write and list consistency.** Both Azure Blob and Amazon S3 are strongly
  consistent for read-after-write and list; an S3-compatible store is not uniformly so, and a
  provider move to one is a move to a store whose consistency has to be re-checked.
- **Delete idempotency.** Normalized in the adapter (§2, Errors), but the underlying behavior
  differs per provider and is worth naming, not just hiding.
- **Large-object chunking thresholds.** Azure block blobs cap a block at 4000 MiB; S3 multipart
  caps a part at 5 GiB with a 5 MiB minimum on every part but the last. Nothing in the reference
  service exercises this at v1 — its objects are small and bounded — so it is named, not designed
  for.

Two items that would otherwise be on this list are closed by convention rather than left as
review items: key naming (§6, `blobfs.md`'s opaque UUID-prefixed key sits inside both providers'
key-character envelopes by construction) and object metadata (§2, kept off the interface
entirely, so there is nothing to compare across providers).

## 6. The write path: two phases, not one transaction

No transaction spans Postgres and an object store, so nothing here can claim atomicity between a
SQL write and an object write — `testing-hierarchy.md` previously described a worked example in
those terms, and that phrasing is corrected alongside this record landing (§8).

The owning SQL row is written first, in a `pending` status; the object is written second, and
that write is atomic, so a failed write leaves no partial object; a second transaction moves the
row to `available`. Delete mirrors it: the row moves to `deleting`
and commits, the object is deleted, the row is removed. Writing the object first and inserting
the row after was considered and rejected — a failed insert then leaves an object nothing
references and nothing records, findable only by listing the entire container. A row written
first in an incomplete state is findable by an ordinary query instead.

This leaves one gap, stated rather than hidden: a crash between the row's `pending` insert and
the object write leaves that row `pending` with no automatic reconciliation. No sweeper ships at
v1. The row is queryable, which is what makes deferring the sweeper survivable rather than
merely convenient, and a future reconciliation job — a natural fit once `v1.messaging`'s event
infrastructure exists — is the named trigger for closing it, not a silent omission.

The row itself, its schema, and everything about how a consuming service composes ownership over
it are `blobfs`'s concern (`standards-lab/context/concepts/blobfs.md`), not this library's.
`go-storage` supplies the object half of the two-phase write; the SQL half belongs to whatever
owns the schema.

## 7. Alternatives considered

**Conditional writes on the standard interface** (`If-Match`/`If-None-Match`). Rejected in §2:
uneven S3-compatible coverage, and nothing needs them once concurrency is the SQL row's version
column's job.

**Object metadata on the interface.** Rejected in §2: the two providers' metadata key grammars
don't round-trip identically, and the SQL row is already the metadata authority regardless.

**A richer interface generally** (copy, move, signed URLs). Rejected on `service-tiers.md`'s own
governing sentence: the tier "is never the a-priori intersection of providers... nor the union of
their features." Five operations plus a probe is the set the reference service's actual use
needs, each verified present with matching semantics on both target APIs.

**Both providers at v1.** Rejected: `backlog.second-providers` holds S3 as post-1.0, and
`service-organization.md`'s co-evolution rule bounds the reference service at one provider per
service.

**A one-shot compose init service creating the azurite container**, in place of
`Store.Start` doing it. Rejected: it keeps provisioning out of both the library and the serving
role at the cost of one more compose service with no operator-facing path. The container is
required configuration, and a process that exits before anything can create it is the worse
failure, so `Start` ensures it (§2). The cost is that the serving credential needs permission to
create the container. A least-privilege deployment would need an opt-out, which is an additive
`Config` field and waits for a consumer that needs it.

**A `go-storage/admin` package mirroring `go-database/admin`.** Rejected: `go-database`'s admin
service owns policy and state (a seeder, a startup set, a lifecycle stage, a migration lock),
while a storage admin service would forward one call and compose three reads, and
`topology-and-naming.md` forbids splitting a package by topic alone.

**A `Provisioner` optional interface for container creation.** Rejected on the reasoning above
for `Capabilities`: a provider could skip it silently, and adding a method after a provider
exists breaks that provider while adding it now breaks none.

**Object-first writes.** Rejected in §6: the failure mode is an unreferenced object discoverable
only by listing the whole container, worse than a queryable `pending` row.

**Plain "interchangeable," or "schema-bound."** Rejected in §5: the former ignores real
consistency and idempotency differences across providers; the latter overstates the cost — none
of the review items touch application Go code, which is what keeps the class at "with review"
rather than sliding toward a port.

**Default caps, or a required `MaxObjectSize`.** Rejected: `baseline-standards.md` names a default
page size and a maximum request size as policy the application owns. A required bound would force
a number on every consumer, including ones that have no reason to set one.

**Pointer fields for the two sizes.** Rejected: an explicit zero has no meaning distinct from
unset, so the plain values with 0 as unset suffice. The cost is that an overlay file cannot lift a
bound back to unbounded, and the environment override still can.

**`Capabilities` outside `Client`, through an optional interface.** Rejected: a provider could
silently skip key validation, which is the case `blobfs` exists for. Adding a method later is a
breaking change for every provider, and with no provider built yet the change is free now.

**A `ProbeTimeout` field, or one client timeout covering every operation.** Rejected: one timeout
over a streaming upload of tens of megabytes would be wrong, and the transport timeouts belong to
the provider adapter. `RequestTimeout` keeps its name and bounds only the calls `Store` makes on its
own behalf.

## 8. Record

This strategy was settled through a `plan` session, given `v1.storage`'s position leading the v1
sequence — the same position `v1.observability` and `v1.auth` held when each got a strategy
record of its own. The design reasoning was escalated to an `opus` agent first, the same
escalation path those two goals used; the architect reviewed that reasoning and substantially
revised its most consequential call. The opus draft proposed a literal virtual path as the object
key; the architect rejected it during the session, for the same reason `auth-strategy.md` §10
already rejected a materialized path for organization lineage — object stores have no atomic
rename, so a key encoding hierarchy makes an ordinary reorganization a non-atomic copy-and-delete
storm. That reasoning produced the opaque-key convention `blobfs.md` now carries, and surfaced
that the virtual-directory layer the demonstration needs is significant and reusable enough to
become its own library rather than a task under this goal — recorded there, not here. The full
reasoning trace lives in this session's own record; nothing here restates it a second time.

## Assumptions

Each claim below is unverified, and a build that falsifies one invalidates the text that names it.

- Azure's blob-name rules in `azureblob`'s `ValidateKey` come from Azure's documentation, and
  Azurite accepts every violation, so no real service has corroborated them.
- Azure leaves a blob's ETag unquoted in a listing's XML and quotes it in headers. That is
  verified against Azurite and matches Azure's documented listing sample, not a live service.
- S3's `CreateBucket` reports an existing bucket distinguishably, and S3 keeps an upload invisible
  until it commits. Both are the basis of the standard tier's `EnsureContainer` and all-or-nothing
  `Put`, and neither has been exercised, because no S3 provider exists.
