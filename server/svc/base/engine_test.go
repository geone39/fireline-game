package base

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"
)

// testPricing is a fixture: the numbers exist only here, content owns the real ones.
const testPricing = `
schema: 1
cost_growth: 2
time_growth: 2
speedup_gems: {factor: 4, exponent: 0.8}
cancel_refund_pct: 50
buildings:
  b_hq:           {cost: {cash: 1000}, time_s: 3600}
  b_cash_mine:    {cost: {cash: 100}, time_s: 60}
  b_fuel_refinery: {cost: {cash: 100, fuel: 10}, time_s: 120}
  b_vault:        {cost: {cash: 50}, time_s: 30}
  d_wall:         {cost: {cash: 10}, time_s: 10, max_level: 3}
`

var t0 = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func testPrices(t *testing.T) *PriceBook {
	t.Helper()
	econ, err := os.ReadFile("../../../content/economy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	pb, err := ParsePriceBook([]byte(testPricing), econ)
	if err != nil {
		t.Fatalf("ParsePriceBook: %v", err)
	}
	return pb
}

func testEngine(t *testing.T) Engine {
	n := 0
	return Engine{Rules: rules(t), Prices: testPrices(t), NewID: func() string { n++; return fmt.Sprintf("timer-%d", n) }}
}

func plan(t *testing.T, e Engine, w World, c Command, now time.Time) (World, Effect, Result) {
	t.Helper()
	w2, eff, res, err := e.Plan(w, c, now)
	if err != nil {
		t.Fatalf("%+v: %v", c, err)
	}
	return w2, eff, res
}

func deltaOf(eff Effect, res string) int64 {
	var n int64
	for _, d := range eff.Deltas {
		if d.Resource == res {
			n += d.Amount
		}
	}
	return n
}

func TestPriceBookFormulas(t *testing.T) {
	p := testPrices(t)
	if got := p.Cost("b_cash_mine", 3)["cash"]; got != 400 { // 100 * 2^2
		t.Errorf("cost L3 = %d", got)
	}
	if got := p.Seconds("b_cash_mine", 3); got != 240 {
		t.Errorf("time L3 = %d", got)
	}
	// ceil(hours^0.8 * 4): 1h -> 4; 10h -> ceil(6.3096*4)=26; tiny remainder -> at least 1
	for secs, want := range map[int64]int64{3600: 4, 36000: 26, 1: 1, 0: 0, -5: 0} {
		if got := p.SpeedupGems(secs); got != want {
			t.Errorf("SpeedupGems(%d) = %d, want %d", secs, got, want)
		}
	}
	if r := p.Refund(map[string]int64{"cash": 101, "fuel": 1}); r["cash"] != 50 || len(r) != 1 {
		t.Errorf("refund = %v", r)
	}
	for hq, want := range map[int]int{1: 2, 3: 3, 5: 4, 7: 5, 10: 5} { // content/economy.yaml builders
		if got := p.Builders(hq); got != want {
			t.Errorf("builders at HQ %d = %d, want %d", hq, got, want)
		}
	}
}

func TestPriceBookRejectsBadFiles(t *testing.T) {
	econ, _ := os.ReadFile("../../../content/economy.yaml")
	for name, y := range map[string]string{
		"no speedup": "buildings: {b_hq: {cost: {cash: 1}, time_s: 1, cost_growth: 1, time_growth: 1}}",
		"no time":    "speedup_gems: {factor: 1, exponent: 1}\nbuildings: {b_hq: {cost: {cash: 1}, cost_growth: 1, time_growth: 1}}",
		"growth < 1": "speedup_gems: {factor: 1, exponent: 1}\ncost_growth: 0.5\ntime_growth: 1\nbuildings: {b_hq: {cost: {cash: 1}, time_s: 1}}",
		"bad refund": "speedup_gems: {factor: 1, exponent: 1}\ncancel_refund_pct: 150",
	} {
		if _, err := ParsePriceBook([]byte(y), econ); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestSettleForwardBackwardIdempotent(t *testing.T) {
	l := Layout{Items: []Item{hqItem(1, 20, 20), {UID: "m", Type: "b_cash_mine", Level: 1, X: 2, Y: 2}}}
	ts := []Timer{
		{ID: "a", Kind: TimerUpgrade, Target: "m", ToLevel: 2, FinishAt: t0.Add(time.Minute)},
		{ID: "b", Kind: TimerBuild, Target: "hq", ToLevel: 1, FinishAt: t0.Add(2 * time.Minute)},
		{ID: "c", Kind: TimerUpgrade, Target: "hq", ToLevel: 2, FinishAt: t0.Add(time.Hour)},
	}
	// before anything is due (clock earlier than every finish_at): untouched
	l1, p1, d1 := Settle(l, ts, t0)
	if len(d1) != 0 || len(p1) != 3 || !reflect.DeepEqual(l1, l) {
		t.Fatalf("t0: %v %v %v", l1, p1, d1)
	}
	// forward past the first two timers
	l2, p2, d2 := Settle(l, ts, t0.Add(5*time.Minute))
	if len(d2) != 2 || d2[0].ID != "a" || d2[1].ID != "b" || len(p2) != 1 || l2.Items[1].Level != 2 || l2.Items[0].Level != 1 {
		t.Fatalf("t+5m: %+v %+v %+v", l2, p2, d2)
	}
	if l.Items[1].Level != 1 {
		t.Fatal("Settle modified its input layout")
	}
	// settling again at the same time changes nothing
	l3, p3, d3 := Settle(l2, p2, t0.Add(5*time.Minute))
	if len(d3) != 0 || !reflect.DeepEqual(l3, l2) || !reflect.DeepEqual(p3, p2) {
		t.Fatalf("second settle: %+v %+v %+v", l3, p3, d3)
	}
	// the clock goes backwards: nothing new completes, nothing is undone
	l4, p4, d4 := Settle(l2, p2, t0.Add(-time.Hour))
	if len(d4) != 0 || !reflect.DeepEqual(l4, l2) || !reflect.DeepEqual(p4, p2) {
		t.Fatalf("backwards: %+v %+v %+v", l4, p4, d4)
	}
	// ...and forward again to the end
	l5, p5, _ := Settle(l4, p4, t0.Add(2*time.Hour))
	if len(p5) != 0 || l5.Items[0].Level != 2 {
		t.Fatalf("end: %+v %+v", l5, p5)
	}
	// a timer whose target vanished is dropped without effect
	l6, p6, d6 := Settle(l, []Timer{{ID: "x", Kind: TimerUpgrade, Target: "gone", ToLevel: 2, FinishAt: t0}}, t0)
	if len(d6) != 1 || len(p6) != 0 || !reflect.DeepEqual(l6, l) {
		t.Fatalf("orphan: %+v %+v %+v", l6, p6, d6)
	}
}

func TestSettleOrderIsDeterministic(t *testing.T) {
	l := Layout{Items: []Item{{UID: "m", Type: "b_cash_mine", Level: 1}}}
	same := t0.Add(time.Minute)
	a := []Timer{{ID: "2", Kind: TimerUpgrade, Target: "m", ToLevel: 3, FinishAt: same}, {ID: "1", Kind: TimerUpgrade, Target: "m", ToLevel: 2, FinishAt: same}}
	b := []Timer{a[1], a[0]}
	_, _, da := Settle(l, a, same)
	_, _, db := Settle(l, b, same)
	if !reflect.DeepEqual(da, db) || da[0].ID != "1" {
		t.Fatalf("order: %v vs %v", da, db)
	}
}

func TestBuildChargesAndStartsTimer(t *testing.T) {
	e := testEngine(t)
	w := World{Layout: base0(1)}
	w2, eff, res := plan(t, e, w, build("m1", "b_fuel_refinery", 2, 2), t0)
	if res.Replayed || deltaOf(eff, "cash") != -100 || deltaOf(eff, "fuel") != -10 || eff.Reason != "build" {
		t.Fatalf("effect: %+v %+v", eff, res)
	}
	if len(w2.Timers) != 1 || !w2.Timers[0].FinishAt.Equal(t0.Add(120*time.Second)) || w2.Timers[0].Kind != TimerBuild {
		t.Fatalf("timers: %+v", w2.Timers)
	}
	if w2.Layout.Find("m1") < 0 || len(w.Layout.Items) != 1 || len(w.Timers) != 0 {
		t.Fatal("layout not updated or input modified")
	}
	// the same command again is a no-op: no charge, no second timer
	w3, eff3, res3 := plan(t, e, w2, build("m1", "b_fuel_refinery", 2, 2), t0)
	if !res3.Replayed || len(eff3.Deltas) != 0 || len(w3.Timers) != 1 {
		t.Fatalf("replay: %+v %+v %+v", w3, eff3, res3)
	}
}

func TestBuildNeedsPriceAndFreeBuilder(t *testing.T) {
	e := testEngine(t)
	w := World{Layout: base0(1)}
	_, _, _, err := e.Plan(w, build("s", "x_flag", 2, 2), t0) // decor has no price
	wantCode(t, err, CodeNoPrice)
	w, _, _ = plan(t, e, w, build("m1", "b_cash_mine", 2, 2), t0)
	w, _, _ = plan(t, e, w, build("m2", "b_cash_mine", 6, 2), t0)
	_, _, _, err = e.Plan(w, build("v", "b_vault", 10, 2), t0) // HQ 1 has 2 builders
	wantCode(t, err, CodeNoBuilder)
	// once one finishes the builder is free again
	l, pending, _ := Settle(w.Layout, w.Timers, t0.Add(time.Hour))
	w = World{l, pending}
	plan(t, e, w, build("v", "b_vault", 10, 2), t0.Add(time.Hour))
}

func TestUpgradeFlow(t *testing.T) {
	e := testEngine(t)
	w := World{Layout: Layout{Items: []Item{hqItem(1, 20, 20), {UID: "m", Type: "b_cash_mine", Level: 1, X: 2, Y: 2}}}}
	w, eff, _ := plan(t, e, w, Command{Op: OpUpgrade, UID: "m", To: 2}, t0)
	if deltaOf(eff, "cash") != -200 || w.Timers[0].ToLevel != 2 || !w.Timers[0].FinishAt.Equal(t0.Add(120*time.Second)) {
		t.Fatalf("upgrade: %+v %+v", eff, w.Timers)
	}
	if w.Layout.Items[1].Level != 1 {
		t.Fatal("level must change only when the timer fires")
	}
	// retry with the explicit target level is a no-op, without it the item is busy
	_, eff, res := plan(t, e, w, Command{Op: OpUpgrade, UID: "m", To: 2}, t0)
	if !res.Replayed || len(eff.Deltas) != 0 {
		t.Fatalf("retry: %+v %+v", eff, res)
	}
	_, _, _, err := e.Plan(w, Command{Op: OpUpgrade, UID: "m"}, t0)
	wantCode(t, err, CodeBusy)
	_, _, _, err = e.Plan(w, Command{Op: OpUpgrade, UID: "nope"}, t0)
	wantCode(t, err, CodeNotFound)
	_, _, _, err = e.Plan(w, Command{Op: OpUpgrade, UID: "m", To: 5}, t0)
	wantCode(t, err, CodeBadCommand)
	// after completion, To=2 is already satisfied
	l, pending, _ := Settle(w.Layout, w.Timers, t0.Add(time.Hour))
	w = World{l, pending}
	_, _, res = plan(t, e, w, Command{Op: OpUpgrade, UID: "m", To: 2}, t0.Add(time.Hour))
	if !res.Replayed {
		t.Fatal("completed upgrade must replay")
	}
}

func TestUpgradeMaxLevelAndNoPrice(t *testing.T) {
	e := testEngine(t)
	mine, _ := e.Rules.Cat.Def("b_cash_mine")
	w := World{Layout: Layout{Items: []Item{hqItem(1, 20, 20), {UID: "m", Type: "b_cash_mine", Level: mine.Levels, X: 2, Y: 2}, {UID: "f", Type: "x_flag", Level: 1, X: 9, Y: 9}}}}
	_, _, _, err := e.Plan(w, Command{Op: OpUpgrade, UID: "m"}, t0)
	wantCode(t, err, CodeMaxLevel)
	_, _, _, err = e.Plan(w, Command{Op: OpUpgrade, UID: "f"}, t0)
	wantCode(t, err, CodeNoPrice)
}

func TestHQUpgradeGate(t *testing.T) {
	e := testEngine(t)
	w := World{Layout: Layout{Items: []Item{hqItem(3, 20, 20), {UID: "m", Type: "b_cash_mine", Level: 1, X: 2, Y: 2}}}}
	_, _, _, err := e.Plan(w, Command{Op: OpUpgrade, UID: "hq"}, t0) // mine L1 < HQ 3 - 1
	wantCode(t, err, CodeHQGate)
	w.Layout.Items[1].Level = 2
	plan(t, e, w, Command{Op: OpUpgrade, UID: "hq"}, t0)
}

func TestRemoveAndCancelRefund(t *testing.T) {
	e := testEngine(t)
	w := World{Layout: base0(1)}
	w, _, _ = plan(t, e, w, build("m", "b_cash_mine", 2, 2), t0)
	// cancel construction: item gone, timer gone, 50% of 100 back
	w2, eff, _ := plan(t, e, w, Command{Op: OpRemove, UID: "m"}, t0)
	if w2.Layout.Find("m") >= 0 || len(w2.Timers) != 0 || deltaOf(eff, "cash") != 50 || eff.Reason != "cancel_refund" {
		t.Fatalf("cancel: %+v %+v", w2, eff)
	}
	// a finished building is removed without refund
	l, pending, _ := Settle(w.Layout, w.Timers, t0.Add(time.Hour))
	w3, eff, _ := plan(t, e, World{l, pending}, Command{Op: OpRemove, UID: "m"}, t0.Add(time.Hour))
	if w3.Layout.Find("m") >= 0 || len(eff.Deltas) != 0 {
		t.Fatalf("remove: %+v %+v", w3, eff)
	}
	// already gone: no-op
	_, _, res := plan(t, e, w3, Command{Op: OpRemove, UID: "m"}, t0)
	if !res.Replayed {
		t.Fatal("remove of a missing uid must replay")
	}
	_, _, _, err := e.Plan(w, Command{Op: OpRemove, UID: "hq"}, t0)
	wantCode(t, err, CodeNotPlaceable)
}

func TestSpeedup(t *testing.T) {
	e := testEngine(t)
	w := World{Layout: base0(1)}
	w, _, _ = plan(t, e, w, Command{Op: OpUpgrade, UID: "hq"}, t0) // 2000 cash, 7200 s
	now := t0.Add(time.Hour)                                       // 7200s total, 1h left -> ceil(1^0.8*4) = 4 gems
	w2, eff, _ := plan(t, e, w, Command{Op: OpSpeedup, UID: "hq"}, now)
	if deltaOf(eff, "gems") != -4 || len(w2.Timers) != 0 || w2.Layout.Items[0].Level != 2 {
		t.Fatalf("speedup: %+v %+v", w2, eff)
	}
	// nothing running any more: replay, no charge
	_, eff, res := plan(t, e, w2, Command{Op: OpSpeedup, UID: "hq"}, now)
	if !res.Replayed || len(eff.Deltas) != 0 {
		t.Fatalf("second speedup: %+v %+v", eff, res)
	}
	_, _, _, err := e.Plan(w2, Command{Op: OpSpeedup, UID: "nope"}, now)
	wantCode(t, err, CodeNotFound)
}

func TestClearObstacle(t *testing.T) {
	e := testEngine(t)
	rock, _ := e.Rules.Cat.Def("o_rock")
	w := World{Layout: Layout{Items: []Item{hqItem(1, 20, 20), {UID: "r", Type: "o_rock", Level: 1, X: 2, Y: 2}}}}
	w2, eff, _ := plan(t, e, w, Command{Op: OpClearObstacle, UID: "r"}, t0)
	if w2.Layout.Find("r") >= 0 || deltaOf(eff, "cash") != -rock.ClearCost["cash"] || deltaOf(eff, "gems") != rock.Reward["gems"] {
		t.Fatalf("clear: %+v %+v (rock %+v)", w2, eff, rock)
	}
	if rock.ClearCost["cash"] == 0 || rock.Reward["gems"] == 0 {
		t.Fatal("content must price o_rock")
	}
	_, _, res := plan(t, e, w2, Command{Op: OpClearObstacle, UID: "r"}, t0)
	if !res.Replayed {
		t.Fatal("clearing a cleared obstacle must replay")
	}
	_, _, _, err := e.Plan(w, Command{Op: OpClearObstacle, UID: "hq"}, t0)
	wantCode(t, err, CodeNotPlaceable)
}

func TestCollectEffectAndUnknownOp(t *testing.T) {
	e := testEngine(t)
	_, eff, _ := plan(t, e, World{Layout: base0(1)}, Command{Op: OpCollect}, t0)
	if !eff.Collect {
		t.Fatal("collect must request a pickup")
	}
	_, _, _, err := e.Plan(World{Layout: base0(1)}, Command{Op: "bogus"}, t0)
	wantCode(t, err, CodeBadCommand)
}

func TestMoveDuringConstruction(t *testing.T) {
	e := testEngine(t)
	w, _, _ := plan(t, e, World{Layout: base0(1)}, build("m", "b_cash_mine", 2, 2), t0)
	w2, eff, _ := plan(t, e, w, Command{Op: OpMove, UID: "m", X: 8, Y: 8}, t0)
	if len(eff.Deltas) != 0 || w2.Layout.Items[w2.Layout.Find("m")].X != 8 || len(w2.Timers) != 1 {
		t.Fatalf("move: %+v", w2)
	}
}

func TestCommandJSONShapes(t *testing.T) {
	var flat, nested []Command
	if err := json.Unmarshal([]byte(`[{"op":"build","uid":"a","type":"b_vault","x":3,"y":4,"rot":1}]`), &flat); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`[{"op":"build","args":{"uid":"a","type":"b_vault","x":3,"y":4,"rot":1}}]`), &nested); err != nil {
		t.Fatal(err)
	}
	want := Command{Op: OpBuild, UID: "a", Type: "b_vault", X: 3, Y: 4, Rot: 1}
	if flat[0] != want || nested[0] != want {
		t.Fatalf("flat %+v nested %+v", flat, nested)
	}
}
