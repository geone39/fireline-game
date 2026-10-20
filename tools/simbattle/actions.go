package main

import (
	"sort"

	"github.com/OWNER/fireline/sim"
)

const (
	candidateK = 14  // nearest targets considered per unit and turn
	breakCap   = 250 // a wall never costs more than this many cells of detour
)

// playTurn is the player phase of the current turn: spells, hero ability, then every unit, front line first.
func (bt *bot) playTurn() {
	w := bt.w
	bt.castSpells()
	bt.heroAbilities()

	type unit struct {
		i    int32
		dist int
	}
	var units []unit
	for i := range w.ents {
		e := &w.ents[i]
		if e.Owner != 1 || !e.Alive || !e.OnField || e.Kind == sim.KindDecoy || w.ud[i] == nil {
			continue
		}
		units = append(units, unit{int32(i), bt.nearestDist(int32(i))})
	}
	sort.SliceStable(units, func(a, b int) bool { return units[a].dist < units[b].dist })
	for _, u := range units {
		if bt.over {
			return
		}
		bt.actUnit(u.i)
	}
}

// nearestDist is the Chebyshev distance from the unit to the nearest enemy goal.
func (bt *bot) nearestDist(i int32) int {
	w := bt.w
	best := 1 << 20
	for t := range w.ents {
		if w.enemyTarget(int32(t)) && !w.passive(int32(t)) {
			best = min(best, w.footDist(w.ents[i].Pos, &w.ents[t]))
		}
	}
	return best
}

func (bt *bot) alive(i int32) bool {
	e := &bt.w.ents[i]
	return e.Alive && e.OnField
}

func (bt *bot) actUnit(i int32) {
	w := bt.w
	ud := w.ud[i]
	switch {
	case ud.Heal > 0:
		bt.actHealer(i)
	case ud.JamTurns > 0:
		bt.actSaboteur(i)
	case ud.Carries > 0 && len(w.ents[i].Cargo) > 0:
		bt.actCarrier(i)
	case ud.Dmg > 0 || ud.Burst > 0:
		bt.actAttacker(i)
	}
}

// ---- damage numbers for planning ----

// dmgPerTurn estimates how much the unit takes off an obstacle per turn (all its action points).
func (bt *bot) dmgPerTurn(i int32) int {
	ud, e := bt.w.ud[i], &bt.w.ents[i]
	d := ud.Dmg
	if ud.Burst > 0 {
		return ud.Burst * max(1, ud.WallMult)
	}
	for l := 1; l < e.Level && ud.DmgGrowthPermille > 0; l++ {
		d = d * ud.DmgGrowthPermille / 1000
	}
	return max(1, d*ud.AP)
}

// breakCost is the detour (in cells) a unit accepts instead of shooting through the obstacle.
func (bt *bot) breakCost(i int32) func(bi int32) int {
	dpt := bt.dmgPerTurn(i)
	mv := max(1, bt.w.ud[i].MoveCells)
	return func(bi int32) int {
		hp := bt.w.ents[bi].HP
		return min(breakCap, ((hp+dpt-1)/dpt)*mv)
	}
}

// ---- target preference ----

func (bt *bot) modeOf(i int32) string {
	m := bt.strat.Mode
	if bt.w.ud[i].Hero && bt.w.ud[i].ID == "h_ghost" {
		return "hq" // the x4 shot at the HQ is the point of a_vanish
	}
	if m != "fav" {
		return m
	}
	if meta := bt.e.Meta[bt.w.ud[i].ID]; meta != nil {
		switch meta.Fav {
		case "defence", "walls":
			return "defence"
		case "resource":
			return "resource"
		case "defence_gunners":
			return "gunners"
		}
	}
	return "nearest"
}

// penalty is added to the path cost of a target: the bigger, the less the unit wants it.
func (bt *bot) penalty(mode string, t int32) int {
	w := bt.w
	d := w.def[t]
	switch mode {
	case "hq":
		if d != nil && d.HQ {
			return 0
		}
		return 300
	case "resource":
		if d != nil && (d.Storage || bt.e.Producer[d.ID]) {
			return 0
		}
		return 25
	case "defence":
		if d != nil && d.Kind == sim.KindDefence && d.Dmg > 0 {
			return 0
		}
		if d == nil { // garrison
			return 4
		}
		return 12
	case "gunners":
		if d != nil && d.Kind == sim.KindDefence && d.Dmg > 0 && d.Vs&sim.ClassBit(sim.ClassINF) != 0 {
			return 0
		}
		if d == nil {
			return 4
		}
		return 12
	}
	return 0
}

