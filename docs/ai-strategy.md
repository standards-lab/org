# AI strategy: detail

The detail behind the coordinator's AI note (`context/ai-strategy.md`): spike-ai-pipeline's
design and evidence, the open experiments' setup, and the remaining tasks' specifics.

## What spike-ai-pipeline built

The spike's six repositories are the references each sub-goal's design review reads. Its
decisions and evidence are in its `context/README.md` and findings, and in its goal record's last
version in the root's history. It lives at `~/experiments/spike-ai-pipeline`, remote
[JaimeStill/spike-ai-pipeline](https://github.com/JaimeStill/spike-ai-pipeline), with its five
layer repositories beside it.

- **`spike-ai-pipeline-client`**, go-ai's reference, gives Go primitives over the engines' native
  APIs, built as owned, optimized infrastructure from the best of spike-harness-driver's `model`
  and tau's libraries, on the standard library alone. Its protocol is Open Responses (spec
  2026-04-24) in its stateless subset; client extensions, audio input first, fill the spec's gaps
  and are removed when it adopts them. Four layers import one way: the `client` transport, with
  Generate, GenerateStream, Embed, and Transcribe; `protocol`; the `format` contract; and a
  `format/<api>` adapter per wire format, named by endpoint and chosen by typed constructor:
  `completions` (Chat Completions), `messages` (Anthropic Messages), and `responses` (Open
  Responses). GenerateStream yields Open Responses' streaming events, which a
  `protocol.Accumulator` folds into the Response Generate returns. Retry is opt-in.
