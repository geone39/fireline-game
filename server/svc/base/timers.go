package base

import (
	"sort"
	"time"
)

// TimerKind says what a construction timer does when it fires.
type TimerKind string

const (
	TimerBuild   TimerKind = "build"   // initial construction of a placed item
	TimerUpgrade TimerKind = "upgrade" // level change of an existing item
)

// Timer is a running construction. Time is the server clock; nothing ticks, the state
// is derived from FinishAt when somebody looks (Settle).
type Timer struct {
	ID       string           `json:"id"`
	Kind     TimerKind        `json:"kind"`
	Target   string           `json:"target_id"` // item uid
	FinishAt time.Time        `json:"finish_at"`
	ToLevel  int              `json:"to_level"`
	Paid     map[string]int64 `json:"-"` // what the player paid (basis of the cancel refund)
}

// Settle applies every timer with FinishAt <= now and returns the new layout, the
// timers still running and the ones that completed (in completion order).
//
// It is lazy and idempotent: Settle(Settle(x, now), now) changes nothing. The clock may
// jump either way: a smaller `now` just completes fewer timers, and effects that already
// happened are never undone (they were persisted when they fired).
func Settle(l Layout, ts []Timer, now time.Time) (Layout, []Timer, []Timer) {
	sorted := append([]Timer(nil), ts...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if !sorted[i].FinishAt.Equal(sorted[j].FinishAt) {
			return sorted[i].FinishAt.Before(sorted[j].FinishAt)
		}
		return sorted[i].ID < sorted[j].ID
	})
	var pending, done []Timer
	out := l
	cloned := false
	for _, t := range sorted {
		if t.FinishAt.After(now) {
			pending = append(pending, t)
			continue
		}
		done = append(done, t)
		if t.Kind != TimerUpgrade {
			continue
		}
		if i := out.Find(t.Target); i >= 0 {
			if !cloned {
				out, cloned = out.Clone(), true
			}
			out.Items[i].Level = max(out.Items[i].Level, t.ToLevel)
		}
	}
	return out, pending, done
}

// TimerFor returns the running timer of an item.
func TimerFor(ts []Timer, uid string) (Timer, bool) {
	for _, t := range ts {
		if t.Target == uid {
			return t, true
		}
	}
	return Timer{}, false
}
