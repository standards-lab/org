# What `sqlate` needs to host a library

The coordinator's ledger of what `sqlate` needs to host a library that ships its own statements,
read models, native variants, and a schema over them, as `blobfs` is the first to do. It excludes
the migrate group, which is `migration-sets.md`'s subject. Every entry states what fails or is
awkward in `sqlate` as it stands, the smallest change that removes it, and what it unblocks;
evidence and every workaround `blobfs.experiment` used live in
[spike-blobfs](https://github.com/JaimeStill/spike-blobfs)'s `REVIEW.md`, cited once here and not
restated. An entry moves from the backlog to a scheduled task by whether the change makes `sqlate`
and its consumers generally stronger, not by whether it is easy or touches a lot of existing code.
An entry `sqlate` lands independently of this ledger is removed once it does.

## Backlog

Recorded with what would need to become true before each moves to a scheduled task.

- **Composing clauses over a recursive base at the base's own level**, rather than wrapping it in a
  derived table, which loses index order for the outer filter. The parameterized base removes
  most of the cost in the cases that matter, and the alternative composition contract would give up
  the projection's guarantee that a base may be any query. Trigger: a consumer with a recursive base
  that cannot be anchored by a parameter and measures the wrap as its own bottleneck.
- **Exporting the catalog's clause renderer**, or the qualifier a clause pattern spells its field
  with. Both serve only a composer built outside a projection, which `blobfs` no longer carries: its
  listings are projections. Exporting either would freeze an internal contract as public API for no
  consumer left to use it. Trigger: a consumer with a composition the projection cannot express.
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
  structured engine name in the native declaration, not yet built, and a second engine to be
  worth the lint's own exemption for its stub dialect.
- **A per-directory tier rule in `sqlint`**, so a directory can declare that every statement in it
  must be one tier. States a layout convention — an engine package holds native statements only —
  that only the engine-package convention itself currently follows. Trigger: a repository's lint
  configuration wanting to state the rule, `blobfs`'s engine sub-module the likeliest; ships as its
  own `sqlint` release.
- **A tool that lists a program's native statements with their ports.** A natural addition once the
  native declaration is structured; not needed before then.
- **Typed slots inside a published pattern.** Neither known consumer has a failure from an untyped
  bind inside a pattern, only a style inconsistency, and the two candidate grammars have no evidence
  yet to choose between them.
- **Engine-keyed statement overlays**, for a statement of the same shape declared once and respelled
  per engine. No instance exists to design against, even inside `blobfs`: the returning command
  covers its engine's `RETURNING` forms, and its native statements either have no standard-tier twin
  or, like its hold, change the statement's shape, a locking read against the baseline's update. Trigger: a library with a genuine
  same-shape statement pair across two engines.
- **A cheaper counted page on PostgreSQL.** The counted collection read's window count holds every
  filtered row before it pages, where a count in a scalar subquery beside a page that walks the
  index would stop early. The subquery repeats the base's and the filters' placeholders, which
  PostgreSQL's numbered parameters allow, and on PostgreSQL one statement shares one snapshot, so
  the total cannot disagree with the page; the form is an overlay of the counted pattern for that
  engine alone. Trigger: the plan-cost log in sqlate's integration tier, or a consumer measuring
  the window as its own bottleneck.

## What the built releases unblock

`blobfs` v0.1.0 is built on `sqlate` v0.4.0: its listings are the library's projections with the
counted total, its write, complete, move, and delete steps are returning commands, and its engine's
variant holds only what an engine does better, the tree lock, path resolution, and a hold that
writes no row version. `v1.auth`'s parameterized base requirement is met. `go-web-service` moving
from v0.1.1 to v0.4.0 changes its organization domain's `domain/organization/database.go` at its
two collection-read call sites (v0.2.0), retypes any hand-written scan from `*sql.Rows` to
`query.Row` (v0.4.0), and sees an empty page after the first report `query.NoTotal`; the guard,
verification, and mapper changes are additive.
