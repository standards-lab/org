# reset · slab-surface

- **Status:** closeout
- **Session:** start
- **Project:** go-web-service
- **Branch:** slab-surface

## Disposition

- **Retained:** `go-web-service/context/design/slab-conventions.md` — rewritten to describe the
  package layout as it actually landed (`domain/organization`, `admin/database`, `output`,
  `input`, the composition-root split in `internal/app`) in place of the `internal/api`/
  scenario-registry description that predated this branch; still the design note of record for
  slab's conventions.
- **Cross-repo:** `standards-lab/context/roadmap.toml` — `slab.tasks.surface` deleted (finished);
  `slab.tasks.styling` added and set as `next`'s immediate entry in its place. Committed directly
  to `main` (bypassing branch protection, with the architect's explicit approval) rather than
  through a branch and PR, since it's routine bookkeeping from a session whose primary project is
  `go-web-service`.

## Next-focus

A `start` session taking up `slab.tasks.styling`, spanning `go-web-service` and `go-web-sdk`: a
shared styling system for slab's output (`scenario/style.go`'s `ColorEnabled` and `jsonColor` are
the mechanism already proven by the narrated scenarios; `org`/`admin`'s direct commands print
plain JSON today and need the same treatment, extracted to a package both `scenario` and `output`
draw on rather than duplicated) and, discovered alongside it, `web.Problem` implementing the
`error` interface in `go-web-sdk` directly, so slab's own `output.ProblemError` wrapper can be
removed once it lands.
