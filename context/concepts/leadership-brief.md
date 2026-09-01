# The leadership brief

Captured 2026-08-31 at the go-elemental-rename session's close, from the architect's notes
(written before that session's context overhaul) plus the revisions the overhaul implies.
Cited by `backlog.leadership-brief`.

## The deliverable

A simple, standardized brief for the CTO and other technical leaders. Three parts:

1. **The vision overview.** Lead with the vision's thesis sentence (the profile opening):
   clear architectural principles and standards, broken down into appropriately layered
   boundaries, optimize the utility of agentic workflows in creating well-structured software.
   Then the why-now narrative, in the architect's framing: using the current holding pattern
   to establish a software architecture strategy — wrestling with all of the lessons learned
   and progress made, finding the principles and patterns that optimize the conditions for
   long-term success and the longevity of the capabilities we build. How do we ensure the
   investments we make today endure and continuously adapt to evolving requirements? The
   graduation model is the endurance answer on record: the blueprint incubates; production use
   graduates to an organization that owns its evolution. The agentic questions belong here
   too: how to optimally integrate AI agents and capture the principles of successful harness
   programming; how the technology raises not just one person's velocity but the enterprise's;
   how the blueprint serves other disciplines — and, posed as the proposal's ask rather than a
   settled answer, how the enterprise designates experts to cultivate a discipline's evolution
   and adoption strategy.
2. **A brief roadmap summary**, derived from `roadmap.toml`'s `goals.v1` summary.
3. **The link tree**, reusing the profile's Organization Contents, followed by the interview
   section: a code block the reader pastes into their agent of choice to initialize an
   interview session over the organization's context and source —

   ```md
   launch interview: https://github.com/standards-lab/[path-to-interview.md]
   ```

   The hosted `interview.md` instructs the agent: identity, reading order (profile → docs →
   modules), and interviewing behavior (answer questions with cited sources).

## Tone

Honest early-prototype framing, per the architect: this does not have all the answers — the
point is taking the time to find enough of the right ones to build effectively. It is coming
together well and the emerging principles are excellent; dialing it in takes time. The brief
never oversells maturity. Prose uses the settled vocabulary: architecture → standard → module.

## Open items

- Where the brief lives, and where `interview.md` is hosted. With the coordination context now
  public in the `org` repository, a pasteable prompt has publicly fetchable routing into it —
  the former visibility constraint is resolved.

## Relation to marathon-sitrep

The short-brief + interview-prompt pattern feeds
`claude-plugins/context/concepts/marathon-sitrep.md`: a sitrep extension could eventually
generate the brief and keep it current against the git history, session record, and roadmap.
