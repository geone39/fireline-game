package gateway_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/geone39/fireline/server/auth"
	"github.com/geone39/fireline/server/gateway"
)

// clockEnv is an env whose JWT signer and gateway use a shiftable clock.
func clockEnv(t *testing.T) (*env, *atomic.Int64) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("x: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	content, err := gateway.LoadContent(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	offset := &atomic.Int64{} // seconds added to the wall clock
	now := func() time.Time { return time.Now().Add(time.Duration(offset.Load()) * time.Second) }
	g := gateway.New(gateway.Deps{
		Config:  roomy,
		Auth:    &auth.Service{Store: testStore, Signer: &auth.Signer{Secret: bytes.Repeat([]byte("k"), 32), AccessTTL: 15 * time.Minute, RefreshTTL: 30 * 24 * time.Hour, Now: now}},
		Store:   testStore,
		Idem:    testStore,
		Content: content,
		Log:     zerolog.Nop(),
		Now:     now,
	})
	ts := httptest.NewServer(g.Handler())
	t.Cleanup(ts.Close)
	return &env{srv: ts, counter: &atomic.Int64{}, version: content.Version()}, offset
}

func (e *env) guestTokens(t *testing.T) map[string]any {
	t.Helper()
	st, body, _ := e.do(t, "POST", "/v1/auth/guest", "", "", nil)
	if st != 200 {
		t.Fatalf("guest: %d %v", st, body)
	}
	return body["tokens"].(map[string]any)
}

func (e *env) refresh(t *testing.T, tok string) (int, map[string]any) {
	t.Helper()
	st, body, _ := e.do(t, "POST", "/v1/auth/refresh", "", `{"refresh_token":"`+tok+`"}`, nil)
	return st, body
}

