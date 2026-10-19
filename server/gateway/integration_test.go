package gateway_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/geone39/fireline/server/auth"
	"github.com/geone39/fireline/server/db"
	"github.com/geone39/fireline/server/gateway"
)

var testStore *db.Store

func TestMain(m *testing.M) {
	ctx := context.Background()
	ctr, err := tcpg.Run(ctx, "postgres:16-alpine",
		tcpg.WithDatabase("fireline"), tcpg.WithUsername("fireline"), tcpg.WithPassword("fireline"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(90*time.Second)))
	if err != nil {
		// CI must have Docker; locally allow skipping when it is unavailable.
		if os.Getenv("REQUIRE_DOCKER") == "" {
			println("SKIP: docker unavailable:", err.Error())
			os.Exit(0)
		}
		println("FAIL: docker unavailable:", err.Error())
		os.Exit(1)
	}
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(err)
	}
	testStore, err = db.Connect(ctx, dsn)
	if err != nil {
		panic(err)
	}
	if err := testStore.Migrate(ctx); err != nil {
		panic(err)
	}
	code := m.Run()
	testStore.Close()
	_ = testcontainers.TerminateContainer(ctr)
	os.Exit(code)
}

type env struct {
	srv     *httptest.Server
	counter *atomic.Int64
	version string
}

