package base

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/geone39/fireline/server/svc/economy"
)

const maxBody = 1 << 20

// Deps are the HTTP layer's dependencies, kept to small interfaces so the gateway can
// wire them without this package importing it.
type Deps struct {
	// Service does the work.
	Service *Service
	// PlayerID returns the authenticated player (gateway.PlayerFrom).
	PlayerID func(ctx context.Context) (uuid.UUID, bool)
	// ContentVersion is the running content version (added to every response body).
	ContentVersion func() string
	// Now is the server clock for server_time; nil means time.Now.
	Now func() time.Time
	// OnError receives unexpected (5xx) errors; optional.
	OnError func(r *http.Request, err error)
}

// Mount registers the base routes on r, relative to its current prefix. The gateway
// calls it inside the authenticated /v1 group (auth, rate limit, content check and
// Idempotency-Key handling already applied):
//
//	GET  /base          BaseState with timers settled lazily
//	POST /base/commands batch of BaseCommand, atomic and idempotent
//	PUT  /base/layout   save a rearranged layout (ValidateLayout) -> new defence snapshot
func Mount(r chi.Router, d Deps) {
	h := &handler{d}
	r.Get("/base", h.get)
	r.Post("/base/commands", h.commands)
	r.Put("/base/layout", h.layout)
}

type handler struct{ Deps }

func (h *handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h *handler) json(w http.ResponseWriter, status int, v any) {
	raw, err := json.Marshal(v)
	out := map[string]json.RawMessage{}
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	if err != nil {
		status, out = http.StatusInternalServerError, map[string]json.RawMessage{}
	}
	out["server_time"], _ = json.Marshal(h.now().UTC().Format(time.RFC3339Nano))
	if h.ContentVersion != nil {
		out["content_version"], _ = json.Marshal(h.ContentVersion())
	}
	body, _ := json.Marshal(out)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func (h *handler) fail(w http.ResponseWriter, status int, code, msg string, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	h.json(w, status, map[string]any{"error": map[string]any{"code": code, "message": msg, "details": details}})
}

// status maps API codes to HTTP statuses (docs/03-api.md; insufficient_resources is 422).
var status = map[string]int{
	"invalid": http.StatusBadRequest, "not_found": http.StatusNotFound, "conflict": http.StatusConflict,
	"insufficient_resources": http.StatusUnprocessableEntity,
}

func (h *handler) fromError(w http.ResponseWriter, r *http.Request, err error) {
	details := map[string]any{}
	var ce *CommandError
	if errors.As(err, &ce) {
		details["index"] = ce.Index
	}
	var be *Error
	switch {
	case errors.As(err, &be):
		code := APICode(be.Code)
		details["reason"] = string(be.Code)
		if be.UID != "" {
			details["uid"] = be.UID
		}
		h.fail(w, status[code], code, be.Msg, details)
	case errors.Is(err, ErrNoBase):
		h.fail(w, http.StatusNotFound, "not_found", "base not found", nil)
	case errors.Is(err, economy.ErrInsufficientFunds):
		h.fail(w, status["insufficient_resources"], "insufficient_resources", "not enough resources", details)
	default:
		if h.OnError != nil {
			h.OnError(r, err)
		}
		h.fail(w, http.StatusInternalServerError, "internal", "internal error", nil)
	}
}

func (h *handler) player(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, ok := h.PlayerID(r.Context())
	if !ok {
		h.fail(w, http.StatusUnauthorized, "unauthorized", "not authenticated", nil)
	}
	return id, ok
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := h.player(w, r)
	if !ok {
		return
	}
	st, err := h.Service.Get(r.Context(), id)
	if err != nil {
		h.fromError(w, r, err)
		return
	}
	h.json(w, http.StatusOK, st)
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	return bytes.TrimSpace(b), err == nil
}

// commands accepts `[{op,args}...]` or `{"commands":[...]}`; each command may carry its
// fields at the top level or inside args.
func (h *handler) commands(w http.ResponseWriter, r *http.Request) {
	id, ok := h.player(w, r)
	if !ok {
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		h.fail(w, http.StatusBadRequest, "invalid", "Idempotency-Key header is required", nil)
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		h.fail(w, http.StatusBadRequest, "invalid", "unreadable request body", nil)
		return
	}
	var cmds []Command
	var err error
	if bytes.HasPrefix(body, []byte("[")) {
		err = json.Unmarshal(body, &cmds)
	} else {
		var env struct {
			Commands []Command `json:"commands"`
		}
		err = json.Unmarshal(body, &env)
		cmds = env.Commands
	}
	if err != nil {
		h.fail(w, http.StatusBadRequest, "invalid", "body must be a command array or {commands:[...]}", nil)
		return
	}
	st, err := h.Service.Apply(r.Context(), id, key, cmds)
	if err != nil {
		h.fromError(w, r, err)
		return
	}
	h.json(w, http.StatusOK, st)
}

func (h *handler) layout(w http.ResponseWriter, r *http.Request) {
	id, ok := h.player(w, r)
	if !ok {
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		h.fail(w, http.StatusBadRequest, "invalid", "unreadable request body", nil)
		return
	}
	var req struct {
		Layout *Layout `json:"layout"`
		Items  []Item  `json:"items"`
	}
	if err := json.Unmarshal(body, &req); err != nil || (req.Layout == nil && req.Items == nil) {
		h.fail(w, http.StatusBadRequest, "invalid", "body must be {layout:{items:[...]}} or {items:[...]}", nil)
		return
	}
	l := Layout{Items: req.Items}
	if req.Layout != nil {
		l = *req.Layout
	}
	res, err := h.Service.SaveLayout(r.Context(), id, l)
	if err != nil {
		h.fromError(w, r, err)
		return
	}
	h.json(w, http.StatusOK, res)
}
