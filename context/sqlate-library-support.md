# What `sqlate` needs to host a library

The coordinator's ledger of what `sqlate` needs to host a library that ships its own statements,
read models, native variants, and a schema over them, as `blobfs` is the first to do. It excludes
the migrate group, which is `migration-sets.md`'s subject. Every entry states what fails or is
awkward in `sqlate` as it stands, the smallest change that removes it, and what it unblocks;
evidence and every workaround `blobfs.experiment` used live in [spike-blobfs](https://github.com/JaimeStill/spike-blobfs)'s `REVIEW.md`,
cited once here and not restated. An entry moves from the backlog to a scheduled task by whether
the change makes `sqlate` and its consumers generally stronger, not by whether it is easy or
touches a lot of existing code. An entry `sqlate` lands independently of this ledger is removed
once it does.

## Backlog

Recorded with what would need to become true before each moves to a scheduled task.

- **Composing clauses over a recursive base at the base's own level**, rather than wrapping it in a
  derived table, which loses index order for the outer filter. The parameterized base above removes
  most of the cost in the cases that matter, and the alternative composition contract would give up
  the projection's guarantee that a base may be any query. Trigger: a consumer with a recursive base
  that cannot be anchored by a parameter and measures the wrap as its own bottleneck.
- **Exporting the catalog's clause renderer**, or the qualifier a clause pattern spells its field
  with. Both serve only a composer built outside a projection, which the scheduled listing work
  above retires; exporting either would freeze an internal contract as public API for no consumer
  left to use it. Trigger: a second consumer with a composition the projection cannot express after
  the scheduled work lands.
- **Exposing the dialect a statement compiled against.** Serves the same retired composer.
- **Documenting that `Statement.Text()` trims a trailing semicolon.** Already documented in the
  user guide and in the parser's own comment; only the accessor's own godoc is silent. A one-line
  addition that rides along with any later change to that file, not its own task.
- **A library operation that opens its own read-only transaction.** A capability interface a
  library could assert, in the style of the existing error-mapper and locker capabilities, is
  coherent, but the library that needed this chose the more explicit contract instead — typing the
  four transaction-only operations to take a transaction directly. Trigger: a second library over
  `sqlate` that must open its own snapshot without pushing the transaction requirement onto its
  caller.
- **A separate sentinel for a field a statement did not declare.** The one library instance that
  wanted this would have regressed an existing consumer's mapping of an unknown field to a
  client-error response; the fix is a documented type-match contract for a program that composes
  its own filters, not a new sentinel.
- **A compile-time check that a native statement belongs to the dialect it was compiled against.**
  Only useful once a program can be compiled against more than one dialect, which needs a
  structured engine name in the native declaration (scheduled above) and a second engine to be
  worth the lint's own exemption for its stub dialect.
- **A per-directory tier rule in `sqlint`**, so a directory can declare that every statement in it
  must be one tier. States a layout convention — an engine package holds native statements only —
  that only the engine-package convention itself currently follows. Trigger: the promoted `blobfs`
  repository's own lint configuration wanting to state the rule; ships as its own `sqlint` release,
  independent of `sqlate` v0.2.0.
- **A tool that lists a program's native statements with their ports.** A natural addition once the
  native declaration is structured (scheduled above); not needed before then.
- **Typed slots inside a published pattern.** Neither known consumer has a failure from an untyped
  bind inside a pattern, only a style inconsistency, and the two candidate grammars have no evidence
  yet to choose between them.
- **Engine-keyed statement overlays**, for a statement of the same shape declared once and respelled
  per engine. No instance exists to design against, even inside `blobfs`: its own engine's
  `RETURNING` forms change the statement's shape entirely (a different mechanism, below), and its
  other native statements have no standard-tier twin at all. Trigger: a library with a genuine
  same-shape statement pair across two engines.
- **A statement that returns the changed row, declared once**, so `RETURNING` on one engine and an
  insert-then-read on another are one declaration. The right shape is not knowable from one
  library's evidence; a protocol-handle design and a native-only-feature design are both plausible
  and unproven. Trigger: a second library with the same fallback need, or a settled protocol shape
  agreed before the code is written.
- **A window-count total mode for the collection read**, a `COUNT(*) OVER()` column read alongside
  the page instead of a separate query. The column would reach an arbitrary consumer-supplied
  `ScanFunc[T]`, which the projection cannot hide it from without a second, mapped-only binding.
  Trigger: a consumer measuring the separate count's round trip as its own bottleneck.
- **A stricter type grammar for a `field` declaration.** `sqlType`'s regex allows spaces, for real
  multi-word types like "timestamp with time zone," which also lets a typo in the `not null`
  suffix (`not nul l`) fall through as a literal, bizarre type name instead of an error. Trigger: a
  second instance of this class of typo actually reaching review, or a consumer asking for
  stricter validation.

## What v0.2.0 unblocks

`blobfs.build`'s listing composer and cursor can now collapse onto `sqlate`'s own projection
instead of being carried as the library's own code, and its engine's variation-point interface is
expected to shrink once the pattern-overlay mechanism covers the statements that exist only
because it did not. `v1.auth`'s parameterized base requirement is now met independently of
`blobfs`. `go-web-service`'s existing organization domain needs one file changed,
`domain/organization/database.go`, a small signature change at its two call sites; the guard,
verification, and mapper changes are additive and need no change there.
