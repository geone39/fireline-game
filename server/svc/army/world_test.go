package army

import (
	"testing"
	"time"

	"github.com/geone39/fireline/server/svc/base"
)

func testWorld(t *testing.T, hq int, items ...base.Item) *world {
	t.Helper()
	w := &world{c: loadContent(t), hq: hq, built: map[string]base.Item{}, pending: map[string]base.Item{},
		stock: map[string]int{}, done: map[string]bool{}}
	for _, i := range items {
		w.built[i.UID] = i
	}
	return w
}

func TestContentFromYAML(t *testing.T) {
	c := loadContent(t)
	u, ok := c.Unit("u_brawler")
	if !ok || u.Pool != "unit" || u.Producer != "b_barracks" || u.NeedBuilding != 2 || u.NeedHQ != 2 || u.Housing != 4 {
		t.Fatalf("u_brawler: %+v", u)
	}
	if u, _ := c.Unit("v_humvee"); u.Pool != "garage" || u.Producer != "b_factory" || u.Cost["cash"] != 400 {
		t.Fatalf("v_humvee: %+v", u)
	}
	if u, _ := c.Unit("s_gunboat"); u.Pool != "garage" || u.Producer != "b_shipyard" {
		t.Fatalf("s_gunboat: %+v", u)
	}
	if u, _ := c.Unit("c_smoke"); !u.Spell || u.Pool != "spell" || u.Producer != "b_support_bay" || u.TrainS == 0 {
		t.Fatalf("c_smoke: %+v", u)
	}
	if u, _ := c.Unit("u_paratrooper"); u.Group != 4 {
		t.Fatalf("u_paratrooper: %+v", u)
	}
	if c.MaxOrders < 1 || c.RefundUnstart != 100 || c.RefundStarted != 50 || c.PerkKey != "army_housing_pct" {
		t.Fatalf("army.yaml: %+v", c)
	}
}

func TestGroupMultiple(t *testing.T) {
	w := testWorld(t, 8, base.Item{UID: "A", Type: "b_barracks", Level: 8}, base.Item{UID: "c", Type: "b_camp", Level: 10})
	if _, err := w.planTrain(Request{"A", "u_paratrooper", 3}, t0); err == nil || err.Reason != "not_multiple_of_group" {
		t.Fatalf("3 paratroopers: %v", err)
	}
	o, err := w.planTrain(Request{"A", "u_paratrooper", 4}, t0)
	if err != nil || o.Remaining != 4 {
		t.Fatalf("4 paratroopers: %v", err)
	}
	// 70 housing, 5 per trooper: max_count is rounded down to the group size.
	w.stock["u_rifleman"] = 55 // 15 free -> 3 troopers fit, but only whole groups count
	_, err = w.planTrain(Request{"A", "u_paratrooper", 4}, t0)
	if err == nil || err.Reason != "housing_full" || err.Details["max_count"] != 0 {
		t.Fatalf("housing_full: %+v", err)
	}
}

func TestSettleAndRechain(t *testing.T) {
	w := testWorld(t, 3, base.Item{UID: "A", Type: "b_barracks", Level: 2}, base.Item{UID: "c", Type: "b_camp", Level: 10})
	add := func(unit string, n int) order {
		o, err := w.planTrain(Request{"A", unit, n}, t0)
		if err != nil {
			t.Fatal(err)
		}
		o.ID = unit
		w.orders = append(w.orders, o)
		return o
	}
	add("u_brawler", 2)  // 45 s each
	add("u_rifleman", 3) // 12 s each, starts at +90
	if got := w.queueEnd("A"); !got.Equal(t0.Add(90*time.Second + 36*time.Second)) {
		t.Fatalf("queue end %v", got)
	}
	// Cancel the whole head after 50 s: the rifleman order moves up to start now.
	now := t0.Add(50 * time.Second)
	w.settle(now)
	if w.stock["u_brawler"] != 1 {
		t.Fatalf("stock %v", w.stock)
	}
	head := w.orders[w.orderIndex("u_brawler")]
	if !w.started(head, now) {
		t.Fatal("the head must be in progress")
	}
	p := w.planCancel(head, 1, now)
	if p.started != 1 || p.refund["fuel"] != 55 {
		t.Fatalf("plan: %+v", p)
	}
	w.orders = w.orders[:0:0]
	w.orders = append(w.orders, order{ID: "u_rifleman", Building: "A", Unit: "u_rifleman", Count: 3, Remaining: 3, NextAt: t0.Add(90 * time.Second), Seq: 1, Cost: map[string]int64{"fuel": 25}})
	w.rechain("A", false, now)
	if got := w.orders[0].NextAt; !got.Equal(now) {
		t.Fatalf("new head starts at %v", got)
	}
	// Partial cancel of an unstarted tail: the refund is the full price, the head is kept.
	o := w.orders[0]
	if p := w.planCancel(o, 1, now); p.started != 0 || p.unstarted != 1 || p.refund["fuel"] != 25 {
		t.Fatalf("plan: %+v", p)
	}
}
