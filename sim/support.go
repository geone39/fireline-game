package sim

// The support layer (SIM-5, spec 5.3, 9): Ability{heal, jam, unload, hero} and Cast. Everything is counted in
// turns; zone effects live in Battle.zones in creation order and are stepped in the defence phase (spec 7, step 2).
// All numbers come from Rules (content); nothing here is a game constant.

// Ability kinds of Command.ID (Command.Ability). "hero" runs the ability of the hero (UnitDef.Ability).
const (
	AbilityHeal   = "heal"
	AbilityJam    = "jam"
	AbilityUnload = "unload"
	AbilityHero   = "hero"
)

type zoneKind uint8

const (
	zoneAirstrike  zoneKind = iota + 1 // damage to every defender entity inside, `amount` over the steps
	zoneMedkit                         // heal to every own infantry unit inside, `amount` over the steps
	zoneSmoke                          // attacker units inside are not targets of the defence beyond 1 cell
	zoneAdrenaline                     // bonuses for the `members` fixed at the cast
)

// zone is a spell / ability effect on the grid. Active during the turns first..last (spec 9.1:
// expires_after_turn = t + turns - 1); step counts the defence phases it has been stepped in.
type zone struct {
	kind          zoneKind
	centre        Cell
	r             int
	first, last   int
	step          int
	amount        int
	movePm, dmgPm int
	members       []EntityID // adrenaline: ascending ids at the cast
}

func (z *zone) active(t int) bool { return z.first <= t && t <= z.last }

// share is the part of amount paid in the current step: equal parts, the remainder to the first step (spec 9.1).
func (z *zone) share() int {
	n := z.last - z.first + 1
	a := z.amount / n
	if z.step == 0 {
		a += z.amount % n
	}
	return a
}

func (z *zone) has(id EntityID) bool {
	for _, m := range z.members {
		if m == id {
			return true
		}
	}
	return false
}

// inZone reports whether v (any part of its footprint) lies within r of centre (Chebyshev, spec 2.2).
func inZone(centre Cell, r int, v *entity) bool {
	probe := entity{pos: centre, w: 1, h: 1}
	return footDist(&probe, v) <= r
}

// playerFoe is an entity a player effect (airstrike, EMP, breach...) may hurt: defender things on the field
// that are real targets (no decor, no hidden traps, spec 8.4, 8.5).
func playerFoe(v *entity) bool {
	if v.owner != ownerDefender || !v.alive || !v.onField {
		return false
	}
	return v.bd == nil || !(v.bd.Untargetable || (v.bd.Hidden && !v.revealed))
}

// hidden reports whether the hero is out of sight during turn t (a_vanish).
func (e *entity) hidden(t int) bool { return e.stealthUntil > 0 && e.stealthUntil >= t }

// inSmoke reports whether c lies in an active smoke screen.
func (b *Battle) inSmoke(c Cell) bool {
	for i := range b.zones {
		if z := &b.zones[i]; z.kind == zoneSmoke && z.active(b.turn) && Cheb(z.centre, c) <= z.r {
			return true
		}
	}
	return false
}

// concealed says whether the defence may not pick attacker unit t as a target of s at distance d: a hidden hero
// (stealth), or any unit inside a smoke screen unless it is adjacent to the shooter (spec 7.3, 9.1).
func (b *Battle) concealed(s, t *entity, d int) bool {
	if s.owner != ownerDefender || t.owner != ownerAttacker {
		return false
	}
	if t.hidden(b.turn) {
		return true
	}
	return d > 1 && len(b.zones) > 0 && b.inSmoke(t.pos)
}

// bonuses returns the permille bonuses of an attacker unit right now: adrenaline (the best active one it was
// in the zone for; zones do not stack, ASSUMPTION) plus the hero aura of a living commander standing within
// aura_cells (recomputed at the moment of the action, spec 9.2; the hero itself gets none).
func (b *Battle) bonuses(u *entity) (movePm, dmgPm int) {
	if u.owner != ownerAttacker {
		return 0, 0
	}
	for i := range b.zones {
		z := &b.zones[i]
		if z.kind == zoneAdrenaline && z.active(b.turn) && z.has(u.id) {
			movePm, dmgPm = max(movePm, z.movePm), max(dmgPm, z.dmgPm)
		}
	}
	if b.heroID != 0 {
		if h := &b.ents[b.heroID-1]; h != u && h.alive && h.onField && h.ud.AuraCells > 0 && Cheb(h.pos, u.pos) <= h.ud.AuraCells {
			movePm += h.ud.AuraMovePermille
			dmgPm += h.ud.AuraDmgPermille
		}
	}
	return movePm, dmgPm
}

