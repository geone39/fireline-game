package gateway_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"

	"github.com/geone39/fireline/server/auth"
	"github.com/geone39/fireline/server/gateway"
	"github.com/geone39/fireline/server/svc/base"
	"github.com/geone39/fireline/server/svc/economy"
)

const contentDir = "../../content"

// testPricing is a fixture: content/building_costs.yaml is owned by ECON-4 and may not
// exist yet, so these tests never read it.
const testPricing = `
schema: 1
cost_growth: 2
time_growth: 2
speedup_gems: {factor: 4, exponent: 0.8}
cancel_refund_pct: 50
buildings:
  b_hq:          {cost: {cash: 1000}, time_s: 3600}
  b_cash_mine:   {cost: {cash: 100}, time_s: 60}
`

func newBaseEnv(t *testing.T) *env {
	t.Helper()
	econ, err := economy.Load(contentDir + "/economy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cat, err := base.LoadCatalog(contentDir)
	if err != nil {
		t.Fatal(err)
	}
	grid, err := base.LoadGrid(contentDir, "")
	if err != nil {
		t.Fatal(err)
	}
	econYAML, err := os.ReadFile(contentDir + "/economy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	prices, err := base.ParsePriceBook([]byte(testPricing), econYAML)
	if err != nil {
		t.Fatal(err)
	}
	mod, err := gateway.NewBaseModule(gateway.BaseConfig{Store: testStore, Econ: econ, Cat: cat, Grid: grid, Prices: prices})
	if err != nil {
		t.Fatal(err)
	}
	content, err := gateway.LoadContent(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	g := gateway.New(gateway.Deps{
		Config: roomy,
		Auth: &auth.Service{Store: testStore, Provision: mod.Provision,
			Signer: &auth.Signer{Secret: bytes.Repeat([]byte("k"), 32), AccessTTL: 15 * time.Minute, RefreshTTL: 30 * 24 * time.Hour}},
		Store: testStore, Idem: testStore, Content: content, Log: zerolog.Nop(), Base: mod,
	})
	ts := httptest.NewServer(g.Handler())
	t.Cleanup(ts.Close)
	return &env{srv: ts, version: content.Version()}
}

func wallets(t *testing.T, e *env, access string) map[string]any {
	t.Helper()
	st, me, _ := e.do(t, "GET", "/v1/me", access, "", nil)
	if st != 200 {
		t.Fatalf("me: %d %v", st, me)
	}
	return me["wallets"].(map[string]any)
}

func TestGuestBaseGrantBuildReplay(t *testing.T) {
	e := newBaseEnv(t)
	access, device := e.login(t)

	w := wallets(t, e, access)
	if w["cash"] != float64(1500) || w["fuel"] != float64(600) {
		t.Fatalf("start grant: %v", w)
	}
	_, me, _ := e.do(t, "GET", "/v1/me", access, "", nil)
	if bs := me["builders"].([]any); len(bs) == 0 || bs[0].(map[string]any)["busy_until"] != nil {
		t.Fatalf("builders: %v", me["builders"])
	}

	st, b, hdr := e.do(t, "GET", "/v1/base", access, "", nil)
	if st != 200 || b["server_time"] == nil || b["content_version"] != e.version || hdr.Get("X-Content-Version") != e.version {
		t.Fatalf("base: %d %v %v", st, b, hdr)
	}
	items := b["layout"].(map[string]any)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["type"] != "b_hq" {
		t.Fatalf("start layout: %v", items)
	}

	build := `[{"op":"build","uid":"m1","type":"b_cash_mine","x":4,"y":4}]`
	st, first, _ := e.do(t, "POST", "/v1/base/commands", access, build, map[string]string{"Idempotency-Key": "build-1"})
	if st != 200 {
		t.Fatalf("build: %d %v", st, first)
	}
	if got := wallets(t, e, access)["cash"]; got != float64(1400) {
		t.Fatalf("cash after build = %v", got)
	}
	st, again, h2 := e.do(t, "POST", "/v1/base/commands", access, build, map[string]string{"Idempotency-Key": "build-1"})
	if st != 200 || h2.Get("Idempotent-Replay") != "true" || again["server_time"] != first["server_time"] {
		t.Fatalf("replay: %d %v %v", st, again, h2)
	}
	if got := wallets(t, e, access)["cash"]; got != float64(1400) {
		t.Fatalf("cash after replay = %v", got)
	}
	_, me, _ = e.do(t, "GET", "/v1/me", access, "", nil)
	if bs := me["builders"].([]any); bs[0].(map[string]any)["busy_until"] == nil {
		t.Fatalf("builder must be busy: %v", bs)
	}

	// A repeated guest login changes neither base nor wallets.
	_, body, _ := e.do(t, "POST", "/v1/auth/guest", "", `{"device_id":"`+device+`"}`, nil)
	access2 := body["tokens"].(map[string]any)["access_token"].(string)
	if got := wallets(t, e, access2)["cash"]; got != float64(1400) {
		t.Fatalf("cash after re-login = %v", got)
	}

	// Errors keep the common envelope; a missing key is rejected by the gateway.
	st, eb, _ := e.do(t, "POST", "/v1/base/commands", access, build, nil)
	if st != 400 || eb["error"].(map[string]any)["code"] != "invalid" {
		t.Fatalf("no key: %d %v", st, eb)
	}
	// HQ 1 allows two mines (buildings.yaml): the second build fits, the third is a rule violation.
	if st, out, _ := e.do(t, "POST", "/v1/base/commands", access, `[{"op":"build","uid":"m2","type":"b_cash_mine","x":9,"y":9}]`,
		map[string]string{"Idempotency-Key": "build-2"}); st != 200 {
		t.Fatalf("second build: %d %v", st, out)
	}
	st, eb, _ = e.do(t, "POST", "/v1/base/commands", access, `[{"op":"build","uid":"m3","type":"b_cash_mine","x":14,"y":14}]`,
		map[string]string{"Idempotency-Key": "build-3"})
	if st != 400 || eb["error"].(map[string]any)["code"] != "invalid" || eb["server_time"] == nil {
		t.Fatalf("limit reached: %d %v", st, eb)
	}
	if st, _, _ := e.do(t, "GET", "/v1/base", "", "", nil); st != 401 {
		t.Fatalf("unauthenticated: %d", st)
	}
}

func TestSaveLayoutWritesSnapshot(t *testing.T) {
	e := newBaseEnv(t)
	access, _ := e.login(t)
	player := playerOf(t, e, access)

	count := func() int {
		var n int
		if err := testStore.Pool.QueryRow(context.Background(), `SELECT count(*) FROM defence_snapshots WHERE player_id=$1`, player).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	put := func(x int, key string) (int, map[string]any) {
		body := fmt.Sprintf(`{"items":[{"uid":"hq","type":"b_hq","level":1,"x":%d,"y":10,"rot":0}]}`, x)
		st, out, _ := e.do(t, "PUT", "/v1/base/layout", access, body, map[string]string{"Idempotency-Key": key})
		return st, out
	}
	st, out := put(10, "l-1")
	if st != 200 || out["unchanged"] != false || count() != 1 {
		t.Fatalf("save: %d %v snapshots=%d", st, out, count())
	}
	var lvl int
	if err := testStore.Pool.QueryRow(context.Background(),
		`SELECT hq_level FROM defence_snapshots WHERE player_id=$1`, player).Scan(&lvl); err != nil || lvl != 1 {
		t.Fatalf("snapshot hq_level = %d, %v", lvl, err)
	}
	if st, out := put(10, "l-2"); st != 200 || out["unchanged"] != true || count() != 1 {
		t.Fatalf("identical layout must not add a snapshot: %d %v", st, out)
	}
	if st, _ := put(12, "l-3"); st != 200 || count() != 2 {
		t.Fatalf("second layout: %d snapshots=%d", st, count())
	}
	if _, err := testStore.Pool.Exec(context.Background(), `UPDATE defence_snapshots SET hq_level = 9 WHERE player_id=$1`, player); err == nil {
		t.Fatal("defence_snapshots must be immutable")
	}
}

func TestProvisionFailureRollsBackGuest(t *testing.T) {
	svc := &auth.Service{Store: testStore,
		Signer:    &auth.Signer{Secret: bytes.Repeat([]byte("k"), 32), AccessTTL: time.Minute, RefreshTTL: time.Hour},
		Provision: func(context.Context, pgx.Tx, uuid.UUID) error { return errors.New("boom") }}
	device := "dev-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:20]
	if _, _, _, err := svc.Guest(context.Background(), device); err == nil {
		t.Fatal("want error")
	}
	var n int
	if err := testStore.Pool.QueryRow(context.Background(), `SELECT count(*) FROM players WHERE ext_id=$1`, device).Scan(&n); err != nil || n != 0 {
		t.Fatalf("player must not exist: %d %v", n, err)
	}
}

func playerOf(t *testing.T, e *env, access string) uuid.UUID {
	t.Helper()
	_, me, _ := e.do(t, "GET", "/v1/me", access, "", nil)
	id, err := uuid.Parse(me["profile"].(map[string]any)["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	return id
}