- **`spike-ai-pipeline-processing`**, docproc's library reference, prepares messages, documents,
  and streams for native model capabilities before inference; a layer above defines and
  orchestrates the inference and embeddings, and processing runs none. It runs mature system
  tools through adapters it owns (`system-tools.md`), with herald's `internal/format` as its prior
  art.
  - `tool`, on the standard library alone, is the contract: a Tool bound to one program, found
    and version-checked at construction, runs an invocation under the caller's context through
    the exec runner (binaries, and scripts through their interpreter) or the container runner
    (the docker CLI, an image pinned by digest, each run locked down), and reports the run's CPU
    time.
  - `route` identifies content by its signature and names the stage it goes to next, for every
    format in the spike's inventory, "no processor yet" where no stage exists.
  - `picture` converts TIFF, EMF, WMF, and SVG pictures to PNG, EMF and WMF through LibreOffice in
    a digest-pinned container, and holds ImageMagick's page corrections, their catalog for a
    prompt, and a page assessment; no stage re-encodes a picture lossily.
  - `document` prepares a PDF's pages: Ghostscript, which won the renderer bake-off against
    pdftoppm, pdftocairo, vips, and ImageMagick over gs, renders pages and regions as PNG and
    reads text layers, and a page stream yields the prepared pages in order.
  - `office` runs the spike's Open XML CLI over docx, xlsx, and pptx, and converts legacy .doc,
    .xls, and .ppt to OOXML through LibreOffice in the same container.
  - `email` runs the spike's mail CLI over .pst stores, .msg messages, .eml messages, and mbox
    files, streams their records as typed values, rebuilds threads by RFC 5256's REFERENCES
    algorithm with conversation-index parents, and resumes a store after a message.
  - `onenote` runs the spike's OneNote CLI over sections, notebooks, and packages, streams their
    records as typed values, and lists each page's images, embedded files, and ink with their
    routes.
  - Each format package's text, a Markdown body or a text layer, is complete without inference;
    only pictures and page images go to vision.
  - `inspect` inspects every part before its stage: active content, such as macros, scripts,
    embeddings, and executables, is flagged and still prepared, since no stage executes it, and
    content unsafe to read, such as a ZIP entry escaping its directory, a symbolic link, or a
    bomb, is quarantined.
  - `dispatch` walks an input and every part found in it (attachments, embedded messages, media,
    and ZIP entries) to those stages with no default caps, under a content-addressed index that
    prepares each distinct content once across a share's inputs, runs, and workers, removes each
    part's file once the caller has consumed it, and resumes a store by cursor.
  - `audio` runs ffmpeg and ffprobe: it cuts a recording at its silences into windows of at most
    30 s that overlap by 2 s, as 16 kHz mono WAV, and joins their transcripts by aligning the
    overlapping words at each seam; the dispatcher's audio stage yields a part's plan and windows
    for a layer above to transcribe. Its
    [audio finding](https://github.com/JaimeStill/spike-ai-pipeline-processing/blob/main/docs/findings/audio.md)
    records windowed transcription 2.6 to 3.4 times faster than one unsplit request and less
    accurate on multi-speaker audio, and a gpt-oss pass over the seams making every run worse
    (evidence 5).
  - `media` reads a live or recorded source through one ffmpeg run, a reader, a file paced at
    real time or read unpaced, or an srt, rtmp, http(s), live HLS, or udp URL, each run confined
    to its source's protocols, and streams cancellable events as they arrive: audio windows cut
    at silences a noise-floor tracker finds as the samples come, and frames taken at the first
    frame, at each scene change, and after 10 s without one. The dispatcher's video stage yields
    a recorded video's frames and windows the same way. Its
    [streams finding](https://github.com/JaimeStill/spike-ai-pipeline-processing/blob/main/docs/findings/streams.md)
    records three videos read at real time with no window, frame, or request failed: each window
    came once the stream passed its 30 s limit and was transcribed 0.8 to 4.6 s later, each frame
    was described within 4 s, and the live transcript matched audio's file path under the same
    detector (evidence 13).
- **`spike-ai-pipeline-office`**, docproc-ooxml's reference, is a command-line tool on Microsoft's
  Open XML SDK, published as NativeAOT binaries, that turns docx, xlsx, and pptx into a JSON
  document model and Markdown, with chart data and embedded pictures in place of page images. Its
  JSON model is a versioned output contract (schema `1` at v0.1.0). Its Markdown is the document's
  text, complete without inference, and its media blocks are the only content a higher layer may
  send to vision; it runs no inference itself. It refuses legacy .doc, .xls, and .ppt, which
  processing's `office` package converts to OOXML through LibreOffice in a container first. No Go
  OOXML library is both well supported and freely licensed across the three formats, and the .NET
  converters for the legacy formats are unmaintained or paid.
- **`spike-ai-pipeline-mail`**, docproc-mail's reference, is a command-line tool in Rust, published
  as static musl binaries, that reads Outlook .pst stores through Microsoft's outlook-pst, .msg
  messages through its own MS-OXMSG reader over cfb, and .eml messages and mbox files through
  Stalwart's mail-parser into JSON Lines of one schema: folders, messages with their threading
  fields as stored, their bodies as stored plus HTML recovered from RTF and a Markdown body, and
  attachments. Processing builds the threads and routes each attachment to its stage. readpst
  (last released 2021) and libpff (alpha) were denied, no maintained Go or .NET .pst reader
  exists, and msg_parser reads no embedded messages or threading fields beyond Message-ID. Its
  known limits: .ost, which outlook-pst doesn't read, is refused; embedded messages in a .pst are
  listed without content, since outlook-pst 1.2.0 can't read them; and every mbox is read as an
  mboxrd, so mboxcl's Content-Length is ignored and an mboxo body line that was ">From " loses its
  ">". Its
  [mail-cli finding](https://github.com/JaimeStill/spike-ai-pipeline-mail/blob/main/docs/findings/mail-cli.md)
  records the fields it recovers per kind and its time and memory per store (evidence 16).
- **`spike-ai-pipeline-onenote`**, docproc-onenote's reference, is a command-line tool in Rust,
  published as static musl binaries, that reads OneNote .one sections, from desktop OneNote 2016
  and later or from OneDrive, .onepkg exports, and notebook directories through onenote_parser
  (MPL-2.0, used unmodified) into JSON Lines of one schema: the notebook, its sections, and each
  page at its latest revision, with its rich text, tables, a Markdown body, its images and
  embedded files, and its ink as recognized text and SVG strokes. It reads files only, never
  Microsoft 365. Processing's `onenote` package runs it as a system tool, and its `dispatch`
  package carries each page's images, embedded files, and ink to their stages (evidence 17). A
  reader of our own for [MS-ONESTORE] was rejected, as were pyOneNote and Apache Tika, which give
  no document model. Its known limits: password-protected sections are refused, since
  [MS-ONESTORE] leaves their encryption undocumented; no OneNote-written .onepkg is tested; and
  onenote_parser issue #48 can make a large OneDrive section with a long edit history need
  gigabytes of memory. Its
  [onenote-cli finding](https://github.com/JaimeStill/spike-ai-pipeline-onenote/blob/main/docs/findings/onenote-cli.md)
  records every expected field recovered against the public fixtures' hand-written expectations,
  and its time and peak memory per input. Processing's
  [onenote finding](https://github.com/JaimeStill/spike-ai-pipeline-processing/blob/main/docs/findings/onenote.md)
  records each medium routed to its real stage and vision reading page ink at a character error
  rate of 0.028, against 0.68 for OneNote's stored recognition (evidence 17).
- **`spike-ai-pipeline`**, the root, holds gb10-2's host scripts, the consumer check, and two
  CLIs, the references for docproc's `cmd/docproc` and docinfer's `cmd/docinfer`. `docproc` runs
  processing's whole suite over an input set into a versioned run directory, with no inference.
  `docinfer` sends a docproc run's outputs through the client to any configured workstation:
  every image to vision with reasoning off, every audio window to transcription, joined per part
  by `audio.Join`, and every text and each input's metadata, chunked, to embeddings. It writes a
  directory of its own, schema "1"; the capstone's input is `cmd/docinfer`'s directory at schema
  "2" (docproc's `docs/api.md`, "Run directory"). Its
  [finding](https://github.com/JaimeStill/spike-ai-pipeline/blob/main/docs/findings/docinfer.md)
  records a run over all of ~/docs through set A on gb10-2: 3,470 images, 1,254 windows, and
  28,937 chunk lines in 6.0 h, with 0 failures.

## The spike's answer

**Question:** Can a Go model client and a message, document, and stream processing layer, built
under go-elemental, carry a transactional, evented pipeline (metadata capture, inference
extraction, and embeddings stored in PostgreSQL with pgvector for semantic search) that a browser
follows stage by stage in real time?

**Answer:** Yes for the two layers it kept: a standard-library Go client over three wire formats
and a processing layer of owned system-tool adapters carried every image, audio window, and text
prepared from a real share, ~/docs, through vision, transcription and embeddings with no request
failed; the transactional, evented pipeline in pgvector and the browser that follows it moved,
unproven, to capstone.data-intelligence.

The items keep the spike's own evidence numbers.

**1.** The client: chat with tools and a reasoning round trip, streamed and unstreamed, vision,
audio input, embeddings, and transcription each pass against set A; its retry policy is proven
only by tests. docinfer ran 3,470 images, 1,254 audio windows, and 28,937 chunk lines of ~/docs
through it in 21,634 s, 0 of 6,172 requests failed. Proven by the client's integration tier
(b11669) and a running demo, docinfer over ~/docs (b12098).

**2.** Anthropic Messages and Open Responses fit under Chat Completions' request and response model
with no change to callers; the router drops some hints: `/v1/messages` reasoning effort, structured
output's schema, `strict`, and the `none` tool choice; `/v1/responses` `text`. Proven only by tests:
the client's integration tier (b11669).

**3.** The client's base module is standard library only. Proven only by the check: a strict
depguard rule over every Go file.

**4.** A PDF's pages go through vision whole and as a page stream; Ghostscript wins the renderer
bake-off (0.126 s a page, 27 MiB of memory), run as a separate program since it is AGPL, and the
model reads with reasoning off; page 7's NOFORN marking is read on no build. Proven only by tests:
processing's integration and bake-off tiers (b11669).

**5.** Mixed: long audio cut at silences and transcribed in windows is 2.6 to 3.4 times faster
than one unsplit pass, but less accurate on many speakers (word error rate 0.298 against 0.258 on
a 4-speaker meeting, 0.069 against 0.053 on an 8-speaker call); the gpt-oss seam pass is
falsified, making every run worse. Proven only by tests: processing's findings tier (b12098).

**6.** Skype for Business conversation history in a PST passes through as conversation records,
each of 4 equal to the hand truth. Fixture only: the PST is generated, its transcript HTML
invented, and no real store read. Proven only by processing's findings tier (no model).

**9.** The dependency runs one way, processing to client: processing imports the client only in
test code and runs no inference; the root's docinfer imports both. Proven by the check, whose
depguard rule keeps the client from importing anything but the standard library, and by
docinfer's running demo (item 1); processing's rule allows the client anywhere, so the test-only
import is how processing v0.9.0 stands, not a rule.

**11.** ImageMagick's corrections don't help vision reliably: which help and which hurt flips with
the DPI and the build. Proven only by tests: processing's bake-off tier (b11669).

**12.** The Open XML CLI keeps 1,768 of 1,768 elements where markitdown and xberg lose most, at a
17.2 ms median from a 28.80 MiB binary; LibreOffice's legacy round trip loses structure in .doc, a
little in .xls, nothing in .ppt, from a 1.02 GB container. Proven by office's bench (no model) and
processing's integration tier (b11669).

**13.** Live media is processed as it arrives: three videos read at real time with no window,
frame, or request failed, each window transcribed 0.8 to 4.6 s after it is yielded, and a
cancellation stops the stream within 0.5 s. Proven only by tests: processing's findings tier's
paced runs (b12098).

**14.** One tool, ffprobe, runs through the exec and container runners under the same contract;
docproc wired all ten stages over ~/docs, 515 of 592 inputs prepared and 4 failed. Proven by
processing's integration tier and a running demo, docproc over ~/docs (no model).

**16.** The mail CLI recovers every stored field of the generated fixtures' truth (147 of 180, 33
not stored, none wrong) and reads a 2.1 GB store in 13.27 s; the adapter places 11 of 11 messages
under the parent the hand truth gives and routes 51 of 51 attachments. Proven by mail's bench (no
model) and processing's findings and integration tiers (b11669).

**17.** The OneNote CLI recovers every expected field of the public fixtures (236 of 236 over the
sections) and reads a 161 MB section in 0.24 s; vision reads a page's ink at a character error
rate of 0.028 against 0.68 for OneNote's own recognition. No authored notebook exists, so no
authored hand truth. Proven by onenote's bench (no model) and processing's findings tier
(b11669).

Moved to capstone.data-intelligence, not failed: 7 (the transactional pipeline into pgvector, its
events and UI), 8 (the client through a gateway), 10 (pgvector as a native-tier extension), 15
(the browser's consumption of the events), and the gates, spike-model-layout's and
spike-model-gateway's answers, which join the capstone's own (`data-intelligence.md`, "Evidence"
and "Gates").

[The answer](https://github.com/JaimeStill/spike-ai-pipeline#the-answer) ·
[spike-ai-pipeline](https://github.com/JaimeStill/spike-ai-pipeline)

## What docproc's library left open

From spike-ai-pipeline-processing and v1.ai.processing's design review, the items the library
left open:

- ODF, WordPerfect, .xlsb, and ODG report "no processor yet", and legacy OLE2 macros go
  unflagged; the architect judged them unlikely on the deployment's share drives (formats r1).
  Text encoding detection (uchardet named 10 of 11 sample encodings) is revisited if the
  windows-1252 fallback misreads many files (formats r3).
- media's live sources stay in the spike until the capstone's `protocol` task names their owner.
  When they move:
  - rtsp sources in `media` need an rtsp server, such as MediaMTX in a container, as the
    integration tier's loopback sender, since ffmpeg 9.0.2's rtsp listener fails (streams r2 q1).
  - Capture devices in `media`: v4l2loopback, a kernel module the architect loads as root, is the
    camera test seam. On Wayland, as on the architect's Omarchy laptop, which has no X11, ffmpeg
    has no screen input: a portal capture tool such as gpu-screen-recorder streams the screen to a
    URL or the reader, and pw-record feeds audio; x11grab applies only to X11 hosts. The container
    runner would need device passthrough, the device's group, and socket mounts (streams r2 q2).
- ClamAV stays out of `inspect`, since it doesn't fit Inspect's contract; processing v0.6.0
  excludes it (onenote r6 q4).
- The email `Threader` holds 362 bytes a message for the whole read (measured), about 362 MB past
  a million messages; spilling its keys to disk would bound it.
- A bare RFC 822 file with no extension is unrouted, since the nested route needs an .eml name or
  a message type.
- RAR, ISO, xz, bzip2, and the other formats the spike left unrouted.
- HTML routing: no task carries saved web pages or HTML `.xls`.

## v1.ai.processing · mail-hardening

The task took docproc-mail's follow-ups from acceptance and released them as docproc-mail 0.1.4
(docproc's `tools/mail/CHANGELOG.md`):

- **Speed.** The task removed the read's waste stage by stage, with the output unchanged byte for
  byte and no time target (docproc's `docs/mail-performance.md`). With the content rule also gone,
  a read of plaso's store takes 1.50 s in place of 3.41 s, and of scale.mboxrd 4.07 s in place of
  7.70 s. musl's allocator was measured and kept: mimalloc read 13 to 35% faster with about twice
  the peak memory, and its C build would add zig or a musl C toolchain to the static release.
- **Markdown.** A message's Markdown comes from its HTML body alone when it has one, appending
  nothing from its other bodies, since the parts of a `multipart/alternative` are one content
  (RFC 2046 §5.1.4).
- **Signed and protected messages.** A clear-signed .msg or .pst message with no body of its own
  is read from its `multipart/signed` attachment. An `IPM.Note.SMIME` message gets the opaque
  S/MIME warning, and a rights-managed message gets a warning that its content is not read.
- **RTF and fixtures.** The RTF reader reads a table one line per row, and a fixture of its own
  pins htmd 0.6's Markdown differences.

## Experiment: spike-scanned-documents

**Question.** spike-ai-pipeline's documents finding judged Gemma 4 26B-A4B's vision transcription
on only 7 scanned pages: the model invented seal text, never read page 7's faded NOFORN, and
scored a scanned CER of about 0.11 to 0.13. Does that transcription hold at scale, on
olmOCR-bench (allenai/olmOCR-bench: 1,403 PDFs and 7,010 checkable assertions, ODC-BY as
reported), and does a dedicated OCR model, olmOCR's own on vLLM, or Tesseract beat it on GB10? It
also measures the vision models themselves on spike-ai-pipeline-processing's committed scanned
fixtures (document/testdata/scanned.pdf, ground truth beside it): a larger image-token budget
than gemma-4-26B-A4B's default, a dense vision model, and a lighter quantization than Q4_0, since
Gemma 4 26B-A4B Q4_0 never read page 7's faded NOFORN, even on a 122-token 300-dpi tile, which
points to the model, not resolution (processing's docs/findings/documents.md).

**Decision it changes.** Whether processing's document stage needs an OCR tier beside vision, and
which vision model, image-token budget, and quantization the vision tier runs. It is
experiment.ocr's one spike, after the capstone, which runs on vision transcription without it.

**Host.** gb10-2, after capstone.data-intelligence. Third-party and crawled documents are
processed only in sandboxed converters.

**Home.** `~/experiments/spike-scanned-documents`, remote
[JaimeStill/spike-scanned-documents](https://github.com/JaimeStill/spike-scanned-documents), set
up by `plan experiment.ocr.spike-scanned-documents`.

## Experiment: spike-go-harness

**Question.** Does a native Go harness behind spike-harness-driver's driver interface, with
sessions, tool calls, skills, and MCP, reach Pi's fidelity on the self-hosted stack? The spike
measures it beside Pi with spike-harness-driver's conform, and takes the failed cells of
spike-model-hosting's harness-fit profiles as its design input.

**Decision it changes.** Whether go-ai carries its own agent loop, and what replacing the
process-backed drivers costs.

**Open for its setup.** An MCP runner for docproc's `tool` contract: whether a long-running
service reached over MCP joins the processing layer's exec and container runners, or stays a
client of its own.

**Home.** `~/experiments/spike-go-harness`, remote
[JaimeStill/spike-go-harness](https://github.com/JaimeStill/spike-go-harness), set up by
`plan experiment.harness.spike-go-harness` after capstone.data-intelligence.

## Experiment: spike-local-models

**Question.** On the stack v1.ai.hosting built (`ai-hosting.md`), how does a Claude Code session
that stays on Anthropic models route subagent tasks to locally hosted models, and for which kinds
of task is the output acceptable? It is harness-layer work at a later priority, run after
spike-go-harness. Application workloads on local models are spike-ai-pipeline's.

There are two candidates:

- a gateway that routes a subagent profile's model name either to the self-hosted stack or to
  Anthropic
- a tool, MCP server, or skill that offloads the work to `pi --print` or directly to a vLLM row

The gateway candidate is agentgateway, the gateway that fronts the self-hosted stack
(`ai-hosting.md`, "The gateway"). Its documented Claude Code integration points
`ANTHROPIC_BASE_URL` at the gateway and maps request model names to providers, so it can stand in
for a purpose-built router. Its documentation shows only a wildcard model sent to one backend, so
the spike first proves the split this candidate depends on: in one configuration, a subagent
profile's model name reaches the self-hosted stack and the session's own model reaches Anthropic,
with tool use, streaming, and thinking blocks intact on both paths.

`claude-plugins/context/tool-based-skills.md` holds the question the spike tests as open: whether
structured, repetitive generation moves to a local model behind a schema-validated tool, and what
would justify it.

**Decision it changes.** `v1.harness.local-models`: the profile format, the routing mechanism,
and which kinds of task may run locally.

**Home.** `~/experiments/spike-local-models`, remote
[JaimeStill/spike-local-models](https://github.com/JaimeStill/spike-local-models), set up by
`plan experiment.harness.spike-local-models` after spike-go-harness.

## spike-harness-driver's evidence

The question's four parts: capabilities (which of skills, tool calls, and the native model
capabilities beyond chat a harness exposes), sessions (work spanning several turns, kept and
resumed), exchanges (one request-and-response block scoped and identified against a persistent
session), and workflows (progress, event streaming, cancellation, and concurrency over those
sessions), on Pi, Claude Code, and OpenCode against local and cloud models.

1. Tools and skills run on all three harnesses: a Go tool reaches Pi through its extension and
   Claude Code and OpenCode through MCP. Proven by a running demo: `clutch conform`, every
   capability on Azure and Anthropic for all three harnesses.
2. Of the native capabilities, a harness passes only vision, and drops an image a model can't
   take quietly; embeddings and audio need the `model` client. Proven by a running demo: `model`
   on the router and Azure, and a Go tool giving a Pi session a transcript.
3. A session outlives its process and resumes by ID with its history; `ErrJournalMismatch`
   catches Pi's cwd-scoped sessions. Proven by `clutch conform`'s `resume` and live follow-ups
   from a new process.
4. A session carries one exchange at a time under a driver-assigned ID, each harness's
   cancellation maps to one outcome, and a structured response is a `respond` tool. Proven by
   `clutch conform`'s `exchange`, `cancel`, and `structured`.
5. A standard-library workflow layer of about 1,800 lines runs a DAG of exchanges with a
   concurrency limit, cancellation, SSE with replay, and resume after SIGINT, `kill -9`, and
   `docker stop`. Proven by a running demo: `clutch workflow` and `clutch serve`, in the
   architect-validated checkpoints 1–3.
6. The service image is 215 MB, idles at about 10 MiB, and adds about 95 MiB per Pi session.
   Proven by a running demo: `deploy/`, checkpoint 3.
7. The Azure managed-identity path and IL6 hold. Checked on paper only; nothing ran against them.

The experiment lives in `~/experiments/spike-harness-driver`
([JaimeStill/spike-harness-driver](https://github.com/JaimeStill/spike-harness-driver)):
[the answer](https://github.com/JaimeStill/spike-harness-driver/blob/main/context/README.md#the-answer).

## Candidates for `marathon retro`

Architecture candidates from spike-ai-pipeline:

- "Follow the spec wherever possible", the architect's standing rule for the client
  (client-streaming), may belong in STANDARDS.md or the architecture layer as a standard.
- processing's STANDARDS.md says a mature tool is never reimplemented in Go, and audio's `Tracker`
  is the recorded exception, its reason in `audio/doc.go` (streams r2 q3). Should the standard
  name it, so later reviews don't flag it?
- processing's findings cite the goal record (`context/goals/…`), which sync deletes, so the links
  break once a spike syncs.
- mail's fixture generator rewrites every output on each run (random GUIDs, MsgKit timestamps), so
  keeping an unchanged output's committed bytes takes a manifest edit its "never edit" note
  forbids. Should the generator keep unchanged outputs, or the note allow the edit?
- Agent shells resolve Arch's /usr/bin/rustc and /usr/bin/dotnet ahead of mise's tools, which
  broke mail's check and fixtures task; a scratch PATH of rustup proxies works around it.
