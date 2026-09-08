# Tooling principles

Eleven principles for software that serves an agent, captured 2026-09-07 for adoption across
the workspace and adopted into the architecture at the `v1.alignment.docs` session
(2026-09-08). The pages are the statement; this note is the map.

- Principles 1, 2, 3, 4, 6, 8, 9, 10, and 11, the tool-based-skill principles, are one harness
  page: `docs/harness/tool-based-skills.md`. A skill encodes judgment and a tool encodes
  procedure; the layers, the schema as the contract, the specification-and-instance split,
  extensions calling the tooling, offloading as a tool, minimal custom code, and building
  against a working loop are its sections. Principle 6, declarative one-way reconciliation, is
  folded into the first section as the plan-and-apply shape of a reconciling tool.
- Principle 5, providers as adapters with one projection out, is a section of
  `docs/principles/service-tiers.md`, with sqlate's dialect interface as the reference pattern.
- Principle 7, a standalone tool alongside the library, is `docs/principles/tool-beside-library.md`,
  with sqlate's `sqlint` as the exhibit.

The harness tasks apply them: `v1.harness.hardening` calls `sqlint` as a package rather than
re-implementing its checks (principles 1 and 8), `backlog.marathon-sitrep` calls a dev-blog skill's
pipeline rather than re-encoding it (principle 8), and `backlog.harness-tooling` is the
tooling layer beneath the harness the principles ask for (principles 1, 2, and 3).
