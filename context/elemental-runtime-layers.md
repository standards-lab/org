# Elemental layers beyond the app class

The architect's direction beyond the module level (architecture → standard → module, classes
library | template | app). Raw and deliberately unprojected; nothing here is convention.

## The direction

The app class's output has a follow-on elemental sequence:

- An app produces a binary. The binary either runs on bare metal or is wrapped into another
  elemental layer — the container — which represents the runtime that owns the binary.
- Beyond the runtime layer sit application-layer hierarchies, likely little different from Go
  package hierarchy layers, differentiated by the external services the application connects
  to: infrastructure services and reactor services alike.

## When to revisit

When the reference service's deployment work (`goals.v1.deployment`) or the runtime story makes
the container layer concrete. Until then the module classes and tier vocabulary in the
architecture repository are the extent of the hierarchy.
