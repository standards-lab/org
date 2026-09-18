# reset · v1-storage-azureblob

- **Status:** closeout
- **Session:** start
- **Project:** go-storage, architecture, standards-lab
- **Branch:** v1-storage-azureblob

## Disposition

- **Authored:** the `azureblob` sub-module, the `storagetest` package, and an all-or-nothing `Put`
  contract in `go-storage`, then both first releases. PR #2 merged as `a44c55e`. `go-storage`
  `v0.1.0` and `azureblob/v0.1.0` were tagged from `main` at `78363ac` and `b506432`, and both
  release workflows succeeded. `Client` gained `EnsureContainer`, and `Store.Start` now ensures
  the container before it probes. `Store` also gained `EnsureContainer` and `Container`, and
  enforces a declared `PutOptions.Size` on the body. `Object.ETag` is an HTTP entity tag on every
  call. `azureblob` uses `azblob` v1.8.1. Validated: build, vet, test, tidy, and lint pass across
  both modules; the conformance suite passes against Azurite started with
  `--skipApiVersionCheck`, including the mid-body failure and `Size` mismatch cases; and a
  scratch module outside the workspace pulled both tags from the proxy and drove a `Store` over
  `azureblob` through the full lifecycle.
- **Integrated:** `design/storage-strategy.md` now records what the build settled. §2 carries
  `EnsureContainer`, the `Start` behavior, the atomic `Put` contract, and the entity-tag form. §4
  drops the `go-storage/admin` package and states that the admin domain is
  `go-web-service/admin/storage`. §6 states the object write is atomic. §7 reverses the rejection
  of `Start` creating the container and adds two rejected alternatives, a base `admin` package
  and a `Provisioner` interface. A new Assumptions section names three unverified claims. The
  record stays, because `blobfs` and the storage service build to it. In `go-storage`,
  `context/README.md` lost the two adapter cautions and the "waits for `azureblob`" sentence,
  which the contract, the suite, and the package now express.
- **Promoted:** nothing. The all-or-nothing `Put` contract and the provider conformance suite were
  designed this session, and a contract waits for a second exerciser before it leaves its
  repository.
- **Culled:** nothing.
- **Retained:** the atomic-write contract and the conformance-suite convention, in
  `design/storage-strategy.md` §2 and `go-storage`'s package documentation, until an S3 provider
  or another library's providers exercise them.
- **Corrected:** three claims in the notes and the plan proved wrong. The strategy record's
  short-graph claim for `azblob` was wrong for v1.8.1, which brings the Arrow packages; the record
  now leaves the SDK's indirect dependencies to the SDK. The plan's note that Azurite leaves ETags
  unquoted was wrong: the service quotes them in headers and leaves them unquoted only in a
  listing's XML, as Azure does. The plan's premise that `release.yml` handled only `v*` tags was
  wrong, since it already handles `**/v*`.
- **Cross-repo:** in `architecture`, the `go-storage` row joined the Go Elemental member table
  and the paragraph of libraries still to be created dropped storage (branch
  `v1-storage-azureblob`). In `standards-lab`, `references.md` moved `go-storage` from in progress
  to released, `concepts/blobfs.md` now says `go-storage` and `azureblob` are built, and
  `concepts/admin-listener.md` gained a posture question on provisioning in the serving role.
  `roadmap.toml` lost `goals.v1.storage.tasks.azureblob` and its `next` entry. The `v1.storage`
  summary now states what remains, and `v1.storage.service` gained the
  `go-web-service/admin/storage` domain and the `--skipApiVersionCheck` requirement for its
  Azurite compose service.

## Next-focus

`blobfs.design`, in `standards-lab`: a `plan` session that settles what
`context/concepts/blobfs.md` left open. It weighs how a consuming service's read models join
against library-owned schema, how `blobfs` ships its schema and migrations for a consumer to
adopt, whether it needs its own engine sub-module or authors portable SQL through the consumer's
`sqlate` instance, and its final module path and taxonomy placement. It changes no code.
`blobfs.experiment` and `blobfs.build` follow, then `v1.storage.service` and `v1.storage.suite`.
