package main

import (
	"github.com/OWNER/fireline/sim"
)

// bestCenter finds the cell whose radius-r square collects the highest total score; entities scoring 0 are skipped.
func (bt *bot) bestCenter(r int, score func(i int32) int) (sim.Cell, int) {
	w := bt.w
	type sc struct {
		i int32
		s int
	}
	var list []sc
	for i := range w.ents {
		if s := score(int32(i)); s > 0 {
			list = append(list, sc{int32(i), s})
		}
	}
	best, bs := sim.Cell{}, 0
	for y := 0; y < w.h; y++ {
		for x := 0; x < w.w; x++ {
			c := sim.Cell{X: int16(x), Y: int16(y)}
			sum := 0
			for _, e := range list {
				if w.footDist(c, &w.ents[e.i]) <= r {
					sum += e.s
				}
			}
			if sum > bs {
				best, bs = c, sum
			}
		}
	}
	return best, bs
}

// ownNear counts the attacker's fighting units within d cells of c.
func (bt *bot) ownNear(c sim.Cell, d int) int {
	w := bt.w
	n := 0
	for i := range w.ents {
		e := &w.ents[i]
		if e.Owner == 1 && e.Alive && e.OnField && w.ud[i] != nil && e.Kind != sim.KindDecoy && max(abs(int(e.Pos.X)-int(c.X)), abs(int(e.Pos.Y)-int(c.Y))) <= d {
			n++
		}
	}
	return n
}

func (bt *bot) cast(id string, to sim.Cell) bool {
	if bt.spellsLeft[id] <= 0 {
		return false
	}
	evs, ok := bt.send(sim.Command{Op: sim.OpCast, ID: id, To: to})
	if !ok {
		return false
	}
	bt.spellsLeft[id]--
	bt.acted = true
	bt.observe(evs)
	bt.refresh()
	return true
}

// castSpells plays the support calls of the army when they are worth it (one try per spell per turn).
func (bt *bot) castSpells() {
	w := bt.w
	if sp := bt.e.Spell["c_airstrike"]; sp != nil && bt.spellsLeft["c_airstrike"] > 0 {
		c, s := bt.bestCenter(sp.RadiusCells, func(i int32) int {
			if !w.enemyTarget(i) {
				return 0
			}
			switch d := w.def[i]; {
			case d == nil:
				return 1
			case d.HQ || (d.Kind == sim.KindDefence && d.Dmg > 0):
				return 3
			case d.Counted && d.Kind == sim.KindBuilding:
				return 1
			}
			return 0
		})
		if s >= 6 && bt.ownNear(c, 12) > 0 {
			bt.cast("c_airstrike", c)
		}
	}
	if sp := bt.e.Spell["c_emp"]; sp != nil && bt.spellsLeft["c_emp"] > 0 {
		c, s := bt.bestCenter(sp.RadiusCells, func(i int32) int {
			if d := w.def[i]; w.enemyTarget(i) && d != nil && d.Kind == sim.KindDefence && d.Dmg > 0 && w.ents[i].DisabledUntil < bt.turn {
				return 1
			}
			return 0
		})
		if s >= 2 && bt.ownNear(c, 9) > 0 {
			bt.cast("c_emp", c)
		}
	}
	if sp := bt.e.Spell["c_medkit"]; sp != nil && bt.spellsLeft["c_medkit"] > 0 {
		c, s := bt.bestCenter(sp.RadiusCells, func(i int32) int {
			e := &w.ents[i]
			if e.Owner == 1 && e.Alive && e.OnField && w.ud[i] != nil && w.ud[i].Class == sim.ClassINF && e.Kind != sim.KindDecoy && e.HP*100 < e.MaxHP*65 {
				return 1
			}
			return 0
		})
		if s >= 3 {
			bt.cast("c_medkit", c)
		}
	}
	if sp := bt.e.Spell["c_adrenaline"]; sp != nil && bt.spellsLeft["c_adrenaline"] > 0 {
		c, s := bt.bestCenter(sp.RadiusCells, func(i int32) int {
			e := &w.ents[i]
			if e.Owner == 1 && e.Alive && e.OnField && w.ud[i] != nil && e.Kind != sim.KindDecoy && (w.ud[i].Dmg > 0 || w.ud[i].Burst > 0) {
				return 1
			}
			return 0
		})
		if s >= 4 && bt.enemyWithin(c, 10) {
			bt.cast("c_adrenaline", c)
		}
	}
	if sp := bt.e.Spell["c_smoke"]; sp != nil && bt.spellsLeft["c_smoke"] > 0 {
		c, s := bt.bestCenter(sp.RadiusCells, func(i int32) int {
			e := &w.ents[i]
			if e.Owner == 1 && e.Alive && e.OnField && w.ud[i] != nil && e.Kind != sim.KindDecoy {
				return 1
			}
			return 0
		})
		if s >= 3 && bt.underFire(c) && !bt.smokeAt(c) {
			bt.cast("c_smoke", c)
		}
	}
}

