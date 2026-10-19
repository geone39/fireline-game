package gateway

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// limiterSet keeps one token bucket per key and evicts idle ones.
type limiterSet struct {
	mu      sync.Mutex
	rps     rate.Limit
	burst   int
	idle    time.Duration
	entries map[string]*limiterEntry
	lastGC  time.Time
	now     func() time.Time
}

type limiterEntry struct {
	l    *rate.Limiter
	seen time.Time
}

func newLimiterSet(rps float64, burst int, now func() time.Time) *limiterSet {
	return &limiterSet{rps: rate.Limit(rps), burst: burst, idle: 5 * time.Minute, entries: map[string]*limiterEntry{}, now: now, lastGC: now()}
}

// allow reports whether key may proceed; if not, retry is the suggested wait.
func (s *limiterSet) allow(key string) (ok bool, retry time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if now.Sub(s.lastGC) > s.idle {
		for k, e := range s.entries {
			if now.Sub(e.seen) > s.idle {
				delete(s.entries, k)
			}
		}
		s.lastGC = now
	}
	e := s.entries[key]
	if e == nil {
		e = &limiterEntry{l: rate.NewLimiter(s.rps, s.burst)}
		s.entries[key] = e
	}
	e.seen = now
	r := e.l.ReserveN(now, 1)
	if d := r.DelayFrom(now); d > 0 {
		r.CancelAt(now)
		return false, d
	}
	return true, 0
}

func (s *Server) rejectRateLimited(w http.ResponseWriter, scope string, retry time.Duration) {
	s.metrics.rateLimited.WithLabelValues(scope).Inc()
	secs := int(retry.Seconds()) + 1
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	s.writeError(w, http.StatusTooManyRequests, CodeRateLimited, "too many requests", map[string]any{"retry_after_s": secs})
}

// ipRateLimit limits per client IP. ASSUMPTION: X-Forwarded-For is not trusted
// (spoofable); behind a proxy, put real-IP handling in front of the gateway.
func (s *Server) ipRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		if ok, retry := s.ipLimits.allow(host); !ok {
			s.rejectRateLimited(w, "ip", retry)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// userRateLimit limits per authenticated player; must run after authentication.
func (s *Server) userRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := PlayerFrom(r.Context()); ok {
			if ok, retry := s.userLimits.allow(id.String()); !ok {
				s.rejectRateLimited(w, "user", retry)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
