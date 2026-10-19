package base

import (
	"time"

	"github.com/geone39/fireline/server/svc/economy"
)

// World is the mutable part of a base: the layout and its running timers.
type World struct {
	Layout Layout
	Timers []Timer
}

// Effect is what a command asks the caller to do outside the World: a wallet change
// (to be applied through economy.Ledger, in the same transaction) or a collector pickup.
type Effect struct {
	Deltas  []economy.Delta
	Reason  string
	Collect bool
}

// Engine evaluates construction commands against the content. It is pure: no clock, no
// DB, no randomness except NewID, which the service sets to uuid.NewString.
type Engine struct {
	Rules  Rules
	Prices *PriceBook
	NewID  func() string
}

// Plan applies one command to an already settled world at time now. On error the world
// is returned unchanged. Result.Replayed means the command was already in effect.
//
// ASSUMPTION: remove on an item that is under construction cancels it (refund of
// cancel_refund_pct of what was paid) and deletes the item; removing a finished item
// refunds nothing. remove/speedup/clear_obstacle on an uid that is gone or has nothing
// left to do are no-ops (Replayed), so a retry after a lost response is harmless.
// ASSUMPTION: upgrading the HQ from level L needs every KindBuilding item at level >= L-1
// (the "key buildings" gate of docs/design/base-building.md; the doc does not list them).
func (e Engine) Plan(w World, c Command, now time.Time) (World, Effect, Result, error) {
	switch c.Op {
	case OpBuild:
		return e.build(w, c, now)
	case OpMove:
		l, res, err := e.Rules.Apply(w.Layout, c)
		if err != nil {
			return w, Effect{}, Result{}, err
		}
		return World{l, w.Timers}, Effect{}, res, nil
	case OpUpgrade:
		return e.upgrade(w, c, now)
	case OpRemove:
		return e.remove(w, c)
	case OpSpeedup:
		return e.speedup(w, c, now)
	case OpClearObstacle:
		return e.clearObstacle(w, c)
	case OpCollect:
		return w, Effect{Collect: true, Reason: "collect"}, Result{}, nil
	}
	return w, Effect{}, Result{}, fail(CodeBadCommand, c.UID, "unknown op %q", c.Op)
}

func (e Engine) freeBuilder(w World, uid string) *Error {
	if n := e.Prices.Builders(w.Layout.HQLevel(e.Rules.Cat)); len(w.Timers) >= n {
		return fail(CodeNoBuilder, uid, "all %d builders are busy", n)
	}
	return nil
}

func (e Engine) startTimer(w World, kind TimerKind, uid, typ string, to int, paid map[string]int64, now time.Time) World {
	t := Timer{
		ID: e.NewID(), Kind: kind, Target: uid, ToLevel: to, Paid: paid,
		FinishAt: now.Add(time.Duration(e.Prices.Seconds(typ, to)) * time.Second),
	}
	w.Timers = append(append([]Timer(nil), w.Timers...), t)
	return w
}

func (e Engine) build(w World, c Command, now time.Time) (World, Effect, Result, error) {
	l, res, err := e.Rules.Apply(w.Layout, c)
	if err != nil || res.Replayed {
		return w, Effect{}, res, err
	}
	if !e.Prices.Has(c.Type) {
		return w, Effect{}, Result{}, fail(CodeNoPrice, c.UID, "%s has no price in content", c.Type)
	}
	if err := e.freeBuilder(w, c.UID); err != nil {
		return w, Effect{}, Result{}, err
	}
	cost := e.Prices.Cost(c.Type, 1)
	w.Layout = l
	w = e.startTimer(w, TimerBuild, c.UID, c.Type, 1, cost, now)
	return w, Effect{Deltas: Spend(cost), Reason: "build"}, Result{}, nil
}

