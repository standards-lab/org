# reset · problem-vocabulary

- **Status:** closeout
- **Session:** start
- **Project:** go-web-sdk, standards-lab
- **Branch:** problem-vocabulary

## Disposition

- **Integrated:** unified `Problem`'s two serializers. `Problem` gained `Extras map[string]any`
  and a `MarshalJSON`/`UnmarshalJSON` pair (a `problemMembers` twin type avoids `MarshalJSON`
  recursing into itself); `WriteFor` replaces the split between `Problem.Write` and the deleted
  `WriteProblemWith`, defaulting `Instance` from the request path.
- **Integrated:** widened the matcher. `StatusMatcher func(error) (int, bool)` is replaced
  outright by `ProblemMatcher func(error) (Problem, bool)` — settled via an `opus` escalation as
  a breaking change rather than a parallel type, since the one real consumer composes two
  matchers with a deliberate precedence a parallel type couldn't express, and no workspace
  repository links against this SDK's working tree (each pins a released version), so nothing
  breaks until a consumer bumps the pin. `ErrorWriter.Problem` centralizes the mapping; a
  matcher's own `Detail` now always ships, rather than being gated by the writer's detail set.
  `statusError` stays sealed: most of what it maps today are third-party sentinel errors a
  consumer can never teach to carry a problem regardless of the interface's visibility.
- **Integrated:** gave `Readiness`/`RegisterHealth` a `notReady Problem` parameter — a consumer's
  type, title, and detail reach the wire; `Status` and the `checks` extension member stay the
  probe's own regardless of what the consumer sets, via a fresh extras map built per request
  rather than mutating the caller's.
- **Integrated:** validation findings from a `fable` peer review, fixed before closeout:
  `Problem.UnmarshalJSON`'s key-stripping pass now matches field names case-insensitively, like
  the typed pass it has to stay in sync with; added a concurrent-requests test for `Readiness`'s
  fresh-map logic, previously correct only by inspection; the `CHANGELOG` now flags all three of
  this step's breaking changes explicitly, and three stale doc-comment attributions were fixed.
- **Integrated:** struck the resolved item from `go-web-sdk/context/concepts/error-handling.md`,
  renumbered what remains, and recorded the deliberate choice not to let a matcher override
  `statusError`'s precedence as a new "wait for a consumer to ask" item beside writer
  inheritance. Brought `doc.go`'s Error mapping, Problem responses, and Health sections, and
  `context/README.md`'s capability map, current.
- **Cross-repo:** `standards-lab/context/roadmap.toml` — `goals.v1.web.tasks.adapter`'s summary
  updated: the problem-vocabulary item marked done, the remaining five items named, and a note
  that the reference service's own matchers and readiness call sites still take the old shapes
  pending their own migration step. The task is not finished — five items remain — so it is not
  deleted, and `v1.web` stays open.

## Next-focus

Both `v1.web` tasks now have real remaining work but nothing urgent enough to lead the next
session on its own (adapter: router hooks, the `ErrorLog` bridge, writer inheritance,
`statusError` precedence, the config env segment — all "wait for a consumer to ask" or
independently small; middleware: the rest of the hand-rolled set, request ID). Per the
architect, the next session turns to **planning** `v1.observability`'s tasks instead — a `plan`
session in `standards-lab`, not a `start`. That planning session should also settle whether the
`go-web-service`/`go-web-sdk-template` migration onto this SDK's new `ProblemMatcher`/`Readiness`
shapes (deferred from this step, tracked nowhere yet as a task) belongs in the roadmap now or
waits until this SDK actually releases past v0.7.0.
