# reset · shared-writer-panic-path

- **Status:** closeout
- **Session:** start
- **Project:** go-web-sdk, standards-lab
- **Branch:** shared-writer-panic-path

## Disposition

- **Integrated:** exported `handler.go`'s private wrapped-response-writer type as `web.Recorder`,
  with an idempotent `WrapWriter` constructor, so `Handle` and the middleware package's
  `RequestLogger` and `Recoverer` share one instance per request instead of each carrying its own.
- **Integrated:** rewrote `middleware.RequestLogger` onto the shared `Recorder`, deleting the
  private `statusRecorder` it carried, which was missing the `Write` intercept `Recorder` has — a
  live bug where a handler that wrote a body and only then called `WriteHeader(500)` was logged as
  500 when the client actually got the already-committed 200.
- **Integrated:** added `middleware.Recoverer` as the chain's sole recovery point.
  `RequestLogger` no longer recovers panics itself; before this step, a handler panic was logged
  and re-panicked into `net/http`'s own recovery, which writes nothing — no RFC 9457 problem
  document existed on the panic path anywhere in the architecture. `Recoverer` now answers an
  unrecovered panic with a 500 problem document when nothing was committed yet.
- **Integrated:** validation findings from a `fable` code review, fixed before closeout since they
  were correctness gaps in this step's own new code: `Recoverer` re-raises `http.ErrAbortHandler`
  after logging when the response was already committed, so a panic mid-stream drops the
  connection again instead of completing a truncated body as a clean 200; `Recorder` gained
  `FlushError` so a `Flush` through `http.ResponseController` commits the same way a `Write` does;
  `Recorder.WriteHeader` no longer commits on a 1xx status other than 101, matching `net/http`'s
  own treatment of informational responses; `RequestLogger(nil)` and `Recoverer(nil)` now panic
  clearly at construction instead of obscurely (and, for `Recoverer`, destructively) on first use.
- **Integrated:** struck the resolved items from `go-web-sdk/context/concepts/error-handling.md`
  (the recorder export, renumbering what remains) and `middleware-sourcing.md` (the
  dropped-connection and ordering-trap build findings, the Recoverer row moved from remaining work
  to built); brought `doc.go`, `middleware/doc.go`, and `context/README.md`'s capability map
  current; added the `CHANGELOG.md` `Unreleased` entry.
- **Cross-repo:** `standards-lab/context/roadmap.toml` — `goals.v1.web.tasks.adapter`'s summary
  updated to mark the recorder/logger/recoverer items done, and its dangling `integration.service`
  citation (pointing at a since-deleted task) removed: the reference service's handlers already
  use `HandleErr` (`domain/organization`, `admin/database`). `goals.v1.web.tasks.middleware`'s
  summary updated to mark the recoverer/logger ordering resolved. Neither task's remaining items
  are finished, so neither is deleted and `next` is unchanged.

## Next-focus

`goals.v1.web.tasks.adapter`'s remaining item — the `ErrorWriter` problem vocabulary, in
`go-web-sdk`: widen `StatusMatcher` (or add a problem-returning matcher alongside it) so a
consumer's matcher can carry a type URI, title, and extension members, not just a status; decide
whether the sealed `statusError` interface in `errors.go` opens so a consumer's own error type can
carry its problem the way the SDK's built-in errors do; unify `Problem.Write`'s struct marshal
with `WriteProblemWith`'s independent `map[string]any` construction into one serializer; once the
vocabulary exists, give `/readyz` a type hook on `Readiness` instead of its `checks` extension
member riding a bare `about:blank` problem. Three coupled, independently breaking API decisions —
flagged at this step's own SETTLE as needing a session of its own. The architect asked for this to
be handled immediately rather than deferred; it follows in this same session, on its own branch,
closing out separately. After it lands, the next session turns to planning `v1.observability`'s
tasks.
