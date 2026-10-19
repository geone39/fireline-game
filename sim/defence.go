package sim

// The defence phase (spec 7), run by endTurn after the player's EndTurn. Fixed order:
//  1. bunker triggers release their garrison,
//  2. zone effects: airstrike / medkit steps in the order cast, then wire damage over time,
//  3. defence buildings by EntityID: pick a target (dist, hp, EntityID), fire, overheat,
//  4. garrison units by EntityID: shoot, or walk towards the nearest target (Dijkstra, spec 7.4),
//  5. end of turn (timers; the max_turns fuse is in endTurn).
//
// The end conditions (spec 10.1) are checked after every action; the phase stops at once when the battle ends.
// The hero-warden trigger is out of R1 (OQ-7.6). Hidden traps never fire on defender units.

// defencePhase returns the animation events of the whole phase.
func (b *Battle) defencePhase() []Event {
	var evs []Event
	// done folds the events of one action in and reports whether the battle is over.
	done := func(more []Event) bool {
		evs = append(evs, more...)
		if len(more) == 0 {
			return false
		}
		if r := b.checkEnd(); r != EndNone {
			b.finish(r)
			evs = append(evs, Event{Kind: EventBattleEnded, Value: int(r)})
			return true
		}
		return false
	}
	for i := range b.ents { // 1. triggers
		if e := &b.ents[i]; e.bd != nil && e.bd.Garrison > 0 && done(b.releaseGarrison(e)) {
			return evs
		}
	}
	for i := range b.zones { // 2a. zone effects of the spells and abilities, in the order they were cast
		z := &b.zones[i]
		if z.active(b.turn) {
			more := b.zoneStep(z)
			z.step++
			if done(more) {
				return evs
			}
		}
	}
	for i := range b.ents { // 2b. zone effects: wire
		u := &b.ents[i]
		if u.owner != ownerAttacker || !u.alive || !u.onField || u.class() != ClassINF {
			continue
		}
		w := b.ent(b.bld[b.idx(u.pos)])
		if w == nil || !w.alive || w.bd.DotDmg <= 0 {
			continue
		}
		more := append([]Event{{Kind: EventEffectTick, Actor: w.id, Target: u.id, Cell: u.pos, Value: w.bd.DotDmg}}, b.hurt(w.id, u, w.bd.DotDmg)...)
		if done(more) {
			return evs
		}
	}
	for i := range b.ents { // 3. buildings
		if e := &b.ents[i]; e.bd != nil && e.alive && e.bd.Dmg > 0 && done(b.fireBuilding(e)) {
			return evs
		}
	}
	for i := range b.ents { // 4. garrison
		if e := &b.ents[i]; e.owner == ownerDefender && e.ud != nil && e.alive && e.onField && b.actGarrison(e, done) {
			return evs
		}
	}
	return append(evs, b.endOfDefence()...) // 5. finished zones go, finished decoys vanish
}

// releaseGarrison lets the garrison of bunker k out when it has been hurt, or an attacker unit stands within
// trigger_cells of its footprint (spec 7.2). A jammed bunker keeps its units. The units are put on the free
// passable cells of the one-cell ring around the footprint scanned by (Y, X) and act this very phase; the ones
// that find no place wait for the next turn. ids were fixed in NewBattle (canonical numbering, spec 2.4).
func (b *Battle) releaseGarrison(k *entity) []Event {
	if k.disabled(b.turn) {
		return nil
	}
	radius := k.bd.TriggerCells
	waiting := false
	for i := range b.ents {
		if g := &b.ents[i]; g.inside == k.id {
			waiting = true
			if k.bd.TriggerCells == 0 {
				radius = max(radius, g.ud.RangeCells)
			}
		}
	}
	if !waiting {
		return nil
	}
	if !k.damaged {
		hit := false
		for i := range b.ents {
			if a := &b.ents[i]; a.owner == ownerAttacker && a.alive && a.onField && footDist(k, a) <= radius {
				hit = true
				break
			}
		}
		if !hit {
			return nil
		}
	}
	var evs []Event
	for i := range b.ents {
		g := &b.ents[i]
		if g.inside != k.id {
			continue
		}
		c, ok := b.freeNear(g, k)
		if !ok {
			continue
		}
		g.inside = 0
		g.apLeft = g.ap
		b.place(g, c)
		evs = append(evs, Event{Kind: EventReleased, Actor: g.id, Target: k.id, Cell: c})
	}
	return evs
}

