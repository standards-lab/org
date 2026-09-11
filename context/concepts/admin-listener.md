# The management listener

The admin mount on its own listener, the `v1.admin-listener` goal. The mount today serves on
the API listener of go-web-service, which the service's README states is not for a public
deployment. The strategy record `design/dsl-driven-services.md` (§6.3) states the requirement;
this note carries what a session found when it explored the build on 2026-09-07 and settled
that the build waits on the auth layer. It is a concept: nothing below is decided, and the goal's
own sessions settle it.

## The requirement

The requirement is `design/dsl-driven-services.md` §6.3. The listener is a consumer of what two
later layers settle: its authentication comes from `goals.v1.auth`, and the audit record on
anything that mutates from `goals.v1.observability`.

## Why it waits

The session that opened on the listener found that every design choice it would have to make (the
token's mechanism, the listener's authentication, the audit record) is a choice the auth and
observability layers make properly later. A reference architecture that shipped an interim token
scheme would document a pattern it intends to replace. The listener therefore follows auth and
observability in the roadmap's sequence, and the exploration below is kept so the later session
does not repeat it.

## What the exploration found

The findings are grouped by repository, lowest dependency first. Line references are as of
2026-09-07 and drift with the code; the claims are what matter.

### go-core

- No redaction primitive exists: no secret type, no `String` or `slog.LogValuer` on any config
  value, and no `ReplaceAttr` hook in `logging.New`. The natural shape is a `config.Redacted`
  string type beside `config.Duration`, with `MarshalJSON`, `String`, and `LogValue` emitting a
  placeholder and an accessor returning the value. `slog` honors `LogValuer` natively, so the
  logging package needs no change.
- The lifecycle coordinator supports a second listener today: a second `lifecycle.Service` under
  a distinct name at `StageRoot`, started concurrently with the API server and drained first.
  A listener that is disabled is simply not registered.

### go-database

- `Config.Password` is a bare string read at two sites, the env override in `config.go` and the
  pgx assignment in `postgres/postgres.go`; the DSN never carries it. The redaction contract
  changes the field's type, a breaking release of both modules, which pin go-core v0.3.0 and
  would move to the release that carries the contract.
- The destructive class of `Down`, `Force`, and `Reset` is documented, not encoded: the `admin`
  package exposes no class marker, and its package comment states that the administrative
  surface decides who may call them. Whether the class becomes machine-readable is
  the goal's decision.

### go-web-sdk

- `web.NewEnv` hardcodes the `"server"` segment in every composed name, so a second `web.Config`
  block cannot exist under one prefix. Recorded as item 6 of
  `go-web-sdk/context/concepts/error-handling.md`; the fix is a block-name parameter on the env
  composition, owned by the SDK and released ahead of the listener. `database.NewEnv` has the
  same shape and the same fix if a second database block is ever needed.
- `web.Server` holds no package state; two servers are two `NewServer` calls. The listener is
  TCP only; no Unix socket option exists. `RegisterHealth` is written to be called once; whether
  the management listener serves probes is the goal's decision.
- No auth, token, or recoverer middleware exists. `concepts/middleware-sourcing.md` keeps
  cryptographic token verification out of the SDK and in the auth library; a shared-secret
  comparison has no stated home yet.

### go-web-service

- The composition root is singular by shape: `App` holds one server, `routes()` feeds one router,
  `middleware()` is one stack, `RegisterHealth` is called once. The second listener reshapes
  `internal/app`, which already gains an `auth.go` layer file for the API module's authentication
  middleware (`context/design/auth-strategy.md` §6).
- `admin/database.Routes` has no gate on the three destructive verbs. The route group is unsealed
  until `NewModule`, so a per-route middleware on `down`, `force`, and `state` is the seam; the
  handler's error vocabulary already maps refusals to 400, 403, and 409.
- The `admin` configuration block holds the seed switch and an `Env` struct recording the
  variable names it composed; a management block follows the same shape beside it, with the
  token in the secrets layer.
- The integration harness holds one address and one client; every admin helper in
  `integration/state.go` takes a client, so callers switch to a management client without a
  signature change, and the destructive helpers gain the token. The harness clears `APP_ENV`, so
  a listener that is off by default is off in every integration run unless the harness enables
  it. `mise run db-state` hardcodes the API port.
- No config rendering exists in the service, so nothing leaks today.

### go-web-sdk-template

- The template's composition root has the identical singular shape with an empty `/admin` group,
  and takes the same reshape: the management block, the second listener, the empty group moved
  onto it, and the harness's second client. It stays engine-free; nothing database-specific
  crosses.

## The posture questions

Open in `design/dsl-driven-services.md` §9 and reassigned to this goal:

- **DDL in the serving role.** Whether a process that serves traffic should hold DDL privileges,
  and whether the standard should mandate a separate migration role and a one-shot invocation of
  the same binary. The reference applies migrations at startup today.
- **The listener's default.** Whether network isolation is sufficient, or the standard requires
  the listener to be off by default and enabled per environment. The exploration leans to the
  explicit switch, because isolation is a deployment property the service cannot verify.
- **The confirmation token.** Its mechanism, settled with the listener's authentication: a
  configured secret in a header, a field in the body, or a request that names what it destroys.

## Assumptions

- Assumes the auth layer produces a middleware the listener can mount, per
  `go-web-sdk/context/concepts/service-middleware.md`: infrastructure-backed middleware lives in
  the infrastructure library, exported over stdlib types.
- Assumes the observability layer produces the audit record; if it does not, the listener's
  request logger is the audit log.
- Assumes go-core's redaction contract lands before config rendering, and that go-database's
  breaking release rides the same coordinated snapshot.