func (e Engine) upgrade(w World, c Command, now time.Time) (World, Effect, Result, error) {
	i := w.Layout.Find(c.UID)
	if i < 0 {
		return w, Effect{}, Result{}, fail(CodeNotFound, c.UID, "no such item")
	}
	it := w.Layout.Items[i]
	d, _ := e.Rules.Cat.Def(it.Type)
	if c.To > 0 {
		if it.Level >= c.To {
			return w, Effect{}, Result{Replayed: true}, nil
		}
		if t, ok := TimerFor(w.Timers, c.UID); ok && t.Kind == TimerUpgrade && t.ToLevel == c.To {
			return w, Effect{}, Result{Replayed: true}, nil
		}
		if c.To != it.Level+1 {
			return w, Effect{}, Result{}, fail(CodeBadCommand, c.UID, "can only upgrade to level %d", it.Level+1)
		}
	}
	if !e.Prices.Has(it.Type) {
		return w, Effect{}, Result{}, fail(CodeNoPrice, c.UID, "%s cannot be upgraded", it.Type)
	}
	if _, busy := TimerFor(w.Timers, c.UID); busy {
		return w, Effect{}, Result{}, fail(CodeBusy, c.UID, "already under construction")
	}
	if it.Level >= e.Prices.MaxLevel(d) {
		return w, Effect{}, Result{}, fail(CodeMaxLevel, c.UID, "%s is at max level %d", it.Type, it.Level)
	}
	if d.Kind == KindCore {
		for _, o := range w.Layout.Items {
			if od, _ := e.Rules.Cat.Def(o.Type); od.Kind == KindBuilding && o.Level < it.Level-1 {
				return w, Effect{}, Result{}, fail(CodeHQGate, o.UID, "%s is level %d, HQ %d needs all buildings >= %d", o.Type, o.Level, it.Level, it.Level-1)
			}
		}
	}
	if err := e.freeBuilder(w, c.UID); err != nil {
		return w, Effect{}, Result{}, err
	}
	to := it.Level + 1
	cost := e.Prices.Cost(it.Type, to)
	w = e.startTimer(w, TimerUpgrade, c.UID, it.Type, to, cost, now)
	return w, Effect{Deltas: Spend(cost), Reason: "upgrade"}, Result{}, nil
}

func (e Engine) remove(w World, c Command) (World, Effect, Result, error) {
	i := w.Layout.Find(c.UID)
	if i < 0 {
		return w, Effect{}, Result{Replayed: true}, nil
	}
	d, _ := e.Rules.Cat.Def(w.Layout.Items[i].Type)
	if d.Kind == KindCore || d.Kind == KindObstacle {
		return w, Effect{}, Result{}, fail(CodeNotPlaceable, c.UID, "%s cannot be removed", d.ID)
	}
	var eff Effect
	out := World{Layout: w.Layout.Clone()}
	out.Layout.Items = append(out.Layout.Items[:i], out.Layout.Items[i+1:]...)
	for _, t := range w.Timers {
		if t.Target != c.UID {
			out.Timers = append(out.Timers, t)
			continue
		}
		if r := e.Prices.Refund(t.Paid); len(r) > 0 {
			eff = Effect{Deltas: Credit(r), Reason: "cancel_refund"}
		}
	}
	return out, eff, Result{}, nil
}

func (e Engine) speedup(w World, c Command, now time.Time) (World, Effect, Result, error) {
	t, ok := TimerFor(w.Timers, c.UID)
	if !ok {
		if w.Layout.Find(c.UID) < 0 {
			return w, Effect{}, Result{}, fail(CodeNotFound, c.UID, "no such item")
		}
		return w, Effect{}, Result{Replayed: true}, nil
	}
	gems := e.Prices.SpeedupGems(int64(t.FinishAt.Sub(now) / time.Second))
	var timers []Timer
	for _, o := range w.Timers {
		if o.ID == t.ID {
			o.FinishAt = now
		}
		timers = append(timers, o)
	}
	l, pending, _ := Settle(w.Layout, timers, now)
	eff := Effect{Reason: "speedup"}
	if gems > 0 {
		eff.Deltas = Spend(map[string]int64{"gems": gems})
	}
	return World{l, pending}, eff, Result{}, nil
}

func (e Engine) clearObstacle(w World, c Command) (World, Effect, Result, error) {
	i := w.Layout.Find(c.UID)
	if i < 0 {
		return w, Effect{}, Result{Replayed: true}, nil
	}
	d, _ := e.Rules.Cat.Def(w.Layout.Items[i].Type)
	if d.Kind != KindObstacle {
		return w, Effect{}, Result{}, fail(CodeNotPlaceable, c.UID, "%s is not an obstacle", d.ID)
	}
	out := World{Layout: w.Layout.Clone(), Timers: w.Timers}
	out.Layout.Items = append(out.Layout.Items[:i], out.Layout.Items[i+1:]...)
	net := map[string]int64{}
	for r, a := range d.Reward {
		net[r] += a
	}
	for r, a := range d.ClearCost {
		net[r] -= a
	}
	return out, Effect{Deltas: Credit(net), Reason: "clear_obstacle"}, Result{}, nil
}
