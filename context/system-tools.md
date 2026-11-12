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

## The quarry-* tools and their contracts

The spike's three CLIs become quarry's system tools, nested in quarry under `tools/<format>`:
quarry-ooxml (docx, xlsx, and pptx), quarry-mail, and quarry-onenote (`ai-strategy.md`,
"Repositories and placement"). Each tool's output contract follows these rules (plan,
2026-11-12):

- **One contract per tool.** Each tool writes one JSON or JSON Lines contract and has no separate
  Markdown output mode. The contract carries Markdown as the text layer, the text that is chunked,
  embedded, and read by people. Structured fields carry only what Markdown can't.
- **Additions.** quarry-ooxml adds optional document `properties` (core, app, and custom), writes
  embedded packages and OLE payloads as media, and promises byte-identical output under test.
  quarry-mail adds `smtp_address` for Exchange addresses.
- **Golden outputs prove a contract.** Each tool commits golden outputs; no JSON Schema is
  written.
- **The tools stay narrow.** Batch IDs, per-item digests, provenance, warnings, and typed
  metadata belong to quarry and Cerebro, not the tools.

v1.ai.processing's `layers` task settles whether the contracts stay schema "1", grown by
addition, or move to "2".
