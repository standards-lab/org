# The waves

The workspace's plan to its first capstone, capstone.data-intelligence (`data-intelligence.md`),
and past it. The roadmap's arrays hold the goals' order; this note holds the plan behind it.

## The path to the capstone

1. Take in the pre-capstone experiments and build out the libraries and infrastructure the capstone
   needs. Each library promoted into ~/architecture opens with a design review, against the full
   architecture standards and the workspace, and closes with an acceptance task that reruns the real
   workloads.
2. Design the capstone's protocols and practices, its first tasks.
3. Build the capstone.
4. Continue the roadmap.

## The waves

Each wave runs its goals side by side.

- **B**: v1.ai.hosting built ai-hosting before the capstone; v1.ai.processing built docproc and
  its system tools beside it, and v1.ai.inference built docinfer on go-ai's released library.
  v1.client.stack built the web client stack in B and C; with its `foundation`
  task shipped, sites.status-reports and sites.training.foundation can start.
- **C**: the rest of v1.ai's sub-goals, all since shipped. v1.data.extensions, the
  Postgres-extension layer, ran beside C in sqlate and go-database.
- **D**: capstone.data-intelligence, run as layer sub-goals side by side: `.foundation` (the
  shared contracts and the stack), `.torii`, `.bunrei`, and `.mikoshi`, then `.showcase` (the
  acceptance run and the presentation). A capstone task can fix a library in its goal's `repos`
  and use the fix.
- **E**: v1.client.reference first; experiment.ocr, experiment.admin, and experiment.harness;
  then v1.data, v1.auth, v1.admin-listener, v1.deployment, v1.repository-docs, and v1.middleware;
  and sites last.

## The line at the capstone

- **Before it**: all of v1.client.stack, the web standard included; sites.status-reports' layout
  and first report, a backfill of ~/architecture as it stands at the capstone's start; and
  sites.training.foundation's setup and syllabus.
- **During it** (wave D): sites.status-reports' second report, an orientation to the capstone,
  and sites.training.foundation's writing tasks.
- **After it**: v1.client.reference, v1.repository-docs, and the rest of sites, last.