// freeNear is the first free cell within cheb 1 of the footprint of k that unit g may stand on, by (Y, X).
func (b *Battle) freeNear(g, k *entity) (Cell, bool) {
	for y := int(k.pos.Y) - 1; y <= int(k.pos.Y)+k.h; y++ {
		for x := int(k.pos.X) - 1; x <= int(k.pos.X)+k.w; x++ {
			c := Cell{int16(x), int16(y)}
			if ok, occ := b.cellState(g.class(), c, 0); ok && !occ {
				return c, true
			}
		}
	}
	return Cell{}, false
}

// fireBuilding is the action of one defence building: once per phase at pick-target's choice (spec 7.1, 7.3).
// d_flame_trap style overheating: after overheat_fire_turns consecutive turns of fire it is dark for cool_turns
// turns (counted in cd, one per defence phase); a turn without a shot restarts the count. A jammed building
// neither fires nor cools.
func (b *Battle) fireBuilding(s *entity) []Event {
	if s.disabled(b.turn) {
		return nil
	}
	if s.cd > 0 {
		s.cd--
		return nil
	}
	t := b.pickTarget(s)
	if t == nil {
		s.heat = 0
		return nil
	}
	if s.bd.OverheatFireTurns > 0 {
		if s.heat++; s.heat >= s.bd.OverheatFireTurns {
			s.heat, s.cd = 0, s.bd.CoolTurns
		}
	}
	if s.bd.Chain > 1 {
		return b.chainShot(s, t)
	}
	return b.blast(s, t, aimCell(s, t), s.bd.SplashCells, func(*entity) int { return s.bd.Dmg }, nil)
}

// chainShot is the d_tesla arc (spec 6.2): the primary target and up to Chain-1 more, each the nearest (EntityID
// on ties) not yet hit enemy of an allowed class within chain_cells of the previous one; full damage each.
// All victims are chosen before anyone is hurt.
func (b *Battle) chainShot(s, first *entity) []Event {
	victims := []*entity{first}
	for len(victims) < s.bd.Chain {
		prev := victims[len(victims)-1]
		var next *entity
		nd := 0
		for i := range b.ents {
			v := &b.ents[i]
			if !b.isCandidate(s, v) || v.ud == nil || s.bd.Vs&ClassBit(v.ud.Class) == 0 || b.concealed(s, v, footDist(s, v)) {
				continue
			}
			hit := false
			for _, o := range victims {
				hit = hit || o == v
			}
			if d := footDist(prev, v); !hit && d <= s.bd.ChainCells && (next == nil || d < nd) {
				next, nd = v, d
			}
		}
		if next == nil {
			break
		}
		victims = append(victims, next)
	}
	var evs []Event
	for _, v := range victims {
		evs = append(evs, b.hurt(s.id, v, s.bd.Dmg)...)
	}
	return evs
}

// actGarrison is the AI of one defender unit (spec 7.4): while it has AP, shoot the best target in range; with
// none, walk (once per turn, 1 AP) as far as move_cells towards the cheapest approach to the nearest target.
// It reports whether the battle is over; done folds events in and checks the end.
func (b *Battle) actGarrison(u *entity, done func([]Event) bool) bool {
	if !u.ud.canAttack() {
		return false // support units are not used by the defence (spec 7.4)
	}
	u.apLeft = u.ap
	moved := false
	for u.apLeft > 0 && u.alive {
		if t := b.pickTarget(u); t != nil {
			if done(b.shoot(u, t)) {
				return true
			}
			continue
		}
		if moved || u.ud.MoveCells == 0 {
			break
		}
		walk, _, ok := b.approach(u)
		if !ok || len(walk) == 0 {
			break
		}
		cost := 0
		for _, c := range walk {
			cost += b.stepCost(u, c)
		}
		end := walk[len(walk)-1]
		b.lift(u)
		b.place(u, end)
		u.apLeft--
		moved = true
		if done([]Event{{Kind: EventMoved, Actor: u.id, Cell: end, Value: cost}}) {
			return true
		}
	}
	return false
}
