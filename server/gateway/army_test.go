package gateway_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/geone39/fireline/server/auth"
	"github.com/geone39/fireline/server/gateway"
	"github.com/geone39/fireline/server/svc/base"
	"github.com/geone39/fireline/server/svc/economy"
)

type armyEnv struct {
	*env
	ledger *economy.Ledger
}

func newArmyEnv(t *testing.T) *armyEnv { return newArmyEnvIn(t, contentDir) }

func newArmyEnvIn(t *testing.T, contentDir string) *armyEnv {
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
	prices, err := base.LoadPriceBook(contentDir)
	if err != nil {
		t.Fatal(err)
	}
	mod, err := gateway.NewBaseModule(gateway.BaseConfig{Store: testStore, Econ: econ, Cat: cat, Grid: grid, Prices: prices, ContentDir: contentDir})
	if err != nil {
		t.Fatal(err)
	}
	am, err := gateway.NewArmyModule(gateway.ArmyConfig{Store: testStore, Ledger: mod.Ledger, ContentDir: contentDir, Cat: cat, Prices: prices})
	if err != nil {
		t.Fatal(err)
	}
	mod.Army = am
	content, err := gateway.LoadContent(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	g := gateway.New(gateway.Deps{
		Config: roomy,
		Auth: &auth.Service{Store: testStore, Provision: mod.Provision,
			Signer: &auth.Signer{Secret: bytes.Repeat([]byte("k"), 32), AccessTTL: 15 * time.Minute, RefreshTTL: 30 * 24 * time.Hour}},
		Store: testStore, Idem: testStore, Content: content, Log: zerolog.Nop(), Base: mod, Army: am,
	})
	ts := httptest.NewServer(g.Handler())
	t.Cleanup(ts.Close)
	return &armyEnv{env: &env{srv: ts, version: content.Version()}, ledger: mod.Ledger}
}

// buildReady adds finished buildings to the player's layout (the tutorial builds them in the game).
func buildReady(t *testing.T, player uuid.UUID, items ...base.Item) {
	t.Helper()
	var raw string
	if err := testStore.Pool.QueryRow(context.Background(), `SELECT layout::text FROM base_state WHERE player_id=$1`, player).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var l base.Layout
	if err := json.Unmarshal([]byte(raw), &l); err != nil {
		t.Fatal(err)
	}
	l.Items = append(l.Items, items...)
	b, _ := json.Marshal(l)
	if _, err := testStore.Pool.Exec(context.Background(), `UPDATE base_state SET layout=$2::jsonb WHERE player_id=$1`, player, string(b)); err != nil {
		t.Fatal(err)
	}
}

func obj(t *testing.T, m map[string]any, k string) map[string]any {
	t.Helper()
	v, ok := m[k].(map[string]any)
	if !ok {
		t.Fatalf("%s is not an object in %v", k, m)
	}
	return v
}

func errOf(m map[string]any) (code, reason string) {
	e, _ := m["error"].(map[string]any)
	d, _ := e["details"].(map[string]any)
	code, _ = e["code"].(string)
	reason, _ = d["reason"].(string)
	return code, reason
}

func TestNewGuestArmyLabTrainResearch(t *testing.T) {
	e := newArmyEnv(t)
	access, _ := e.login(t)
	player := playerOf(t, e.env, access)
	idem := func(k string) map[string]string { return map[string]string{"Idempotency-Key": k} }

	// A new guest sees an empty army with a camp (start_layout.yaml) and no laboratory.
	st, a, hdr := e.do(t, "GET", "/v1/army", access, "", nil)
	if st != 200 || a["server_time"] == nil || a["content_version"] != e.version || hdr.Get("X-Content-Version") != e.version {
		t.Fatalf("army: %d %v %v", st, a, hdr)
	}
	if unit := obj(t, obj(t, a, "pools"), "unit"); unit["capacity"] != float64(20) || unit["used"] != float64(0) {
		t.Fatalf("unit pool: %v", unit)
	}
	if len(a["stock"].([]any)) != 0 {
		t.Fatalf("start_army_auto_grant is false: stock must be empty: %v", a["stock"])
	}
	st, lab, _ := e.do(t, "GET", "/v1/lab", access, "", nil)
	if st != 200 || lab["lab_level"] != float64(0) || lab["active"] != nil {
		t.Fatalf("lab: %d %v", st, lab)
	}
	_, me, _ := e.do(t, "GET", "/v1/me", access, "", nil)
	if p := obj(t, obj(t, me, "army"), "pools")["unit"].(map[string]any); p["capacity"] != float64(20) {
		t.Fatalf("me.army: %v", me["army"])
	}
	if l := obj(t, me, "lab"); l["level"] != float64(0) || l["active"] != nil {
		t.Fatalf("me.lab: %v", l)
	}

	buildReady(t, player,
		base.Item{UID: "rax", Type: "b_barracks", Level: 1, X: 4, Y: 4},
		base.Item{UID: "lab", Type: "b_laboratory", Level: 1, X: 30, Y: 30})
	fuel := func() float64 { return wallets(t, e.env, access)["fuel"].(float64) }
	startFuel := fuel()

	// The first order is finished for free (OQ-ARMY-8): units are in stock, the queue is empty.
	train := `{"building_id":"rax","unit_id":"u_rifleman","count":5}`
	st, first, _ := e.do(t, "POST", "/v1/army/train", access, train, idem("train-1"))
	if st != 200 {
		t.Fatalf("train: %d %v", st, first)
	}
	stock := first["stock"].([]any)
	if len(stock) != 1 || stock[0].(map[string]any)["count"] != float64(5) {
		t.Fatalf("first order must be finished free: %v", first)
	}
	for _, q := range first["queues"].([]any) {
		if len(q.(map[string]any)["orders"].([]any)) != 0 {
			t.Fatalf("queue must be empty after the free finish: %v", first["queues"])
		}
	}
	if got := fuel(); got != startFuel-125 {
		t.Fatalf("fuel after 5 riflemen = %v, want %v", got, startFuel-125)
	}
	// Same Idempotency-Key: the stored response, nothing charged twice.
	st, again, h2 := e.do(t, "POST", "/v1/army/train", access, train, idem("train-1"))
	if st != 200 || h2.Get("Idempotent-Replay") != "true" || again["server_time"] != first["server_time"] || fuel() != startFuel-125 {
		t.Fatalf("replay: %d %v %v", st, again, h2)
	}

	// The second order is a normal one: charged and timed.
	st, second, _ := e.do(t, "POST", "/v1/army/train", access, `{"building_id":"rax","unit_id":"u_rifleman","count":2}`, idem("train-2"))
	if st != 200 {
		t.Fatalf("second train: %d %v", st, second)
	}
	orders := second["queues"].([]any)[0].(map[string]any)["orders"].([]any)
	if len(orders) != 1 || orders[0].(map[string]any)["remaining"] != float64(2) || fuel() != startFuel-125-50 {
		t.Fatalf("second order must be queued and charged: %v fuel=%v", second["queues"], fuel())
	}
	if unit := obj(t, obj(t, second, "pools"), "unit"); unit["used"] != float64(7) {
		t.Fatalf("pool counts queued units: %v", unit)
	}
	if g, _ := wallets(t, e.env, access)["gems"].(float64); g != 0 {
		t.Fatalf("the tutorial finish must not spend gems: %v", g)
	}

	// Errors keep the common envelope with the contract codes.
	st, eb, _ := e.do(t, "POST", "/v1/army/train", access, `{"building_id":"rax","unit_id":"u_rifleman","count":100}`, idem("train-3"))
	if c, r := errOf(eb); st != 409 || c != "conflict" || r != "housing_full" || eb["server_time"] == nil {
		t.Fatalf("housing_full: %d %v", st, eb)
	}
	st, eb, _ = e.do(t, "POST", "/v1/army/train", access, `{"building_id":"nope","unit_id":"u_rifleman","count":1}`, idem("train-4"))
	if c, _ := errOf(eb); st != 404 || c != "not_found" {
		t.Fatalf("not_found: %d %v", st, eb)
	}
	if st, eb, _ = e.do(t, "POST", "/v1/army/train", access, train, nil); st != 400 {
		t.Fatalf("no idempotency key: %d %v", st, eb)
	}
	if st, _, _ = e.do(t, "GET", "/v1/army", "", "", nil); st != 401 {
		t.Fatalf("unauthenticated: %d", st)
	}

	// Research: tier 1 costs 800 fuel; the player has too little first.
	research := `{"node_id":"r_rifle_1"}`
	st, eb, _ = e.do(t, "POST", "/v1/lab/research", access, research, idem("res-0"))
	if c, _ := errOf(eb); st != 422 || c != "insufficient_resources" {
		t.Fatalf("insufficient_resources: %d %v", st, eb)
	}
	if _, err := e.ledger.Apply(context.Background(), player, base.Credit(map[string]int64{"fuel": 2000}), "test_credit", "test-credit-"+player.String()); err != nil {
		t.Fatal(err)
	}
	before := fuel()
	st, r1, _ := e.do(t, "POST", "/v1/lab/research", access, research, idem("res-1"))
	if st != 200 || obj(t, r1, "active")["node_id"] != "r_rifle_1" || fuel() != before-800 {
		t.Fatalf("research: %d %v fuel=%v", st, r1, fuel())
	}
	st, r2, h3 := e.do(t, "POST", "/v1/lab/research", access, research, idem("res-1"))
	if st != 200 || h3.Get("Idempotent-Replay") != "true" || r2["server_time"] != r1["server_time"] || fuel() != before-800 {
		t.Fatalf("research replay: %d %v", st, r2)
	}
	st, eb, _ = e.do(t, "POST", "/v1/lab/research", access, research, idem("res-2"))
	if c, r := errOf(eb); st != 409 || c != "conflict" || r != "lab_busy" {
		t.Fatalf("lab_busy: %d %v", st, eb)
	}
	_, me, _ = e.do(t, "GET", "/v1/me", access, "", nil)
	if l := obj(t, me, "lab"); l["level"] != float64(1) || obj(t, l, "active")["node_id"] != "r_rifle_1" {
		t.Fatalf("me.lab: %v", l)
	}
}

func TestStartLayoutFromContent(t *testing.T) {
	e := newArmyEnv(t)
	access, _ := e.login(t)
	_, b, _ := e.do(t, "GET", "/v1/base", access, "", nil)
	types := map[string]bool{}
	for _, it := range b["layout"].(map[string]any)["items"].([]any) {
		types[it.(map[string]any)["type"].(string)] = true
	}
	if !types["b_hq"] || !types["b_camp"] || len(types) != 2 {
		t.Fatalf("start layout: %v", types)
	}
}

// With start_army_auto_grant a new guest gets start_army once, in the guest transaction.
func TestStartArmyAutoGrant(t *testing.T) {
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(contentDir)); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "army.yaml")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "start_army_auto_grant: false") {
		t.Fatal("fixture: army.yaml has no start_army_auto_grant: false")
	}
	if err := os.WriteFile(p, []byte(strings.Replace(string(raw), "start_army_auto_grant: false", "start_army_auto_grant: true", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	e := newArmyEnvIn(t, dir)
	access, device := e.login(t)
	check := func(access string) {
		t.Helper()
		_, a, _ := e.do(t, "GET", "/v1/army", access, "", nil)
		stock := a["stock"].([]any)
		if len(stock) != 1 || stock[0].(map[string]any)["id"] != "u_rifleman" || stock[0].(map[string]any)["count"] != float64(5) {
			t.Fatalf("start army: %v", a["stock"])
		}
	}
	check(access)
	_, body, _ := e.do(t, "POST", "/v1/auth/guest", "", `{"device_id":"`+device+`"}`, nil)
	check(body["tokens"].(map[string]any)["access_token"].(string))
}
