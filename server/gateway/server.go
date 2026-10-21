// Package gateway is the HTTP layer: router, middleware (request-id, rate limit,
// idempotency, auth) and the v1 handlers owned by server-core.
package gateway

import (
	"context"
	"net/http"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/geone39/fireline/server/auth"
	"github.com/geone39/fireline/server/db"
)

// Config holds gateway tunables (all come from env in cmd/api).
type Config struct {
	IPRateRPS     float64
	IPRateBurst   int
	UserRateRPS   float64
	UserRateBurst int
}

// Deps are the collaborators of the gateway.
type Deps struct {
	Config  Config
	Auth    *auth.Service
	Store   *db.Store
	Idem    IdemStore
	Content *Content
	Log     zerolog.Logger
	Now     func() time.Time // optional, for tests

	// ExtraAuthedRoutes lets other modules (and tests) mount routes behind
	// auth + rate limit + content check + idempotency.
	ExtraAuthedRoutes func(r chi.Router)

	// Base, when set, mounts /v1/base* and fills builders in GET /v1/me.
	Base *BaseModule

	// Army, when set, mounts /v1/army* and /v1/lab* and adds army and lab to GET /v1/me.
	Army *ArmyModule
}

// Server is the HTTP gateway.
type Server struct {
	cfg        Config
	auth       *auth.Service
	store      *db.Store
	idem       IdemStore
	content    *Content
	log        zerolog.Logger
	nowFn      func() time.Time
	ipLimits   *limiterSet
	userLimits *limiterSet
	extra      func(r chi.Router)
	base       *BaseModule
	army       *ArmyModule
	metrics    *metrics
}

// New builds a Server.
func New(d Deps) *Server {
	s := &Server{cfg: d.Config, auth: d.Auth, store: d.Store, idem: d.Idem, content: d.Content, log: d.Log, nowFn: d.Now, extra: d.ExtraAuthedRoutes, base: d.Base, army: d.Army}
	var pool *pgxpool.Pool
	if d.Store != nil {
		pool = d.Store.Pool
	}
	s.metrics = newMetrics(pool)
	s.ipLimits = newLimiterSet(d.Config.IPRateRPS, d.Config.IPRateBurst, s.now)
	s.userLimits = newLimiterSet(d.Config.UserRateRPS, d.Config.UserRateBurst, s.now)
	return s
}

func (s *Server) now() time.Time {
	if s.nowFn != nil {
		return s.nowFn()
	}
	return time.Now()
}

// Handler returns the root http.Handler.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(s.requestID, s.instrument, s.recoverer, s.accessLog, s.ipRateLimit)
	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		s.writeError(w, http.StatusNotFound, CodeNotFound, "route not found", nil)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		s.writeError(w, http.StatusBadRequest, CodeInvalid, "method not allowed", nil)
	})

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	// ASSUMPTION: /metrics is served on the public listener without auth; restrict it at
	// the reverse proxy / network policy in production.
	r.Method(http.MethodGet, "/metrics", s.metrics.handler())
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		if err := s.store.Ping(req.Context()); err != nil {
			s.writeError(w, http.StatusServiceUnavailable, CodeInternal, "database unavailable", nil)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	r.Route("/v1", func(r chi.Router) {
		r.Post("/auth/guest", s.handleGuest)
		r.Post("/auth/refresh", s.handleRefresh)
		r.Get("/content/manifest", s.handleManifest)

		r.Group(func(r chi.Router) {
			r.Use(s.contentHeader, s.authenticate, s.userRateLimit, s.contentCheck, s.idempotency)
			r.Get("/me", s.handleMe)
			if s.base != nil {
				s.base.routes(s)(r)
			}
			if s.army != nil {
				s.army.routes(s)(r)
			}
			if s.extra != nil {
				s.extra(r)
			}
		})
	})
	return r
}

type ctxKey int

const (
	ctxRequestID ctxKey = iota
	ctxPlayer
)

// RequestIDFrom returns the request id stored by the requestID middleware.
func RequestIDFrom(ctx context.Context) string { v, _ := ctx.Value(ctxRequestID).(string); return v }

// PlayerFrom returns the authenticated player id.
func PlayerFrom(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(ctxPlayer).(uuid.UUID)
	return v, ok
}

var requestIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]{8,64}$`)

func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(headerRequestID)
		if !requestIDRe.MatchString(id) {
			id = uuid.NewString()
		}
		w.Header().Set(headerRequestID, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxRequestID, id)))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := s.now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		ev := s.log.Info()
		if sw.status >= 500 {
			ev = s.log.Error()
		}
		ev.Str("request_id", RequestIDFrom(r.Context())).Str("method", r.Method).Str("path", r.URL.Path).
			Int("status", sw.status).Dur("dur", s.now().Sub(start)).Msg("request")
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				s.log.Error().Str("request_id", RequestIDFrom(r.Context())).Interface("panic", p).Bytes("stack", debug.Stack()).Msg("panic")
				s.writeError(w, http.StatusInternalServerError, CodeInternal, "internal error", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) internal(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error().Err(err).Str("request_id", RequestIDFrom(r.Context())).Msg("internal error")
	s.writeError(w, http.StatusInternalServerError, CodeInternal, "internal error", nil)
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		tok, found := strings.CutPrefix(h, "Bearer ")
		if !found || tok == "" {
			s.writeError(w, http.StatusUnauthorized, CodeUnauthorized, "missing bearer token", nil)
			return
		}
		claims, err := s.auth.Signer.Verify(tok, auth.KindAccess)
		if err != nil {
			s.writeError(w, http.StatusUnauthorized, CodeUnauthorized, "invalid or expired token", nil)
			return
		}
		id, err := claims.PlayerID()
		if err != nil {
			s.writeError(w, http.StatusUnauthorized, CodeUnauthorized, "invalid token subject", nil)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxPlayer, id)))
	})
}

// contentCheck rejects authenticated requests from a client whose X-Content-Version
// differs from the server's (docs/03-api.md: content_outdated, HTTP 409). A missing
// header is accepted.
func (s *Server) contentCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Header.Get(headerContentVer); v != "" && v != s.content.Version() {
			s.writeError(w, http.StatusConflict, CodeContentOld, "client content version is outdated",
				map[string]any{"content_version": s.content.Version(), "bundle_url": s.content.bundleURL})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// contentHeader puts X-Content-Version on every authenticated response, including the
// ones written by mounted modules (svc/base writes its own bodies).
func (s *Server) contentHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(headerContentVer, s.content.Version())
		next.ServeHTTP(w, r)
	})
}
