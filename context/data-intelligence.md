# data-intelligence

The workspace's first capstone, `capstone.data-intelligence` in the roadmap: an application that
brings the workspace's libraries together end to end. Its name is the point of it: the
organization's data sits in binary files on a file share, and the capstone gets it out of them.
It spans three repositories, one per layer, each under JaimeStill, checked out at
`~/capstones/<repo>`, and created when the goal is staged (`naming.md`, "Capstones"):
capstone-data-intelligence, the integration root; capstone-cerebro; and capstone-athena. This
note holds what the capstone carries until they exist; the root's `init` takes the note in.

## What it is

Two services and the model gateway beneath them ("Cerebro and Athena"): Cerebro, a processing
service on go-ai's model client and unravel's processing library, and Athena, a prototype
client-facing service with a web UI on the web client stack experiment.html's intake promotes. It takes in messages, uploads, and streams; captures their
metadata, extracts data by inference, and stores embeddings in PostgreSQL with pgvector for
semantic search; and shows each stage as it runs. It came out of spike-ai-pipeline, which proved
the client and processing layers and planned the capstone as its last step (`ai-strategy.md`,
"The client and processing layers"); the capstone moved to a repository of its own,
outside the experiment (docinfer r6; plan, 2026-11-09).

Athena's events go through go-messaging, an outbox in each batch's transaction relayed to
JetStream; Cerebro's are appended to its job streams ("Cerebro and Athena"). pgvector is a
declared native-tier extension in capstone-athena, and v1.data.extensions writes the workspace's
extension strategy from this case. Source blobs stay in Athena's blob storage; its database holds what the
stages derive from them, each stage's output a versioned artifact (source blob hash → extraction → chunk → embedding)
keyed by its input and configuration hash (model, revision, prompt, parameters, chunker version),
so a stage reruns only when its input or configuration changes. Embeddings live in registered
embedding spaces (model, revision, dims, metric, normalization, prompt template, chunker), vectors
partitioned by space with one partial ANN index each. A model change adds a space and re-embeds
the stored chunks (blue/green, switched after a recall evaluation), a chunker change re-chunks,
and only an extractor change reruns inference. Translation between spaces is limited to
truncation and quantization within one model; a learned mapping only bridges a migration. The
`chunks` design task settles this.

