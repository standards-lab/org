# standards-lab

This directory holds the Standards Lab organization's context about itself: the workspace
coordination context, the references catalog, and the organizational profiles. It is managed with
the marathon workflow. Start from `context/README.md`.

## Three repositories in one directory

This directory is the working tree of the public `standards-lab/org` repository — the workspace
coordinator: `context/`, the public references catalog, and the Claude configuration. Two profile
repositories are checked out nested inside it, each with its own `.git`, both gitignored by `org`:

- `public/` is the working tree of the public `standards-lab/.github` repository — the baseline
  organizational profile (`public/profile/README.md`), the authored source of the profile body.
- `private/` is the working tree of the private `standards-lab/.github-private` repository — the
  extended organizational profile (`private/profile/README.md`, the baseline body plus the
  member-only section) and the private references annex.

Git routes commits by working tree: edits under `public/` land in `.github`, edits under `private/`
land in `.github-private`, and edits anywhere else land in `org`. A change that touches more than
one produces one commit and one pull request per touched remote.

## A documented marathon exception

Marathon's default is a single source of truth inside a single repository, and `org` follows it.
The exception is the profile nesting: GitHub renders the baseline profile from `.github` and the
extended profile from `.github-private`, so one concern spans two repositories, both checked out
here and kept coherent by the mirror convention (`context/design/workspace-structure.md`). The
exception is scoped to the profiles; do not generalize it.

This is distinct from marathon's **workspace coordination**, which this repository does use as the
declared coordinator (see the `[workspace]` block in `.claude/marathon.toml`). That mechanism
orchestrates a change across the sibling repositories in the workspace — each its own working tree
and remote — and deliberately holds no context of its own. The two are different mechanisms for
different problems.

## Visibility tiers (public / member / private)

Material belongs to exactly one visibility tier (distinct from the service tiers defined in the
docs landing zone). Place it where it goes and do not duplicate it upward:

- **Public** — `context/`, the references catalog, and `public/profile/README.md`. The working
  context is public by design: the blueprint's lived context — roadmap, design notes, session
  records — is part of what it demonstrates. The baseline profile is the most curated surface;
  nothing internal.
- **Member** — the appended member-only section of `private/profile/README.md`. The rest of that
  file is the baseline body mirrored verbatim; the mirror is the one sanctioned duplication —
  GitHub renders exactly one profile per viewer, so the extended file must carry the baseline body
  to show members the whole landing.
- **Private** — the references annex in `private/` (`private/references.toml`,
  `private/references.md`): prior R&D from private engagements. Nothing in `org` names its
  entries; the annex extends the public catalog only where the member checkout is present
  (`context/design/repo-references.md`).

Curation removes detail as material moves toward the profiles; it never asserts what the working
context does not support. A claim in a profile must be true under `context/design/`, in narrower
words. Anything engagement-specific belongs in `private/`, never in `org` — check before
committing.
