# data-intelligence: design detail

The detail `capstone.data-intelligence`'s design tasks start from. The capstone's intent and
decisions are in [its note](../context/data-intelligence.md); Cerebro's boundaries as signatures
and schemas are in docproc's `context/cerebro.md`.

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
- **Results.** ~/docs' 28,937 chunk lines came to about 200 MB, sized with granite's
  768-dimension vectors; e5's have 1,024. Each result-batch event carries chunk records and
  vectors; the final event's manifest carries counts, a digest, and the sequence range. The
  consumer de-duplicates batches through go-messaging's inbox and checks completeness against the
  manifest.
- **Distribution.** Inference inside a job is bounded per capability. docinfer ships no defaults
  (docinfer's [`docs/no-defaults.md`](https://github.com/standards-lab/docinfer/blob/main/docs/no-defaults.md)),
  so Cerebro sets the starting values of `Bounds`, `Pending`, `BatchBytes`, and
  `join.Request.WordsPerSecond` from spike-model-layout's capacity targets, and of
  `job.Config.CheckpointEvery`, which sets how much work a crash repeats. A crash recovers through
  the result cache and the job's checkpoint (docinfer's `job`).
- **Caching.** docproc's dispatch index keys by content SHA-256 and stage identity, docinfer's
  cache by capability, model, settings, and content (docinfer's root
  [`doc.go`](https://github.com/standards-lab/docinfer/blob/main/doc.go)); the content they name is
  held in blob storage by its SHA-256.
- **Models and stages.** Profiles live in Cerebro's configuration; the resolved identity names the
  models, dimensions, and chunker version. Cerebro calls the gateway with its own consumer key.
  Stages: inspect, extract, vision, transcribe, chunk, embed, and an extraction stage driven by a
  caller's prompt and JSON schema, whose hash joins the cache key.

### docinfer's configuration

Cerebro's docinfer configuration starts from the settings docinfer's acceptance settled (docinfer's
[`docs/acceptance.md`](https://github.com/standards-lab/docinfer/blob/main/docs/acceptance.md)):

- **Tokenizer.** The gateway serves the embedding tokenizer at `/embedding/v1/tokenize`, so through
  the gateway `embeddings.tokenizer` names the embeddings endpoint, and straight to a row it names
  the row's root. Either way it names the embeddings' own deployment.
- **Chunk length.** `embeddings.chunk_runes` is 8,192, the input length vLLM refuses before
  tokenizing, less the document prefix.
- **Served models.** Each capability's served model comes from the model listing's `root`, else
  its `served_model`.
- **Embedding prefixes.** e5's `input_types` are
  `{"document":{"prefix":"passage: "},"query":{"prefix":"query: "}}`, and Cerebro applies
  `Profile`'s `embedding_query` to its search queries.
- **Vision.** Vision sends Gemma 4's card sampling `{"temperature":1.0,"top_p":0.95,"top_k":64}` at
  concurrency 4, with `max_pixels` 2,580,480 and a 600 s `response_header_timeout`, since a
  non-streaming reply's header waits for up to 2,048 tokens.
- **Transcription.** Chat transcription sends `{"temperature":0}` with `max_tokens` 512 at
  concurrency 32.
- **Hosting rows.** The hosting rows are tuned as general endpoints, never to docinfer's
  requests, so Cerebro shapes its requests to each row's measured limits.

### Open items

Questions the layers leave to the capstone's tasks: `protocol` (live sources, the small-document
size), `events` (the batch cap, the `dataschema` form), `state` (the stores', streams', and keys'
names and retentions, and how a job whose last delivery died is found and failed), `chunks`
(storing chunks by identity), `metadata` (typed metadata shapes, a message's threading keys on its
output event), `athena-mapping` (typed cells for `sparse_rows`), `text-layer` (OCR text read from
the Markdown's image text), and go-ai (transcription retry).

## Athena onto Cerebro

For the `athena-mapping` task: the facts from `~/s2va/athena` the mapping starts from.

- Its `gateway/` is an Envoy OAuth2 and JWT proxy, not a model gateway; models route in-process
  (`server/internal/infrastructure/ai`, chains with a circuit breaker). Cerebro and the gateway
  replace both `cmd/ingest` and that router.
- It runs no NATS: a Postgres `ingest_jobs` queue (`SKIP LOCKED`, leases, fencing) and
  `pg_notify`, and the browser sees only claimed, done, and failed.
- Chat reads `chunks` rows (vault, `<version>-<i>` id, content, page, blob key, chunk 0's summary
  and `processing_metadata`), a 1024-dimension snowflake-arctic-embed-l-v2.0 embedding, and
  `entities` and `sparse_rows`. The workspace embeds at 1,024 dimensions with
  multilingual-e5-large, which replaced granite-embedding-english-r2, an English-only model, in
  v1.ai.hosting's `embedding` task, so anything indexed with granite is re-embedded
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

For the `tool-packaging` task: what Cerebro's image holds for docproc's tools, which run through
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
