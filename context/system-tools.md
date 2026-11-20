# System tools

Go builds only the infrastructure the organization must own. Where a mature tool of the Linux
ecosystem already does the specialized work (PDF and image processing, audio and video encoding,
document conversion), a Go system uses that tool through an interface of its own, and doesn't
reimplement it or bind to its library (architect, 2026-10-26). herald's processing layer
established the pattern: its `internal/format` runs ImageMagick behind handlers keyed by media
type.

The principle extends go-elemental's sourcing rule (architecture,
`standards/go-elemental/principles/dependencies.md`), which writes a capability in-house or
sources a library, with a third choice: run a mature tool through an adapter.

- **The adapter is the boundary.** It isolates the tool's effect on the design: the system's
  interfaces (a page renderer, an audio preparer) say what it needs, and each adapter says how one
  tool provides it, so tools stay swappable.
- **A tool is a process.** A binary, a script run through its interpreter, or a pinned container
  image all fit the same contract. A tool that is a long-running service is a client of its own.
- **Startup fails, not the first request.** An adapter finds its tool and checks its version
  when it is constructed, and runs it under the caller's context.
- **Licenses travel with the image.** Running a tool links nothing, so a tool's license, AGPL
  included, binds the runtime image that ships it, not the code. A container runner keeps a
  heavy or restrictively licensed tool out of the service's own image.
- **Tools are chosen for support.** A tool or library joins only when it is mature, actively
  maintained, and freely licensed; an unmaintained or paid project is never adopted.

## First case

spike-ai-pipeline's processing layer (`ai-strategy.md`, "The client and processing layers") runs
the PDF renderers, ImageMagick, ffmpeg, LibreOffice in a container for legacy office formats and EMF
and WMF pictures, and the spike's own Open XML and mail CLIs, each built as a system tool so the
processing layer runs it like the others. spike-ai-pipeline-processing's `tool` package is the
contract: a binary, a script through its interpreter, and a digest-pinned image through the docker
CLI each run as one Tool, which its constructor finds and version-checks (evidence 14, processing
v0.1.0).
v1.ai.processing's `architecture` task promotes the principle to a go-elemental page from that
evidence.

## The docproc-* tools and their contracts

The spike's three CLIs become docproc's system tools, nested in docproc under `tools/<format>`:
docproc-ooxml (docx, xlsx, and pptx), docproc-mail, and docproc-onenote (`ai-strategy.md`,
"Repositories and placement"). Each tool's output contract follows these rules (plan,
2026-11-12):

- **One contract per tool.** Each tool writes one JSON Lines contract and has no separate
  Markdown output mode. The contract carries Markdown as the text layer, the text that is chunked,
  embedded, and read by people. Every piece of text a file holds exists once: content in the
  Markdown, labels the Markdown leaves out (a mail store's name, OneNote's ink alternatives) as
  kept plain-text fields; a structured field is cut only when its text is already carried, or it
  can't be plain text and has no processor, and content a tool reads but its Markdown lacks is
  rendered into the Markdown.
- **Additions.** docproc-ooxml adds optional document `properties` (core, app, and custom), writes
  embedded packages and OLE payloads as media, and promises byte-identical output under test.
  docproc-mail adds `smtp_address` for Exchange addresses, and docproc-onenote keeps its ink
  recognizer's alternates as `ink_alternatives`.
- **Golden outputs prove a contract.** Each tool commits golden outputs; no JSON Schema is
  written.
- **The tools stay narrow.** Batch IDs, per-item digests, provenance, warnings, and typed
  metadata belong to docproc, docinfer, and Cerebro, not the tools.

The contracts move to schema "2", since the cuts remove fields each spike reader of "1" declares,
and grow by addition only from there (docproc's `context/layers.md`, "Versioning").

## Inputs for the tools' tasks

What the tools' spikes and v1.ai.processing's design review left for the `ooxml`, `mail`, and
`onenote` tasks:

- **docproc-ooxml**, from spike-ai-pipeline-office:
  - fixtures/README.md calls slide-objects' pictures "beside another", but generate.py places both
    of slide 3's at one default position, so the blue square covers the red; say so, or place
    them apart.
  - Backlog: reading comments, SmartArt, text boxes, equations, and cell notes (design review).
- **docproc-mail**, from spike-ai-pipeline-mail:
  - The static musl release binary takes 2.0 times a glibc build's time on the 2.1 GB scale store
    (13.27 s against 6.51 s at v0.1.0, 14.82 s against 6.88 s at v0.2.0), 2.3 times on plaso's
    store (1.59 s against 0.71 s), and 1.7 times on the 2.7 GB scale mboxrd (6.86 s against
    4.09 s). Its system time (3.2 s against 0.7 s on the store at v0.1.0, 2.6 to 2.8 s against
    1.0 s on the mboxrd) points at musl's allocator (docs/findings/mail-cli.md, "The release
    binary against a glibc build" and ".eml and mbox"); measure a replacement allocator if
    processing's email runs find the time matters.
  - Backlog: mail caps a .msg's nesting at 16 levels; raise or lift the cap if real stores nest
    deeper.
  - Backlog from the design review: a .pst's embedded-message content, waiting on outlook-pst's
    unmerged fix; class-specific fields (contacts, appointments, and tasks); TNEF and opaque
    S/MIME; and an ANSI .pst fixture.
- **docproc-onenote**, from spike-ai-pipeline-onenote (backlog):
  - Section decryption waits for a specification or an open implementation, since
    [MS-ONESTORE] 2.5.19 leaves the scheme undocumented (onenote-cli, round 2 q7).
  - An authored OneNote notebook, committed with hand truth from onenote-cli's round 2 authoring
    spec, needs desktop OneNote on Windows or a Mac, since OneNote for the web can neither
    download nor export a notebook (onenote r6 q1).
