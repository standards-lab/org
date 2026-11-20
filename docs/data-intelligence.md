# data-intelligence: design detail

The detail `capstone.data-intelligence`'s design tasks start from. The capstone's intent and
decisions are in [its note](../context/data-intelligence.md); Cerebro's boundaries as signatures
and schemas are in docproc's `context/layers.md`, "Cerebro's API and events".

## Embedding spaces

For the `chunks` task. Source blobs stay in Athena's blob storage; its database holds what the
stages derive from them, each stage's output a versioned artifact (source blob hash → extraction →
chunk → embedding) keyed by its input and configuration hash (model, revision, prompt, parameters,
chunker version), so a stage reruns only when its input or configuration changes. Embeddings live
in registered embedding spaces (model, revision, dims, metric, normalization, prompt template,
chunker), vectors partitioned by space with one partial ANN index each. A model change adds a space
and re-embeds the stored chunks (blue/green, switched after a recall evaluation), a chunker change
re-chunks, and only an extractor change reruns inference. Translation between spaces is limited to
truncation and quantization within one model; a learned mapping only bridges a migration.

## Cerebro

For the `protocol`, `state`, and `events` tasks.

- **State.** PostgreSQL holds docproc's index and docinfer's cache, and go-messaging's outbox and
  inbox through `go-messaging/postgres` for Cerebro's events other than a job's; blob storage holds
  large content (outputs, staged inputs, upload chunks, content by SHA-256); JetStream on file
  storage holds the work queue and the job stream: one shared CEREBRO_JOBS stream with a subject
  per job, `cerebro.job.<job>`, for the job's gap-free sequence (and CEREBRO_UPLOADS if the upload
  keeps one). `messaging.md`'s subject and stream conventions govern a web service's events for
  committed command mutations, not a job's progress and result stream. Cerebro holds no state for
  an external service.
- **API.** The Commands start from Submit, OpenUpload, AppendChunk, CompleteUpload, AbortUpload,
  and CancelJob, the Queries from GetJob, GetUpload, GetOutput, and ListProfiles. The chunked
  upload opens a session, appends chunks with offsets and digests, completes, and resumes after a
  drop. Chunks wait in blob storage, so any replica takes any chunk. Once graduated, the protocol
  moves to go-web-sdk and its browser half to web-app-sdk. The nested `client` module is tagged
  `client/vX`; consumers import it, and Cerebro imports none of them.
- **Results.** ~/docs' 28,937 chunk lines with 768-dimension vectors come to about 200 MB. Each
  result-batch event carries chunk records and vectors; the final event's manifest carries counts,
  a digest, and the sequence range. The consumer de-duplicates batches through go-messaging's inbox
  and checks completeness against the manifest.
- **Distribution.** Inference inside a job is bounded per capability, spike-model-layout's
  capacity targets setting the defaults; a crash recovers through the result cache and the job's
  checkpoint (docinfer's `job`).
- **Caching.** docproc's dispatch index keys by content SHA-256 and stage identity, docinfer's
  cache by capability, model, settings, and content; the content they name is held in blob storage
  by its SHA-256 (docproc's `context/layers.md`, "The cache").
- **Models and stages.** Profiles live in Cerebro's configuration; the resolved identity names the
  models, dimensions, and chunker version. Cerebro calls the gateway with its own consumer key.
  Stages: inspect, extract, vision, transcribe, chunk, embed, and an extraction stage driven by a
  caller's prompt and JSON schema, whose hash joins the cache key.

### Open items

Questions the layers leave to the capstone's tasks: `protocol` (live sources, the small-document
size), `events` (the batch cap, the `dataschema` form), `state` (the stores', streams', and keys'
names and retentions, and how a job whose last delivery died is found and failed), `chunks`
(storing chunks by identity), `metadata` (typed metadata shapes, a message's threading keys on its
output event), `athena-mapping` (typed cells for `sparse_rows`), `text-layer` (OCR text read from
the Markdown's image text), and v1.ai.client (transcription retry).

## Athena onto Cerebro

For the `athena-mapping` task: the facts from `~/s2va/athena` the mapping starts from.

- Its `gateway/` is an Envoy OAuth2 and JWT proxy, not a model gateway; models route in-process
  (`server/internal/infrastructure/ai`, chains with a circuit breaker). Cerebro and the gateway
  replace both `cmd/ingest` and that router.
- It runs no NATS: a Postgres `ingest_jobs` queue (`SKIP LOCKED`, leases, fencing) and
  `pg_notify`, and the browser sees only claimed, done, and failed.
- Chat reads `chunks` rows (vault, `<version>-<i>` id, content, page, blob key, chunk 0's summary
  and `processing_metadata`), a 1024-dimension snowflake-arctic-embed-l-v2.0 embedding, and
  `entities` and `sparse_rows`. The workspace embeds at 768 dimensions with
  granite-embedding-english-r2, an English-only model that v1.ai.hosting's `embedding` task
  replaces with a multilingual one, so anything indexed with granite is re-embedded
  (`ai-hosting.md`, "Set A's embedding model"). Athena's registered embedding spaces carry the
  blue/green re-embed.
- Its ingest also runs Tesseract OCR, Whisper, a summary chain, regex entities, tabular rows into
  `sparse_rows`, and TEI reranking. Summary maps onto Cerebro's extraction stage; the rest stays
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

## The input reports

v1.ai.processing's `reports` task makes `cmd/docproc`'s report holistic: an index page plus one
page per input, keyed by the input's SHA-256, each holding the source link, the file and record
metadata, and the whole part tree with every output inline, uncut. docinfer's report, its pages
linked to docproc's by the input's SHA-256 and holding the full vision texts, transcripts, and
chunks, is v1.ai.inference's.