// candidates: the K most attractive goals (penalty + straight-line distance), obstacles excluded.
func (bt *bot) candidates(i int32, mode string) []int32 {
	w := bt.w
	type c struct{ t, key int32 }
	var cs []c
	for t := range w.ents {
		ti := int32(t)
		if !w.enemyTarget(ti) || w.passive(ti) {
			continue
		}
		cs = append(cs, c{ti, int32(bt.penalty(mode, ti) + w.footDist(w.ents[i].Pos, &w.ents[t]))})
	}
	sort.SliceStable(cs, func(a, b int) bool { return cs[a].key < cs[b].key })
	n := min(len(cs), candidateK)
	out := make([]int32, 0, n+1)
	for _, x := range cs[:n] {
		out = append(out, x.t)
	}
	if mode == "hq" && bt.hqIdx >= 0 && w.enemyTarget(bt.hqIdx) {
		have := false
		for _, t := range out {
			have = have || t == bt.hqIdx
		}
		if !have {
			out = append(out, bt.hqIdx)
		}
	}
	return out
}

// ---- commands ----

func (bt *bot) doAttack(i, t int32) bool {
	w := bt.w
	evs, ok := bt.send(sim.Command{Op: sim.OpAttack, Unit: w.ents[i].ID, Target: w.ents[t].ID})
	if !ok {
		return false
	}
	bt.acted = true
	bt.observe(evs)
	w.ents[i].APLeft--
	needRefresh := false
	for _, ev := range evs {
		switch ev.Kind {
		case sim.EventKilled, sim.EventDestroyed, sim.EventReleased, sim.EventSpawned, sim.EventTrapTriggered:
			needRefresh = true
		case sim.EventAttacked:
			if ti := w.get(ev.Target); ti >= 0 {
				w.ents[ti].HP -= ev.Value
			}
		}
	}
	if needRefresh {
		bt.refresh()
	}
	return true
}

func (bt *bot) doMove(i int32, path []sim.Cell) bool {
	w := bt.w
	evs, ok := bt.send(sim.Command{Op: sim.OpMove, Unit: w.ents[i].ID, Path: path})
	if !ok {
		return false
	}
	bt.acted = true
	bt.moved[w.ents[i].ID] = true
	if len(evs) == 1 && evs[0].Kind == sim.EventMoved {
		w.ents[i].APLeft--
		w.moveUnit(i, evs[0].Cell)
	} else {
		bt.refresh() // a trap went off: it shows in State now, so observe can name it
		bt.observe(evs)
	}
	return true
}

func (bt *bot) doAbility(i int32, kind string, target int32, to sim.Cell) bool {
	w := bt.w
	c := sim.Command{Op: sim.OpAbility, Unit: w.ents[i].ID, ID: kind, To: to}
	if target >= 0 {
		c.Target = w.ents[target].ID
	}
	evs, ok := bt.send(c)
	if !ok {
		return false
	}
	bt.acted = true
	bt.observe(evs)
	bt.refresh()
	return true
}

// ---- the main fighter ----

func (bt *bot) otherFightersAlive(self int32) bool {
	w := bt.w
	for i := range w.ents {
		e := &w.ents[i]
		if int32(i) != self && e.Owner == 1 && e.Alive && e.OnField && w.ud[i] != nil && w.ud[i].Dmg > 0 {
			return true
		}
	}
	return false
}

func (bt *bot) actAttacker(i int32) {
	w := bt.w
	ud := w.ud[i]
	mode := bt.modeOf(i)
	lo, hi := ud.MinRangeCells, ud.RangeCells
	for guard := 0; guard < 4; guard++ {
		e := &w.ents[i]
		if !bt.alive(i) || e.APLeft < 1 {
			return
		}
		pen := func(t int32) int { return bt.penalty(mode, t) }
		pl := w.approach(i, bt.candidates(i, mode), lo, hi, bt.budget(i), pen, bt.breakCost(i))
		if !pl.ok {
			bt.opportunistic(i, mode)
			return
		}
		// a bomber only spends itself on an obstacle, unless nobody else is left to fight
		if ud.Burst > 0 && !pl.interim && bt.otherFightersAlive(i) {
			if len(pl.path) > 0 && !bt.moved[e.ID] {
				bt.doMove(i, pl.path)
			}
			return
		}
		if pl.interim && w.def[pl.target] != nil && w.def[pl.target].Kind == sim.KindWall && bt.breachWall(i, pl.target) {
			continue
		}
		switch {
		case pl.inRange && len(pl.path) == 0:
			if !bt.doAttack(i, pl.target) {
				return
			}
		case len(pl.path) > 0 && !bt.moved[e.ID]:
			if !bt.doMove(i, pl.path) {
				return
			}
		default:
			bt.opportunistic(i, mode)
			return
		}
	}
}

