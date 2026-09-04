# Context architecture

Settled 2026-08-31, at the retrospective. The organizational principle for how written context
is structured across every layer that carries it: harness instructions (the marathon skill and
its extensions), the docs landing zone, repository `CLAUDE.md` files, and each project's
`context/` tree.

## The principle

Every contextual detail has exactly one authoritative home. Any other layer that needs it links
to that home; it never restates or redefines it. A restatement is a second source of truth the
moment it is written, and it drifts the moment the original changes.

The retrospective found the symptom in three unrelated places, which is what makes it a
principle rather than a bug class:

- The marathon role boundary was defined in the skill and restated in six repositories'
  `CLAUDE.md` files and their `marathon.toml` comments; changing it means a seven-file sweep.
- Marathon's decay rule was defined with its protective qualifier in one reference and restated
  without the qualifier in three command playbooks — the commonly loaded copies authorized
  deleting notes the authoritative copy protects.
- The docs landing zone, the repository READMEs, and the references catalog restate each
  repository's package inventory; the READMEs and catalog were two releases stale while the
  landing zone and `doc.go` files were current.

## In practice

- **One home per detail.** A mechanic, principle, pattern, or boundary is defined where its
  authority lives: API behavior in the code and its `doc.go`; a repository's design reasoning in
  its landing-zone page; org-level principles in `docs/principles/`; workflow mechanics in the
  marathon skill's single owning file; volatile direction in the owning repo's `context/`.
- **Link, don't restate.** A layer that needs a detail defined elsewhere cites the home — a
  command playbook cites the reference, a `CLAUDE.md` links the skill, a profile links the
  landing zone. A summary that adds no information beyond the link is a restatement; a
  *narrowing* (a profile stating less than the design supports, a repo declaring a tighter
  dependency line) is not, because it asserts something the home does not.
- **Layer for the reader.** The workspace carries large, technically dense information; the
  layering is also how a human digests it. Each layer answers its own question — what the org
  believes (principles), what a repo is (landing zone), how to work here (`CLAUDE.md`), what is
  in flight (`context/`) — and hands the reader a link when the question changes.
- **A duplicate found is a defect fixed.** The session that finds a restatement collapses it to
  a link, in the same change, the way a stale claim is already treated.

## Enforcement path

The principle lands in phases:

- marathon v0.9.0 applied it to the harness: command playbooks cite the references instead of
  restating them, the workspace and hook blocks are printed once, and the architect's role is
  defined once in the skill.
- `backlog.context-stratification` is the alignment pass across the landing zone, profiles,
  catalogs, and project context, now that the harness half is in place.
- A docs principle page states the rule for human contributors when the docs task runs.