// enemyWithin says whether an enemy goal stands within d cells of c.
func (bt *bot) enemyWithin(c sim.Cell, d int) bool {
	w := bt.w
	for i := range w.ents {
		if w.enemyTarget(int32(i)) && !w.passive(int32(i)) && w.footDist(c, &w.ents[i]) <= d {
			return true
		}
	}
	return false
}

// underFire: a live defence building could shoot a unit standing at c.
func (bt *bot) underFire(c sim.Cell) bool {
	w := bt.w
	for i := range w.ents {
		d := w.def[i]
		if w.enemyTarget(int32(i)) && d != nil && d.Kind == sim.KindDefence && d.Dmg > 0 && w.footDist(c, &w.ents[i]) <= d.RangeCells+2 {
			return true
		}
	}
	return false
}

func (bt *bot) smokeAt(c sim.Cell) bool {
	for _, z := range bt.w.st.Zones {
		if z.Kind == sim.ZoneSmoke && z.First <= bt.turn && bt.turn <= z.Last &&
			max(abs(int(z.Centre.X)-int(c.X)), abs(int(z.Centre.Y)-int(c.Y))) <= z.R {
			return true
		}
	}
	return false
}

// breachWall casts c_breach on a wall that stands in a unit's way, when it clears at least two wall tiles.
func (bt *bot) breachWall(unit, wall int32) bool {
	sp := bt.e.Spell["c_breach"]
	if sp == nil || bt.spellsLeft["c_breach"] <= 0 {
		return false
	}
	w := bt.w
	d := w.def[wall]
	if d == nil || d.Block != sim.BlockAll {
		return false
	}
	c := w.ents[wall].Pos
	n := 0
	for i := range w.ents {
		if w.enemyTarget(int32(i)) && w.def[i] != nil && w.def[i].Kind == sim.KindWall && w.def[i].Block == sim.BlockAll && w.footDist(c, &w.ents[i]) <= sp.RadiusCells {
			n++
		}
	}
	if n < 2 {
		return false
	}
	return bt.cast("c_breach", c)
}

// heroAbilities uses the hero ability when it pays off.
func (bt *bot) heroAbilities() {
	w := bt.w
	for i := range w.ents {
		e := &w.ents[i]
		ud := w.ud[i]
		if e.Owner != 1 || !e.Alive || !e.OnField || ud == nil || !ud.Hero || e.Cooldown > 0 || e.APLeft < 1 {
			continue
		}
		a := ud.Ability
		switch a.Kind {
		case sim.AbilityZoneDamage:
			n := 0
			for t := range w.ents {
				if w.enemyTarget(int32(t)) && !w.passive(int32(t)) && w.footDist(e.Pos, &w.ents[t]) <= a.RadiusCells {
					n++
				}
			}
			if n >= 2 {
				bt.doAbility(int32(i), sim.AbilityHero, -1, sim.Cell{})
			}
		case sim.AbilityDecoy:
			var tgt int32 = -1
			bd := 13
			for t := range w.ents {
				d := w.def[t]
				if w.enemyTarget(int32(t)) && d != nil && d.Kind == sim.KindDefence && d.Dmg > 0 {
					if dd := w.footDist(e.Pos, &w.ents[t]); dd < bd {
						tgt, bd = int32(t), dd
					}
				}
			}
			if tgt < 0 {
				continue
			}
			best, bk := sim.Cell{}, 1<<30
			for dy := -a.RadiusCells; dy <= a.RadiusCells; dy++ {
				for dx := -a.RadiusCells; dx <= a.RadiusCells; dx++ {
					x, y := int(e.Pos.X)+dx, int(e.Pos.Y)+dy
					if !w.inMap(x, y) || (dx == 0 && dy == 0) {
						continue
					}
					ci := y*w.w + x
					if w.water[ci] || w.bld[ci] >= 0 || w.land[ci] >= 0 {
						continue
					}
					c := sim.Cell{X: int16(x), Y: int16(y)}
					if k := w.footDist(c, &w.ents[tgt]); k < bk {
						best, bk = c, k
					}
				}
			}
			if bk < 1<<30 {
				bt.doAbility(int32(i), sim.AbilityHero, -1, best)
			}
		case sim.AbilityStealth:
			if e.StealthUntil >= bt.turn || bt.hqIdx < 0 || !w.enemyTarget(bt.hqIdx) {
				continue
			}
			if w.footDist(e.Pos, &w.ents[bt.hqIdx]) <= ud.RangeCells+ud.MoveCells {
				bt.doAbility(int32(i), sim.AbilityHero, -1, sim.Cell{})
			}
		}
	}
}