Its input is docinfer's directory over docproc's run directory, both schema "1", from unravel's
`cmd/docinfer` and `cmd/docproc`. v1.ai.processing's `reports` task makes their reports holistic:
an index page plus one page per input, docproc's and docinfer's pages linked by the input's
SHA-256, each holding the source link, the file and record metadata, the whole part tree with
every output inline, and the full vision texts, transcripts, and chunks, uncut. spike-ai-pipeline's
[docinfer finding](https://github.com/JaimeStill/spike-ai-pipeline/blob/main/docs/findings/docinfer.md)
ran them over all of ~/docs through set A on gb10-2: 3,470 images, 1,254 audio windows, and 28,937
chunk lines in 6.0 h, with 0 failures.

## Cerebro and Athena

The architect's whiteboard architecture, settled in plan rounds (plan, 2026-11-10). Its purpose
is to show how a processing service and the model gateway replace the `ingest` layer of the
existing Athena (`~/s2va/athena`) without carrying Athena's complexity into the capstone, so that
Athena becomes a standard web service and what it defines internally sinks into libraries.

**Cerebro** (capstone-cerebro) is a document-processing service from go-web-sdk-template that
never knows its caller. It queues and orchestrates processing, sends inference through the model
gateway with go-ai, and emits its progress, inference messages, and results as events. It could
serve any consumer, a harness among them.

- **State.** No SQL and no data it owns. Its operational state is in JetStream and disposable: a
  work-queue stream of jobs, one event stream per job, and an object store with a TTL for staged
  uploads and cached results. A job is a root Entity whose state is folded from the events on its
  subject; appending an Event with an expected-last-sequence check is the transaction that makes
  the Command's change true, which is all Elemental's Event asks (`architecture/architecture.md`,
  "The elements"). One store, so no outbox and no relay (`messaging.md`, "A stream-backed root").
- **API.** Go Elemental Queries and Commands, with Events published. A small document arrives in
  one Command; a large one through a chunked upload protocol: open a session, append chunks with
  offsets and digests, complete, and resume after a drop. Chunks wait in the object store, so any
  replica takes any chunk. Cerebro proves the protocol shaped to graduate: once capstone-athena
  reuses it for browser uploads, it moves to go-web-sdk, its browser half to v1.client's sdk
  (`chunked-upload-graduation`). A nested `client` module, tagged `client/vX`, holds the protocol
  types and the Go client; consumers import it, and Cerebro imports none of them.
- **Results.** A document's results don't fit one event: NATS's default `max_payload` is 1 MB,
  and ~/docs' 28,937 chunk lines with 768-dimension vectors come to about 200 MB. Cerebro emits
  ordered result-batch events (chunk records and vectors) and a final event carrying a manifest:
  counts, a digest, and the sequence range. The consumer commits each batch in one transaction,
  de-duplicated through go-messaging's inbox, and checks completeness against the manifest, so
  evidence 7's transactional stages are the consumer's.
- **Distribution.** Jobs go through go-messaging's planned work-queue package. One job per input
  is the unit of retry; inference fans out inside it, bounded per capability (spike-model-layout's
  capacity targets set the defaults); a crash recovers through the result cache and unravel's
  checkpoint. Spreading one input's parts across replicas waits until the ~/docs acceptance run
  shows a replica is the bottleneck.
- **Caching.** Results are keyed by content SHA-256 and stage identity (unravel's dispatch index)
  and by capability, profile, and content (docinfer's held results), behind a cache interface
  Cerebro backs with the object store; content seen before is replayed, not recomputed.
- **Models and stages.** A request names a versioned processing profile in Cerebro's
  configuration, never models; each result records the profile's resolved identity (models,
  dimensions, chunker version), so the consumer registers the matching embedding space. Cerebro
  calls the gateway with its own consumer key. Stages: inspect, extract, vision, transcribe,
  chunk, embed, and an extraction stage driven by a caller's prompt and JSON schema, whose hash
  joins the cache key. Regex entities and sparse rows stay the consumer's.
- **Browser.** Cerebro's events are service-to-service. The browser reads only the consumer's
  stream, so Cerebro needs no per-user authorization on its subjects.

**Athena** (capstone-athena) is a prototype of the client-facing service: upload, blob storage,
SQL with pgvector, RAG chat through the gateway, and live stages, its UI embedded through
`NewClient`. It is the authoritative source of data and a standard web service. No code comes from
`~/s2va/athena`; the `athena-mapping` design task maps the real Athena onto Cerebro, and its own
migration is later work outside the workspace. Facts from `~/s2va/athena` the mapping starts from:

- Its `gateway/` is an Envoy OAuth2 and JWT proxy, not a model gateway; models route in-process
  (`server/internal/infrastructure/ai`, chains with a circuit breaker). Cerebro and the gateway
  replace both `cmd/ingest` and that router.
- It runs no NATS: a Postgres `ingest_jobs` queue (`SKIP LOCKED`, leases, fencing) and
  `pg_notify`, and the browser sees only claimed, done, and failed.
- Chat reads `chunks` rows (vault, `<version>-<i>` id, content, page, blob key, chunk 0's summary
  and `processing_metadata`), a 1024-dimension snowflake-arctic-embed-l-v2.0 embedding, and
  `entities` and `sparse_rows`. The workspace embeds at 768 dimensions (granite-embedding-english-r2);
  Athena's registered embedding spaces carry the blue/green re-embed.
- Its ingest also runs Tesseract OCR, Whisper, a summary chain, regex entities, tabular rows into
  `sparse_rows`, and TEI reranking. Summary maps onto Cerebro's extraction stage; the rest stays
  Athena's.

**capstone-data-intelligence** is the integration root: the goal record, the compose stack
(Cerebro, Athena, NATS, Postgres with pgvector, the gateway), and the acceptance run over ~/docs
(592 files, 1.7 GB), measured against docinfer's baseline: 6.0 h, and a 3.1 s rerun.

**Missing layers** beyond the roadmap: the capstone designs service-to-service credentials
(Athena to Cerebro, the consumer keys, NATS credentials, ahead of v1.auth and `nats-credentials`)
and the packaging of the tools into Cerebro's image (unravel's docker runner can't run in a
container without the host's socket); `retrieval` (hybrid ANN with tsvector and rerank; set A has
no reranker) and `cross-service-tracing` wait in the backlog.

### What it asks of the goals beneath it

Each is recorded on its roadmap entry, so the goal designs around Cerebro:

- **v1.ai.client (go-ai):** Cerebro is its first long-running service consumer. Retry proven
  against the live gateway's 429, 503, and Retry-After; per-call timeouts and cancellation by
  context; a per-consumer key header; Embed batching; streaming Generate whose events Cerebro can
  relay. Concurrency bounds stay the caller's.
- **v1.ai.processing (unravel):** docinfer's orchestration (vision, transcription, chunking,
  embedding, the result cache) moves into the library behind inference interfaces unravel
  defines, free of go-ai, with `cmd/docinfer` a thin go-ai composition; results come out in
  batches as made; the index and cache behind interfaces; input from a stream or spooled to
  scratch; stable exported event shapes; cancellation per job; the exec runner inside an image
  that bundles the tools.
