package army

import (
	"fmt"
	"sort"
	"time"

	"github.com/geone39/fireline/server/svc/base"
)

// Error is a rule violation. Kind is the API code of docs/03-api.md (invalid, not_found,
// conflict, insufficient_resources); Reason goes to error.details.reason.
type Error struct {
	Kind    string
	Reason  string
	Msg     string
	Details map[string]any
}

func (e *Error) Error() string { return fmt.Sprintf("army: %s/%s: %s", e.Kind, e.Reason, e.Msg) }

func fail(kind, reason, format string, a ...any) *Error {
	return &Error{Kind: kind, Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

func (e *Error) with(k string, v any) *Error {
	if e.Details == nil {
		e.Details = map[string]any{}
	}
	e.Details[k] = v
	return e
}

// order is one queued training order. Units are produced one by one: the unit in
// progress started at NextAt and finishes at NextAt+train_s; the next unit (and the next
// order of the building) starts at that instant, so offline progress is exact.
type order struct {
	ID        string
	Building  string // uid of the producing building
	Unit      string
	Count     int
	Remaining int
	NextAt    time.Time
	Seq       int64
	Cost      map[string]int64 // per unit, as paid
}

func (o order) finish(trainS int64) time.Time {
	return o.NextAt.Add(time.Duration(int64(o.Remaining)*trainS) * time.Second)
}

// deployment is the escrow of one raid: units and spells taken out of the stock.
type deployment struct {
	RaidID  string
	Expires time.Time
	Units   []Slot
	Spells  []Slot
}

// Slot is a unit/spell type with its level and count; Level is frozen when the army is
// formed (army-lab.md §6.2).
type Slot struct {
	ID    string `json:"id"`
	Level int    `json:"level"`
	Count int    `json:"count"`
}

// world is the player's army-relevant state as of one moment.
type world struct {
	c         *Content
	hq        int
	built     map[string]base.Item // built items by uid, at their current level
	pending   map[string]base.Item // items still under their first construction
	stock     map[string]int       // units and spells at home
	done      map[string]bool      // finished research nodes
	orders    []order              // sorted by (building, seq)
	deployed  []deployment
	lab       *activeResearch // research in flight, nil if the lab is idle
	newlyDone []string        // nodes that became done and are not yet stored as such
}

// level is the research-derived level of a unit or spell (army-lab.md §2.1).
func (w *world) level(id string) int {
	lv := 1
	for n := range w.done {
		if nd := w.c.nodes[n]; nd.Item == id && nd.ToLevel > lv {
			lv = nd.ToLevel
		}
	}
	return lv
}

func (w *world) perkPct() int {
	t := 0
	for n := range w.done {
		if nd := w.c.nodes[n]; nd.Perk != "" && nd.Perk == w.c.PerkKey {
			t += nd.Value
		}
	}
	return t
}

// Perks returns the summed perks of the finished research (bool perks as 0/1).
func (w *world) perks() map[string]int {
	out := map[string]int{}
	for n := range w.done {
		if nd := w.c.nodes[n]; nd.Perk != "" {
			out[nd.Perk] += nd.Value
		}
	}
	return out
}

func (w *world) baseCap(pool string) int {
	b := w.c.buildingByPool[pool]
	total := 0
	for _, it := range w.built {
		if it.Type != b {
			continue
		}
		caps := w.c.capacity[b]
		lv := min(max(it.Level, 1), len(caps))
		total += caps[lv-1]
	}
	return total
}

// cap applies the housing perk once, rounding down (army-lab.md §3.2).
func (w *world) cap(pool string) int {
	b := w.baseCap(pool)
	if !w.c.perkPools[pool] {
		return b
	}
	return b * (1000 + 10*w.perkPct()) / 1000
}

// used counts home stock, queued units and deployed units (army-lab.md §3.3).
func (w *world) used(pool string) int {
	n := 0
	add := func(id string, cnt int) {
		if u, ok := w.c.units[id]; ok && u.Pool == pool {
			n += u.Housing * cnt
		}
	}
	for id, cnt := range w.stock {
		add(id, cnt)
	}
	for _, o := range w.orders {
		add(o.Unit, o.Remaining)
	}
	for _, d := range w.deployed {
		for _, s := range d.Units {
			add(s.ID, s.Count)
		}
		for _, s := range d.Spells {
			add(s.ID, s.Count)
		}
	}
	return n
}

// queue returns the building's orders in FIFO order.
func (w *world) queue(building string) []order {
	var q []order
	for _, o := range w.orders {
		if o.Building == building {
			q = append(q, o)
		}
	}
	sort.Slice(q, func(i, j int) bool { return q[i].Seq < q[j].Seq })
	return q
}

func (w *world) sortOrders() {
	sort.Slice(w.orders, func(i, j int) bool {
		a, b := w.orders[i], w.orders[j]
		if a.Building != b.Building {
			return a.Building < b.Building
		}
		return a.Seq < b.Seq
	})
}

func (w *world) buildings() []string {
	seen := map[string]bool{}
	var bs []string
	for _, o := range w.orders {
		if !seen[o.Building] {
			seen[o.Building] = true
			bs = append(bs, o.Building)
		}
	}
	sort.Strings(bs)
	return bs
}

// settle delivers every unit whose training finished by now into the stock. It is
// idempotent and monotone: a smaller now delivers less, nothing is ever taken back.
// It returns the changed and the finished (removed) orders.
func (w *world) settle(now time.Time) (changed []order, removed []string) {
	out := w.orders[:0:0]
	for _, o := range w.orders {
		u := w.c.units[o.Unit]
		if k := int(min(int64(o.Remaining), elapsed(o.NextAt, now)/u.TrainS)); k > 0 && o.Remaining > 0 {
			w.stock[o.Unit] += k
			o.Remaining -= k
			o.NextAt = o.NextAt.Add(time.Duration(int64(k)*u.TrainS) * time.Second)
			if o.Remaining == 0 {
				removed = append(removed, o.ID)
				continue
			}
			changed = append(changed, o)
		}
		out = append(out, o)
	}
	w.orders = out
	return changed, removed
}

func elapsed(from, to time.Time) int64 {
	if to.Before(from) {
		return 0
	}
	return int64(to.Sub(from) / time.Second)
}

// started reports whether the head unit of the order is in progress right now.
func (w *world) started(o order, now time.Time) bool {
	q := w.queue(o.Building)
	return len(q) > 0 && q[0].ID == o.ID && o.Remaining > 0 && !o.NextAt.After(now)
}

// queueEnd is when the whole queue of the building is done (zero time if empty).
func (w *world) queueEnd(building string) time.Time {
	var end time.Time
	for _, o := range w.queue(building) {
		if f := o.finish(w.c.units[o.Unit].TrainS); f.After(end) {
			end = f
		}
	}
	return end
}

// Request is a TrainCommand (POST /v1/army/train).
type Request struct {
	BuildingID string `json:"building_id"`
	UnitID     string `json:"unit_id"`
	Count      int    `json:"count"`
}

// planTrain runs the checks of army-lab.md §4.3 steps 1-5 (nothing is paid yet) and
// returns the order that would be queued.
func (w *world) planTrain(req Request, now time.Time) (order, *Error) {
	u, ok := w.c.units[req.UnitID]
	if !ok || req.Count < 1 {
		return order{}, fail("invalid", "bad_request", "unknown unit %q or non-positive count", req.UnitID)
	}
	it, ok := w.itemAnyState(req.BuildingID)
	if !ok {
		return order{}, fail("not_found", "no_building", "building %q not found", req.BuildingID)
	}
	if it.Type != u.Producer {
		return order{}, fail("invalid", "wrong_building", "%s cannot be trained in %s", u.ID, it.Type)
	}
	b, ok := w.built[req.BuildingID]
	if !ok {
		return order{}, fail("invalid", "not_built", "building %q is still under construction", req.BuildingID)
	}
	if b.Level < u.NeedBuilding || w.hq < u.NeedHQ {
		need := map[string]int{"hq": u.NeedHQ}
		if u.NeedBuilding > 0 {
			need[u.Producer] = u.NeedBuilding
		}
		return order{}, fail("conflict", "locked", "%s is locked", u.ID).with("need", need)
	}
	if u.Group > 1 && req.Count%u.Group != 0 {
		return order{}, fail("invalid", "not_multiple_of_group", "count must be a multiple of %d", u.Group)
	}
	q := w.queue(req.BuildingID)
	if len(q) >= w.c.MaxOrders {
		return order{}, fail("conflict", "queue_full", "queue holds at most %d orders", w.c.MaxOrders)
	}
	capacity, used := w.cap(u.Pool), w.used(u.Pool)
	if need := req.Count * u.Housing; need > capacity-used {
		free := max(capacity-used, 0)
		maxCount := free / u.Housing
		if u.Group > 1 {
			maxCount -= maxCount % u.Group
		}
		return order{}, fail("conflict", "housing_full", "not enough room in pool %s", u.Pool).
			with("pool", u.Pool).with("capacity", capacity).with("used", used).with("free", free).
			with("need", need).with("max_count", maxCount)
	}
	start := now
	if len(q) > 0 {
		last := q[len(q)-1]
		start = last.finish(w.c.units[last.Unit].TrainS)
	}
	var seq int64
	for _, o := range q {
		seq = max(seq, o.Seq+1)
	}
	return order{Building: req.BuildingID, Unit: u.ID, Count: req.Count, Remaining: req.Count, NextAt: start, Seq: seq, Cost: u.Cost}, nil
}

// itemAnyState finds a building by uid whether it is built or still under construction.
func (w *world) itemAnyState(uid string) (base.Item, bool) {
	if it, ok := w.built[uid]; ok {
		return it, true
	}
	it, ok := w.pending[uid]
	return it, ok
}

// cancelPlan computes what cancelling n units from the tail of order o costs and refunds.
type cancelPlan struct {
	unstarted, started int
	refund             map[string]int64
}

func (w *world) planCancel(o order, n int, now time.Time) cancelPlan {
	p := cancelPlan{}
	if n == o.Remaining && w.started(o, now) {
		p.started = 1
	}
	p.unstarted = n - p.started
	p.refund = map[string]int64{}
	for r, c := range o.Cost {
		v := (int64(p.unstarted)*w.c.RefundUnstart + int64(p.started)*w.c.RefundStarted) * c / 100
		if v > 0 {
			p.refund[r] = v
		}
	}
	return p
}

// rechain restarts the building's queue after an order shrank or vanished: orders keep
// their sequence and each starts when the previous one ends. A surviving head keeps its
// running unit (keepHead); otherwise the new head starts at headAt.
func (w *world) rechain(building string, keepHead bool, headAt time.Time) {
	idx := map[string]int{}
	for i, o := range w.orders {
		idx[o.ID] = i
	}
	var cursor time.Time
	for n, o := range w.queue(building) {
		i := idx[o.ID]
		switch {
		case n > 0:
			w.orders[i].NextAt = cursor
		case !keepHead:
			w.orders[i].NextAt = headAt
		}
		cursor = w.orders[i].finish(w.c.units[o.Unit].TrainS)
	}
}
