package army

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/geone39/fireline/server/svc/base"
)

// labBase is standard() with HQ4 and a laboratory of the given level.
func labBase(labLevel int) []base.Item {
	items := standard()
	items[0] = it("hq", "b_hq", 4)
	if labLevel > 0 {
		items = append(items, it("lab", "b_laboratory", labLevel))
	}
	return items
}

func (e *env) startResearch(key, node string) (LabState, error) {
	return e.svc.StartResearch(context.Background(), e.player, key, node)
}

func (e *env) mustResearch(key, node string) LabState {
	e.t.Helper()
	st, err := e.startResearch(key, node)
	if err != nil {
		e.t.Fatalf("research %s: %v", node, err)
	}
	return st
}

func (e *env) lab() LabState {
	e.t.Helper()
	st, err := e.svc.Lab(context.Background(), e.player)
	if err != nil {
		e.t.Fatal(err)
	}
	return st
}

func levelOf(st State, id string) int {
	for _, c := range append(append([]Count(nil), st.Stock...), st.Spells...) {
		if c.ID == id {
			return c.Level
		}
	}
	return 0
}

func nodeOf(st LabState, id string) NodeView {
	for _, n := range st.Nodes {
		if n.ID == id {
			return n
		}
	}
	return NodeView{}
}

func TestResearchContent(t *testing.T) {
	c := loadContent(t)
	nd, ok := c.nodes["r_rifle_2"]
	if !ok || nd.Lab != 3 || nd.Fuel < 1 || nd.Hours < 1 || len(nd.Requires) != 1 {
		t.Fatalf("r_rifle_2: %+v", nd)
	}
	if !c.nodes["r_smoke_1"].Soon || c.LabBuilding == "" || c.LabRefund != 50 {
		t.Fatalf("soon/lab_building/refund: %+v %q %d", c.nodes["r_smoke_1"], c.LabBuilding, c.LabRefund)
	}
}

// F-LAB: the 9.2 table of army-lab.md.
func TestResearchLevelAndTime(t *testing.T) {
	e := newEnv(t, labBase(3))
	e.setStock("u_rifleman", 20)
	e.grant("fuel", 4000)
	nd := e.svc.cfg.Content.nodes["r_rifle_1"]

	st := e.mustResearch("k1", "r_rifle_1")
	if e.bal("fuel") != 4000-nd.Fuel || e.timers("research") != 1 {
		t.Fatalf("fuel %d timers %d", e.bal("fuel"), e.timers("research"))
	}
	if st.Active == nil || st.Active.NodeID != "r_rifle_1" || st.Active.FinishAt != t0.Add(time.Duration(nd.Hours)*time.Hour).Unix() {
		t.Fatalf("active: %+v", st.Active)
	}
	// One at a time.
	e.at(30 * 60)
	_, err := e.startResearch("k2", "r_brawler_1")
	wantErr(t, err, "conflict", "lab_busy")

	secs := int(nd.Hours * 3600)
	e.at(secs - 1)
	if lv := levelOf(e.get(), "u_rifleman"); lv != 1 {
		t.Fatalf("one second early: level %d", lv)
	}
	if st = e.lab(); st.Active == nil || len(st.Done) != 0 {
		t.Fatalf("still running: %+v", st)
	}
	e.at(secs)
	if lv := levelOf(e.get(), "u_rifleman"); lv != 2 {
		t.Fatalf("at finish: level %d", lv)
	}
	if st = e.lab(); st.Active != nil || len(st.Done) != 1 || e.timers("research") != 0 {
		t.Fatalf("settled: %+v timers %d", st, e.timers("research"))
	}
	// Time going back never takes a finished node back (monotone, persisted).
	e.at(60)
	if lv := levelOf(e.get(), "u_rifleman"); lv != 2 {
		t.Fatalf("clock moved back: level %d", lv)
	}
	// The lab is free again and the follow-up node is open.
	e.at(secs + 5)
	e.grant("fuel", 100000)
	st = e.mustResearch("k3", "r_rifle_2")
	if st.Active == nil || st.Active.NodeID != "r_rifle_2" {
		t.Fatalf("second research: %+v", st)
	}
	// Offline for months: both nodes are applied, level jumps 2 -> 4.
	e.at(secs + 90*24*3600)
	if lv := levelOf(e.get(), "u_rifleman"); lv != 4 {
		t.Fatalf("after r_rifle_2: level %d", lv)
	}
}

