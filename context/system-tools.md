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

spike-ai-pipeline's processing layer (`ai-strategy.md`, "Experiment: spike-ai-pipeline") runs the
PDF renderers, ImageMagick, ffmpeg, LibreOffice in a container for legacy office formats, and the
spike's own Open XML CLI, built as a system tool so the processing layer runs it like the others.
experiment.ai's intake promotes the principle to a go-elemental page from that evidence.