// moveLimit is move_cells with the move bonuses applied: + floor(move x bonus / 1000), at least one cell (spec 9.1).
func (b *Battle) moveLimit(u *entity) int {
	m := u.ud.MoveCells
	if m == 0 {
		return 0
	}
	if pm, _ := b.bonuses(u); pm > 0 {
		m += max(1, m*pm/1000)
	}
	return m
}

// stealthMult is the multiplier (permille) of a hidden hero's shot at v, 1000 when it does not apply.
func (b *Battle) stealthMult(s, v *entity) int {
	if a := &s.ud.Ability; a.Kind == AbilityStealth && a.BonusPermille > 0 && s.hidden(b.turn) && v.hasTag(a.BonusVs) {
		return a.BonusPermille
	}
	return 1000
}

// cmdAbility is Ability{Unit, ID: kind, Target, To} (spec 5.3). Check order: ids, owner, shooter state and that the
// unit has the ability (invalid_target), AP, cooldown, then the kind's own checks.
func (b *Battle) cmdAbility(c Command) ([]Event, RejectCode) {
	u := b.ent(c.Unit)
	if u == nil {
		return nil, RejectUnknownEntity
	}
	if u.owner != ownerAttacker {
		return nil, RejectNotOwner
	}
	if !u.alive || !u.onField || u.ud == nil || u.kind == KindDecoy {
		return nil, RejectInvalidTarget
	}
	has := false
	switch c.ID {
	case AbilityHeal:
		has = u.ud.Heal > 0
	case AbilityJam:
		has = u.ud.JamTurns > 0
	case AbilityUnload:
		has = u.ud.Carries > 0
	case AbilityHero:
		has = u.ud.Ability.Kind != AbilityNone
	}
	if !has {
		return nil, RejectInvalidTarget
	}
	if u.apLeft < 1 {
		return nil, RejectNoAP
	}
	if c.ID == AbilityHero && u.cd > 0 {
		return nil, RejectCooldown
	}
	switch c.ID {
	case AbilityHeal:
		return b.abilityHeal(u, c)
	case AbilityJam:
		return b.abilityJam(u, c)
	case AbilityUnload:
		return b.abilityUnload(u, c)
	}
	return b.abilityHero(u, c)
}

// abilityHeal restores heal hit points (not above max) to a wounded own unit of a class in heal_vs within
// range_cells; not oneself.
func (b *Battle) abilityHeal(u *entity, c Command) ([]Event, RejectCode) {
	t := b.ent(c.Target)
	if t == nil {
		return nil, RejectUnknownEntity
	}
	if t.owner != ownerAttacker || t == u || !t.alive || !t.onField || t.ud == nil || t.kind == KindDecoy ||
		u.ud.HealVs&ClassBit(t.ud.Class) == 0 || t.hp >= t.maxHP {
		return nil, RejectInvalidTarget
	}
	if footDist(u, t) > u.ud.RangeCells {
		return nil, RejectOutOfRange
	}
	u.apLeft--
	n := min(u.ud.Heal, t.maxHP-t.hp)
	t.hp += n
	return []Event{{Kind: EventHealed, Actor: u.id, Target: t.id, Cell: t.pos, Value: n}}, 0
}

// abilityJam disables a live defence building that has an attack and stands right next to the saboteur for
// jam_turns turns (spec 7.5); an already jammed building keeps the later end.
func (b *Battle) abilityJam(u *entity, c Command) ([]Event, RejectCode) {
	t := b.ent(c.Target)
	if t == nil {
		return nil, RejectUnknownEntity
	}
	if t.bd == nil || t.bd.Dmg <= 0 || !b.isCandidate(u, t) {
		return nil, RejectInvalidTarget
	}
	if footDist(u, t) > 1 {
		return nil, RejectOutOfRange
	}
	u.apLeft--
	t.disabledUntil = max(t.disabledUntil, b.turn+u.ud.JamTurns-1)
	return []Event{{Kind: EventEffectTick, Actor: u.id, Target: t.id, Cell: t.pos, Value: u.ud.JamTurns}}, 0
}

