# Standards and the principle hierarchy

How the organization's principles are scoped and how a set of repositories is grouped under a
named standard. The hierarchy itself — architecture, standard, module — and
its definitions are documented in the [docs landing zone](https://github.com/standards-lab/docs);
this note records the declaration mechanics the landing zone does not.

Vocabulary: an **architecture** is a technology-agnostic definition of a domain's elements and
rules; a **standard** is a technology-specific implementation of an architecture; a
**principle** is a singular convention attached to any level. The narrowing rule binds the
levels: a lower level enhances a principle it derives from and never loosens it. Declaration and
review are the mechanism throughout; there is no enforcement machinery.

## Where each level is declared

- **Definitions** live in the landing zone: the architecture at `architecture.md` with its
  principles in `principles/`, and a standard in `standards/<key>/` with its principles and
  module pages beneath it.
- **The catalog** (`references.toml`) is the machine-readable declaration: a `[standards.<key>]`
  entry names the standard's `architecture` and `definition` URL, and a member repository's
  `[repos.<key>]` entry declares `standard = "<key>"` — membership is declared where the
  repository is cataloged and never duplicated in a list. A derived standard declares
  `derives = "<key>"`.
- **A repository** declares its standard in its README's Standard section: a link to the
  standard's definition plus the repository's own principles, stated as enhancements.

## Adoption across standards

A repository belongs to exactly one standard — the one whose author answers for it. Another
standard that finds a member's modules sufficient adopts them as ordinary dependencies at
pinned releases, never as members. This is the downward-dependency principle applied across
standards; no second membership mechanism exists. The rule is documented in the landing zone's
downward-dependencies principle; it is restated here because the catalog is where a violation
would first appear.

## Rollout

`go-elemental` is the first standard, defined in the landing zone with its members declared in
the catalog. `dotnet-minimal` is anticipated as its derived standard.
