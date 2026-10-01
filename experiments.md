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
| spike-blobfs | Does the blobfs design, a SQL-backed virtual directory and file-metadata library over any object store, hold up when built? | [JaimeStill/spike-blobfs](https://github.com/JaimeStill/spike-blobfs), archived; promoted to [standards-lab/blobfs](https://github.com/standards-lab/blobfs) |
| spike-messaging | Can one broker-agnostic event and reactor contract, built on CloudEvents with outbox emission, run a service's reactors on NATS JetStream and on an in-memory provider? | [JaimeStill/spike-messaging](https://github.com/JaimeStill/spike-messaging) |
| spike-harness-driver | Can Go drive an external agent harness (Pi, Claude Code, OpenCode) as the infrastructure for agentic work: capabilities, sessions, scoped exchanges, and long-running workflows? | [JaimeStill/spike-harness-driver](https://github.com/JaimeStill/spike-harness-driver) |
