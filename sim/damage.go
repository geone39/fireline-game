package sim

// Attack, damage, splash, destruction (spec 5.2, 6, 10). Integer arithmetic only.

func (u *UnitDef) canAttack() bool { return u.Dmg > 0 || u.Burst > 0 }

// isWallLike: passive obstacles that block movement or slow it (walls, fences, tank traps, wire): the targets
// of a breacher's wall_mult (spec 6.3). Hidden traps are KindWall too but block nothing.
func (d *BuildingDef) isWallLike() bool {
	return d.Kind == KindWall && (d.Block != BlockNone || d.SlowPct > 0)
}

// hasTag says whether an entity belongs to a vs_mult group.
func (e *entity) hasTag(t VsTag) bool {
	if e.bd != nil {
		switch t {
		case VsWire:
			return e.bd.Kind == KindWall && e.bd.SlowPct > 0
		case VsWall:
			return e.bd.Kind == KindWall && e.bd.Block != BlockNone
		case VsBuilding:
			return e.bd.Kind == KindBuilding
		case VsDefence:
			return e.bd.Kind == KindDefence
		}
		return false
	}
	switch t {
	case VsINF:
		return e.ud.Class == ClassINF
	case VsVEH:
		return e.ud.Class == ClassVEH
	case VsAIR:
		return e.ud.Class == ClassAIR
	case VsSEA:
		return e.ud.Class == ClassSEA
	}
	return false
}

// shotDamage is the damage of one attack of unit s on victim v at the centre of the blast (spec 6.1):
// base (Dmg, or Burst x wall_mult on walls/wire) x (1000 + sum of vs_mult bonuses) / 1000, one floor.
// Adrenaline and the commander aura (SIM-5) will join the same bonus sum.
func shotDamage(s, v *entity) int {
	base := s.ud.Dmg
	if s.ud.Burst > 0 {
		base = s.ud.Burst
		if v.bd != nil && v.bd.isWallLike() && s.ud.WallMult > 1 {
			base *= s.ud.WallMult
		}
	}
	bonus := 0
	for _, m := range s.ud.VsMult {
		if v.hasTag(m.Tag) {
			bonus += m.Permille - 1000
		}
	}
	return base * max(0, 1000+bonus) / 1000
}

// splashDamage applies the edge falloff: dmg x (100 - falloff x d / r) / 100 with one floor (spec 6.2).
func (b *Battle) splashDamage(dmg, d, r int) int {
	if r <= 0 || d <= 0 {
		return dmg
	}
	return dmg * (100*r - b.rules.SplashFalloffPct*d) / (100 * r)
}

// aimCell is the cell of the target footprint closest to the shooter: the blast centre (spec 6.2).
func aimCell(s, t *entity) Cell {
	cl := func(v, lo, n int) int { return max(lo, min(v, lo+n-1)) }
	return Cell{int16(cl(int(s.pos.X), int(t.pos.X), t.w)), int16(cl(int(s.pos.Y), int(t.pos.Y), t.h))}
}

// blast hurts every enemy of actor within r of centre, the primary victim first and always (spec 6.2), the
// rest by ascending EntityID. dmg gives the undiminished damage on a victim; in (if not nil) restricts splash
// victims. All victims are chosen before anyone is hurt.
func (b *Battle) blast(actor, primary *entity, centre Cell, r int, dmg func(v *entity) int, in func(v *entity) bool) []Event {
	probe := entity{pos: centre, w: 1, h: 1}
	var victims []*entity
	var dists []int
	if primary != nil {
		victims, dists = append(victims, primary), append(dists, footDist(&probe, primary))
	}
	if r > 0 {
		for i := range b.ents {
			v := &b.ents[i]
			if v == primary || !b.isCandidate(actor, v) || (in != nil && !in(v)) {
				continue
			}
			if d := footDist(&probe, v); d <= r {
				victims, dists = append(victims, v), append(dists, d)
			}
		}
	}
	var evs []Event
	for i, v := range victims {
		evs = append(evs, b.hurt(actor.id, v, b.splashDamage(dmg(v), dists[i], r))...)
	}
	return evs
}

