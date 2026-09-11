# reset · marathon-updates

- **Status:** closeout
- **Session:** start
- **Project:** claude-plugins, standards-lab
- **Branch:** marathon-updates

## Disposition

- **Integrated:** added the `marathon-architecture` extension (claude-plugins) — the architecture
  layer's artifact, hooks, and reference doc, mirroring `marathon-roadmap`'s shape; registered
  with the plugin host; removed the layer from marathon core entirely
  (`context-engineering.md`, `init.md`, `close.md`, `review.md`, `configuration.md`) — no
  residual mention. The workspace case resolves its target repository from the extension's own
  config file, never a core `marathon.toml` key.
- **Integrated:** `references/extensions.md` gains the rule that an extension never adds a key to
  core's `marathon.toml` schema. `context-engineering.md` gains the pre-write curation check, the
  rule against promoting a note or authoring a skill in the same session that designed the shape
  it documents, and the rule that `design/`/`concepts/` notes state current truth only, never a
  changelog. `staged-execution.md`'s stage loop now states the delegation call as a visible line
  before each stage. marathon core released as 0.12.0; `marathon-roadmap` patched to 0.1.6
  targeting it.
- **Culled:** `claude-plugins/context/concepts/marathon-updates.md` — all four findings landed,
  fully expressed by the skill files now.
- **Integrated:** `claude-plugins/context/README.md`'s capability map gains a
  `marathon-architecture` entry.
- **Cross-repo:** `standards-lab/.claude/marathon.toml` — `marathon-architecture` added to
  `[workspace] extensions`; the old `architecture` key deleted outright. New
  `.claude/marathon-architecture.toml` (`repo = "architecture"`) holds the extension's own
  config.
- **Cross-repo:** `standards-lab/context/design/architecture-layer.md` brought to
  current-truth-only form — dropped its dated session-provenance sentence, repointed its
  repository citation at the extension's own config.
- **Cross-repo:** `standards-lab/context/roadmap.toml` — `goals.v1.harness.tasks.marathon-updates`
  deleted (done); `next` advances to `v1.web`.

## Next-focus

`v1.web` is next — the web SDK's handler contract (`goals.v1.web`, in `go-web-sdk`): the
adapter's remaining whole-response problem story (`goals.v1.web.tasks.adapter`) and the
middleware set (`goals.v1.web.tasks.middleware`), which `goals.v1.auth` depends on. The roadmap
carries each task's detail; settle which one leads at that session's SETTLE.
