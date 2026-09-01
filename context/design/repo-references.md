# References system

This effort draws on experience built up across many earlier repositories, alongside the organization's
own repositories. The references system records each one with a portable identity and lets every machine
map that identity to a local checkout, so the workspace can be reconstructed anywhere without hard-coding
paths.

## The three files

- `references.toml` — committed. One key per repository, mapping to its canonical remote.
- `references.local.toml` — not committed (gitignored). The same keys mapping to local directories on this
  machine.
- `references.md` — committed. A description of each key: what the repository is and, for prior R&D, what
  to draw from or correct.

The key is the join. To clone a missing repository, look up its remote in `references.toml` and its target
directory in `references.local.toml` under the same key. Locations are never duplicated across the two
TOML files: remotes live only in `references.toml`, local paths only in `references.local.toml`.

## The private annex

Prior R&D from private engagements cannot be catalogued publicly. The private repository (checked
out at `private/`) carries an annex in the catalog's exact format — its own `references.toml`,
`references.md`, and untracked `references.local.toml` — and the catalog is the union: read the
public files, then the annex files when the member checkout is present. The two share one key
namespace; the annex only adds entries, never redefines a public one, so a key resolves
identically wherever it can resolve at all. The public `references.toml` and `references.md`
declare that the annex exists without naming its entries.

## Access caveats

Per-organization access caveats (such as an enterprise account switch required before cloning) are
recorded at the point of use: in `references.md` alongside the affected entries, and in the
`references.toml` header.
