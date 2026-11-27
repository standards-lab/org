# System tools

Go builds only the infrastructure the organization must own; where a mature tool of the Linux
ecosystem already does the specialized work, a Go system runs that tool through an adapter it
owns. The principle is architecture's go-elemental
[System tools](https://github.com/standards-lab/architecture/blob/main/standards/go-elemental/principles/system-tools.md)
page.

## First case

spike-ai-pipeline's processing layer (`ai-strategy.md`, "The client and processing layers") runs
the PDF renderers, ImageMagick, ffmpeg, LibreOffice in a container for legacy office formats and EMF
and WMF pictures, and the spike's own Open XML and mail CLIs, each built as a system tool so the
processing layer runs it like the others. spike-ai-pipeline-processing's `tool` package is the
contract: a binary, a script through its interpreter, and a digest-pinned image through the docker
CLI each run as one Tool, which its constructor finds and version-checks (evidence 14, processing
v0.1.0).

## The docproc-* tools and their contracts

The spike's three CLIs become docproc's system tools, nested in docproc under `tools/<format>`:
docproc-ooxml (docx, xlsx, and pptx), docproc-mail, and docproc-onenote (`ai-strategy.md`,
"Repositories and placement"). Each tool's output contract follows these rules:

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
  metadata belong to docproc, docinfer, and Bunrei, not the tools.

The contracts move to schema "2", since the cuts remove fields each spike reader of "1" declares,
and grow by addition only from there (docproc's `docs/tool-contract.md`, "Versioning").

## What the tools' tasks left open

Beyond what `roadmap/docproc-backlog.toml` lists,
v1.ai.processing's design review left docproc-mail's opaque S/MIME in its backlog.
