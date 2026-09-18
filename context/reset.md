# reset · v1-middleware

- **Status:** closeout
- **Session:** plan
- **Project:** go-web-sdk, standards-lab
- **Branch:** v1-middleware

## Disposition

- **Retained:** `standards-lab/context/roadmap.toml` — `goals.v1.middleware.tasks.rate-limiting`
  added: rate limiting's concrete design (`golang.org/x/time/rate` + `github.com/go-chi/httprate`,
  keyed on `RemoteAddr` until real client IP lands, `go-web-sdk/middleware` placement, the
  dependency-line statement as part of its own build). The goal summary sharpened to say why real
  client IP, compression, and CORS wait. `backlog.real-client-ip` and `backlog.compression`
  added — parked members of `v1.middleware`'s already-recorded sourced set, pending their trigger.
  `backlog.response-caching` added — raised alongside rate limiting but never part of that
  recorded set; split out entirely, no stack decision made.
- **Retained:** `go-web-sdk/context/concepts/middleware-sourcing.md` — sharpened with the rate
  limiting keying decision (`RemoteAddr` now, real client IP's output once it lands) and the
  trigger conditions for real client IP and compression. Still the concept note of record for the
  sourced middleware set.

## Next-focus

A `start` session for `v1.middleware.rate-limiting`: build the rate limiting middleware in
go-web-sdk (`x/time/rate` + `httprate`, `RemoteAddr` keying, go-web-sdk's per-block `Config`
pattern, and the README's dependency-line statement since this is the first sourced middleware to
land), then wire it into go-web-service's `internal/app/middleware.go`.
