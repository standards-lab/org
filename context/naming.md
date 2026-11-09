# Naming

How the organization is named in prose and where repository naming is defined.

## The organization

In prose the organization is Standards Lab. The lowercase `standards-lab` is the GitHub slug, and
it belongs only where an identifier is required: URLs, module paths, repository names, and tags.

Write "Go reference libraries for Standards Lab". The forms "the standards lab" and
"standards-lab" do not belong in prose.

This applies everywhere the organization is written about: the profiles, the repository READMEs
and changelogs, license notices, GitHub repository descriptions, and the context notes.

The same pattern names the standards: Go Elemental in prose, `go-elemental` where an identifier
is required. A standard's name carries the architecture it implements — Go Elemental is the Go
implementation of the Elemental Architecture — not an inherited property of that architecture. A
framework-heavy Go standard would not be a competing implementation of Elemental; it would not
implement Elemental at all.

## Repositories

Repository, module, package, and tag naming is documented in the architecture repository:
[topology and naming](https://github.com/standards-lab/architecture/blob/main/standards/go-elemental/principles/topology-and-naming.md),
under the Go Elemental standard. How focused reference architectures are named is not yet a
convention; the first spin-off settles it.

## Capstones

A capstone is an application that brings the workspace's libraries together end to end, built
after the experiments and libraries it needs (plan, 2026-11-09). It is a workspace convention,
not a marathon concept; a marathon proposal waits until the first capstone ships.

- **The goal** is `capstone.<slug>`, under the `capstone` container goal in `roadmap.toml`.
- **The repository** is named by a descriptive slug for what the application does, with no
  prefix: the container and the checkout directory already mark the category, and the name stays
  right if the application outlives its capstone role.
- **The remote** is under JaimeStill, outside the standards-lab organization:
  `https://github.com/JaimeStill/<slug>`.
- **The checkout** is `~/capstones/<slug>`, listed in `[workspace.paths]` in
  `.claude/marathon.toml`, outside `order` and the references catalog.

The first is data-intelligence (`data-intelligence.md`).