func TestResearchGates(t *testing.T) {
	e := newEnv(t, labBase(1))
	e.grant("fuel", 100000)

	_, err := e.startResearch("a", "nope")
	wantErr(t, err, "invalid", "no_node")
	_, err = e.startResearch("b", "r_smoke_1")
	wantErr(t, err, "invalid", "coming_soon")
	_, err = e.startResearch("c", "r_brawler_1") // lab 2
	ae := wantErr(t, err, "conflict", "lab_level")
	if ae.Details["need"] != 2 || ae.Details["have"] != 1 {
		t.Fatalf("details: %v", ae.Details)
	}
	if n := nodeOf(e.lab(), "r_brawler_1"); n.Available || n.Reason != "lab_level" {
		t.Fatalf("node view: %+v", n)
	}
	if n := nodeOf(e.lab(), "r_rifle_1"); !n.Available {
		t.Fatalf("r_rifle_1 should be available: %+v", n)
	}
	if e.bal("fuel") != 100000 || e.timers("research") != 0 {
		t.Fatal("refused requests must change nothing")
	}

	// requires: r_rifle_2 (lab 3) needs r_rifle_1 even when the lab is high enough.
	e2 := newEnv(t, labBase(3))
	e2.grant("fuel", 100000)
	_, err = e2.startResearch("d", "r_rifle_2")
	ae = wantErr(t, err, "conflict", "prerequisite")
	if m, _ := ae.Details["missing"].([]string); len(m) != 1 || m[0] != "r_rifle_1" {
		t.Fatalf("missing: %v", ae.Details)
	}
	e2.research("r_rifle_1")
	e2.mustResearch("e", "r_rifle_2")

	// Done nodes cannot be repeated.
	e3 := newEnv(t, labBase(3))
	e3.grant("fuel", 100000)
	e3.research("r_rifle_1")
	_, err = e3.startResearch("f", "r_rifle_1")
	wantErr(t, err, "invalid", "already_done")

	// No laboratory at all.
	e4 := newEnv(t, labBase(0))
	e4.grant("fuel", 100000)
	_, err = e4.startResearch("g", "r_rifle_1")
	ae = wantErr(t, err, "conflict", "lab_level")
	if ae.Details["have"] != 0 {
		t.Fatalf("details: %v", ae.Details)
	}
}

func TestResearchInsufficientFuelIsAtomic(t *testing.T) {
	e := newEnv(t, labBase(3))
	nd := e.svc.cfg.Content.nodes["r_rifle_1"]
	e.grant("fuel", nd.Fuel-1)
	_, err := e.startResearch("k", "r_rifle_1")
	ae := wantErr(t, err, "insufficient_resources", "insufficient_resources")
	if m, _ := ae.Details["missing"].(map[string]int64); m["fuel"] != 1 {
		t.Fatalf("details: %v", ae.Details)
	}
	if e.bal("fuel") != nd.Fuel-1 || e.timers("research") != 0 || e.lab().Active != nil {
		t.Fatal("a failed purchase must change nothing")
	}
	var rows int
	if err := testStore(t).Pool.QueryRow(context.Background(), `SELECT count(*) FROM research WHERE player_id = $1`, e.player).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("research rows %d (%v)", rows, err)
	}
	e.grant("fuel", 1)
	e.mustResearch("k", "r_rifle_1") // same key, now affordable
}

func TestResearchIdempotent(t *testing.T) {
	e := newEnv(t, labBase(3))
	e.grant("fuel", 100000)
	st1 := e.mustResearch("same", "r_rifle_1")
	fuel := e.bal("fuel")
	st2 := e.mustResearch("same", "r_rifle_1") // would be lab_busy without the replay check
	if e.bal("fuel") != fuel || e.timers("research") != 1 || st1.Active.FinishAt != st2.Active.FinishAt {
		t.Fatalf("replay changed things: fuel %d timers %d", e.bal("fuel"), e.timers("research"))
	}
	// A replay after the research finished is still silent.
	e.at(30 * 24 * 3600)
	if st := e.mustResearch("same", "r_rifle_1"); st.Active != nil || e.bal("fuel") != fuel {
		t.Fatalf("late replay: %+v fuel %d", st, e.bal("fuel"))
	}
	if _, err := e.startResearch("", "r_brawler_1"); err == nil {
		t.Fatal("an empty idempotency key must be refused")
	}
}

