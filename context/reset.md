# reset · v1-middleware-rate-limiting

- **Status:** closeout
- **Session:** start
- **Project:** architecture, standards-lab, go-web-sdk, go-web-service
- **Branch:** v1-middleware-rate-limiting

## Disposition

- **Promoted:** `go-web-sdk/context/concepts/middleware-sourcing.md`'s placement reasoning →
  `architecture/standards/go-elemental/principles/topology-and-naming.md` and `dependencies.md`:
  an application SDK may isolate a sourced dependency in a capability sub-module under
  `middleware/<concern>`, the same mechanism infrastructure libraries use for a provider, on a
  different axis (dependency weight, not a swap). `standards-lab/context/design/dependency-sourcing.md`
  gains the matching obligation, since it already owns the placement rule this generalizes.
- **Integrated:** `go-web-sdk/context/concepts/middleware-sourcing.md` — the flat-package-only
  placement rule and the dependency-line restatement are gone; `middleware/rate-limit` and
  `README.md` now express them. The note keeps the sourced-set table, the keying rationale, and
  the unfired triggers for real client IP and compression.
- **Retained:** `standards-lab/context/roadmap.toml` — `backlog.real-client-ip`,
  `backlog.compression`, and `backlog.response-caching`, still unbuilt, still waiting on a
  trigger none of them has yet.

## Next-focus

A `plan` session for `v1.storage`: no task is scoped yet, and none of the goal's decisions are
made — the standard tier (the minimal operation set common to Azure Blob and S3), the declared
provider, and the service's demonstration layer. Start there.

`v1.middleware` moved to the end of the roadmap's `next` sequence: rate limiting is the only task
it had, and it is closed; CORS, real client IP, and compression stay in the goal's summary,
backlogged behind triggers that have not fired.
