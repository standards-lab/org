# data-intelligence

The workspace's first capstone, `capstone.data-intelligence` in the roadmap: an application that
brings the workspace's libraries together end to end. Its name is the point of it: the
organization's data sits in binary files on a file share, and the capstone gets it out of them.
It spans three repositories, one per layer, created when the goal is staged (`naming.md`,
"Capstones"): capstone-data-intelligence, the integration root; capstone-cerebro; and
capstone-athena. This note holds what the capstone carries until they exist; the root's `init`
takes the note in. The detail its design tasks start from is on
[the capstone's design page](../docs/data-intelligence.md).

## What it is

Two services and the model gateway beneath them ("Cerebro and Athena"): Cerebro, a processing
service on go-ai's model client, docproc's extraction library, and docinfer's inference library,
and Athena, a prototype client-facing service with a web UI on the web client stack
v1.client.stack builds. It takes in messages, uploads, and streams, captures their metadata,
extracts data by inference, stores embeddings in PostgreSQL with pgvector for semantic search,
and shows each stage as it runs. It came out of spike-ai-pipeline (`ai-strategy.md`, "The client
and processing layers") but lives outside the experiment.

pgvector is a declared native-tier extension in capstone-athena only, and v1.data.extensions writes
the workspace's extension strategy from this case. Each stage's output is a versioned artifact
keyed by its input and configuration hash, so a stage reruns only when either changes; embeddings
live in registered embedding spaces, so a model change re-embeds into a new space (design page,
"Embedding spaces"). Its input is the schema "2" run directories of docproc's `cmd/docproc` and
docinfer's `cmd/docinfer` (docproc's `docs/api.md`, "Run directory").

## Cerebro and Athena

The architect's whiteboard architecture: it shows how a processing service and the model gateway
replace the `ingest` layer of the existing Athena (`~/s2va/athena`) without carrying its
complexity, so that Athena becomes a standard web service and what it defines internally sinks
into libraries (detail on the design page, "Cerebro").

**Cerebro** (capstone-cerebro) is a document-processing service from go-web-sdk-template that
never knows its caller. It queues and orchestrates processing, infers through the gateway with
go-ai, and emits progress, inference messages, and results as events, so it could serve any
consumer, a harness among them.

- **State.** Cerebro owns only its jobs, its uploads, and each job's event stream; docproc's and
  docinfer's stores are facets of its internal state. A job is a root Entity folded from the
  events appended to its own JetStream subject under the expected-last-subject-sequence check,
  all Elemental's Event asks (`architecture/architecture.md`, "The elements");
  go-messaging's outbox serves its other events (`messaging.md`, "A stream-backed root").
- **API.** Go Elemental Queries and Commands, with Events published; a large document arrives
  through a resumable chunked upload shaped to graduate to go-web-sdk once capstone-athena reuses
  it (`chunked-upload-graduation`). A nested `client` module holds the protocol types and the Go
  client.
- **Results.** Results don't fit one NATS event, so Cerebro emits ordered result-batch events
  closed by a manifest; the consumer commits each batch in one transaction (evidence 7).
- **Distribution.** Jobs go through go-messaging's work-queue package, one job per input the
  unit of retry, inference fanned out inside it; spreading an input's parts across replicas
  waits until the acceptance run shows a replica is the bottleneck.
- **Caching.** Results are keyed by content and stage identity (docproc) and by capability,
  model, settings, and content (docinfer); content seen before is replayed.
- **Models and stages.** A request names a versioned processing profile, never models; each result
  records the profile's resolved identity, so the consumer registers the matching embedding space.
  Regex entities and sparse rows stay the consumer's.
- **Browser.** The browser reads only the consumer's stream, so Cerebro's events stay
  service-to-service, with no per-user authorization on its subjects.

**Athena** (capstone-athena) prototypes the client-facing service, the authoritative source of
data: upload, blob storage, SQL with pgvector, RAG chat through the
gateway, and live stages, its UI embedded through `NewClient`, its events through go-messaging's
outbox in each batch's transaction. No code comes from `~/s2va/athena`: `athena-mapping` maps it
onto Cerebro (design page, "Athena onto Cerebro"), and its migration is later work outside the
workspace.

**capstone-data-intelligence** is the integration root: the goal record, the compose stack
(Cerebro, Athena, NATS, Postgres with pgvector, the gateway), and the acceptance run over ~/docs
(592 files, 1.7 GB), measured against docinfer's baseline: 6.0 h, and a 3.1 s rerun.

**Auth stays out of Cerebro**: Athena has solved its auth strategy, and the capstone focuses on
signal flow. The gateway's per-consumer keys stay, carrying the priority between bulk and chat.

**Missing layers** beyond the roadmap: the capstone designs the packaging of the docproc-* tools
into Cerebro's image, since docproc's docker runner can't run in a container without the host's
socket (design page, "Tool packaging"); `retrieval` (hybrid ANN with tsvector and rerank; set A
has no reranker) and `cross-service-tracing` wait in the backlog.

### The layers

Five layers: system tools → docproc → docinfer ← go-ai, and Cerebro over docproc, docinfer, and
go-ai. docproc's [`docs/layers.md`](https://github.com/standards-lab/docproc/blob/main/docs/layers.md)
states each boundary as signatures and schemas, and its
[`context/cerebro.md`](https://github.com/standards-lab/docproc/blob/main/context/cerebro.md)
Cerebro's API, state, and events, whose open questions pass to the capstone's tasks (design page,
"Open items"). The go-ai surface docinfer calls is kept in docproc's
[`context/layers.md` at commit f3633c3](https://github.com/standards-lab/docproc/blob/f3633c3/context/layers.md) ("go-ai's surface").

### What it asks of the goals beneath it

Each goal beneath the capstone designs around Cerebro:

- **go-ai** (built by v1.ai.client), Cerebro its first long-running service consumer: retry
  proven against the live gateway, cancellation by context, a per-consumer key, Embed batching,
  and a relayable streaming Generate (docproc's
  [`context/layers.md` at commit f3633c3](https://github.com/standards-lab/docproc/blob/f3633c3/context/layers.md),
  "Requests to go-ai"); concurrency bounds stay the caller's.
- **v1.ai.processing and v1.ai.inference:** stores behind interfaces with PostgreSQL and object
  storage providers, stable event shapes, results in batches as made, cancellation per job, and
  the tools pinned in Cerebro's image (docproc's README, "The system tools").
- **v1.ai.hosting:** Cerebro (bulk) and Athena (chat) as separate gateway consumers, each with
  its own key and budget, chat responsive during a ~/docs-scale run (design page, "Gateway
  load").
- **go-messaging:** the expected-sequence append and the work-queue package, which the capstone
  builds itself.
- **go-web-sdk** and **v1.client.stack:** the chunked upload after graduation, an SSE helper, a
  stage timeline driven by the event protocol, and citations in the chat thread.
- **go-core's graph API:** both composition roots are written against it.

## Gates

The capstone runs in wave D of the roadmap (`waves.md`), as the sole active goal apart from
sites.status-reports and sites.training. Before it:

- v1.ai.hosting builds the hosts, the gateway with its overflow, and the multilingual embedding
  model.
- v1.ai.processing built docproc and its system tools, closed by acceptance over ~/docs, and
  v1.ai.inference builds docinfer on go-ai's released library, closed by acceptance that reruns
  the real workloads.
- v1.client.stack builds the web client stack the UI builds on.
- v1.data.extensions lands the Postgres-extension strategy.

It needs no harness: its model calls go through go-ai's client, whose Open Responses protocol and
wire-format packages reach every engine. Its repos gain docproc and the libraries the intakes
promote at staging, so a capstone task can fix a tool or a library and use the fix.

## Evidence

Carried from spike-ai-pipeline, which keeps the rest of its evidence:

7. Metadata capture, inference extraction, and embeddings in pgvector with a semantic search,
   each stage transactional on the consumer's side (Athena commits each of Cerebro's result
   batches). The event layer tracks every stage event in sequence, in real time, and the UI shows
   each stage as it runs.
8. The client runs unchanged through spike-model-gateway's gateway.
10. pgvector as a declared native-tier extension: its install, migration, image pin, and the
    packages allowed to use it. v1.data.extensions designs the strategy from these terms, since no
    spike ran pgvector.
15. The browser's consumption of the event sequence, NATS over WebSocket against a backend relay,
    compared on latency, ordering, auth, ports, and native-tier use, on Athena's stream only.

## Design

The first tasks design the protocols and practices, each one's output the next one's input;
build tasks are written once they close. `metadata` also settles where authoritative metadata
capture lives: the web layer, the processing layer, or elsewhere. `text-layer`'s skip rule must
tell a thin layer from a whole one (design page, "Text layer and vision").

## Carried notes

- Web content (HTML, MHTML, WARC, EPUB: text extraction or rendering for vision) waits, unplanned,
  until the capstone needs it.
- The scanned-document OCR tier isn't required: the capstone runs on vision transcription, and
  experiment.ocr settles the tier after it.
- go-ai's `aitest` is its public API, which docinfer, go-ai's `tools/infer`, and Cerebro's tests
  depend on, so a change to it breaks them. The capstone is the next goal to hold go-ai after
  v1.ai.inference and v1.ai.hosting's `secrets`.
