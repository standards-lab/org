# The docs corpus

docs-corpus holds the real workloads' corpus: redistributable samples in ten format areas, each
with its source, license, truth, and scenarios in `MANIFEST.json`. Its
[README.md](https://github.com/standards-lab/docs-corpus/blob/main/README.md),
[docs/coverage.md](https://github.com/standards-lab/docs-corpus/blob/main/docs/coverage.md), and
[docs/baseline.md](https://github.com/standards-lab/docs-corpus/blob/main/docs/baseline.md) hold
the manifest, the fetch, the scenarios, the gaps, the measurements, and the times; this note
doesn't restate them.

## How it changes

A goal's task adds, replaces, or drops samples under docs-corpus's STANDARDS.md, and releases the
change under a new version tag, since a released tag is never re-cut. A sample stays when it
exercises a scenario no other sample covers, or when a measurement in `docs/measurements.json`
reads it. The manifest and coverage are never edited by hand: docs-corpus's `mise` tasks
regenerate them, without `~/docs`, which corpus.docs's baseline removed.

Later changes keep these rules, which docs-corpus doesn't state. Its
[STANDARDS.md](https://github.com/standards-lab/docs-corpus/blob/main/STANDARDS.md) holds the
realistic-content standard and DVIDS's unqualified release,
[docs/coverage.md](https://github.com/standards-lab/docs-corpus/blob/main/docs/coverage.md) the
vocabulary's pinned links, and
[docs/measurements.json](https://github.com/standards-lab/docs-corpus/blob/main/docs/measurements.json)
DVIDS 882019 as `video-interpretation`'s input.

- docproc releases after v0.9.0 go to a later corpus goal.
- The corpus's docproc runs hold gotenberg at 9.10.1-libreoffice, since 9.10.2 changes nothing
  docproc uses.
- DVIDS 882019's interpretation key, `video-interpretation`'s, is sourced, never authored: a
  revision adds or rewords published readings only.
- The realistic-content standard admits limit checks whose content is real (archive-8, pdf-7,
  `archive/high-ratio-200mb.zip`, `text/json/deeply-nested-64.json`) and
  `text/csv/tab-separated.tsv`.
- The vocabulary's links stay pinned to docproc `48216e1`, and an ID a newer run adds links to
  that run's release.
- A copyleft fixture is acceptable when it ships unmodified with its license.
- The corpus has no size cap.

## How it is shared

The repository is public. Each version tag's release assets hold the sample files, one tar per
format area, from v0.1.0, and `mise run fetch` places them in a checkout once each matches the
manifest.

## The verified checkout and the baseline

gb10-2 holds the verified v0.2.0 checkout at `~/docs-corpus`, and docs-corpus's
[docs/baseline.md](https://github.com/standards-lab/docs-corpus/blob/main/docs/baseline.md)
records docproc's and docinfer's baseline runs over it.