func (bt *bot) budget(i int32) int {
	if bt.moved[bt.w.ents[i].ID] || bt.w.ud[i].MoveCells == 0 {
		return 0
	}
	return bt.w.ud[i].MoveCells
}

// opportunistic shoots whatever is in range when there is no better plan.
func (bt *bot) opportunistic(i int32, mode string) {
	w := bt.w
	ud := w.ud[i]
	for bt.alive(i) && w.ents[i].APLeft > 0 {
		e := &w.ents[i] // re-read: a refresh replaces the slice
		best, bk := int32(-1), 1<<30
		for t := range w.ents {
			ti := int32(t)
			if !w.enemyTarget(ti) {
				continue
			}
			if ud.Burst > 0 && !w.passive(ti) {
				continue
			}
			d := w.footDist(e.Pos, &w.ents[t])
			if d < ud.MinRangeCells || d > ud.RangeCells {
				continue
			}
			k := bt.penalty(mode, ti)*100 + d*10
			if w.passive(ti) {
				k += 5000
			}
			if k < bk {
				best, bk = ti, k
			}
		}
		if best < 0 || !bt.doAttack(i, best) {
			return
		}
	}
}

// ---- support units ----

func (bt *bot) actHealer(i int32) {
	w := bt.w
	ud := w.ud[i]
	for guard := 0; guard < 3; guard++ {
		e := &w.ents[i]
		if !bt.alive(i) || e.APLeft < 1 {
			return
		}
		var wounded, allies []int32
		for t := range w.ents {
			o := &w.ents[t]
			if int32(t) == i || o.Owner != 1 || !o.Alive || !o.OnField || w.ud[t] == nil || o.Kind == sim.KindDecoy ||
				ud.HealVs&sim.ClassBit(w.ud[t].Class) == 0 {
				continue
			}
			if w.ud[t].Dmg > 0 || w.ud[t].Burst > 0 {
				allies = append(allies, int32(t))
			}
			if o.HP < o.MaxHP {
				wounded = append(wounded, int32(t))
			}
		}
		var tgt int32 = -1
		bestFrac := 1 << 30
		for _, t := range wounded {
			o := &w.ents[t]
			if w.footDist(e.Pos, o) <= ud.RangeCells {
				if f := o.HP * 1000 / max(1, o.MaxHP); f < bestFrac {
					tgt, bestFrac = t, f
				}
			}
		}
		if tgt >= 0 {
			if !bt.doAbility(i, sim.AbilityHeal, tgt, sim.Cell{}) {
				return
			}
			continue
		}
		if bt.moved[e.ID] || len(allies) == 0 || ud.MoveCells == 0 {
			return
		}
		cand := wounded
		if len(cand) == 0 {
			cand = allies
		}
		pl := w.approach(i, cand, 0, max(1, ud.RangeCells-1), bt.budget(i), func(int32) int { return 0 }, func(int32) int { return breakCap })
		if !pl.ok || len(pl.path) == 0 || !bt.doMove(i, pl.path) {
			return
		}
	}
}

func (bt *bot) actSaboteur(i int32) {
	w := bt.w
	for guard := 0; guard < 3; guard++ {
		e := &w.ents[i]
		if !bt.alive(i) || e.APLeft < 1 {
			return
		}
		var cand []int32
		for t := range w.ents {
			o, d := &w.ents[t], w.def[t]
			if w.enemyTarget(int32(t)) && d != nil && d.Kind == sim.KindDefence && d.Dmg > 0 && o.DisabledUntil < bt.turn {
				cand = append(cand, int32(t))
			}
		}
		if len(cand) == 0 {
			return
		}
		sort.SliceStable(cand, func(a, b int) bool {
			return w.footDist(e.Pos, &w.ents[cand[a]]) < w.footDist(e.Pos, &w.ents[cand[b]])
		})
		cand = cand[:min(len(cand), candidateK)]
		pl := w.approach(i, cand, 0, 1, bt.budget(i), func(int32) int { return 0 }, bt.breakCost(i))
		if !pl.ok {
			return
		}
		switch {
		case pl.inRange && len(pl.path) == 0 && !pl.interim:
			if !bt.doAbility(i, sim.AbilityJam, pl.target, sim.Cell{}) {
				return
			}
		case len(pl.path) > 0 && !bt.moved[e.ID]:
			if !bt.doMove(i, pl.path) {
				return
			}
		default:
			return
		}
	}
}

