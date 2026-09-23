# Experiments

This file catalogs the spikes run for this workspace. Each spike is a standalone marathon project
outside the workspace, and marathon's `experiment` command proposes this hosting convention when it
sets one up:

- **Local directory:** `~/experiments/spike-<slug>`.
- **Remote:** `github.com/JaimeStill/spike-<slug>`, public, with the module path to match.
- **After its close:** a `plan` session here reads the result and records what the workspace takes
  from it, then archives the remote. The entry below stays as the pointer.

## Catalog

| Experiment | Question | Remote |
|---|---|---|
| spike-sql-dsl | Can the whole SQL-to-Go layer run on authored SQL files instead of a Go statement vocabulary? Its library became sqlate. | [JaimeStill/spike-sql-dsl](https://github.com/JaimeStill/spike-sql-dsl), archived |
| spike-blobfs | Does the blobfs design, a SQL-backed virtual directory and file-metadata library over any object store, hold up when built? | [JaimeStill/spike-blobfs](https://github.com/JaimeStill/spike-blobfs), archived |
