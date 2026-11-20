# Harness adoption

How every repository adopts marathon 0.21.0, for the `harness.adoption` goal and its sibling
`harness.adoption-<slug>` goals. ai-hosting's adoption set the pattern; the rules below settle
what every adoption goal would otherwise decide again. The last adoption goal's sync deletes
this note.

## The pattern

Each repository's adoption, as ai-hosting's shows:

- **Pin.** `mise.toml` pins marathon's tools at the latest `marathon/v*` release:
  `"github:standards-lab/claude-plugins" = { version = "<version>", version_prefix = "marathon/v" }`.
  The repository's currency command reports a newer marathon tag.
- **Check.** The check runs lint-context and lint-references beside its existing steps and fails
  when either fails. A lint missing from PATH fails the check with a message naming the
  repository's install command. The check's header and task description name what it runs. CI
  installs the mise tools before the check.
- **Context.** Every note and README is within its budget, and `context/README.md` has a
  capability map whose entries are marked `(built)` or `(planned)`. Each planned capability's
  open points sit in its note.
- **Comments.** Every comment in code, tests, scripts, and configuration (TOML, YAML,
  `.gitignore`, Dockerfiles, compose) is one of the five kinds in
  `architecture/principles/comments.md`. A comment goes where the code already says it (a
  name, a task description), into the note for planned work, or behind a one-line reference
  comment to the page that holds its reasoning. A script's section comments become named
  functions. The repository's `STANDARDS.md` comment rule covers every configuration file.
- **Pointers.** Every pointer to the coordinator's roadmap names `standards-lab/roadmap/index.toml`.
- **Upgrades first.** What the repository's currency command reports trailing is upgraded in
  the task's first slices.

## Settled for every adoption goal

- **Package documentation is public.** In Go, package comments and doc comments on exported
  identifiers stay; doc comments on unexported identifiers and on test functions go. In Rust,
  `//!` headers and docs on `pub` items stay; in C#, docs on `public` and `protected` members.
  Reasoning with no other home moves into names, exported docs, or `docs/`.
- **Suppressions carry reasons.** Each adopting repository enables its ecosystem's stock rule:
  golangci-lint's `nolintlint` with `require-explanation` and `require-specific`;
  `@eslint-community/eslint-plugin-eslint-comments`' `require-description` and
  typescript-eslint's `ban-ts-comment` with `allow-with-description`; clippy's
  `allow_attributes_without_reason`.
- **Detail moves to `docs/`.** A README over budget keeps its overview and links `docs/` pages
  that hold its detail, content unchanged. A note about built work keeps only reasoning with no
  home in the code, package docs, or README, and that reasoning moves to a `docs/` page. Every
  pointer to a moved section names its new page, and coordinator citations become pending
  edits.
- **PostgreSQL 19.** Every development and integration PostgreSQL stack moves to
  `19.0-alpine`; no container volume holds data worth keeping.
- **Releases.** Only the two Go templates tag: go-web-sdk-template `template/v0.16.0` and
  go-cli-sdk-template `template/v0.3.1`. Every other adoption leaves its changes under
  Unreleased.
- **References in CI.** `architecture/...` references skip in CI, where no architecture checkout
  sits beside the repository.

## Locks and running

- **One-time lock exceptions.** The adoption goals stage over the locks of v1.ai.processing,
  v1.client.stack, v1.ai.inference, and v1.ai.hosting, and over the `exclusive` group, so the Go
  adoptions run side by side. No session runs on those v1 goals until every adoption goal syncs.
- **One orchestrating session.** A single session runs the adoption goals in waves of up to
  five, each goal's pipeline through marathon's profile subagents. Each goal keeps its own
  record, branches, pull requests, and sync, and the orchestrator validates each task as a
  session does. It batches the architect's touch points: one combined plan round per wave, each
  goal's questions under its own heading, and one combined review of the wave's session briefs.
  It is a one-time exception to "a session advances one task of one goal", for a retro to judge.
- **Pre-plans.** Each goal's first PLAN starts from its pre-plan, a read-only survey with a draft
  brief, kept beside the harness.adoption root's plans
  (`claude-plugins/.claude/plans/preplan-harness.adoption-<task>.md`, untracked).
- **webappgen** adopts within v1.client.stack's `template` task, since its default branch holds
  only marathon's init files.