// actCarrier drives a loaded APC / lander toward the enemy and unloads it near the target.
func (bt *bot) actCarrier(i int32) {
	w := bt.w
	ud := w.ud[i]
	for guard := 0; guard < 2; guard++ {
		e := &w.ents[i]
		if !bt.alive(i) || e.APLeft < 1 || len(e.Cargo) == 0 {
			return
		}
		hurt := e.HP*2 < e.MaxHP
		if ud.Class == sim.ClassSEA {
			if bt.sailLander(i, hurt) {
				continue
			}
			return
		}
		mode := bt.modeOf(i)
		cand := bt.candidates(i, mode)
		near := false
		for _, t := range cand {
			near = near || w.footDist(e.Pos, &w.ents[t]) <= 4
		}
		room := bt.landFree(e.Pos) > 0
		if (near || hurt) && room {
			if bt.doAbility(i, sim.AbilityUnload, -1, sim.Cell{}) {
				return
			}
		}
		pl := w.approach(i, cand, 0, 3, bt.budget(i), func(t int32) int { return bt.penalty(mode, t) }, bt.breakCost(i))
		if !pl.ok || len(pl.path) == 0 || bt.moved[e.ID] {
			if room {
				bt.doAbility(i, sim.AbilityUnload, -1, sim.Cell{})
			}
			return
		}
		if !bt.doMove(i, pl.path) {
			return
		}
	}
}

// landFree counts the free dry cells around c (where the passengers would step out).
func (bt *bot) landFree(c sim.Cell) int {
	w := bt.w
	n := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			x, y := int(c.X)+dx, int(c.Y)+dy
			if !w.inMap(x, y) {
				continue
			}
			ci := y*w.w + x
			if !w.water[ci] && w.bld[ci] < 0 && w.land[ci] < 0 {
				n++
			}
		}
	}
	return n
}

// sailLander moves a lander to the water cell with room to land next to the base (nearest to the HQ), unloads
// there. true = it moved and may act again.
func (bt *bot) sailLander(i int32, hurt bool) bool {
	w := bt.w
	e := &w.ents[i]
	if bt.landFree(e.Pos) >= 2 && (hurt || bt.nearHQ(e.Pos)) {
		if bt.doAbility(i, sim.AbilityUnload, -1, sim.Cell{}) {
			return false
		}
	}
	if bt.moved[e.ID] {
		return false
	}
	w.dijkstra(i, func(int32) int { return breakCap })
	var hq sim.Cell
	if bt.hqIdx >= 0 {
		hq = w.ents[bt.hqIdx].Pos
	}
	best, bk := -1, 1<<30
	for ci := range w.dist {
		if w.dist[ci] < 0 {
			continue
		}
		c := w.cell(ci)
		if bt.landFree(c) < 2 {
			continue
		}
		k := int(w.dist[ci]) + 3*max(abs(int(c.X)-int(hq.X)), abs(int(c.Y)-int(hq.Y)))
		if k < bk {
			best, bk = ci, k
		}
	}
	if best < 0 {
		return false
	}
	if best == w.cidx(e.Pos) { // already at the best landing: step out
		if bt.landFree(e.Pos) > 0 {
			bt.doAbility(i, sim.AbilityUnload, -1, sim.Cell{})
		}
		return false
	}
	spent := 0
	var path []sim.Cell
	for _, c := range w.chainTo(int32(best)) {
		if spent+1 > w.ud[i].MoveCells {
			break
		}
		spent++
		path = append(path, c)
	}
	if len(path) == 0 {
		return false
	}
	return bt.doMove(i, path)
}

func (bt *bot) nearHQ(c sim.Cell) bool {
	if bt.hqIdx < 0 {
		return false
	}
	return bt.w.footDist(c, &bt.w.ents[bt.hqIdx]) <= 14
}
