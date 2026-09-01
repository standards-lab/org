# Rolling currency: pinned and current

Captured 2026-08-31 during the workspace-sweep session, from the architect's direction after the
sweep pinned golangci-lint across the Go repositories. Candidate universal principle for the
docs landing zone; everything here is direction for the session that authors the page.

## The posture

Components of the architecture keep their minimal dependency sets at the latest released
versions — the spiritual difference between a stable distribution and a rolling release, adopted
deliberately. Staleness is technical debt by definition rather than by accident: the moment a
new release of a language, dependency, or tool is cut, applying the upgrade becomes the front of
the queue.

The posture is pinned-and-current, not unpinned. Every version is explicit — the Go toolchain,
module requirements, CI actions, dev tools in mise and CI alike — so every commit is
reproducible and the gate moves only by a deliberate bump; the stream of bumps itself moves
promptly. The sweep's golangci-lint decision is the worked example: `latest` made the lint gate
irreproducible, while an unmaintained pin would rot against the Go release it must track.
Pinning and currency are two halves of one discipline.

The pairing with the minimal-footprint principle is what makes it tractable: a handful of direct
dependencies can be kept current the week they release; fifty cannot. Each principle justifies
the other.

## Calibrations for the page

- **Immediate priority, not mid-session interrupt.** An upstream release never widens a running
  session. The upgrade becomes its own small step, usually next — a patch or minor bump across
  the workspace is one cheap cross-repo step; a major version may earn a planned session.
- **Scope of "dependency."** The language toolchain, direct module dependencies, CI actions, and
  dev tools are different surfaces under the same discipline; the page names them all so no
  surface is a special case.

## Open questions

- Placement: an organizational principle beside minimal-footprint, or absorbed into the
  architecture the way the supply-chain rationale suggests (see `concepts/elemental-naming.md`).
- Whether automation (a release-watching bot opening the bump PRs) is part of the principle or a
  later convenience; the principle stands without it.
