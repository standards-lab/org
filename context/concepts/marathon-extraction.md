# marathon-extraction: the architectural record derived from a consumer

Captured 2026-09-05 from the architect's direction, ahead of a possible reassignment that would
move the architect's build time from the reference service to a private operational consumer of
the standard. The concept is a marathon extension that extracts the architectural record (the
patterns, conventions, and implementation details a consumer proves) out of that consumer and
into the blueprint's context and docs, whether the consumer is a workspace member or a
repository the workspace can never see. Everything here is candidate direction for a session on
`claude-plugins`, recorded as `backlog.marathon-extraction`; the one roadmap ruling it required
of this coordinator is recorded on `goals.v1`.

Filed in `org`'s public `context/`; the consumer that prompted it is never named here. Its
identity, if the effort proceeds, is an annex key in `private/`, and extraction is what writes it.

## The gap

Standards Lab standardizes what gets proven by building. Until now the proving ground has been
inside the workspace, the reference service and the coordinator's experiments, so the record of
what a build proved lands by the normal loop: the session's context, the docs pass, the roadmap.
`goals.v1` depends on that record: each layer closes only with its docs page and its promotion
evaluation ("what moved outward … and what stayed the service's own").

Two things break the loop once the proving ground is a consumer outside the workspace:

- **The record has to cross by hand.** `design/service-organization.md` creates an SDK, template,
  or focused reference "when a consumer demands it." The first production consumer may sit in a
  workspace that cannot reach this one, on a network where nothing from it may be catalogued
  publicly. What it proved has to be a self-contained, sanitized prose record before it is
  anything else.
- **Awareness runs downward.** No member repository, and nothing in `org`'s public context, may
  name a consumer. The reverse flow, consumer knowledge reaching the blueprint, has to run
  through sessions and coordinator-only artifacts, the way `references/workspace-coordination.md`
  already routes coordinator conventions to members.

## Scope

Extraction carries the description and never the decision. What crosses is the architecture as
the consumer implemented it, one claim per pattern, stated generally. It does not carry code, and
it does not decide what promotes into a library or the template: those decisions are made the way
they already are in the workspace. A design note or docs page names the pattern, and a later
session builds it. The promotion evaluation the v1 criteria require is written from the
extracted record; it is not the record.

## Proposal

`marathon-extraction`, a hybrid extension in the taxonomy `concepts/marathon-sitrep.md` proposes:

- **Integration facet: capture, consumer side.** Enabled in the consumer's `marathon.toml`
  (`[workspace]` at a coordinator, `[project]` standalone). Owns `context/extraction.toml`, the
  ledger of patterns the consumer has nominated. Fires at `on-close`: the closing session checks
  the stage against the want-list (below) and any nomination markers, drafts entries, and the
  architect accepts or rejects them in the review the stage already gets.
- **Package: user-invoked, consumer side.** Renders accepted ledger entries into a bundle: one
  markdown file, complete on its own terms, carrying the entries and a manifest of the sources
  they were drawn from. This is the artifact that crosses a boundary by hand. Package refuses an
  entry with no `evidence`, so the general-versus-domain judgment is made where the code is.
- **Enhancement facet: intake, blueprint side.** Invoked at this coordinator against a bundle.
  For each entry it produces work rather than pages: a roadmap task when `satisfies` cites an
  existing dotted path, a backlog task plus a concept note when it does not. The bundle lands
  under `private/` beside the annex catalog and is cited from each task's `context`, the way the
  DSL work cited `experiments/sql-dsl/REVIEW.md`. Intake never edits a design note, a docs page,
  a library, or the template; the sessions the tasks earn do that under the normal loop.

Intake is a session reading a projection, never a projection writing back: the bundle is a
source record like a review, consumed and decayed by the sessions it produces.

### The references append

A bundle names its sources: the consumer itself and anything the consumer drew the pattern
from, a prior repository or an external illustration. Intake checks each against the references
catalog and appends what is missing, by `design/repo-references.md`'s contract:

- A private source is appended to the annex: `private/references.toml` (the key and its
  canonical remote) and `private/references.md` (what it is and what to draw from). A public
  source is appended to the public catalog.
- The key namespace is one; intake never redefines a key that resolves, and an entry it cannot
  resolve to a remote is appended with the remote left for the architect and flagged in the
  session record.
- `references.local.toml` is never written by intake: the local path is per machine, and the
  architect maps it when the checkout exists.

