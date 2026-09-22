# What `sqlate` needs to host a library

The coordinator's ledger of what `sqlate` needs to host a library that ships its own statements,
read models, native variants, and a schema over them, as `blobfs` is the first to do. It excludes
the migrate group, which is `migration-sets.md`'s subject. Every entry states what fails or is
awkward in `sqlate` as it stands, the smallest change that removes it, and what it unblocks;
evidence and every workaround `blobfs.experiment` used live in `experiments/blobfs/REVIEW.md`,
cited once here and not restated. Every entry stands against `sqlate` main at v0.1.1 with an empty
`[Unreleased]` section; an entry `sqlate` lands independently of this ledger is removed once it
does.

The test that sorted each item between the scheduled section and the backlog is the architect's:
whether the change makes `sqlate` and its consumers generally stronger, not whether it is easy or
whether it touches a lot of existing code. `blobfs.sources` was chosen to carry the scheduled items
alongside the multi-set migrator, so `sqlate.v0.2.0`'s scope is six areas, not the migrator alone.

## Scheduled in `blobfs.sources`

**The collection read.** A projection base may bind its own parameters: `List` and `One` take a
variadic `base ...Args`, so a call with nothing to bind is unchanged and a scoped call passes one
value built with a new package function, `query.With(name, v) Args`, which returns a fresh `Args`
carrying one binding and chains with the existing `Args.With` method for more. This is also a
`v1.auth` requirement (`design/auth-strategy.md` §4) independent of `blobfs`: any scoped read model
in v1 needs a base that can bind the scoping value the server's own code supplies, not a value a
request could supply or omit. `Directives` gains a total mode with three values — exact (today's
only behavior), a window count, and none — and a keyset cursor with the rules the experiment
settled: the declared key as the tie-breaker in the sort's own direction, one direction per query,
no nullable field in the terms. A projection's `--| key:` header accepts a comma-separated list for
a composite key, checked at the same depth `sqlate` already applies to a single key — that the
named fields are declared fields of the statement, nothing more; no schema-level uniqueness check
exists for the single-field case today, so none is built for the composite case either. `List`'s
result reports the items, the total when one was asked for, and whether more rows remain, in one
value rather than an overloaded return. The Postgres module supplies its first pattern overlay for
this: the row-value keyset comparison in place of the standard tier's expanded chain, which needs
the overlay mechanism relaxed to accept a subset of the source pattern's slots, since the two forms
fill different ones.

**The guard.** A typed guard whose check can carry a second predicate and returns the row: on zero
rows affected, it distinguishes no row at all, a row at another version, and a row at the expected
version that the command's own predicate refused, rather than reporting every such case as a false
version mismatch. The existing `Guard` keeps its shape for a consumer with only the plain
version-checked predicate.

**Verification and the header.** `Verify` probes each declared field's type against the cast it
would render, so a misspelled standard-tier type fails at startup rather than at first request. A
native statement's declaration may span more than one line, under a `port` key, with today's
one-line form still accepted; the header parser's matching change ships in the same release as the
grammar change, not trailing it, and `sqlint`'s native-forms check is extended to recognize the new
form in lockstep, per standing policy that the lint tool tracks the format it enforces. A library's
own shipped statements reserve the `sql.` namespace and resolve it by identity rather than by
name, so a consumer that aliases `sqlate`'s own source with `As` cannot break a library's compile;
this is settled now, before `go-auth` becomes the second shipper to depend on it.

**The mapper.** The struct-tag scanner and the args-from-struct binder both flatten an embedded
struct's fields, so a read model that reuses a library's entity type, or a shared identity or audit
type, does not have to restate every column by hand.

**Errors.** `ConstraintError` carries the table and column name the driver already reports, when the
driver exposes them, beside the constraint name and class it carries today; this closes a real gap,
since a not-null violation on Postgres carries no constraint name at all, leaving the column name
as the only handle a consumer has. Additive; no existing caller changes.

**`migrate`.** `HistoryExists` is qualified by the database's current schema through a
dialect-supplied lookup, closing a defect where a same-named table in any schema satisfies the
check today; this rides with the migrate group's own rework of the history protocol.

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

## What each scheduled item unblocks

`blobfs.build`, once `sources` lands: the listing composer and the cursor can collapse onto
`sqlate`'s own projection instead of being carried as the library's own code, and the engine's
variation-point interface is expected to shrink once the pattern-overlay mechanism covers the
statements that exist only because it did not. `v1.auth`: the parameterized base is a stated
requirement of the auth strategy independently of `blobfs`. `go-web-service`'s existing organization
domain: the guard, verification, and mapper changes are additive and force no code change there; the
parameterized base may cost one file, `domain/organization/database.go`, a small signature change
at its two call sites.
