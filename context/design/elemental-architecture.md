# Elemental Architecture

The organization's first application architecture, defined in the docs landing zone at
[architecture.md](https://github.com/standards-lab/docs/blob/main/architecture.md),
with the composition root defined as its first attached principle. This note records only the
open implementation items the definition defers.

## Open implementation items

- The event emission interface: its API, and how "reports a committed mutation" is honored
  against the transaction that raised it. Settled at its first consumer, the reference
  service's data work.
- Query and Command mechanics under CQRS: vocabulary, dispatch, and read models. Settled in the
  roadmap re-plan.
