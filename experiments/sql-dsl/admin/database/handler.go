package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/standards-lab/go-web-sdk"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/migrate"
)

// maxBody bounds an administrative request body.
const maxBody = 1 << 10

type handler struct {
	service *Service
	errors  *web.ErrorWriter
}

// Routes builds the admin domain's route group, rooted at /database. Reads:
// diagnostics, the schema status, the pattern catalog, and the statements
// registry. Operations: verify, up, down, steps,
// force, each a POST whose response is the resulting status, and seed,
// whose response is the rows it inserted. force sets the history without
// running any file — the operator's override for dirty state, after the
// schema has been repaired by hand. Every rejection is an RFC 9457 problem;
// a schema-state conflict and a disabled seed carry their reason. The
// composition root mounts the group into the admin mount. The confirmation
// token the strategy requires for down and force arrives with the
// management surface.
func Routes(service *Service) *web.Group {
	h := &handler{service: service, errors: web.NewErrorWriter(status)}
	g := web.NewGroup("/database")
	g.HandleFunc("GET", "/diagnostics", h.diagnostics)
	g.HandleFunc("GET", "/schema", h.status)
	g.HandleFunc("GET", "/patterns", h.patterns)
	g.HandleFunc("GET", "/statements", h.statements)
	g.HandleFunc("POST", "/schema/verify", h.verify)
	g.HandleFunc("POST", "/schema/up", h.up)
	g.HandleFunc("POST", "/schema/down", h.down)
	g.HandleFunc("POST", "/schema/steps", h.steps)
	g.HandleFunc("POST", "/schema/force", h.force)
	g.HandleFunc("POST", "/seed", h.seed)
	return g
}

func (h *handler) seed(w http.ResponseWriter, r *http.Request) {
	n, err := h.service.Seed(r.Context())
	if err != nil {
		h.reject(w, r, err)
		return
	}
	_ = web.WriteJSON(w, http.StatusOK, n)
}

func (h *handler) statements(w http.ResponseWriter, r *http.Request) {
	_ = web.WriteJSON(w, http.StatusOK, h.service.Statements())
}

func (h *handler) patterns(w http.ResponseWriter, r *http.Request) {
	_ = web.WriteJSON(w, http.StatusOK, h.service.Catalog())
}

func (h *handler) diagnostics(w http.ResponseWriter, r *http.Request) {
	d, err := h.service.Diagnose(r.Context())
	if err != nil {
		_ = h.errors.Write(w, r, err)
		return
	}
	_ = web.WriteJSON(w, http.StatusOK, d)
}

func (h *handler) status(w http.ResponseWriter, r *http.Request) {
	st, err := h.service.Status(r.Context())
	if err != nil {
		_ = h.errors.Write(w, r, err)
		return
	}
	_ = web.WriteJSON(w, http.StatusOK, st)
}

func (h *handler) verify(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Verify(r.Context()); err != nil {
		h.reject(w, r, err)
		return
	}
	h.status(w, r)
}

func (h *handler) up(w http.ResponseWriter, r *http.Request) {
	h.respond(w, r)(h.service.Up(r.Context()))
}

func (h *handler) down(w http.ResponseWriter, r *http.Request) {
	body := Steps{Steps: 1}
	if r.ContentLength != 0 {
		if err := decode(w, r, &body); err != nil {
			_ = h.errors.Write(w, r, err)
			return
		}
	}
	h.respond(w, r)(h.service.Down(r.Context(), body.Steps))
}

func (h *handler) steps(w http.ResponseWriter, r *http.Request) {
	var body Steps
	if err := decode(w, r, &body); err != nil {
		_ = h.errors.Write(w, r, err)
		return
	}
	h.respond(w, r)(h.service.Steps(r.Context(), body.Steps))
}

func (h *handler) force(w http.ResponseWriter, r *http.Request) {
	var body Force
	if err := decode(w, r, &body); err != nil {
		_ = h.errors.Write(w, r, err)
		return
	}
	h.respond(w, r)(h.service.Force(r.Context(), body.Version))
}

// respond writes an operation's resulting status, or its rejection.
func (h *handler) respond(w http.ResponseWriter, r *http.Request) func(Status, error) {
	return func(st Status, err error) {
		if err != nil {
			h.reject(w, r, err)
			return
		}
		_ = web.WriteJSON(w, http.StatusOK, st)
	}
}

// reject writes err as a problem. A schema-state conflict and a disabled
// seed carry the error text as their detail: the SDK's writer sends text
// only on a 400, the right policy for a public API, but an operator surface
// needs to know which version is dirty or pending, or that this
// environment does not seed. Everything else follows the writer.
func (h *handler) reject(w http.ResponseWriter, r *http.Request, err error) {
	if code, ok := status(err); ok && (code == http.StatusConflict || code == http.StatusForbidden) {
		_ = web.WriteProblem(w, r, code, http.StatusText(code), err.Error())
		return
	}
	_ = h.errors.Write(w, r, err)
}

// decode reads a request body strictly: unknown fields are rejected, and
// any decode failure is a validation rejection carrying the reason.
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: body: %v", ErrValidation, err)
	}
	return nil
}

// status is the domain's error vocabulary as one web.StatusMatcher: a
// rejected request (400), a version outside the set (400), a seed the
// environment forbids (403), and the schema states an operation cannot
// proceed from — dirty, pending, a history the set does not carry, a
// migration with no down — as conflicts (409). A dialect without the lock
// capability stays unmatched: it is a wiring defect (500).
func status(err error) (int, bool) {
	switch {
	case errors.Is(err, ErrValidation), errors.Is(err, migrate.ErrVersionNotFound):
		return http.StatusBadRequest, true
	case errors.Is(err, ErrSeedDisabled):
		return http.StatusForbidden, true
	case errors.Is(err, migrate.ErrDirty),
		errors.Is(err, migrate.ErrPending),
		errors.Is(err, migrate.ErrUnknownVersion),
		errors.Is(err, migrate.ErrNoDown):
		return http.StatusConflict, true
	}
	return 0, false
}
