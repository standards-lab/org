# reset · blobfs-sources

- **Status:** closeout
- **Session:** start
- **Project:** sqlate
- **Branch:** blobfs-sources

## Disposition

- **Promoted:** nothing new to `sqlate`'s own `context/` — every shape this session built is now
  fully expressed by `sqlate`'s own code and `docs/features.md`/`quick-start.md`/`glossary.md`/
  `concepts.md`, so there was no unbuilt intent left over to promote there.
- **Integrated (Cross-repo, in `standards-lab`):** `context/concepts/migration-sets.md`
  deconstructed to what's still unbuilt — "A set is a layer," "The migrator in `sqlate`," "The
  hooks `sqlate` needs," and "Where the experiment's shim goes" are gone, all now expressed by
  `sqlate`'s own "migrate: schema versioning" documentation; "What a shipper guarantees," "What
  the admin surface exposes," and "How a consumer adopts a set" retained, since `blobfs.build`,
  `blobfs.admin`, and `go-auth` haven't landed yet. `context/concepts/sqlate-library-support.md`'s
  entire "Scheduled in `blobfs.sources`" section removed — all six areas are built; its Backlog
  and "What v0.2.0 unblocks" sections retained and reworded to present tense.
- **Culled:** the `blobfs.sources` roadmap task, finished, and its entry in `next`.
- **Retained:** `sqlate-library-support.md`'s "Assumptions" trimmed to the two still-open claims
  (`go-auth`'s set fitting the model, no checksum column needed yet); the two validated
  assumptions (the shim's shape moving in unchanged, one connection sufficing) are gone, now
  simply true of the built code.
- **Roadmap (Cross-repo, in `standards-lab`):** `sqlate-library-support.md`'s Backlog gains two
  entries this session surfaced and deliberately deferred: a window-count total mode for the
  collection read (rejected for this release — a `COUNT(*) OVER()` column can't be hidden from an
  arbitrary consumer-supplied `ScanFunc[T]`), and a stricter type grammar for a `field`
  declaration (`sqlType`'s regex is lenient enough that a `not null` typo like `not nul l` is
  silently accepted as a bizarre type name rather than refused — pre-existing, surfaced
  incidentally, out of scope for this release).
- **Architecture layer:** nothing promoted at this close. The candidates the prior session named
  ("What a shipper guarantees," the consumer-adoption pattern) stay deferred pending a second
  shipper (`go-auth`), per that session's own recorded trigger — not met yet.
- **Cross-repo:** the roadmap manifest edit (`context/roadmap.toml`: `blobfs.sources` deleted,
  `next` advanced) and the two `context/concepts/` edits above are this session's own commit in
  `standards-lab`, the coordinator, since the session ran in the `sqlate` member repo.

## Next-focus

`blobfs.build`, in a new `blobfs` repository (doesn't exist yet — this task promotes the
`blobfs` experiment out of `standards-lab/experiments/` into its own repository, per
`context/concepts/blobfs.md` and `blobfs-api.md`). Start there next session.
