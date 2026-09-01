# Elemental layers beyond the app class

Captured 2026-08-31 during the go-elemental-rename session, from the architect's direction
while settling the module level (architecture → standard → module, classes
library | template | app). Raw direction, deliberately unprojected; nothing here is documented
convention yet.

## The direction

The app class's output has a follow-on elemental sequence:

- An app produces a binary. The binary either runs on bare metal or is wrapped into another
  elemental layer — the container — which represents the runtime that owns the binary.
- Beyond the runtime layer sit application-layer hierarchies, likely little different from Go
  package hierarchy layers, differentiated by the external services the application connects
  to: infrastructure services and reactor services alike.

## Status

The architect stopped the projection deliberately ("my brain already hurts projecting this far
ahead"). Revisit when the reference service's deployment work (`goals.v1.deployment`) or the
runtime story makes the container layer concrete; until then the module classes and tier
vocabulary in the docs landing zone are the settled extent of the hierarchy.