The append is what makes a consumer's identity exist on this side at all: the annex key the
`origin` field cites is created by the first intake that carries it. It is also the first
concrete consumer of `backlog.marathon-references`. If that extension lands first, intake calls
it; if not, intake edits the three files by the design note directly, and the references
extension inherits the behavior when it is built.

### The candidate record

Each ledger entry, and each bundle entry, carries:

- `kind`: `design` (a pattern for a module's design note or landing-zone page) or `convention`
  (an org or standard principle with no single module).
- `claim`: the pattern stated as a docs page would state it, general, with no domain nouns.
- `evidence`: why it is the standard's and not the domain's. The DSL docs pass draws this line
  already ("status filters, search, hierarchy CTEs as the domain's, never the library's"); the
  field applies that judgment per entry.
- `satisfies`: the dotted citation into this coordinator's roadmap, when one exists.
- `origin`: the consumer's annex key and the commit the pattern was proven at, so re-packaging
  never duplicates an entry.
- `sources`: the references keys of anything beyond the consumer the pattern was drawn from;
  what the append resolves.

### The want-list projection

Capture is sharp only if the consumer knows what the blueprint is waiting on. The consumer is
downstream, so it may know the blueprint's remaining goals; but copying them into the consumer's
context is a restatement under `design/context-architecture.md`. The projection is declared as
what `references.local.toml` is: generated from `context/roadmap.toml`, stamped with the
roadmap's source commit, regenerated at each crossing, never hand-edited, gitignored or marked as
generated. When the two workspaces are co-resident the extension reads the roadmap through the
references key and no file exists.

### Nomination signals

The cheapest signal is in the code: a marker comment (`// extract:` or the consumer's
equivalent) on a helper the session wrote that an SDK should have owned, a middleware it
reworked, a template element it forked. `on-close` collects markers and drafts entries from
them. Without markers the session nominates from the stage's diff against the want-list, which
is noisier and puts more on the architect's review. Whether the marker is a convention the
standard states, or the consumer's choice, is open.

### Tooling below the skill

Package and intake follow `concepts/tooling-principles.md`: the bundle's schema validation, the
`evidence` gate, origin de-duplication, the references append, and the want-list generation are
deterministic and belong in a tool the skill calls, keeping the skill's text to the judgment of
what is general and where it lands. `backlog.harness-tooling` is that tooling layer's home.

## The first member is already here

go-web-service is a consumer. `v1.data.evaluation` needs the extracted record of the data layer
before it can rule on promotion; running that capture on `context/extraction.toml` at this
coordinator, with the origin a workspace member and no crossing, validates the record and the
`on-close` nomination before any bundle exists. Package, the annex origin, the references
append, and intake are built when a bundle first needs to cross. Nothing in the extension's
first slice depends on the reassignment happening.

## The ruling on `goals.v1`

`goals.v1`'s criteria prove each capability layer "complete in the running composition" of
go-web-service. If a private consumer absorbs the build time those layers would have taken, they
get proven there, and the criteria as written would not close. The ruling, recorded on
`goals.v1` on 2026-09-07, splits the two levels: a layer goal closes once its extracted record
has produced the library, the template element, and the docs page, wherever the layer was
proven; v1.0 keeps the running-composition criterion, with adoption in go-web-service a sweep
task per layer proven elsewhere. The reference stays a reference, and a consumer may carry the
proving. `design/blueprint-organization.md`'s graduation-timing question gets its first real
data from a production consumer.

## Where the roadmap records it

- `backlog.marathon-extraction` (repos: claude-plugins) carries the extension; its place
  relative to `v1.harness.sitrep` and `v1.harness.hardening` is `roadmap.toml`'s to state. The
  taxonomy amendment to `references/extensions.md` that the hybrid facet needs lands with
  whichever hybrid extension ships first.
- `v1.data.evaluation` rules from the extracted record if the extension has landed, and its
  findings are the ledger's first entries either way.
- If a consumer proceeds, its annex key is created by the first intake per
  `design/repo-references.md`; nothing here or in `org` names it.

## Open questions

- Ledger location when the consumer is a workspace: coordinator `context/extraction.toml`,
  matching the roadmap manifest; when standalone, the project's own.
- Whether intake may read a co-resident ledger directly, or only ever a bundle. Design for the
  bundle; treat co-residence as the degenerate case.
- The marker convention: stated by the standard (a per-module page names it) or left to the
  consumer.
- How the enhancement facet records enablement and targets a marathon version: the same
  questions sitrep leaves open, and one answer serves both.
- Whether the references append is intake's own behavior or the references extension's, called
  from intake. The contract is the design note's either way; the question is only which skill
  owns the edit once both exist.
