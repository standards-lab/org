# The workspace docs tier

How human-facing documentation is organized across the workspace. Settled when the docs landing
zone was initialized; the convention is codified in the marathon skill in its own claude-plugins
session.

## One landing zone per workspace

In a marathon workspace, the architecture's documentation is centralized in one landing-zone
repository, here `docs`, rather than authored per repository: the principles, the standards,
and the pages that describe each member's place in the architecture. A member repository never
grows a `docs/` directory for that. A standalone marathon project, outside any workspace, keeps
its own `docs/` under the same conventions.

## A library's user guide lives with the library

A standalone library the organization builds adjacent to the architecture, one any project can
adopt on its own, keeps its user guide in its own repository: a README that indexes the guide
and a `docs/` directory the README orders. Its audience is outside the organization, and the
module zip carries the guide with the code. The landing zone still documents the library's
place in the architecture, and its page links to the guide for usage rather than restating it.
sqlate is the first case.

The landing zone is where context ultimately strives to end up: a design note that settles and
generalizes migrates into a landing-zone page, and the repository's `context/` links to that
page instead of restating it. Repository context records only working knowledge the landing
zone and the code do not express.

## Enhancements are stated beside the link

The narrowing rule extends to documentation. A repository that tightens a documented principle
states that enhancement beside its link to the principle — in its README's standard declaration
or its context — the way go-core states that it admits the standard library alone. A lower
level enhances a principle it derives from and never loosens it.

## Lifecycles

The landing zone's pages describe what exists, so marathon's decay rule does not apply to them:
a page restating the code is doing its job. A page the code has moved out from under is a
defect, fixed in the change that moved the code or in the next docs session. Repository context
keeps the decay rule: once a note's content lands in the landing zone or the code, the note is
reduced to a link or deleted.
