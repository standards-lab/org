# Workspace structure

The organization's context about itself lives in the public `org` repository, the workspace
coordinator. GitHub renders organizational profiles from two dedicated repositories — the public
profile from `.github`, the member profile from `.github-private` — so the profile concern spans
two more repositories even though it is one concern. This note records the layout that lets one
directory coordinate all three.

## Layout

- `~/architecture/` is a plain, unversioned container. The effort's capability repositories (such as
  `claude-plugins`) are independent siblings directly under it.
- `~/architecture/standards-lab/` is the working tree of the public `standards-lab/org` repository.
  Marathon is anchored here. It versions the workspace context (`context/`), the public references
  catalog, and the Claude configuration.
- `~/architecture/standards-lab/public/` is the working tree of the public `standards-lab/.github`
  repository: the baseline organizational profile (`profile/README.md`), the authored source of the
  profile body. It keeps its own `.git` and is gitignored by `org`.
- `~/architecture/standards-lab/private/` is the working tree of the private
  `standards-lab/.github-private` repository: the extended organizational profile
  (`profile/README.md`) and the private references annex (`design/repo-references.md`). It keeps
  its own `.git` and is gitignored by `org`, so nothing private can be staged into a public
  repository.

## The profile mirror

GitHub renders exactly one profile per viewer: members see `.github-private`'s
`profile/README.md`, everyone else sees `.github`'s. The extended profile is therefore a
superset, not a sibling: the baseline profile's body mirrored verbatim, with a member-only
section (`## Member orientation`) appended. The baseline file is authored first; any edit to the
shared body is copied to the extended profile in the same session, and a divergence between the
shared bodies is a defect. Member-only material lives only in the appended section.

## How commits route

Git selects the repository by working tree, with no special configuration: edits under `public/`
belong to `.github`, edits under `private/` belong to `.github-private`, and edits anywhere else
under `standards-lab/` belong to `org`. The nested-clone boundaries encode the routing. A session
that changes more than one produces one commit and one pull request per touched remote.

## Why the profiles nest, and the limit of the deviation

Marathon's default keeps a single source of truth inside a single repository, and `org` follows
it: the coordinator is an ordinary public repository. The deviation that remains is the profile
nesting — GitHub forces the baseline and extended profiles into the two dedicated repositories
while they remain one concern, so both are checked out inside the coordinator's directory and
maintained by the mirror convention above. It is scoped to the profiles and is documented in
`CLAUDE.md`. It is not a general workspace tier; that abstraction is introduced only if more
cases appear.
