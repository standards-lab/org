# The waves

The workspace's plan to its first capstone, capstone.data-intelligence (`data-intelligence.md`),
and past it. The roadmap's arrays hold the goals' order; this note holds the plan behind it.

## The path to the capstone

1. Finish the pre-capstone experiments.
2. Take them in and build out the libraries and infrastructure the capstone needs. Each library
   promoted into ~/architecture opens with a design review, against the full architecture
   standards and the workspace, and closes with an acceptance task that reruns the real
   workloads.
3. Design the capstone's protocols and practices, its first tasks.
4. Build the capstone.
5. Continue the roadmap.

## The waves

Each wave runs its goals side by side.

- **B**: v1.ai.hosting builds ai-hosting before the capstone, and v1.ai.processing (docproc and
  its system tools) runs beside it, ahead of wave C. v1.ai.hosting shares the architecture lock
  with v1.ai.processing, and the architect frees the lock before v1.ai.hosting's `architecture`
  task merges. v1.ai.inference (docinfer) builds on go-ai's released library. v1.client.stack,
  rooted in web-foundation, runs in B; once its `foundation` task ships, sites.status-reports and
  sites.training.foundation can start.
- **C**: v1.ai's remaining sub-goals, beside the rest of v1.client.stack. go-ai and docproc stay
  outside the exclusive group until C ends, and then join it. Then v1.data.extensions, the
  Postgres-extension strategy.
- **D**: capstone.data-intelligence, the sole active goal apart from sites.status-reports and
  sites.training. It holds the libraries it may change, so a capstone task can fix a library and
  use the fix.
- **E**: v1.client.reference first; experiment.ocr, experiment.admin, and experiment.harness;
  then v1.data, v1.auth, v1.admin-listener, v1.deployment, v1.repository-docs, and v1.middleware;
  and sites last. sites.training units that lock libraries run where the exclusive group is free.

## The line at the capstone

- **Before it**: all of v1.client.stack, the web standard included; sites.status-reports' layout
  and first report, a backfill of ~/architecture as it stands at the capstone's start; and
  sites.training.foundation's setup and syllabus.
- **During it** (wave D): sites.status-reports' second report, an orientation to the capstone,
  and sites.training.foundation's writing tasks.
- **After it**: v1.client.reference, v1.repository-docs, client-create, and the rest of sites,
  last.
