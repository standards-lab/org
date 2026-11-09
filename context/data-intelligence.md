# data-intelligence

The workspace's first capstone, `capstone.data-intelligence` in the roadmap: an application that
brings the workspace's libraries together end to end. Its name is the point of it: the
organization's data sits in binary files on a file share, and the capstone gets it out of them.
It lives in its own repository, [JaimeStill/data-intelligence](https://github.com/JaimeStill/data-intelligence),
checked out at `~/capstones/data-intelligence` and created when the goal is staged (`naming.md`,
"Capstones"). This note holds what the capstone carries until that repository exists; its `init`
takes the note in.

## What it is

A backend on go-ai's model client and unravel's processing library, and a web UI on the web client
stack experiment.html's intake promotes. It takes in messages, uploads, and streams; captures their
metadata, extracts data by inference, and stores embeddings in PostgreSQL with pgvector for
semantic search; and shows each stage as it runs. It came out of spike-ai-pipeline, which proved
the client and processing layers and planned the capstone as its last step (`ai-strategy.md`,
"The client and processing layers"); the capstone moved to a repository of its own,
outside the experiment (docinfer r6; plan, 2026-11-09).

Its events go through go-messaging, an outbox in each stage's transaction relayed to JetStream.
pgvector is a declared native-tier extension, and v1.data.extensions writes the workspace's
extension strategy from this case. Source blobs stay in blob storage; the database holds what the
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
   each stage transactional. The event layer tracks every stage event in sequence, in real time,
   and the UI shows each stage as it runs.
8. The client runs unchanged through spike-model-gateway's gateway.
10. pgvector as a declared native-tier extension: its install, migration, image pin, and the
    packages allowed to use it. v1.data.extensions designs the strategy from these terms, since no
    spike ran pgvector.
15. The browser's consumption of the event sequence, NATS over WebSocket against a backend relay,
    compared on latency, ordering, auth, ports, and native-tier use.

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
- **events**: the stage event protocol and the browser's transport (evidence 15).

## Carried notes

- Web content (HTML, MHTML, WARC, EPUB: text extraction or rendering for vision) waits, unplanned,
  until the capstone needs it (plan, 2026-11-03).
- The scanned-document OCR tier isn't required: the capstone runs on vision transcription, and
  experiment.ocr settles the tier after it.
