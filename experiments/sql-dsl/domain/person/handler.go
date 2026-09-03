package person

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"uuid"

	"github.com/standards-lab/go-database"
	"github.com/standards-lab/go-web-sdk"
	"github.com/standards-lab/org/experiments/sql-dsl/internal/sdk"
	"github.com/standards-lab/org/experiments/sql-dsl/lib/query"
)

const maxCommandBody = 1 << 16

type handler struct {
	service *Service
	limits  web.Limits
	errors  *web.ErrorWriter
}

// Routes builds the layer's route group, rooted at /people: the reads, the
// three commands, and the three actions as POST /{id}/<action>. The
// guarded operations take their version precondition from If-Match.
func Routes(service *Service, limits web.Limits) *web.Group {
	h := &handler{service: service, limits: limits, errors: web.NewErrorWriter(status)}
	g := web.NewGroup("/people")
	g.HandleFunc("GET", "", h.list)
	g.HandleFunc("GET", "/{id}", h.find)
	g.HandleFunc("POST", "", h.create)
	g.HandleFunc("PATCH", "/{id}", h.edit)
	g.HandleFunc("DELETE", "/{id}", h.delete)
	g.HandleFunc("POST", "/{id}/activate", h.action(func(ctx *http.Request, id string, v int64) (Identity, error) {
		return service.Activate(ctx.Context(), id, v)
	}))
	g.HandleFunc("POST", "/{id}/deactivate", h.action(func(ctx *http.Request, id string, v int64) (Identity, error) {
		return service.Deactivate(ctx.Context(), id, v)
	}))
	g.HandleFunc("POST", "/{id}/transfer-unit", h.transferUnit)
	return g
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	q, err := web.ParseQuery(r.URL.Query(), h.limits)
	if err != nil {
		_ = h.errors.Write(w, r, err)
		return
	}
	items, total, err := h.service.List(r.Context(), q)
	if err != nil {
		_ = h.errors.Write(w, r, err)
		return
	}
	_ = web.WriteJSON(w, http.StatusOK, web.NewPage(items, q, total))
}

func (h *handler) find(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathID(w, r)
	if !ok {
		return
	}
	p, err := h.service.Find(r.Context(), id)
	if err != nil {
		_ = h.errors.Write(w, r, err)
		return
	}
	_ = web.WriteJSON(w, http.StatusOK, p)
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	body, err := decode[CreatePerson](w, r)
	if err != nil {
		_ = h.errors.Write(w, r, err)
		return
	}
	ident, err := h.service.Create(r.Context(), body)
	if err != nil {
		_ = h.errors.Write(w, r, err)
		return
	}
	w.Header().Set("Location", r.URL.Path+"/"+ident.ID)
	_ = web.WriteJSON(w, http.StatusCreated, ident)
}

func (h *handler) edit(w http.ResponseWriter, r *http.Request) {
	id, version, body, ok := command[EditPerson](h, w, r)
	if !ok {
		return
	}
	h.identity(w, r)(h.service.Edit(r.Context(), id, version, body))
}

func (h *handler) transferUnit(w http.ResponseWriter, r *http.Request) {
	id, version, body, ok := command[TransferUnit](h, w, r)
	if !ok {
		return
	}
	h.identity(w, r)(h.service.TransferUnit(r.Context(), id, version, body))
}

func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	id, version, ok := h.guarded(w, r)
	if !ok {
		return
	}
	if err := h.service.Delete(r.Context(), id, version); err != nil {
		_ = h.errors.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// action binds a bodiless action — id and version in, identity out.
func (h *handler) action(run func(*http.Request, string, int64) (Identity, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, version, ok := h.guarded(w, r)
		if !ok {
			return
		}
		h.identity(w, r)(run(r, id, version))
	}
}

// identity writes a command's envelope, or its rejection.
func (h *handler) identity(w http.ResponseWriter, r *http.Request) func(Identity, error) {
	return func(ident Identity, err error) {
		if err != nil {
			_ = h.errors.Write(w, r, err)
			return
		}
		_ = web.WriteJSON(w, http.StatusOK, ident)
	}
}

// guarded reads a guarded operation's id and If-Match version.
func (h *handler) guarded(w http.ResponseWriter, r *http.Request) (string, int64, bool) {
	id, ok := h.pathID(w, r)
	if !ok {
		return "", 0, false
	}
	version, err := sdk.IfMatch(r)
	if err != nil {
		_ = h.errors.Write(w, r, err)
		return "", 0, false
	}
	return id, version, true
}

// command reads a guarded command's three inputs in order.
func command[T any](h *handler, w http.ResponseWriter, r *http.Request) (id string, version int64, body T, ok bool) {
	id, version, ok = h.guarded(w, r)
	if !ok {
		return "", 0, body, false
	}
	body, err := decode[T](w, r)
	if err != nil {
		_ = h.errors.Write(w, r, err)
		return "", 0, body, false
	}
	return id, version, body, true
}

func (h *handler) pathID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		_ = h.errors.Write(w, r, fmt.Errorf("%w: id must be a UUID", ErrValidation))
		return "", false
	}
	return id, true
}

func decode[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	var v T
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCommandBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return v, fmt.Errorf("%w: body: %v", ErrValidation, err)
	}
	return v, nil
}

// status is the layer's error vocabulary as one web.StatusMatcher; the
// transition rule joins the state conflicts at 409.
func status(err error) (int, bool) {
	var precondition *sdk.PreconditionError
	switch {
	case errors.As(err, &precondition):
		if precondition.Missing {
			return http.StatusPreconditionRequired, true
		}
		return http.StatusBadRequest, true
	case errors.Is(err, ErrValidation), errors.Is(err, query.ErrDirectives):
		return http.StatusBadRequest, true
	case errors.Is(err, sql.ErrNoRows):
		return http.StatusNotFound, true
	case errors.Is(err, ErrTransition),
		errors.Is(err, database.ErrUniqueViolation),
		errors.Is(err, database.ErrForeignKeyViolation):
		return http.StatusConflict, true
	case errors.Is(err, query.ErrVersionMismatch):
		return http.StatusPreconditionFailed, true
	}
	return 0, false
}
