package gateway

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// metrics holds the Prometheus collectors of one Server. Each Server owns its
// registry so tests can build many servers in one process.
type metrics struct {
	reg         *prometheus.Registry
	requests    *prometheus.CounterVec
	duration    *prometheus.HistogramVec
	inFlight    prometheus.Gauge
	rateLimited *prometheus.CounterVec
	idemReplays prometheus.Counter
	refresh     *prometheus.CounterVec
}

func newMetrics(pool *pgxpool.Pool) *metrics {
	m := &metrics{
		reg: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fireline_http_requests_total", Help: "HTTP requests by method, route pattern and status."},
			[]string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "fireline_http_request_duration_seconds", Help: "HTTP request latency by method and route pattern.",
			Buckets: prometheus.DefBuckets}, []string{"method", "route"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "fireline_http_requests_in_flight", Help: "Requests currently being served."}),
		rateLimited: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fireline_rate_limited_total", Help: "Requests rejected by the rate limiter."}, []string{"scope"}),
		idemReplays: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "fireline_idempotent_replays_total", Help: "Responses replayed from the idempotency store."}),
		refresh: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fireline_auth_refresh_total", Help: "Refresh-token exchanges by result (ok, invalid, error)."}, []string{"result"}),
	}
	m.reg.MustRegister(m.requests, m.duration, m.inFlight, m.rateLimited, m.idemReplays, m.refresh,
		collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	if pool != nil {
		m.reg.MustRegister(poolCollector{pool: pool})
	}
	return m
}

func (m *metrics) handler() http.Handler {
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{})
}

// instrument records request count, latency and in-flight gauge. The route label is the
// chi route pattern (low cardinality); unmatched requests are labelled "unmatched".
func (s *Server) instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := s.now()
		s.metrics.inFlight.Inc()
		defer s.metrics.inFlight.Dec()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		route := "unmatched"
		if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" && !strings.HasSuffix(rc.RoutePattern(), "/*") {
			route = rc.RoutePattern()
		}
		s.metrics.requests.WithLabelValues(r.Method, route, strconv.Itoa(sw.status)).Inc()
		s.metrics.duration.WithLabelValues(r.Method, route).Observe(s.now().Sub(start).Seconds())
	})
}

// poolCollector exposes pgxpool statistics.
type poolCollector struct{ pool *pgxpool.Pool }

var (
	descPoolConns = prometheus.NewDesc("fireline_db_pool_conns", "Database pool connections by state.", []string{"state"}, nil)
	descPoolWait  = prometheus.NewDesc("fireline_db_pool_acquire_total", "Total connection acquires.", nil, nil)
)

func (poolCollector) Describe(ch chan<- *prometheus.Desc) { ch <- descPoolConns; ch <- descPoolWait }

func (c poolCollector) Collect(ch chan<- prometheus.Metric) {
	st := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(descPoolConns, prometheus.GaugeValue, float64(st.AcquiredConns()), "acquired")
	ch <- prometheus.MustNewConstMetric(descPoolConns, prometheus.GaugeValue, float64(st.IdleConns()), "idle")
	ch <- prometheus.MustNewConstMetric(descPoolConns, prometheus.GaugeValue, float64(st.TotalConns()), "total")
	ch <- prometheus.MustNewConstMetric(descPoolWait, prometheus.CounterValue, float64(st.AcquireCount()))
}