// hurt removes up to dmg hit points; at 0 the entity is destroyed at once (spec 6.1).
func (b *Battle) hurt(by EntityID, v *entity, dmg int) []Event {
	dmg = min(dmg, v.hp)
	if dmg <= 0 || !v.alive {
		return nil
	}
	v.hp -= dmg
	v.damaged = true
	if v.kind == KindHero && v.owner == ownerAttacker {
		b.heroDmg += dmg
	}
	evs := []Event{{Kind: EventAttacked, Actor: by, Target: v.id, Cell: v.pos, Value: dmg}}
	if v.hp == 0 {
		evs = append(evs, b.kill(by, v)...)
	}
	return evs
}

// kill destroys an entity: cells are free at once (buildings are checked by alive), a unit leaves the grid, a
// carrier lets its passengers out (spec 5.4).
func (b *Battle) kill(by EntityID, v *entity) []Event {
	v.alive, v.hp = false, 0
	if v.bd != nil {
		return []Event{{Kind: EventDestroyed, Actor: by, Target: v.id, Cell: v.pos}}
	}
	at := v.pos
	b.lift(v)
	evs := []Event{{Kind: EventKilled, Actor: by, Target: v.id, Cell: at}}
	if len(v.cargo) > 0 {
		evs = append(evs, b.disembark(v, at)...)
	}
	return evs
}

// disembark puts the cargo of a destroyed carrier on free ground cells within cheb 1, scanned by (Y, X),
// passengers in boarding order, ap_left 0; the ones that find no place die (spec 5.4).
func (b *Battle) disembark(k *entity, at Cell) []Event {
	var evs []Event
	cargo := k.cargo
	k.cargo = nil
	for _, pid := range cargo {
		p := &b.ents[pid-1]
		p.aboard = 0
		placed := false
		for y := int(at.Y) - 1; y <= int(at.Y)+1 && !placed; y++ {
			for x := int(at.X) - 1; x <= int(at.X)+1 && !placed; x++ {
				c := Cell{int16(x), int16(y)}
				if ok, occ := b.cellState(p.class(), c, 0); ok && !occ {
					b.place(p, c)
					p.apLeft = 0
					placed = true
					evs = append(evs, Event{Kind: EventReleased, Actor: p.id, Target: k.id, Cell: c})
				}
			}
		}
		if !placed {
			p.alive, p.hp = false, 0
			evs = append(evs, Event{Kind: EventKilled, Actor: k.id, Target: p.id, Cell: at})
		}
	}
	return evs
}

// cmdAttack is Attack{Unit, Target} (spec 5.2): 1 AP, target in [min_range, range] by Chebyshev distance to its
// footprint, no line of fire (R1). Check order: ids, owner, shooter state, AP, target, range.
func (b *Battle) cmdAttack(c Command) ([]Event, RejectCode) {
	u, t := b.ent(c.Unit), b.ent(c.Target)
	if u == nil || t == nil {
		return nil, RejectUnknownEntity
	}
	if u.owner != ownerAttacker {
		return nil, RejectNotOwner
	}
	if !u.alive || !u.onField || !u.ud.canAttack() {
		return nil, RejectInvalidTarget
	}
	if u.apLeft < 1 {
		return nil, RejectNoAP
	}
	if !b.isCandidate(u, t) {
		return nil, RejectInvalidTarget
	}
	d := footDist(u, t)
	if d < u.ud.MinRangeCells {
		return nil, RejectBelowMinRange
	}
	if d > u.ud.RangeCells || !lineOfFire(u.pos, t.pos) {
		return nil, RejectOutOfRange
	}
	return b.shoot(u, t), 0
}

// shoot is one attack action of a unit (player or garrison): 1 AP, damage with splash, a bomber is spent by its
// own blast. The caller has checked legality.
func (b *Battle) shoot(u, t *entity) []Event {
	u.apLeft--
	evs := b.blast(u, t, aimCell(u, t), u.ud.SplashCells, func(v *entity) int { return shotDamage(u, v) }, nil)
	if u.ud.Burst > 0 && u.alive {
		evs = append(evs, b.kill(u.id, u)...)
	}
	return evs
}
