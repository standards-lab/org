# Graduation

An architecture or standard intended for production use graduates from Standards Lab to an
organization of its own, whose name carries the standard's identity. The profile and the
architecture repository's README state the blueprint, boundary, and catalog roles; this note
holds what graduation implies and what is still open.

- The blueprint's repositories keep their single-concern, language-prefixed names
  (`go-web-sdk-template`) and are never renamed as the strategy matures. The module namespace
  (`go-elemental/core`) arrives with the graduated organization, never as a refactor here.
- The architecture repository's catalog references graduated organizations and external
  implementations rather than hosting their documentation. It has no entries until the Elemental
  Architecture graduates.

## Open questions

- **Timing relative to v1.0.** Graduating at or before the v1.0 cut means no production consumer
  ever pins the blueprint's module paths.
- **Mechanics.** Whether repositories move, or the graduated organization re-roots cleanly with
  the blueprint kept as the incubator's record, and who owns the graduated organization.
