# data-intelligence

The workspace's first capstone, `capstone.data-intelligence` in the roadmap: an application that
brings the workspace's libraries together end to end. Its name is the point of it: the
organization's data sits in binary files on a file share, and the capstone gets it out of them.
It spans three repositories, one per layer, created when the goal is staged (`naming.md`,
"Capstones"): capstone-data-intelligence, the integration root; capstone-bunrei; and
capstone-mikoshi. This note holds what the workspace needs to know about the capstone: its
architecture, its scope, its sub-goals and where they block, and what it asks of the goals
beneath it. The root's `context/` holds the capstone's working detail, and the detail its design
tasks start from is on [the capstone's design page](../docs/data-intelligence.md).

## What it is

Two services and the model gateway beneath them ("Bunrei and Mikoshi"): Bunrei, a processing
service on go-ai's model client, docproc's extraction library, and docinfer's inference library,
and Mikoshi, a prototype client-facing service with a web UI on the web client stack
v1.client.stack builds. It takes in messages, uploads, and streams, captures their metadata,
extracts data by inference, stores embeddings in PostgreSQL with pgvector for semantic search,
and shows each stage as it runs. It came out of spike-ai-pipeline (`ai-strategy.md`, "The client
and processing layers") but lives outside the experiment.

pgvector is a declared native-tier extension in capstone-mikoshi only, and v1.data.extensions wrote
the workspace's extension strategy from this case. Each stage's output is a versioned artifact,
and embeddings live in registered embedding spaces (design page, "Embedding spaces"). Its input is
the schema "2" run directories of docproc's `cmd/docproc` (docproc's `docs/api.md`, "Run
directory") and docinfer's `cmd/docinfer` (docinfer's `context/command-api.md` and
`context/command-report.md`).

## Bunrei and Mikoshi

The architect's whiteboard shows how a processing service and the model gateway replace the `ingest`
layer of the existing Athena (`~/s2va/athena`) without carrying its complexity, so that Athena
becomes a standard web service and what it defines internally sinks into libraries (design page,
"Bunrei").

**Bunrei** (capstone-bunrei) is a document-processing service from go-web-sdk-template that
never knows its caller. It queues and orchestrates processing, infers through the gateway with
go-ai, and emits progress, inference messages, and results as events, so it could serve any
consumer, a harness among them.

- **State.** Its jobs, uploads, and job streams only; a job is a root Entity folded from its own
  JetStream subject (`messaging.md`, "A stream-backed root").
- **API.** Queries and Commands, a resumable chunked upload (`chunked-upload-graduation`), and a
  nested `client` module.
- **Results.** Ordered result-batch events closed by a manifest (evidence 7).
- **Browser.** The browser reads only the consumer's stream; Bunrei's events stay
  service-to-service.

The rest (distribution, caching, models and stages) is on the design page, "Bunrei".

**Mikoshi** (capstone-mikoshi) prototypes the client-facing service, the authoritative source of
data: upload, blob storage, SQL with pgvector, RAG chat through the
gateway, and live stages, its UI embedded through `NewClient`, its events through go-messaging's
outbox in each batch's transaction. No code comes from `~/s2va/athena`: `athena-mapping` maps it
onto Bunrei (design page, "Athena onto Bunrei"), and its migration is later work outside the
workspace.

**capstone-data-intelligence** is the integration root: the goal record, the compose stack
(Bunrei, Mikoshi, NATS, Postgres with pgvector, the gateway), and the acceptance run over
docs-corpus v0.2.0 (299 files), measured against
[its baseline](https://github.com/standards-lab/docs-corpus/blob/main/docs/baseline.md): docproc's
run took 123.5 s, docinfer's 8,330.5 s (2.3 h), and docinfer's cached rerun 1.4 s.

**Auth stays out of Bunrei**: Athena has solved its auth strategy, and the capstone focuses on
signal flow. The gateway's per-consumer keys stay, carrying the priority between bulk and chat.

**Missing layers** beyond the roadmap: the capstone designs the packaging of the docproc-* tools
into Bunrei's image, since docproc's docker runner can't run in a container without the host's
socket (design page, "Tool packaging"); `retrieval` (hybrid ANN with tsvector and rerank; set A
has no reranker) and `cross-service-tracing` wait in the backlog.

### The layers

Five layers: system tools → docproc → docinfer ← go-ai, and Bunrei over docproc, docinfer, and
go-ai. docproc's [`docs/layers.md`](https://github.com/standards-lab/docproc/blob/main/docs/layers.md)
states docproc's boundaries as signatures and schemas, and its
[`context/bunrei.md`](https://github.com/standards-lab/docproc/blob/main/context/bunrei.md)
Bunrei's API, state, and events, whose open questions pass to the capstone's tasks (design page,
"Open items"). docinfer's [capability map](https://github.com/standards-lab/docinfer/blob/main/context/README.md#capability-map)
links its boundaries, and its root [`doc.go`](https://github.com/standards-lab/docinfer/blob/main/doc.go)'s
failure table maps each go-ai result.

### What it asks of the goals beneath it

Each goal beneath the capstone designs around Bunrei:

- **go-ai** (built by v1.ai.client), Bunrei its first long-running service consumer: retry
  proven against the live gateway, cancellation by context, a consumer key in a header other than
  the bearer token if the gateway asks for one, Embed batching, and a relayable streaming
  Generate; concurrency bounds stay the caller's. A request's time is the host's transport, not
  a per-call timeout: Bunrei sets `ai.Options.StreamIdleTimeout` for extraction's stream;
  cmd/docinfer doesn't.
- **v1.ai.processing and v1.ai.inference:** stores behind interfaces with PostgreSQL and object
  storage providers, stable event shapes, results in batches as made, cancellation per job, and
  the tools pinned in Bunrei's image (docproc's README, "The system tools").
- **v1.ai.hosting:** Bunrei (bulk) and Mikoshi (chat) as separate gateway consumers, each with a key
  and no budget, chat responsive during a run over docs-corpus v0.2.0. The capstone tunes each
  client's concurrency per alias live; clients transcribe in chat mode (design page, "Gateway
  load").
- **go-messaging:** the expected-sequence append and the work-queue package, which the capstone
  builds itself.
- **go-web-sdk:** an SSE helper. The capstone builds the resumable upload, which graduates after
  it, a stage timeline fed by events, and citations in the chat thread itself.
- **go-core's graph API:** both composition roots are written against it.

## Gates

The capstone runs in wave D (`waves.md`). Its gates have all closed: v1.ai.hosting,
v1.ai.processing, v1.ai.inference, v1.client.stack, and v1.data.extensions, whose one open check
is whether go-database's `database/sql` text format is fast enough for the batch loads of
embeddings.

It needs no harness: its model calls go through go-ai's client, whose Open Responses protocol and
wire-format packages reach every engine. Its repos gain docproc and the libraries the intakes
promote at staging, so a capstone task can fix a tool or a library and use the fix.

## Evidence

Carried from spike-ai-pipeline, which keeps the rest of its evidence:

7. Metadata capture, inference extraction, and embeddings in pgvector with a semantic search,
   each stage transactional on the consumer's side (Mikoshi commits each of Bunrei's result
   batches). The event layer tracks every stage event in sequence, in real time, and the UI shows
   each stage as it runs.
8. The client runs unchanged through spike-model-gateway's gateway.
10. pgvector as a declared native-tier extension: its install, migration, image pin, and the
    packages allowed to use it, built by v1.data.extensions (Gates).
15. The browser's consumption of the event sequence, NATS over WebSocket against a backend relay,
    compared on latency, ordering, auth, ports, and native-tier use, on Mikoshi's stream only.

## Scope

Beyond file-type coverage, the capstone shows AI-driven analysis over bounded data. **Data
contexts** form a tree in Mikoshi: files, API-authored records, and analyses join a context by
reference, a context covers its descendants, and a query context is a saved filter. A context
bounds hybrid search, a tool-calling chat with no harness, and analyses saved back as items.
**Events** carry lineage, several views, and standing analyses, not only a log. A **live
presentation** runs through the Mikoshi client, directed by a presenterm deck. Detail: design
page, "Data contexts", "Events", and "The presentation".

## Sub-goals and blocking points

The goal is a container; its layer sub-goals run side by side, each design-first.

| Sub-goal | Root | Holds |
|---|---|---|
| `.foundation` | capstone-data-intelligence | the shared contracts (chunks, protocol, events), the stack, athena-mapping, the demo dataset |
| `.torii` | ai-hosting | the gateway's rename to torii, redeployed |
| `.bunrei` | capstone-bunrei | metadata, text-layer, vision, go-messaging's tasks, then the service; the only one in the exclusive group (docproc, docinfer, go-messaging) |
| `.mikoshi` | capstone-mikoshi | the web shell, the database, contexts, items, ingest, hybrid search, chat, standing analyses, lineage, signal flow |
| `.showcase` | capstone-data-intelligence | the acceptance run and the presentation, staged once `.foundation` syncs |

Each blocked task's summary opens with "Waits on":

- `.foundation` `events` waits on `.bunrei` `metadata` (the metadata shapes it carries).
- `.bunrei` `state` and `api` wait on `.foundation` `protocol` and `events`.
- `.mikoshi` `store` and `contexts` wait on `.foundation` `chunks`; `ingest` on `events` and on
  `.bunrei` `api`'s release of `client/v0.1.0`.
- `.showcase` `acceptance` waits on `.torii`, `.bunrei`, and `.mikoshi` having synced.

## Carried notes

- Web content: docproc-coverage reads HTML and EPUB chapters as raw text; MHTML, WARC, and
  rendering for vision wait, unplanned. docproc-backlog holds pandoc (rejected a Go converter and
  htmd), XPS through mutool (rejected libgxps, GhostXPS), and ISO and UDF images.
- The scanned-document OCR tier isn't required: the capstone runs on vision transcription, and
  experiment.ocr settles the tier after it.
- go-ai's `aitest` is its public API, which docinfer, go-ai's `tools/infer`, and Bunrei's tests
  depend on, so a change to it breaks them.
