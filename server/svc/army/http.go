package army

import (
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

const maxBody = 1 << 16

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

// Mount registers the army routes on r, relative to its current prefix. The gateway
// calls it inside the authenticated /v1 group (auth, rate limit, content check and
// Idempotency-Key handling already applied):
//
//	GET  /army          ArmyState with finished training settled lazily
//	POST /army/train    {building_id, unit_id, count}
//	POST /army/cancel   {order_id, count?}   (proposal, army-lab.md §8)
//	POST /army/speedup  {building_id}        (proposal, army-lab.md §8)
func Mount(r chi.Router, d Deps) {
	h := &handler{d}
	r.Get("/army", h.get)
	r.Post("/army/train", h.train)
	r.Post("/army/cancel", h.cancel)
	r.Post("/army/speedup", h.speedup)
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
	var ae *Error
	switch {
	case errors.As(err, &ae):
		details := map[string]any{"reason": ae.Reason}
		for k, v := range ae.Details {
			details[k] = v
		}
		h.fail(w, status[ae.Kind], ae.Kind, ae.Msg, details)
	case errors.Is(err, ErrNoBase):
		h.fail(w, http.StatusNotFound, "not_found", "base not found", nil)
	case errors.Is(err, economy.ErrInsufficientFunds):
		h.fail(w, status["insufficient_resources"], "insufficient_resources", "not enough resources", nil)
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

// mutate parses the Idempotency-Key and the JSON body, then runs f.
func (h *handler) mutate(w http.ResponseWriter, r *http.Request, body any, f func(id uuid.UUID, key string) (State, error)) {
	id, ok := h.player(w, r)
	if !ok {
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		h.fail(w, http.StatusBadRequest, "invalid", "Idempotency-Key header is required", nil)
		return
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err == nil {
		err = json.Unmarshal(b, body)
	}
	if err != nil {
		h.fail(w, http.StatusBadRequest, "invalid", "body must be a JSON object", nil)
		return
	}
	st, err := f(id, key)
	if err != nil {
		h.fromError(w, r, err)
		return
	}
	h.json(w, http.StatusOK, st)
}

func (h *handler) train(w http.ResponseWriter, r *http.Request) {
	var req Request
	h.mutate(w, r, &req, func(id uuid.UUID, key string) (State, error) {
		return h.Service.Train(r.Context(), id, key, req)
	})
}

func (h *handler) cancel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrderID string `json:"order_id"`
		Count   int    `json:"count"`
	}
	h.mutate(w, r, &req, func(id uuid.UUID, key string) (State, error) {
		return h.Service.Cancel(r.Context(), id, key, req.OrderID, req.Count)
	})
}

func (h *handler) speedup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BuildingID string `json:"building_id"`
	}
	h.mutate(w, r, &req, func(id uuid.UUID, key string) (State, error) {
		return h.Service.Speedup(r.Context(), id, key, req.BuildingID)
	})
}
