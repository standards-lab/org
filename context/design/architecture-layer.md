# The architecture layer

How the workspace's written knowledge divides between the architecture and each repository.

## One architecture repository per workspace

The workspace keeps its architecture in one repository, named by the `repo` key in the
`marathon-architecture` extension's own configuration: the Elemental Architecture, its
principles, each standard's definition and principles, the harness principles, and a catalog of
the repositories that implement them, with a description and a link for each and nothing
deeper. The repository is a `context` project, its tree is the architecture, and a README in
every directory is the index GitHub renders.

The architecture holds only what has generalized past one repository. Anything a reader can
infer from a repository's source does not belong in it; anything that serves as a general
guideline or development principle does. That is what keeps the architecture stable while the
repositories change beneath it, and a page that restates a repository's implementation is a
defect, reduced to the principle it states or removed.

## Each repository documents itself

A repository's implementation is documented in the repository: its README, its package
documentation, and its source. An optional `docs/` directory, indexed by the README in reading
order, is the accessibility layer over those three, the guide a person reads to use or
contribute to the repository, and the module zip carries it with the code. sqlate is the
pattern: a README that indexes a `docs/` guide of concepts, a quick start, the features, and a
glossary. The code is the documentation's source of truth, a page the code moves out from under
is fixed in the change that moved the code or in the next documentation step, and a
documentation step is a `start` step like any other built work. The Go repositories write
their guides as one task at v1 completion (`v1.repository-docs`), so no layer's documentation
is revised while the roadmap still churns it.

## Knowledge arrives by promotion

A page reaches the architecture through the promotion sequence and only by it: a concept in
the repository that owns it, a design note once it settles, and a page once the design has
generalized past that one repository. The promotion is a cross-repository step: a member's
`close` or `review` finds that a design note has generalized and lands it as a concept in the
architecture repository, recorded under Cross-repo, and the architecture repository authors
the page in a session of its own. The member's `context/` then links the page instead of
restating it, and repository context records only working knowledge the architecture and the
code do not express.

A repository links the principles it follows from its README's Standard section. A repository
that tightens a principle states that enhancement beside its link, the way go-core states that
it admits the standard library alone: a lower level enhances a principle it derives from and
never loosens it.

## The catalog is the only reach into a repository

The architecture's catalog names each repository with its tier, a one-sentence purpose, and a
link, and the references catalog (`references.md`) states purpose and points at the README for
the packages. Neither restates a package inventory. The profiles and the orientation brief link
the architecture and the repositories the same way.
