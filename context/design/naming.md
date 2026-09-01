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
implementation of the Elemental Architecture — not an inherited property of that architecture;
`design/blueprint-organization.md` records the reasoning.

## Repositories

Repository, module, package, and tag naming is documented in the docs landing zone:
[topology and naming](https://github.com/standards-lab/docs/blob/main/standards/go-elemental/principles/topology-and-naming.md),
under the Go Elemental standard. How focused reference architectures are named is not yet a
convention; the first spin-off settles it.