func (e *env) raw(t *testing.T, path string) string {
	t.Helper()
	resp, err := http.Get(e.srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func errCode(body map[string]any) any {
	er, _ := body["error"].(map[string]any)
	return er["code"]
}

func TestRefreshRotation(t *testing.T) {
	e := newEnv(t, roomy)
	t0 := e.guestTokens(t)
	old := t0["refresh_token"].(string)

	st, body := e.refresh(t, old)
	if st != 200 {
		t.Fatalf("refresh: %d %v", st, body)
	}
	t1 := body["tokens"].(map[string]any)
	if body["server_time"] == nil || body["content_version"] == nil || body["player_id"] == "" {
		t.Errorf("missing fields: %v", body)
	}
	if t1["refresh_token"] == old || t1["access_token"] == t0["access_token"] {
		t.Error("tokens were not rotated")
	}
	if st, _, _ := e.do(t, "GET", "/v1/me", t1["access_token"].(string), "", nil); st != 200 {
		t.Errorf("new access token: %d", st)
	}
	// Reuse of the revoked token -> unauthorized.
	if st, body = e.refresh(t, old); st != 401 || errCode(body) != "unauthorized" {
		t.Errorf("reuse of revoked token: %d %v", st, body)
	}
	// The new refresh token is valid and chains.
	if st, body = e.refresh(t, t1["refresh_token"].(string)); st != 200 {
		t.Fatalf("second rotation: %d %v", st, body)
	}
	// Exactly one active token remains for the player; the older ones are revoked.
	var revoked, active int
	if err := testStore.Pool.QueryRow(context.Background(), `
		SELECT count(*) FILTER (WHERE revoked_at IS NOT NULL), count(*) FILTER (WHERE revoked_at IS NULL)
		FROM refresh_tokens WHERE player_id = $1`, body["player_id"]).Scan(&revoked, &active); err != nil {
		t.Fatal(err)
	}
	if revoked != 2 || active != 1 {
		t.Errorf("revoked=%d active=%d, want 2 and 1", revoked, active)
	}
}

func TestRefreshRejects(t *testing.T) {
	e := newEnv(t, roomy)
	tk := e.guestTokens(t)
	for name, tok := range map[string]string{"garbage": "not-a-jwt", "access token": tk["access_token"].(string)} {
		if st, body := e.refresh(t, tok); st != 401 || errCode(body) != "unauthorized" {
			t.Errorf("%s: %d %v", name, st, body)
		}
	}
	for name, body := range map[string]string{"empty": ``, "no field": `{}`, "bad json": `{`} {
		if st, resp, _ := e.do(t, "POST", "/v1/auth/refresh", "", body, nil); st != 400 || errCode(resp) != "invalid" {
			t.Errorf("%s: %d %v", name, st, resp)
		}
	}
}

func TestRefreshExpired(t *testing.T) {
	e, offset := clockEnv(t)
	tk := e.guestTokens(t)
	offset.Store(int64((30*24*time.Hour + time.Minute) / time.Second))
	if st, body := e.refresh(t, tk["refresh_token"].(string)); st != 401 || errCode(body) != "unauthorized" {
		t.Errorf("expired refresh: %d %v", st, body)
	}
	if st, _, _ := e.do(t, "GET", "/v1/me", tk["access_token"].(string), "", nil); st != 401 {
		t.Errorf("expired access: %d", st)
	}
}

func TestRefreshConcurrentSingleWinner(t *testing.T) {
	e := newEnv(t, roomy)
	tok := e.guestTokens(t)["refresh_token"].(string)
	var wg sync.WaitGroup
	var ok, denied atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch st, _ := e.refresh(t, tok); st {
			case 200:
				ok.Add(1)
			case 401:
				denied.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || denied.Load() != 7 {
		t.Errorf("ok=%d denied=%d, want 1 and 7", ok.Load(), denied.Load())
	}
}

func TestMeBuilders(t *testing.T) {
	e := newEnv(t, roomy)
	access, _ := e.login(t)
	_, me, _ := e.do(t, "GET", "/v1/me", access, "", nil)
	if b, ok := me["builders"].([]any); !ok || len(b) != 0 {
		t.Errorf("builders = %#v, want empty list", me["builders"])
	}
}

func TestStatusMapAndHeaders(t *testing.T) {
	e := newEnv(t, roomy)
	access, _ := e.login(t)
	for _, c := range []struct {
		method, path, token string
		hdr                 map[string]string
		status              int
		code                string
	}{
		{"GET", "/v1/me", "", nil, 401, "unauthorized"},
		{"GET", "/v1/missing", "", nil, 404, "not_found"},
		{"DELETE", "/v1/auth/guest", "", nil, 400, "invalid"},
		{"GET", "/v1/me", access, map[string]string{"X-Content-Version": "stale"}, 409, "content_outdated"},
		{"POST", "/v1/test/inc", access, nil, 400, "invalid"}, // missing Idempotency-Key
	} {
		st, body, hdr := e.do(t, c.method, c.path, c.token, "", c.hdr)
		if st != c.status || errCode(body) != c.code {
			t.Errorf("%s %s: %d %v, want %d %s", c.method, c.path, st, body, c.status, c.code)
		}
		if hdr.Get("X-Content-Version") != e.version {
			t.Errorf("%s %s: X-Content-Version = %q", c.method, c.path, hdr.Get("X-Content-Version"))
		}
	}
	if st, _, _ := e.do(t, "GET", "/v1/me", access, "", map[string]string{"X-Content-Version": e.version}); st != 200 {
		t.Errorf("matching X-Content-Version: %d", st)
	}
}

func TestMetrics(t *testing.T) {
	e := newEnv(t, roomy)
	access, _ := e.login(t)
	e.do(t, "GET", "/v1/me", access, "", nil)
	e.refresh(t, "garbage")
	e.do(t, "GET", "/v1/nope", "", "", nil)

	raw := e.raw(t, "/metrics")
	for _, want := range []string{
		`fireline_http_requests_total{method="GET",route="/v1/me",status="200"} 1`,
		`fireline_http_requests_total{method="POST",route="/v1/auth/guest",status="200"} 1`,
		`fireline_http_requests_total{method="GET",route="unmatched",status="404"} 1`,
		`fireline_auth_refresh_total{result="invalid"} 1`,
		`fireline_http_request_duration_seconds_bucket{method="GET",route="/v1/me"`,
		`fireline_http_requests_in_flight`,
		`fireline_db_pool_conns{state="total"}`,
		`go_goroutines`,
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("metrics missing %q", want)
		}
	}
}

func TestCollectorPoolsMigration(t *testing.T) {
	ctx := context.Background()
	p, err := testStore.UpsertPlayer(ctx, "guest", "collector-test-device-id", "C")
	if err != nil {
		t.Fatal(err)
	}
	ins := `INSERT INTO collector_pools (player_id, resource, amount) VALUES ($1, $2, $3)`
	if _, err := testStore.Pool.Exec(ctx, ins, p.ID, "cash", 5); err != nil {
		t.Fatal(err)
	}
	if _, err := testStore.Pool.Exec(ctx, ins, p.ID, "cash", 6); err == nil {
		t.Error("duplicate (player_id, resource) must fail")
	}
	if _, err := testStore.Pool.Exec(ctx, ins, p.ID, "fuel", -1); err == nil {
		t.Error("negative amount must fail")
	}
	var updated time.Time
	if err := testStore.Pool.QueryRow(ctx, `SELECT updated_at FROM collector_pools WHERE player_id = $1`, p.ID).Scan(&updated); err != nil || updated.IsZero() {
		t.Errorf("updated_at default: %v %v", updated, err)
	}
	if _, err := testStore.Pool.Exec(ctx, `DELETE FROM players WHERE id = $1`, p.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = testStore.Pool.QueryRow(ctx, `SELECT count(*) FROM collector_pools WHERE player_id = $1`, p.ID).Scan(&n)
	if n != 0 {
		t.Error("collector_pools must cascade on player delete")
	}
}