func newEnv(t *testing.T, cfg gateway.Config) *env {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("x: 1\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	content, err := gateway.LoadContent(dir, "https://cdn.example/bundle.zip")
	if err != nil {
		t.Fatal(err)
	}
	counter := &atomic.Int64{}
	g := gateway.New(gateway.Deps{
		Config:  cfg,
		Auth:    &auth.Service{Store: testStore, Signer: &auth.Signer{Secret: bytes.Repeat([]byte("k"), 32), AccessTTL: 15 * time.Minute, RefreshTTL: 30 * 24 * time.Hour}},
		Store:   testStore,
		Idem:    testStore,
		Content: content,
		Log:     zerolog.Nop(),
		ExtraAuthedRoutes: func(r chi.Router) {
			r.Post("/test/inc", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"n":` + string(rune('0'+counter.Add(1))) + `}`))
			})
		},
	})
	ts := httptest.NewServer(g.Handler())
	t.Cleanup(ts.Close)
	return &env{srv: ts, counter: counter, version: content.Version()}
}

var roomy = gateway.Config{IPRateRPS: 1000, IPRateBurst: 1000, UserRateRPS: 1000, UserRateBurst: 1000}

func (e *env) do(t *testing.T, method, path, token string, body string, hdr map[string]string) (int, map[string]any, http.Header) {
	t.Helper()
	req, _ := http.NewRequest(method, e.srv.URL+path, bytes.NewReader([]byte(body)))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%s %s: non-JSON body %q", method, path, raw)
	}
	return resp.StatusCode, out, resp.Header
}

func (e *env) login(t *testing.T) (access, deviceID string) {
	t.Helper()
	st, body, _ := e.do(t, "POST", "/v1/auth/guest", "", "", nil)
	if st != 200 {
		t.Fatalf("guest: %d %v", st, body)
	}
	tokens := body["tokens"].(map[string]any)
	return tokens["access_token"].(string), body["device_id"].(string)
}

func TestGuestMeManifest(t *testing.T) {
	e := newEnv(t, roomy)
	access, device := e.login(t)

	st, me, hdr := e.do(t, "GET", "/v1/me", access, "", nil)
	if st != 200 {
		t.Fatalf("me: %d %v", st, me)
	}
	if hdr.Get("X-Request-ID") == "" {
		t.Error("missing X-Request-ID")
	}
	if me["content_version"] != e.version {
		t.Errorf("content_version = %v, want %s", me["content_version"], e.version)
	}
	if _, err := time.Parse(time.RFC3339Nano, me["server_time"].(string)); err != nil {
		t.Errorf("server_time: %v", err)
	}
	if me["profile"].(map[string]any)["id"] == "" {
		t.Error("empty profile id")
	}

	// Same device -> same player.
	st, body, _ := e.do(t, "POST", "/v1/auth/guest", "", `{"device_id":"`+device+`"}`, nil)
	if st != 200 || body["player_id"] != me["profile"].(map[string]any)["id"] {
		t.Errorf("repeat guest login gave different player: %v", body)
	}

	st, man, _ := e.do(t, "GET", "/v1/content/manifest", "", "", nil)
	if st != 200 || man["content_version"] != e.version || len(man["files"].([]any)) != 1 {
		t.Errorf("manifest: %d %v", st, man)
	}
}

func TestAccessTokenTTL(t *testing.T) {
	e := newEnv(t, roomy)
	_, body, _ := e.do(t, "POST", "/v1/auth/guest", "", "", nil)
	tk := body["tokens"].(map[string]any)
	now := time.Now()
	acc, _ := time.Parse(time.RFC3339Nano, tk["access_expires_at"].(string))
	ref, _ := time.Parse(time.RFC3339Nano, tk["refresh_expires_at"].(string))
	if d := acc.Sub(now); d < 14*time.Minute || d > 16*time.Minute {
		t.Errorf("access ttl %v", d)
	}
	if d := ref.Sub(now); d < 29*24*time.Hour || d > 31*24*time.Hour {
		t.Errorf("refresh ttl %v", d)
	}
	// A refresh token must not work as an access token.
	st, _, _ := e.do(t, "GET", "/v1/me", tk["refresh_token"].(string), "", nil)
	if st != 401 {
		t.Errorf("refresh token accepted as access: %d", st)
	}
}

func TestErrorEnvelope(t *testing.T) {
	e := newEnv(t, roomy)
	check := func(path, token string, wantStatus int, wantCode string) {
		t.Helper()
		st, body, _ := e.do(t, "GET", path, token, "", nil)
		if st != wantStatus {
			t.Errorf("%s: status %d, want %d", path, st, wantStatus)
		}
		er, ok := body["error"].(map[string]any)
		if !ok || er["code"] != wantCode || er["message"] == "" {
			t.Errorf("%s: bad error body %v", path, body)
		}
		if _, ok := er["details"]; !ok {
			t.Errorf("%s: no details", path)
		}
		if body["server_time"] == nil || body["content_version"] == nil {
			t.Errorf("%s: no meta in error response", path)
		}
	}
	check("/v1/me", "", 401, "unauthorized")
	check("/v1/me", "garbage", 401, "unauthorized")
	check("/v1/nope", "", 404, "not_found")

	st, _, _ := e.do(t, "POST", "/v1/auth/guest", "", `{"device_id":"short"}`, nil)
	if st != 400 {
		t.Errorf("bad device_id: %d", st)
	}
}

func TestIdempotency(t *testing.T) {
	e := newEnv(t, roomy)
	access, _ := e.login(t)
	key := map[string]string{"Idempotency-Key": "k-1"}

	st, _, _ := e.do(t, "POST", "/v1/test/inc", access, "{}", nil)
	if st != 400 {
		t.Fatalf("missing key: %d, want 400", st)
	}
	st1, b1, _ := e.do(t, "POST", "/v1/test/inc", access, "{}", key)
	st2, b2, h2 := e.do(t, "POST", "/v1/test/inc", access, "{}", key)
	if st1 != 200 || st2 != 200 || b1["n"] != b2["n"] || e.counter.Load() != 1 {
		t.Errorf("replay failed: %d %v / %d %v counter=%d", st1, b1, st2, b2, e.counter.Load())
	}
	if h2.Get("Idempotent-Replay") != "true" {
		t.Error("missing Idempotent-Replay header")
	}
	st, b, _ := e.do(t, "POST", "/v1/test/inc", access, `{"x":1}`, key)
	if st != 409 || b["error"].(map[string]any)["code"] != "conflict" {
		t.Errorf("key reuse with other body: %d %v", st, b)
	}
}

func TestContentOutdated(t *testing.T) {
	e := newEnv(t, roomy)
	access, _ := e.login(t)
	st, b, _ := e.do(t, "POST", "/v1/test/inc", access, "{}", map[string]string{"Idempotency-Key": "k-c", "X-Content-Version": "stale"})
	er, _ := b["error"].(map[string]any)
	if st != 409 || er["code"] != "content_outdated" || e.counter.Load() != 0 {
		t.Errorf("content check: %d %v", st, b)
	}
	if er["details"].(map[string]any)["bundle_url"] != "https://cdn.example/bundle.zip" {
		t.Errorf("no bundle url: %v", er)
	}
}

func TestRateLimit(t *testing.T) {
	e := newEnv(t, gateway.Config{IPRateRPS: 0.001, IPRateBurst: 3, UserRateRPS: 1000, UserRateBurst: 1000})
	var limited http.Header
	var code int
	for i := 0; i < 5; i++ {
		var h http.Header
		code, _, h = e.do(t, "GET", "/healthz", "", "", nil)
		if code == 429 {
			limited = h
			break
		}
	}
	if code != 429 || limited.Get("Retry-After") == "" {
		t.Errorf("expected 429 with Retry-After, got %d", code)
	}
}

func TestMigrationsAndLedger(t *testing.T) {
	ctx := context.Background()
	for _, tbl := range []string{"players", "wallets", "ledger", "base_state", "base_events", "timers"} {
		var ok bool
		if err := testStore.Pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, tbl).Scan(&ok); err != nil || !ok {
			t.Errorf("table %s missing (err=%v)", tbl, err)
		}
	}
	// Migrating again is a no-op.
	if err := testStore.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := testStore.UpsertPlayer(ctx, "guest", "ledger-test-device-id", "L")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testStore.Pool.Exec(ctx, `INSERT INTO ledger (player_id, resource, delta, reason) VALUES ($1,'cash',10,'test')`, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := testStore.Pool.Exec(ctx, `UPDATE ledger SET delta = 99 WHERE player_id = $1`, p.ID); err == nil {
		t.Error("ledger UPDATE must fail")
	}
	if _, err := testStore.Pool.Exec(ctx, `DELETE FROM ledger WHERE player_id = $1`, p.ID); err == nil {
		t.Error("ledger DELETE must fail")
	}
}