- **v1.ai.tools:** office-read, mail-read, and onenote-read ship inside Cerebro's image, pinned by
  their contracts' releases.
- **v1.ai.hosting and spike-model-gateway:** Cerebro (bulk) and Athena (chat) are separate
  consumers, each with a key and a budget; 429 and queueing behave as go-ai's retry expects; chat
  stays responsive during a ~/docs-scale run (3,470 images at 38.9 s each on gb10-2).
- **go-messaging (`messaging-streams`):** the expected-sequence append, the work-queue package,
  and an object-store API.
- **go-web-sdk:** the chunked upload primitive after graduation, and an SSE helper for Athena's
  chat.
- **v1.data.extensions:** pgvector lives in capstone-athena only.
- **v1.client:** resumable chunked upload, a stage timeline driven by the event protocol rather
  than hard-coded, and citations in the chat thread.
- **graph-ergonomics:** both composition roots are written against its release.

## Gates

The capstone runs in wave D of the roadmap (`roadmap.toml`, header), as the sole active goal
apart from status-reports and training. Before it:

- experiment.hosting's spike-model-layout and spike-model-gateway answer, and experiment.hosting's
  intake runs; v1.ai.hosting builds ai-hosting from it.
- v1.ai.tools, v1.ai.client, and v1.ai.processing build the tools, go-ai, and unravel, each
  closed by an acceptance task that reruns the real workloads: v1.ai.client's reruns infer
  against set A, and v1.ai.processing's reruns docproc and docinfer over ~/docs.
- experiment.html's intake runs, and v1.client promotes the web client stack the UI builds on.
- graph-ergonomics settles go-core's `graph` and `lifecycle`, and v1.data.extensions lands the
  Postgres-extension strategy.

The capstone needs no harness: its model calls go through go-ai's client, whose Open Responses
protocol and wire-format packages reach every engine. Its repos gain the libraries the intakes promote,
so a capstone task can update a library and use the change.

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

The capstone's first tasks design its protocols and practices, each one's output the next one's
input; build tasks are written once they close.

- **metadata**: compositional metadata structures that extend docproc's base file metadata per
  format (PDF, office, mail, OneNote, media), and where authoritative metadata capture lives: the
  web layer, the processing layer, or elsewhere (the architect, docproc r4 q6). docproc's schema
  "1" metadata and docinfer's chunk records are the input the pgvector load reads.
- **chunks**: the chunk record and the embedding spaces the pgvector load reads, from the schema
  "1" output of unravel's `cmd/docproc` and `cmd/docinfer`.
- **text-layer**: whether to skip vision on a page with a text layer. The docinfer finding's
  text-layer comparison is the evidence for a skip rule: where a layer holds the page, vision
  repeats it (median word recall 0.91 to 1.00); vision rescued 49 pages with a thin layer and 61
  with none; two-column text layers interleave their columns; and 24 of 1,289 text-heavy pages
  came back described instead of transcribed. A skip rule must tell a thin layer from a whole one.
- **vision**: the vision prompt and sampling. The finding recommends a stricter prompt
  ("Transcribe all the text in this picture, word for word, in reading order. Do not summarize or
  describe text…") and a lower temperature against described pages.
- **protocol**: Cerebro's Queries and Commands, the small-document Command, and the chunked
  upload protocol.
- **state**: the job stream, the work queue, and the object store, and the job's fold.
- **events**: Cerebro's stage and result-batch event protocol, and the browser's transport on
  Athena's stream (evidence 15).
- **athena-mapping**: `~/s2va/athena`'s ingest contract onto Cerebro ("Cerebro and Athena").
- **credentials**: service-to-service credentials ahead of v1.auth.
- **tool-packaging**: the tools in Cerebro's image, run through unravel's exec runner.

## Carried notes

- Web content (HTML, MHTML, WARC, EPUB: text extraction or rendering for vision) waits, unplanned,
  until the capstone needs it (plan, 2026-11-03).
- The scanned-document OCR tier isn't required: the capstone runs on vision transcription, and
  experiment.ocr settles the tier after it.
