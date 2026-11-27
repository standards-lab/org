# data-intelligence: design detail

The detail `capstone.data-intelligence`'s design tasks start from. The capstone's intent and
decisions are in [its note](../context/data-intelligence.md); Bunrei's boundaries as signatures
and schemas are in docproc's `context/bunrei.md`.

## Embedding spaces

For the `chunks` task. Source blobs stay in Mikoshi's blob storage; its database holds what the
stages derive from them, each stage's output a versioned artifact (source blob hash → extraction →
chunk → embedding) keyed by its input and configuration hash (model, revision, prompt, parameters,
chunker version), so a stage reruns only when its input or configuration changes. Embeddings live
in registered embedding spaces (model, dims, metric, normalization, prompt template), in one table
with a partial ANN index per space ([chunks](https://github.com/JaimeStill/capstone-data-intelligence/blob/main/context/chunks.md)). A model change adds a space
and re-embeds the stored chunks (blue/green, switched after a recall evaluation), a chunker change
re-chunks, and only an extractor change reruns inference. Translation between spaces is limited to
truncation and quantization within one model; a learned mapping only bridges a migration.

## Bunrei

The service's settled shape:

- **State.** Bunrei owns only its jobs, its uploads, and each job's event stream; docproc's and
  docinfer's stores are facets of its internal state. A job is a root Entity folded from the
  events appended to its own JetStream subject under the expected-last-subject-sequence check,
  all Elemental's Event asks (`architecture/architecture.md`, "The elements");
  go-messaging's outbox serves its other events (`messaging.md`, "A stream-backed root").
- **API.** Go Elemental Queries and Commands over HTTP, with Events published on NATS; a large
  document arrives through go-web-sdk's resumable upload (draft-ietf-httpbis-resumable-upload-16),
  which Mikoshi mounts too. A nested `client` module holds the protocol types and the Go client
  ([protocol](https://github.com/JaimeStill/capstone-data-intelligence/blob/main/context/protocol.md), [upload](https://github.com/JaimeStill/capstone-data-intelligence/blob/main/context/upload.md)).
- **Results.** Results don't fit one NATS event, so Bunrei emits ordered result-batch events
  closed by a manifest; the consumer commits each batch in one transaction (evidence 7).
- **Distribution.** Jobs go through go-messaging's work-queue package, one job per input the
  unit of retry, inference fanned out inside it; spreading an input's parts across replicas
  waits until the acceptance run shows a replica is the bottleneck. docinfer ships no defaults;
  Bunrei sets every starting value (design page, "docinfer's configuration").
- **Caching.** Results are keyed by content and stage identity (docproc) and by capability,
  model, settings, and content (docinfer's root `doc.go`); content seen before is replayed.
- **Models and stages.** A request names a versioned processing profile, never models; each result
  records the profile's resolved identity, so the consumer registers the matching embedding space.
  Regex entities and sparse rows stay the consumer's.
- **Browser.** The browser reads only the consumer's stream, so Bunrei's events stay
  service-to-service, with no per-user authorization on its subjects.

The detail for the `protocol`, `state`, and `events` tasks:

- **State.** PostgreSQL holds docproc's index and docinfer's cache, and go-messaging's outbox and
  inbox through `go-messaging/postgres` for Bunrei's events other than a job's; blob storage holds
  large content (outputs, staged inputs, upload chunks, content by SHA-256); JetStream on file
  storage holds the work queue and the job stream: one shared BUNREI_JOBS stream with a subject
  per job, `bunrei.job.<job>`, for the job's gap-free sequence (and BUNREI_UPLOADS if the upload
  keeps one). `messaging.md`'s subject and stream conventions govern a web service's events for
  committed command mutations, not a job's progress and result stream. Bunrei holds no state for
  an external service.
- **API.** The Commands are Submit, CompleteUpload, and CancelJob, the Queries GetJob, GetOutput,
  and ListProfiles ([protocol](https://github.com/JaimeStill/capstone-data-intelligence/blob/main/context/protocol.md)). The upload itself is go-web-sdk's handler,
  mounted with a chunk store over Bunrei's blob storage, so any replica takes any chunk; its
  browser client is web-app-sdk's ([upload](https://github.com/JaimeStill/capstone-data-intelligence/blob/main/context/upload.md)). The nested `client` module is tagged
  `client/vX`; consumers import it, and Bunrei imports none of them.
- **Results.** ~/docs' 28,937 chunk lines came to about 200 MB, sized with granite's
  768-dimension vectors; e5's have 1,024. Each result-batch event carries chunk records and
  vectors; the final event's manifest carries counts, a digest, and the sequence range. The
  consumer de-duplicates batches through go-messaging's inbox and checks completeness against the
  manifest.
- **Distribution.** Inference inside a job is bounded per capability. docinfer ships no defaults
  (docinfer's [`docs/no-defaults.md`](https://github.com/standards-lab/docinfer/blob/main/docs/no-defaults.md)),
  so Bunrei sets the starting values of `Bounds`, `Pending`, `BatchBytes`, and
  `join.Request.WordsPerSecond` from spike-model-layout's capacity targets, and of
  `job.Config.CheckpointEvery`, which sets how much work a crash repeats. A crash recovers through
  the result cache and the job's checkpoint (docinfer's `job`).
- **Caching.** docproc's dispatch index keys by content SHA-256 and stage identity, docinfer's
  cache by capability, model, settings, and content (docinfer's root
  [`doc.go`](https://github.com/standards-lab/docinfer/blob/main/doc.go)); the content they name is
  held in blob storage by its SHA-256.
- **Models and stages.** Profiles live in Bunrei's configuration; the resolved identity names the
  models, dimensions, and chunker version. Bunrei calls the gateway with its own consumer key.
  Stages: inspect, extract, vision, transcribe, chunk, embed, and an extraction stage driven by a
  caller's prompt and JSON schema, whose hash joins the cache key.

### docinfer's configuration

Bunrei's docinfer configuration starts from the settings docinfer's acceptance settled (docinfer's
[`docs/acceptance.md`](https://github.com/standards-lab/docinfer/blob/main/docs/acceptance.md)):

- **Tokenizer.** The gateway serves the embedding tokenizer at `/embedding/v1/tokenize`, so through
  the gateway `embeddings.tokenizer` names the embeddings endpoint, and straight to a row it names
  the row's root. Either way it names the embeddings' own deployment.
- **Chunk length.** `embeddings.chunk_runes` is 8,192, the input length vLLM refuses before
  tokenizing, less the document prefix.
- **Served models.** Each capability's served model comes from the model listing's `root`, else
  its `served_model`.
- **Embedding prefixes.** e5's `input_types` are
  `{"document":{"prefix":"passage: "},"query":{"prefix":"query: "}}`, and Bunrei applies
  `Profile`'s `embedding_query` to its search queries.
- **Vision.** Vision sends Gemma 4's card sampling `{"temperature":1.0,"top_p":0.95,"top_k":64}` at
  concurrency 4, with `max_pixels` 2,580,480 and a 600 s `response_header_timeout`, since a
  non-streaming reply's header waits for up to 2,048 tokens.
- **Transcription.** Chat transcription sends `{"temperature":0}` with `max_tokens` 512 at
  concurrency 32.
- **Hosting rows.** The hosting rows are tuned as general endpoints, never to docinfer's
  requests, so Bunrei shapes its requests to each row's measured limits.

### Open items

Questions the layers leave to the capstone's tasks: `events` (the batch cap, the `dataschema` form),
`state` (the stores', streams', and keys' names and retentions, and how a job whose last delivery
died is found and failed), `metadata` (typed metadata shapes, a message's threading keys on its
output event), `athena-mapping` (typed cells for `sparse_rows`), `text-layer` (OCR text read from
the Markdown's image text), and go-ai (transcription retry).

## Athena onto Bunrei

For the `athena-mapping` task: the facts from `~/s2va/athena` the mapping starts from.

- Its `gateway/` is an Envoy OAuth2 and JWT proxy, not a model gateway; models route in-process
  (`server/internal/infrastructure/ai`, chains with a circuit breaker). Bunrei and the gateway
  replace both `cmd/ingest` and that router.
- It runs no NATS: a Postgres `ingest_jobs` queue (`SKIP LOCKED`, leases, fencing) and
  `pg_notify`, and the browser sees only claimed, done, and failed.
- Chat reads `chunks` rows (vault, `<version>-<i>` id, content, page, blob key, chunk 0's summary
  and `processing_metadata`), a 1024-dimension snowflake-arctic-embed-l-v2.0 embedding, and
  `entities` and `sparse_rows`. The workspace embeds at 1,024 dimensions with
  multilingual-e5-large, which replaced granite-embedding-english-r2, an English-only model, in
  v1.ai.hosting's `embedding` task, so anything indexed with granite is re-embedded
  (`ai-hosting.md`, "Set A's embedding model"). Mikoshi's registered embedding spaces carry the
  blue/green re-embed.
- Its ingest also runs Tesseract OCR, Whisper, a summary chain, regex entities, tabular rows into
  `sparse_rows`, and TEI reranking. Summary maps onto Bunrei's extraction stage; the rest stays
  Athena's.

## Text layer and vision

For the `text-layer` and `vision` tasks. spike-ai-pipeline's
[docinfer finding](https://github.com/JaimeStill/spike-ai-pipeline/blob/main/docs/findings/docinfer.md)
ran the spike's commands over all of ~/docs through set A on gb10-2: 3,470 images, 1,254 audio
windows, and 28,937 chunk lines in 6.0 h, with 0 failures.

- **text-layer.** The finding's text-layer comparison is the evidence for a skip rule: where a
  layer holds the page, vision repeats it (median word recall 0.91 to 1.00); vision rescued 49
  pages with a thin layer and 61 with none; two-column text layers interleave their columns; and
  24 of 1,289 text-heavy pages came back described instead of transcribed.
- **vision.** The finding recommends a stricter prompt ("Transcribe all the text in this picture,
  word for word, in reading order. Do not summarize or describe text…") and a lower temperature
  against described pages.

## Gateway load

For v1.ai.hosting's ask that chat stays responsive during a ~/docs-scale run: docinfer's run sent
3,470 vision images, 20,395 s of vision, and vision latency on gb10-2 rose to 38.9 s an image
under mixed load (spike-model-layout's `docs/layout.md`).

The capstone tunes each client's concurrency per alias live, against ai-hosting's row caps and
findings; a cap bounds only the worst case (ai-hosting's class profiles). No row serves
`/v1/audio/transcriptions`, so clients transcribe in chat mode (`context/ai-hosting.md`, "The audio
role transcribes over chat").

## The input reports

`cmd/docproc`'s report is holistic: an index page plus one page per distinct content, keyed by its
SHA-256, each listing the inputs that hold it, its part tree with each part's record metadata, and
its outputs inline (docproc's `docs/command.md`). docinfer's report, its pages linked to docproc's
by the input's SHA-256 and holding the full vision texts, transcripts, and chunks, is
v1.ai.inference's.

## Tool packaging

For the `tool-packaging` task: what Bunrei's image holds for docproc's tools, which run through
the exec runner.

- **Versions.** The image pins 7zz at 24.07 or later, the floor docproc's README names. It fetches
  each docproc-* tool by its `tools/<format>/v*` tag, never as `latest`, which docproc's tool and
  Go module tags share. docproc builds on 64-bit platforms only.
- **docproc-ooxml.** It links glibc, with its floor in its README. Whether to publish a static musl
  build instead is this task's call.
- **Environment.** The tools run under this task's user with a minimal environment. The image sets
  `TMPDIR` to its scratch volume, since each tool run's temporary files go under docproc's
  `TMPDIR`.
- **Memory.** Replicas are sized from each tool's stated memory bound (docproc's `docs/limits.md`).
  Whenever a memory bound is set, the image bundles util-linux's `prlimit`, since the exec runner
  applies `RLIMIT_AS` through it before exec. docproc's `docs/acceptance.md` gives each tool's
  peak memory, measured on v0.4.0's tool versions ("Peak memory"), under the 2 GiB starting bound,
  which stood, and the bytes cap that bounds what a stage writes ("Bombs").

## Data contexts

For the `contexts`, `items`, `hybrid-search`, `context-tools`, and `standing-analyses` tasks.


The capstone's centerpiece, and the base of its AI story: a context is a node in a tree, as a
blobfs directory is, and bounds what the AI sees, as a working directory bounds a Claude Code
session.

- **Items** are files, records authored or posted through Mikoshi's API, and analyses. An item
  joins a context by reference and may join many; a context covers everything beneath it. Chunks
  and embeddings stay keyed by content, so a reference never re-embeds.
- **Query contexts** are saved filters: their items are whatever matches when read, and each
  new item's "item added" event is checked against them.
- **Hybrid search** fuses tsvector and pgvector rankings by reciprocal rank fusion inside the
  selected context, narrowed by facet filters; no reranker, since set A serves none.
- **Chat** is a tool-calling loop on go-ai's agent alias whose tools (search, list, read, run an
  extraction, read the event history) see only the selected context. It needs no harness, and it
  is design input for experiment.harness.spike-go-harness.
- **Analyses** are saved as items with provenance (context, prompt or schema, model identity,
  cited items), searchable and citable later; **standing analyses** rerun when events say their
  context changed. They are the primitive of a later artifact system of dynamic applications.

The tree lives in Mikoshi, shaped to graduate (`context-tree-graduation`), as do Mikoshi's web
pieces under its `web/` boundary (`mikoshi-web-graduation`).

## Events

For the `events`, `lineage`, `standing-analyses`, and `signal-flow` tasks.


Events carry the state, the lineage, and the triggers, not only a log: causation and correlation
ids let a citation be walked back to the source bytes; one stream feeds the signal-flow view, a
per-layer latency waterfall, queue and retry pressure, and a context's activity; standing
analyses fire from them; and the chat reads them as data. A replica killed mid-job is a scripted
demo of recovery through redelivery, the cache, the checkpoint, and the manifest check.

The signal boundary: a model's token stream travels as SSE inside the one request that asked for
it, and dies with it. The chat's answer goes from go-ai to the browser through Mikoshi, and the
browser disconnecting cancels the generation. Only committed outcomes become go-messaging events,
one per committed turn or analysis, carrying lineage; live views (the stage timeline, signal flow)
read events, never streams. Deltas stay off JetStream: each publish waits on an ack, each outbox
event costs a row and a transaction, and the stream keeps every event until its `MaxAge`. The SSE
codec is go-core's, its writer go-web-sdk's, and go-ai reads through the codec (`.bunrei`'s
`sse-codec`, `sse-writer`, and `go-ai-sse`).

## The presentation

For the `presentation` task.


A live run through the Mikoshi client, directed by a presenterm deck in the root's
`presentation/` (diagrams from `~/tau/diagrams/ingredients`, laid out as
`~/presentations/open-inference-stack`): the signal flow layer by layer, the merits of an
elemental architecture, and each layer's expansion points (external connectors into the context
model, the fold-scrubber, replay to a new consumer, the artifact system, the graduations).