func TestResearchCancelRefundsHalf(t *testing.T) {
	e := newEnv(t, labBase(3))
	e.setStock("u_rifleman", 3)
	nd := e.svc.cfg.Content.nodes["r_rifle_1"]
	e.grant("fuel", 4000)
	_, err := e.svc.CancelResearch(context.Background(), e.player, "c0")
	wantErr(t, err, "conflict", "no_research")

	e.mustResearch("k", "r_rifle_1")
	e.at(600)
	st, err := e.svc.CancelResearch(context.Background(), e.player, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if want := 4000 - nd.Fuel + nd.Fuel*50/100; e.bal("fuel") != want {
		t.Fatalf("fuel %d, want %d", e.bal("fuel"), want)
	}
	if st.Active != nil || len(st.Done) != 0 || e.timers("research") != 0 {
		t.Fatalf("cancelled: %+v", st)
	}
	if lv := levelOf(e.get(), "u_rifleman"); lv != 1 {
		t.Fatalf("level %d after cancel", lv)
	}
	// Replay: no second refund, no error; the lab is free for another node.
	fuel := e.bal("fuel")
	if _, err := e.svc.CancelResearch(context.Background(), e.player, "c1"); err != nil || e.bal("fuel") != fuel {
		t.Fatalf("replay: %v fuel %d", err, e.bal("fuel"))
	}
	e.mustResearch("k2", "r_rifle_1")
	// A finished research cannot be cancelled (lazy completion comes first).
	e.at(600 + int(nd.Hours)*3600)
	_, err = e.svc.CancelResearch(context.Background(), e.player, "c2")
	wantErr(t, err, "conflict", "no_research")
	if lv := levelOf(e.get(), "u_rifleman"); lv != 2 {
		t.Fatalf("level %d", lv)
	}
}

func TestResearchSpeedup(t *testing.T) {
	e := newEnv(t, labBase(3))
	e.setStock("u_rifleman", 3)
	e.grant("fuel", 4000)
	nd := e.svc.cfg.Content.nodes["r_rifle_1"]
	_, err := e.svc.SpeedupResearch(context.Background(), e.player, "s0")
	wantErr(t, err, "conflict", "no_research")

	e.mustResearch("k", "r_rifle_1")
	e.at(30*60 + 1)
	left := nd.Hours*3600 - (30*60 + 1)
	gems := e.svc.cfg.Prices.SpeedupGems(left)
	_, err = e.svc.SpeedupResearch(context.Background(), e.player, "s1")
	wantErr(t, err, "insufficient_resources", "insufficient_resources")
	if e.lab().Active == nil {
		t.Fatal("a refused speed-up must leave the research running")
	}

	e.grant("gems", gems+7)
	st, err := e.svc.SpeedupResearch(context.Background(), e.player, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if e.bal("gems") != 7 || st.Active != nil || len(st.Done) != 1 || e.timers("research") != 0 {
		t.Fatalf("gems %d state %+v", e.bal("gems"), st)
	}
	if lv := levelOf(e.get(), "u_rifleman"); lv != 2 {
		t.Fatalf("level %d", lv)
	}
	// Replay charges nothing.
	if _, err := e.svc.SpeedupResearch(context.Background(), e.player, "s1"); err != nil || e.bal("gems") != 7 {
		t.Fatalf("replay: %v gems %d", err, e.bal("gems"))
	}
	// The lab is free: the next node starts right away.
	e.mustResearch("k2", "r_rifle_2")
}

func TestResearchPerkRaisesHousing(t *testing.T) {
	e := newEnv(t, append(labBase(5), it("camp3", "b_camp", 1)))
	e.grant("fuel", 100000)
	pre := e.get()
	before := pre.Pools["unit"]
	e.mustResearch("k", "r_camp_cap")
	if p := e.get().Pools["unit"]; p.Capacity != before.Capacity {
		t.Fatalf("capacity must not change before the research ends: %+v", p)
	}
	nd := e.svc.cfg.Content.nodes["r_camp_cap"]
	e.at(int(nd.Hours) * 3600)
	after := e.get()
	if after.Pools["unit"].Capacity != before.Base*(1000+10*nd.Value)/1000 || after.Pools["unit"].PerkPct != nd.Value {
		t.Fatalf("pool: %+v (base %d, perk %d)", after.Pools["unit"], before.Base, nd.Value)
	}
	if after.Pools["garage"] != pre.Pools["garage"] {
		t.Fatalf("the perk must not touch the garage: %+v", after.Pools["garage"])
	}
	if after.Perks["army_housing_pct"] != nd.Value {
		t.Fatalf("perks: %v", after.Perks)
	}
	if lab := e.lab(); lab.Perks["army_housing_pct"] != nd.Value {
		t.Fatalf("lab perks: %v", lab.Perks)
	}
	// More units fit now: fill up to the new cap with the free room the perk added.
	e.setStock("u_rifleman", before.Capacity)
	if _, err := e.train("t", "A", "u_rifleman", 1); err != nil {
		t.Fatalf("one more unit must fit: %v", err)
	}
}

func TestPerksForOtherModules(t *testing.T) {
	e := newEnv(t, labBase(6))
	e.research("r_loot_1")
	e.research("r_radar_scan")
	tx, err := testStore(t).Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	p, err := e.svc.Perks(context.Background(), tx, e.player)
	if err != nil {
		t.Fatal(err)
	}
	if p["raid_loot_pct"] != e.svc.cfg.Content.nodes["r_loot_1"].Value || p["scan_reveals_traps"] != 1 {
		t.Fatalf("perks: %v", p)
	}
}

func TestResearchSurvivesFormArmyAndTrain(t *testing.T) {
	// Army saves must not drop the research in flight.
	e := newEnv(t, labBase(3))
	e.grant("fuel", 100000)
	e.mustResearch("k", "r_rifle_1")
	e.mustTrain("t", "A", "u_rifleman", 1)
	if e.timers("research") != 1 || e.lab().Active == nil {
		t.Fatal("training must not touch the research timer")
	}
}

func TestLabHTTP(t *testing.T) {
	e := newEnv(t, labBase(3))
	e.grant("fuel", 5000)
	r := chi.NewRouter()
	Mount(r, Deps{Service: e.svc, PlayerID: func(context.Context) (uuid.UUID, bool) { return e.player, true },
		ContentVersion: func() string { return "cv" }, Now: func() time.Time { return e.now }})
	do := func(method, path, key, body string) (int, map[string]any) {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		var m map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &m)
		return rec.Code, m
	}
	if c, _ := do("POST", "/lab/research", "", `{"node_id":"r_rifle_1"}`); c != http.StatusBadRequest {
		t.Fatalf("no key: %d", c)
	}
	c, m := do("POST", "/lab/research", "k1", `{"node_id":"r_rifle_1"}`)
	if c != 200 || m["content_version"] != "cv" || m["active"] == nil || m["lab_level"] != float64(3) {
		t.Fatalf("research: %d %v", c, m)
	}
	c, m = do("POST", "/lab/research", "k2", `{"node_id":"r_brawler_1"}`)
	if d := m["error"].(map[string]any)["details"].(map[string]any); c != http.StatusConflict || d["reason"] != "lab_busy" {
		t.Fatalf("busy: %d %v", c, m)
	}
	if c, m = do("GET", "/lab", "", ""); c != 200 || len(m["nodes"].([]any)) == 0 {
		t.Fatalf("get: %d %v", c, m)
	}
	if c, m = do("POST", "/lab/cancel", "k3", `{}`); c != 200 || m["active"] != nil {
		t.Fatalf("cancel: %d %v", c, m)
	}
	c, m = do("POST", "/lab/speedup", "k4", `{}`)
	if c != http.StatusConflict {
		t.Fatalf("speedup without research: %d %v", c, m)
	}
}