// abilityUnload lets the passengers (or just Target) of a carrier out onto free ground cells within cheb 1,
// scanned by (Y, X), in boarding order; the ones that do not fit stay aboard (spec 5.4). A ship's passengers can
// only land on dry cells. They have no AP this turn. Nobody out = capacity-style refusal, nothing changes.
func (b *Battle) abilityUnload(u *entity, c Command) ([]Event, RejectCode) {
	if len(u.cargo) == 0 {
		return nil, RejectInvalidTarget
	}
	if c.Target != 0 {
		in := false
		for _, id := range u.cargo {
			in = in || id == c.Target
		}
		if !in {
			return nil, RejectInvalidTarget
		}
	}
	var evs []Event
	var rest []EntityID
	for _, pid := range u.cargo {
		if c.Target != 0 && pid != c.Target {
			rest = append(rest, pid)
			continue
		}
		p := &b.ents[pid-1]
		placed := false
		for y := int(u.pos.Y) - 1; y <= int(u.pos.Y)+1 && !placed; y++ {
			for x := int(u.pos.X) - 1; x <= int(u.pos.X)+1 && !placed; x++ {
				cc := Cell{int16(x), int16(y)}
				if ok, occ := b.cellState(p.class(), cc, 0); ok && !occ {
					p.aboard = 0
					b.place(p, cc)
					p.apLeft = 0
					placed = true
					evs = append(evs, Event{Kind: EventReleased, Actor: p.id, Target: u.id, Cell: cc})
				}
			}
		}
		if !placed {
			rest = append(rest, pid)
		}
	}
	if len(evs) == 0 {
		return nil, RejectCellOccupied
	}
	u.cargo = rest
	u.apLeft--
	return evs, 0
}

// abilityHero runs the hero ability (spec 5.3): zone_damage around the hero, spawn_decoy within RadiusCells at To,
// stealth. 1 AP; the cooldown starts. buff_aura (h_warden) is out of R1 (OQ-7.6).
func (b *Battle) abilityHero(u *entity, c Command) ([]Event, RejectCode) {
	a := &u.ud.Ability
	var evs []Event
	switch a.Kind {
	case AbilityZoneDamage:
		b.zones = append(b.zones, zone{kind: zoneAirstrike, centre: u.pos, r: a.RadiusCells,
			first: b.turn, last: b.turn + a.DurationTurns - 1, amount: a.Dmg})
		evs = append(evs, Event{Kind: EventSpellCast, Actor: u.id, Cell: u.pos, Value: a.RadiusCells})
	case AbilityDecoy:
		if !b.inMap(c.To) || Cheb(u.pos, c.To) > a.RadiusCells {
			return nil, RejectOutOfRange
		}
		ud := &b.rules.Units[b.unitIdx[a.DecoyUnit]]
		ok, occ := b.cellState(ud.Class, c.To, 0)
		if !ok {
			return nil, RejectBadPath
		}
		if occ {
			return nil, RejectCellOccupied
		}
		d := entity{id: EntityID(len(b.ents) + 1), kind: KindDecoy, owner: ownerAttacker, contentID: ud.ID,
			level: u.level, w: 1, h: 1, alive: true, slot: -1, ud: ud}
		d.maxHP = scaledHP(ud.HP, ud.HPGrowthPermille, u.level)
		d.hp = d.maxHP
		if a.DurationTurns > 0 {
			d.decoyUntil = b.turn + a.DurationTurns - 1
		}
		b.ents = append(b.ents, d)
		u = &b.ents[c.Unit-1] // the slice may have moved
		b.place(&b.ents[len(b.ents)-1], c.To)
		evs = append(evs, Event{Kind: EventSpawned, Actor: u.id, Target: d.id, Cell: c.To})
	case AbilityStealth:
		u.stealthUntil = b.turn + a.DurationTurns - 1
		evs = append(evs, Event{Kind: EventEffectTick, Actor: u.id, Target: u.id, Cell: u.pos, Value: a.DurationTurns})
	}
	u.apLeft--
	u.cd = a.CooldownTurns
	return evs, 0
}

// jamPct is the weakening of a spell centred on c: the strongest live, not disabled d_jammer whose footprint is
// within aura_cells of c (generators do not stack, ASSUMPTION; spec 7.5).
func (b *Battle) jamPct(c Cell) int {
	pct := 0
	for i := range b.ents {
		e := &b.ents[i]
		if e.bd != nil && e.bd.JamPct > 0 && e.alive && !e.disabled(b.turn) && inZone(c, e.bd.AuraCells, e) {
			pct = max(pct, e.bd.JamPct)
		}
	}
	return pct
}

// weaken is v x (100 - pct) / 100, floor, at least 1 (spec 7.5).
func weaken(v, pct int) int {
	if pct <= 0 {
		return v
	}
	return max(1, v*(100-pct)/100)
}

