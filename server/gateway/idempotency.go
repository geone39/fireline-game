package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/geone39/fireline/server/db"
)

const (
	maxBodyBytes  = 1 << 20
	maxIdemKeyLen = 128
)

// IdemStore persists idempotent responses; implemented by *db.Store.
type IdemStore interface {
	IdemBegin(ctx context.Context, player uuid.UUID, key string, hash []byte) (bool, db.IdemRecord, error)
	IdemComplete(ctx context.Context, player uuid.UUID, key string, status int, contentType string, body []byte) error
	IdemRelease(ctx context.Context, player uuid.UUID, key string) error
}

type recorder struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	r.buf.Write(b)
	return r.ResponseWriter.Write(b)
}

// idempotency makes mutating requests safe to retry. The Idempotency-Key header is
// mandatory for POST/PUT/PATCH/DELETE; the same key with the same request replays the
// stored response, the same key with a different request is a conflict. 5xx responses
// are not stored so the client can retry.
func (s *Server) idempotency(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			next.ServeHTTP(w, r)
			return
		}
		player, ok := PlayerFrom(r.Context())
		key := r.Header.Get(headerIdempotency)
		if !ok || key == "" || len(key) > maxIdemKeyLen {
			s.writeError(w, http.StatusBadRequest, CodeInvalid, "Idempotency-Key header (1..128 chars) is required", nil)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
		if err != nil {
			s.writeError(w, http.StatusBadRequest, CodeInvalid, "request body too large", nil)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		h := sha256.New()
		h.Write([]byte(r.Method + " " + r.URL.Path + "?" + r.URL.RawQuery + "\n"))
		h.Write(body)
		hash := h.Sum(nil)

		started, rec, err := s.idem.IdemBegin(r.Context(), player, key, hash)
		if errors.Is(err, db.ErrNotFound) { // released concurrently: treat as in flight
			s.writeError(w, http.StatusConflict, CodeConflict, "request with this Idempotency-Key is in progress", nil)
			return
		}
		if err != nil {
			s.internal(w, r, err)
			return
		}
		if !started {
			switch {
			case !rec.SameRequest(hash):
				s.writeError(w, http.StatusConflict, CodeConflict, "Idempotency-Key was used with a different request", nil)
			case !rec.Done:
				s.writeError(w, http.StatusConflict, CodeConflict, "request with this Idempotency-Key is in progress", nil)
			default:
				w.Header().Set("Content-Type", rec.ContentType)
				s.metrics.idemReplays.Inc()
				w.Header().Set("Idempotent-Replay", "true")
				w.WriteHeader(rec.Status)
				_, _ = w.Write(rec.Body)
			}
			return
		}

		rw := &recorder{ResponseWriter: w}
		defer func() {
			// Use a detached context: the client may have gone away.
			ctx := context.WithoutCancel(r.Context())
			if p := recover(); p != nil {
				_ = s.idem.IdemRelease(ctx, player, key)
				panic(p)
			}
			if rw.status == 0 || rw.status >= 500 {
				_ = s.idem.IdemRelease(ctx, player, key)
				return
			}
			if err := s.idem.IdemComplete(ctx, player, key, rw.status, w.Header().Get("Content-Type"), rw.buf.Bytes()); err != nil {
				s.log.Error().Err(err).Msg("idempotency: complete failed")
			}
		}()
		next.ServeHTTP(rw, r)
	})
}
