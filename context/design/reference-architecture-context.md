# Reference-architecture context authoring

How the reference architectures in this effort author their written context, so each one stands on its
own.

## Each reference architecture stands alone

A reference architecture's non-volatile context — its `context/README.md` and `context/design/` notes —
describes that architecture's own design and conventions and nothing outside itself. It does not depend
on, or cite, sibling reference architectures, other target languages, or the prior-R&D catalog. A reader
should be able to understand the architecture from its own stable context alone.

## Awareness follows the dependency direction

The tiers of the architecture form a dependency stack (the
[repository topology](https://github.com/standards-lab/docs/blob/main/principles/repository-topology.md)
principle), and awareness runs the way the dependencies do: downward. A higher tier may document its
dependency on and integration with the lower tiers it builds on; a lower tier never names the tiers
that consume it. This mirrors the code — a module imports its dependencies, never its consumers.

This sharpens "stands alone." A repository's stable context avoids binding laterally (sibling
architectures, other-language mirrors, a peer SDK or infrastructure library) and upward (its
consumers), because those drift or don't concern it. Its true dependencies are different: the
reference service's context may describe how it composes the SDK and the services, an
infrastructure library's context may describe how it uses the Core SDK, and the Core SDK's
context describes the Core SDK alone. Where a lower tier needs to mark where its responsibility
ends, it does so in its own terms, as the boundary of what it provides; it does not name its
consumers. The cross-tier view of the whole lives in the docs landing zone (the repository
topology and service tiers principles) and this org context (`service-organization.md`), the
places the organization is described together. The vocabulary those pages define — the standard
and native tiers, the provider-swap classes — is shared vocabulary an architecture uses in its
own terms, not a citation; and the provider-containment lint is a consumer-side check that lives
with the reference service, so an infrastructure library never names the consumers it protects.

## Cross-references live in volatile context

References to anything outside a reference architecture — a sibling architecture, another target language,
or a prior-R&D source — belong only in volatile context (`concepts/`, `reset.md`), and only while a
session is directly deriving from or updating against that thing. They are working material, not settled
design, so they stay where the churn is and never harden into the stable record.

The relationships between repositories, and the catalog of prior R&D, live here in the org context — the
references catalog and `service-organization.md`; the individual architectures do not restate them.

## Conventions stand on their own merit

A design note records a pattern because it is a meaningful pattern for that architecture, not because a
prior effort proved it. Patterns can just as well emerge from experiments within the project itself.
Justify a convention by what it does, not by where it came from.

## Why

The effort has many reference architectures and a body of prior R&D. Confining cross-references to
volatile context keeps each architecture decoupled and keeps its stable design record from quietly binding
it to another repository or to material that will drift. It is the decay discipline marathon applies to
code, applied to the boundaries between repositories.