// cmdCast is Cast{ID, To} (spec 9.1): takes a spell from the first slot of that id with stock, costs no AP, at
// most spells_per_turn per turn (refused as no_ap, ASSUMPTION), To is any cell of the map (out_of_zone otherwise).
func (b *Battle) cmdCast(c Command) ([]Event, RejectCode) {
	si, ok := b.spellIdx[c.ID]
	slot := -1
	if ok {
		for i := range b.spells {
			if b.spells[i].ID == c.ID && b.spells[i].Count > 0 {
				slot = i
				break
			}
		}
	}
	if slot < 0 {
		return nil, RejectNoStock
	}
	if lim := b.rules.SpellsPerTurn; lim > 0 && b.spellsTurn >= lim {
		return nil, RejectNoAP
	}
	if !b.inMap(c.To) {
		return nil, RejectOutOfZone
	}
	sp := &b.rules.Spells[si]
	b.spells[slot].Count--
	b.spellsTurn++
	b.spellsUsed = addSlot(b.spellsUsed, sp.ID, b.spells[slot].Level)
	evs := []Event{{Kind: EventSpellCast, Cell: c.To, Value: sp.RadiusCells}}
	last := b.turn + sp.DurationTurns - 1
	switch sp.Kind {
	case SpellAirstrike:
		b.zones = append(b.zones, zone{kind: zoneAirstrike, centre: c.To, r: sp.RadiusCells, first: b.turn, last: last,
			amount: weaken(sp.DmgTotal, b.jamPct(c.To))})
	case SpellMedkit:
		b.zones = append(b.zones, zone{kind: zoneMedkit, centre: c.To, r: sp.RadiusCells, first: b.turn, last: last, amount: sp.HealTotal})
	case SpellSmoke:
		b.zones = append(b.zones, zone{kind: zoneSmoke, centre: c.To, r: sp.RadiusCells, first: b.turn, last: last})
	case SpellAdrenaline:
		z := zone{kind: zoneAdrenaline, centre: c.To, r: sp.RadiusCells, first: b.turn, last: last,
			movePm: sp.MovePermille, dmgPm: sp.DmgPermille}
		for i := range b.ents {
			if e := &b.ents[i]; e.owner == ownerAttacker && e.alive && e.onField && e.ud != nil && e.kind != KindDecoy && inZone(c.To, sp.RadiusCells, e) {
				z.members = append(z.members, e.id)
			}
		}
		b.zones = append(b.zones, z)
	case SpellEMP:
		n := weaken(sp.DurationTurns, b.jamPct(c.To))
		for i := range b.ents {
			if e := &b.ents[i]; e.bd != nil && e.bd.Kind == KindDefence && e.alive && inZone(c.To, sp.RadiusCells, e) {
				e.disabledUntil = max(e.disabledUntil, b.turn+n-1)
				evs = append(evs, Event{Kind: EventEffectTick, Target: e.id, Cell: e.pos, Value: n})
			}
		}
	case SpellBreach:
		for i := range b.ents {
			if e := &b.ents[i]; e.bd != nil && e.bd.Kind == KindWall && e.bd.Block == BlockAll && e.alive && inZone(c.To, sp.RadiusCells, e) {
				evs = append(evs, b.kill(0, e)...)
			}
		}
	}
	return evs, 0
}

// zoneStep pays one step of an active airstrike / medkit zone (defence phase, step 2). The victims are all chosen
// before anyone is hurt; they are taken by ascending EntityID.
func (b *Battle) zoneStep(z *zone) []Event {
	amt := z.share()
	switch z.kind {
	case zoneAirstrike:
		evs := []Event{{Kind: EventEffectTick, Cell: z.centre, Value: amt}}
		var victims []*entity
		for i := range b.ents {
			if v := &b.ents[i]; playerFoe(v) && inZone(z.centre, z.r, v) {
				victims = append(victims, v)
			}
		}
		for _, v := range victims {
			evs = append(evs, b.hurt(0, v, amt)...)
		}
		return evs
	case zoneMedkit:
		evs := []Event{{Kind: EventEffectTick, Cell: z.centre, Value: amt}}
		for i := range b.ents {
			v := &b.ents[i]
			if v.owner != ownerAttacker || !v.alive || !v.onField || v.ud == nil || v.kind == KindDecoy || v.ud.Class != ClassINF || !inZone(z.centre, z.r, v) {
				continue
			}
			if n := min(amt, v.maxHP-v.hp); n > 0 {
				v.hp += n
				evs = append(evs, Event{Kind: EventHealed, Target: v.id, Cell: v.pos, Value: n})
			}
		}
		return evs
	}
	return nil
}

// endOfDefence is spec 7 step 5: finished zones go, finished decoys vanish (silently: a Killed event, no hurt).
func (b *Battle) endOfDefence() []Event {
	keep := b.zones[:0]
	for _, z := range b.zones {
		if z.last > b.turn {
			keep = append(keep, z)
		}
	}
	b.zones = keep
	var evs []Event
	for i := range b.ents {
		if e := &b.ents[i]; e.kind == KindDecoy && e.alive && e.decoyUntil > 0 && e.decoyUntil <= b.turn {
			evs = append(evs, b.kill(0, e)...)
		}
	}
	return evs
}
