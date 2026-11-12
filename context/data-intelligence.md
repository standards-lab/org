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
service on go-ai's model client, docproc's extraction library, and docinfer's inference library,
and Athena, a prototype
client-facing service with a web UI on the web client stack experiment.html's intake promotes. It takes in messages, uploads, and streams; captures their
metadata, extracts data by inference, and stores embeddings in PostgreSQL with pgvector for
semantic search; and shows each stage as it runs. It came out of spike-ai-pipeline, which proved
the client and processing layers and planned the capstone as its last step (`ai-strategy.md`,
"The client and processing layers"); the capstone moved to a repository of its own,
outside the experiment (docinfer r6; plan, 2026-11-09).

Athena's events go through go-messaging, an outbox in each batch's transaction relayed to
JetStream; Cerebro's job events are appended to its job stream, and its other events go through
go-messaging's outbox ("Cerebro and Athena"). pgvector is a
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

Its input is docinfer's `cmd/docinfer` directory over docproc's `cmd/docproc` run directory, both
schema "2" (docproc's `context/layers.md`, "The nested modules"). v1.ai.processing's `reports`
task makes `cmd/docproc`'s report holistic: an index page plus one page per input, keyed by the
input's SHA-256, each holding the source link, the file and record metadata, and the whole part
tree with every output inline, uncut. docinfer's report, its pages linked to docproc's by the
input's SHA-256 and holding the full vision texts, transcripts, and chunks, is v1.ai.inference's.
spike-ai-pipeline's
[docinfer finding](https://github.com/JaimeStill/spike-ai-pipeline/blob/main/docs/findings/docinfer.md)
ran the spike's commands over all of ~/docs through set A on gb10-2: 3,470 images, 1,254 audio windows, and 28,937
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

- **State.** Cerebro owns its jobs, its uploads, and each job's event stream. PostgreSQL holds
  docproc's index and docinfer's cache, blob storage holds large content (outputs, staged inputs,
  uploads), and JetStream the work queue and the job stream: one shared CEREBRO_JOBS stream
  holding a subject per job. A job is a root Entity whose state is folded from the events on its
  subject. A job's events append directly to its subject under the expected-last-subject-sequence
  check, the transaction that makes the Command's change true, which is all Elemental's Event asks
  (`architecture/architecture.md`, "The elements"), and go-messaging's outbox serves Cerebro's
  other events (`messaging.md`, "A stream-backed root").
- **API.** Go Elemental Queries and Commands, with Events published. A small document arrives in
  one Command; a large one through a chunked upload protocol: open a session, append chunks with
  offsets and digests, complete, and resume after a drop. Chunks wait in blob storage, so any
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
- **Distribution.** Jobs go through go-messaging's work-queue package, which the capstone builds.
  One job per input is the unit of retry; inference fans out inside it, bounded per capability
  (spike-model-layout's capacity targets set the defaults); a crash recovers through the result
  cache and the job's checkpoint (docinfer's `job`). Spreading one input's parts across replicas
  waits until the ~/docs acceptance run shows a replica is the bottleneck.
- **Caching.** Results are keyed by content SHA-256 and stage identity (docproc's dispatch index)
  and by capability, model, settings, and content (docinfer's cache), behind interfaces Cerebro
  backs with PostgreSQL, the content they name held in blob storage by its SHA-256 (docproc's
  `context/layers.md`, "The cache"); content seen before is replayed, not recomputed.
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

**Auth stays out of Cerebro** for the capstone (the architect, 2026-11-11): Athena has already
solved its auth strategy, and the capstone focuses on signal flow without that complexity. The
gateway's per-consumer keys stay, since they carry the priority between bulk work and chat.

**Missing layers** beyond the roadmap: the capstone designs the packaging of the docproc-* tools into
Cerebro's image (docproc's docker runner can't run in a
container without the host's socket); `retrieval` (hybrid ANN with tsvector and rerank; set A has
no reranker) and `cross-service-tracing` wait in the backlog.

### The layers

docproc's `context/layers.md` states each boundary of Cerebro's stack as signatures and schemas,
each traced to this note's design tasks or to spike evidence. The stack has five layers: system
tools → docproc → docinfer ← go-ai, and Cerebro over docproc, docinfer, and go-ai.

- **The tool contracts** are schema "2": JSON Lines with Markdown inside as the text layer, grown
  by addition only from there. Every piece of text a file holds exists once: content in the
  Markdown text layer, labels (a mail store's name, OneNote's ink alternatives) as kept plain-text
  fields. A structured field is cut only when its text is already carried, in the Markdown or in a
  kept field, or it can't be plain text and has no processor, as measured over ~/docs; content a
  tool reads but leaves out of its Markdown is rendered into it, never kept beside it as a
  duplicate field.
- **docproc** (`github.com/standards-lab/docproc`) extracts: inspection, routing, the tool runners
  and readers, dispatch with its content store and its index, the stages, and `cmd/docproc`. Its
  library module requires `golang.org/x/text` alone and depends on no model concept. Each store is
  an interface the library declares, with filesystem and memory implementations in the base module
  and providers as nested sub-modules named for their target systems (`docproc/storage` over
  go-storage, `docproc/postgres` over sqlate), each pinning only its own level. The dispatcher
  stores each output's content by its SHA-256 before its index records it, and a seen part
  replays its recorded log in the order it was prepared.
- **docinfer** (`github.com/standards-lab/docinfer`), an adjacent library above go-ai's tier,
  infers. Its root package holds the pipeline: vision, transcription, extraction, and embedding
  over go-ai's client, called directly, a cache, and inference texts, extraction progress,
  batches, and counts yielded as made. `chunk` is the chunker, and `job` runs one job's dispatch
  and inference, cancelled by its own context, putting each inference text into docproc's content
  store and yielding its updates. `docinfer/postgres` is its cache's provider, and `cmd/docinfer`
  its command. It holds the go-ai failure mapping (`layers.md`, "docinfer's go-ai use"): a
  refusal, recorded and held, only for a verdict on the content, and a permanent error failing the
  job at once.
- **Shapes.** Neither library has an events package, emits CloudEvents itself, or declares event
  kinds, so neither has a `messaging.go`. Each package that produces a change holds its JSON
  shapes in its ordinary files, beside its other types: dispatch's events, parts, outputs, and
  warnings, versioned by `dispatch.Version`; docinfer's inference texts, progress, batches,
  chunks, counts, held values, and profile, and job's checkpoint and done, versioned by
  `docinfer.Version`.
- **Cerebro's API** starts from Go Elemental Commands (Submit, OpenUpload, AppendChunk,
  CompleteUpload, AbortUpload, CancelJob) and Queries (GetJob, GetUpload, GetOutput,
  ListProfiles).
- **Cerebro's state.** Cerebro holds only its jobs, its uploads, and each job's event stream;
  docproc's and docinfer's stores are facets of its internal state, behind the libraries'
  interfaces and wired to providers at its composition root, and it holds no state for an
  external service. It runs blob storage (staged inputs, upload chunks, content by SHA-256),
  PostgreSQL (the index, the cache, and go-messaging's outbox and inbox through
  `go-messaging/postgres` for its events other than a job's), and NATS JetStream on file storage
  (the streams CEREBRO_WORK, CEREBRO_JOBS, and CEREBRO_UPLOADS if the upload keeps one). Each
  job's events go on one subject of its own in CEREBRO_JOBS, `cerebro.job.<job>`, for the job's
  gap-free sequence, appended directly under the expected-last-subject-sequence check.
  `messaging.md`'s subject and stream conventions govern a web service's events for committed
  command mutations, not a job's progress and result stream.
- **Cerebro's events** are CloudEvents 1.0 in binary mode on NATS headers. Cerebro defines their
  `cerebro.job.*` types in its own `messaging.go`, each with `event.Define` over its own payload:
  part, output, and warning over docproc's `dispatch.Event`; inference (vision, transcription, and
  extraction) over docinfer's `Inference`; progress over its `Progress`; batch over its `Batch`,
  carrying the batch's digest and chunk count as extension attributes for a headers-only fold;
  checkpoint over `job.Checkpoint`; and the rest over Cerebro's own data types beside its other
  types. The type and subject carry the kind and the job, so no payload repeats them, and
  `dataschema` carries the payload's version. A batch is identified by its job and its event's
  sequence, never renumbered across attempts. Each attempt's `started` carries the resolved
  profile. A `completed` event carries the manifest: its range is every sequence from 1 through the
  `completed` event's own, with the batch and chunk counts and a digest over every attempt's
  batches, and each attempt's counts as docinfer reported them. A replica whose append fails its
  expected-sequence check reads the subject's tail and stops on a `cancel_requested` or a newer
  `started`.
- **What it asks of go-ai**, from docinfer and Cerebro: a transcription request that can reopen
  its clip, so `Transcribe` is retried and honors Retry-After; the retry of 408, 429, and 503
  proven against the live gateway; and the surface docinfer calls kept (`Generate`,
  `GenerateStream` with its accumulator, `Embed`, `Transcribe`, the API error's status and
  message, the usage fields, and the three formats and query parameters `cmd/docinfer` uses).
- **What it asks of go-messaging**, sized by `state` and `events`: its NATS provider publishes on
  the event's type and filters a durable consumer by types, so Cerebro's job stream needs a
  publish on a subject the caller names, a consumer filtered by subject, the CloudEvents
  `sequence` extension carried, and events delivered in order within a delivery group; and a
  work-queue delivery lasts its `AckWait`, so a long job needs a delivery its handler extends
  while it runs.
- **Open items** pass to `protocol` (live sources, the small-document size), `events` (the batch
  cap, the `dataschema` form), `state` (the stores', streams', and keys' names and retentions, and
  how a job whose last delivery died is found and failed), `chunks` (storing chunks by identity),
  `metadata` (typed metadata shapes, a message's threading keys on its output event),
  `athena-mapping` (typed cells for `sparse_rows`), `text-layer` (OCR text read from the
  Markdown's image text), and v1.ai.client (transcription retry).

### What it asks of the goals beneath it

Each is recorded on its roadmap entry, so the goal designs around Cerebro:

- **v1.ai.client (go-ai):** Cerebro is its first long-running service consumer. Retry proven
  against the live gateway's 429, 503, and Retry-After; per-call timeouts and cancellation by
  context; a per-consumer key, the bearer token, in a header of its own only if the gateway asks
  for one (docproc's `context/layers.md`, "Requests to go-ai"); Embed batching; streaming Generate
  whose events Cerebro can relay. Concurrency bounds stay the caller's.
- **v1.ai.processing (docproc):** the index and the content store behind interfaces docproc
  declares, with filesystem and memory implementations and the providers `docproc/postgres`
  (PostgreSQL through sqlate) and `docproc/storage` (object storage through go-storage); input
  from a stream or spooled to scratch, its SHA-256 checked once; stable exported event shapes; the
  exec runner inside an image that bundles the tools.
- **v1.ai.inference (docinfer):** docinfer's orchestration (vision, transcription, extraction by a
  caller's prompt and schema, chunking, embedding, the result cache) in an adjacent library above
  go-ai that calls go-ai directly, with `cmd/docinfer` its command; results in batches as made;
  the cache behind an interface, with the provider `docinfer/postgres` (PostgreSQL through
  sqlate); cancellation per job; a go-ai failure mapping that refuses only a verdict on the
  content and fails the job at once on a permanent error.
- **v1.ai.processing's tools:** docproc-ooxml, docproc-mail, and docproc-onenote ship inside
  Cerebro's image, each pinned by its `tools/<format>/v<version>` tag.
- **v1.ai.hosting and spike-model-gateway:** Cerebro (bulk) and Athena (chat) are separate
  consumers, each with a key and a budget; 429 and queueing behave as go-ai's retry expects; chat
  stays responsive during a ~/docs-scale run (3,470 vision images, 20,395 s of vision in
  docinfer's run; vision latency on gb10-2 rose to 38.9 s an image under mixed load,
  spike-model-layout's `docs/layout.md`). The gateway routes to the self-hosted models first and
  falls back to cloud-hosted models when they are at capacity; which workloads may leave the
  self-hosted machines is open. The models are laid out across the two GB10s and the Framework
  (AMD AI Max+ 395), with a vLLM instance per model as a target to weigh for concurrent inference
  (the architect, 2026-11-11).
- **go-messaging (the capstone's `messaging-append`, `messaging-work-queue`, and
  `messaging-object-store` tasks):** the expected-sequence append, the work-queue package, and an
  object-store API. The capstone holds go-messaging and builds them itself.
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
- v1.ai.processing builds docproc and its system tools, v1.ai.client builds go-ai, and
  v1.ai.inference builds docinfer once go-ai's library releases, each closed by acceptance that
  reruns the real workloads: v1.ai.client's reruns infer against set A, v1.ai.processing's reruns
  `cmd/docproc` over ~/docs, as corpus.docs settles it, and v1.ai.inference reruns docinfer.
- experiment.html's intake runs, and v1.client promotes the web client stack the UI builds on.
- graph-ergonomics settles go-core's `graph` and `lifecycle`, and v1.data.extensions lands the
  Postgres-extension strategy.

The capstone needs no harness: its model calls go through go-ai's client, whose Open Responses
protocol and wire-format packages reach every engine. Its repos gain docproc at staging, and the
libraries the intakes promote, so a capstone task can fix a tool or a library and use the fix
(plan, 2026-11-12).

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
  "2" metadata and docinfer's chunk records are the input the pgvector load reads.
- **chunks**: the chunk record and the embedding spaces the pgvector load reads, from the schema
  "2" output of docproc's `cmd/docproc` and docinfer's `cmd/docinfer`.
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
- **state**: the stores, the streams, and the job's fold.
- **events**: Cerebro's stage and result-batch event protocol, and the browser's transport on
  Athena's stream (evidence 15).
- **athena-mapping**: `~/s2va/athena`'s ingest contract onto Cerebro ("Cerebro and Athena").
- **tool-packaging**: the docproc-* tools in Cerebro's image, run through docproc's exec runner.

## Carried notes

- Web content (HTML, MHTML, WARC, EPUB: text extraction or rendering for vision) waits, unplanned,
  until the capstone needs it (plan, 2026-11-03).
- The scanned-document OCR tier isn't required: the capstone runs on vision transcription, and
  experiment.ocr settles the tier after it.
