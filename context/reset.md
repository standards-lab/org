# reset · v1-storage-library

- **Status:** closeout
- **Session:** start
- **Project:** go-storage, standards-lab
- **Branch:** v1-storage-library

## Disposition

- **Authored:** the `go-storage` repository, created public under `standards-lab` and founded
  with the scaffold on `main` (`e0010b8`), then its base module on `v1-storage-library`: the
  error sentinels and the `Client` interface with `Capabilities` and `GetOptions`; `Config` on
  go-core's Merge-and-Finalize contract, pinned to go-core v0.4.1; the in-memory fake, kept in
  `fake_test.go`; `Store`; `doc.go`; and the README, changelog, and `context/README.md`.
  Validated: the mise build, vet, test, lint, and tidy tasks pass, coverage is 97.7%, and a
  scratch program drove `Store` through its full lifecycle over the fake.
- **Integrated:** `design/storage-strategy.md` §2 and §7 now record what the code settled.
  `Client` carries `Capabilities()`, `GetOptions` is defined and empty, `Ready` is a live probe,
  `Shutdown` closes the provider once, and the size bound reads every `Put` body through a reader
  that fails past the bound. `MaxObjectSize` and `ListPageSize` are plain values where 0 is unset,
  `RequestTimeout` is the one default and bounds only the probes, and §7 records the four
  rejected alternatives. The record stays, because `azureblob` still builds to it.
- **Promoted:** nothing. No note generalized past this workspace; the no-policy-numbers and
  readiness rules are already in the architecture repository.
- **Retained:** the record's `go-storage/admin` entry, which names the package without saying
  whether it is a base package or a nested module. The next session settles it.
- **Cross-repo:** `.claude/marathon.toml` gained `go-storage` in the order map, and
  `references.toml` and `references.md` gained its entry, marked in progress. `roadmap.toml`
  lost `goals.v1.storage.tasks.library` and its `next` entry, and the `azureblob` summary gained
  the deferred items. `references.local.toml` and the workspace container's `README.md` were
  edited in place and are not versioned; the README also gained the stale `go-observability`
  entry. On GitHub, `go-storage` was created with `delete_branch_on_merge` on and merge commits
  only, matching its siblings, and `go-observability`'s `delete_branch_on_merge` was set to
  `true` after it was found `false`.
- **Corrected:** the prior reset file guessed that a `[workspace.paths]` entry might be needed.
  The order key resolves to the sibling checkout, so none was.

## Next-focus

`v1.storage.azureblob`, in `go-storage`: the `azureblob` sub-module over the Azure SDK's `azblob`
package, and the `admin` half. Its SETTLE has three things to resolve before the stage list:

- Whether `admin` is a base package over an interface the provider implements, as
  `go-database`'s `admin` is, or a nested module. `EnsureContainer` is provider-specific and the
  base module never imports a provider's SDK.
- Release ordering. The base is unreleased, so it tags `v0.1.0` first, the provider pins it, and
  the coordinated releases close the task.
- The two adapter cautions in `go-storage/context/README.md`: a provider that trusts
  `PutOptions.Size` truncates a longer body silently, and a bounded `Put` body arrives
  non-seekable.

The task also publishes the fake as a `storagetest` package with a conformance suite, adds the
`architecture` repository's member-table row, and moves `references.md`'s `go-storage` entry
from in progress to released.
