package sim

import (
	"fmt"
	"sort"
)

const maxLevel = 100 // sanity bound for Level fields (level_growth loops)

type phase uint8

const (
	phaseDeploy phase = iota
	phasePlayer
	phaseOver
)

type owner uint8

const (
	ownerDefender owner = iota
	ownerAttacker
)

// entity is one battle object. Entities live in Battle.ents at index id-1 and are never removed.
type entity struct {
	id        EntityID
	kind      EntityKind
	owner     owner
	contentID string
	level     int
	pos       Cell // top-left tile
	w, h      int
	hp        int
	maxHP     int
	ap        int
	apLeft    int
	cd        int
	alive     bool
	onField   bool     // occupies grid cells (false: inside a bunker / aboard a carrier / not yet released)
	aboard    EntityID // carrier, if boarded
	inside    EntityID // bunker holding a garrison unit
	cargo     []EntityID
	revealed  bool
	slot      int // army slot index for losses, -1 for hero / defender
	ud        *UnitDef
	bd        *BuildingDef
}

func (e *entity) class() UnitClass {
	if e.ud != nil {
		return e.ud.Class
	}
	return ClassINF
}

// Battle is the incremental form of Run for the interactive client: same code, same rules.
type Battle struct {
	snap  Snapshot
	rules *Rules
	rng   *RNG

	w, h     int
	water    []bool
	bld      []EntityID // building footprints
	unit     []EntityID // ground + sea units (water and land are disjoint)
	air      []EntityID
	ents     []entity
	unitIdx  map[string]int // lookup only, never ranged
	bldIdx   map[string]int
	stock    []ArmySlot // remaining army, Count decreases on Deploy
	heroUsed bool
	heroID   EntityID

	ph        phase
	turn      int
	endReason EndReason
	nCmd      int // commands seen (Rejection.Index)
	rejected  []Rejection
	heroDmg   int

	pf pathfinder
}

// NewBattle prepares a battle in phase 0 (deployment).
func NewBattle(snap Snapshot, seed uint64) (*Battle, error) {
	r := &snap.Rules
	if err := r.validate(); err != nil {
		return nil, err
	}
	b := &Battle{snap: snap, rules: r, rng: NewRNG(seed), w: r.GridW, h: r.GridH}
	b.unitIdx = make(map[string]int, len(r.Units))
	for i := range r.Units {
		b.unitIdx[r.Units[i].ID] = i
	}
	b.bldIdx = make(map[string]int, len(r.Buildings))
	for i := range r.Buildings {
		b.bldIdx[r.Buildings[i].ID] = i
	}
	n := b.w * b.h
	b.water = make([]bool, n)
	b.bld = make([]EntityID, n)
	b.unit = make([]EntityID, n)
	b.air = make([]EntityID, n)

	// terrain
	found := false
	for i := range r.Maps {
		if r.Maps[i].ID != snap.Map {
			continue
		}
		found = true
		for _, rc := range r.Maps[i].Water {
			for y := rc.Y; y < rc.Y+rc.H; y++ {
				for x := rc.X; x < rc.X+rc.W; x++ {
					if x >= 0 && y >= 0 && x < b.w && y < b.h {
						b.water[y*b.w+x] = true
					}
				}
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: unknown map %q", ErrBadSnapshot, snap.Map)
	}

	// defender buildings in canonical order (Y, X, ID) - spec 2.4; input order is irrelevant
	order := make([]PlacedBuilding, len(snap.Buildings))
	copy(order, snap.Buildings)
	sort.Slice(order, func(i, j int) bool {
		a, c := order[i], order[j]
		if a.Pos.Y != c.Pos.Y {
			return a.Pos.Y < c.Pos.Y
		}
		if a.Pos.X != c.Pos.X {
			return a.Pos.X < c.Pos.X
		}
		if a.ID != c.ID {
			return a.ID < c.ID
		}
		if a.Level != c.Level {
			return a.Level < c.Level
		}
		return !a.Revealed && c.Revealed
	})
	for _, pb := range order {
		di, ok := b.bldIdx[pb.ID]
		if !ok {
			return nil, fmt.Errorf("%w: unknown building %q", ErrBadSnapshot, pb.ID)
		}
		if pb.Level < 1 || pb.Level > maxLevel {
			return nil, fmt.Errorf("%w: building %q level %d", ErrBadSnapshot, pb.ID, pb.Level)
		}
		d := &r.Buildings[di]
		id := EntityID(len(b.ents) + 1)
		e := entity{id: id, kind: d.Kind, owner: ownerDefender, contentID: d.ID, level: pb.Level, pos: pb.Pos,
			w: d.W, h: d.H, alive: true, onField: true, slot: -1, bd: d, revealed: pb.Revealed}
		e.maxHP = scaledHP(d.HP, d.HPGrowthPermille, pb.Level)
		e.hp = e.maxHP
		for y := int(pb.Pos.Y); y < int(pb.Pos.Y)+d.H; y++ {
			for x := int(pb.Pos.X); x < int(pb.Pos.X)+d.W; x++ {
				c := Cell{int16(x), int16(y)}
				if !r.BuildZone.Contains(c) {
					return nil, fmt.Errorf("%w: building %q at %v leaves the build zone", ErrBadSnapshot, pb.ID, pb.Pos)
				}
				i := y*b.w + x
				if x >= b.w || y >= b.h || b.water[i] {
					return nil, fmt.Errorf("%w: building %q at %v on water or off map", ErrBadSnapshot, pb.ID, pb.Pos)
				}
				if b.bld[i] != 0 {
					return nil, fmt.Errorf("%w: building %q at %v overlaps another", ErrBadSnapshot, pb.ID, pb.Pos)
				}
				b.bld[i] = id
			}
		}
		b.ents = append(b.ents, e)
	}

	// garrison: bunkers in id order take units in list order (spec 7.2); extra units are lost. Units stay
	// inside (not on the grid) until SIM-3 releases them; ids are fixed here to keep numbering canonical.
	gi := 0
	nb := len(b.ents)
	for bi := 0; bi < nb && gi < len(snap.Garrison); bi++ {
		bkID, capacity := b.ents[bi].id, b.ents[bi].bd.Garrison
		for k := 0; k < capacity && gi < len(snap.Garrison); k++ {
			g := snap.Garrison[gi]
			gi++
			ui, ok := b.unitIdx[g.ID]
			if !ok || g.Level < 1 || g.Level > maxLevel {
				return nil, fmt.Errorf("%w: garrison unit %q level %d", ErrBadSnapshot, g.ID, g.Level)
			}
			u := &r.Units[ui]
			e := entity{id: EntityID(len(b.ents) + 1), kind: unitKind(u), owner: ownerDefender, contentID: u.ID,
				level: g.Level, w: 1, h: 1, ap: u.AP, alive: true, inside: bkID, slot: -1, ud: u}
			e.maxHP = scaledHP(u.HP, u.HPGrowthPermille, g.Level)
			e.hp = e.maxHP
			b.ents = append(b.ents, e)
		}
	}
	for gi < len(snap.Garrison) { // validate dropped ones too
		g := snap.Garrison[gi]
		gi++
		if _, ok := b.unitIdx[g.ID]; !ok || g.Level < 1 || g.Level > maxLevel {
			return nil, fmt.Errorf("%w: garrison unit %q level %d", ErrBadSnapshot, g.ID, g.Level)
		}
	}

	// attacker stock
	b.stock = make([]ArmySlot, len(snap.Army))
	copy(b.stock, snap.Army)
	for _, s := range snap.Army {
		if _, ok := b.unitIdx[s.ID]; !ok || s.Level < 1 || s.Level > maxLevel || s.Count < 0 {
			return nil, fmt.Errorf("%w: army slot %q level %d count %d", ErrBadSnapshot, s.ID, s.Level, s.Count)
		}
	}
	if snap.Hero != "" {
		ui, ok := b.unitIdx[snap.Hero]
		if !ok || !r.Units[ui].Hero || snap.HeroLevel < 1 || snap.HeroLevel > maxLevel {
			return nil, fmt.Errorf("%w: hero %q level %d", ErrBadSnapshot, snap.Hero, snap.HeroLevel)
		}
	}
	return b, nil
}

func unitKind(u *UnitDef) EntityKind {
	switch {
	case u.Hero:
		return KindHero
	case u.Class == ClassVEH:
		return KindVehicle
	case u.Class == ClassSEA:
		return KindShip
	}
	return KindTroop
}

func (b *Battle) ent(id EntityID) *entity {
	if id == 0 || int(id) > len(b.ents) {
		return nil
	}
	return &b.ents[id-1]
}

func (b *Battle) inMap(c Cell) bool {
	return c.X >= 0 && c.Y >= 0 && int(c.X) < b.w && int(c.Y) < b.h
}

func (b *Battle) idx(c Cell) int { return int(c.Y)*b.w + int(c.X) }

func (b *Battle) cellOf(i int) Cell { return Cell{int16(i % b.w), int16(i / b.w)} }

// layer returns the occupancy grid a unit of this class lives in.
func (b *Battle) layer(c UnitClass) []EntityID {
	if c == ClassAIR {
		return b.air
	}
	return b.unit
}

func (b *Battle) place(e *entity, c Cell) {
	e.pos = c
	e.onField = true
	b.layer(e.class())[b.idx(c)] = e.id
}

func (b *Battle) lift(e *entity) {
	if e.onField {
		b.layer(e.class())[b.idx(e.pos)] = 0
		e.onField = false
	}
}

// cellFree reports whether a unit of class cl (other than self) may stand on c.
// ok=false means impassable terrain/building; occupied=true means another unit stands there.
func (b *Battle) cellState(cl UnitClass, c Cell, self EntityID) (ok, occupied bool) {
	if !b.inMap(c) {
		return false, false
	}
	i := b.idx(c)
	switch cl {
	case ClassAIR:
		if o := b.air[i]; o != 0 && o != self {
			return true, true
		}
		return true, false
	case ClassSEA:
		if !b.water[i] {
			return false, false
		}
	default:
		if b.water[i] {
			return false, false
		}
		if id := b.bld[i]; id != 0 {
			e := &b.ents[id-1]
			if e.alive {
				switch e.bd.Block {
				case BlockAll:
					return false, false
				case BlockVehicles:
					if cl == ClassVEH {
						return false, false
					}
				}
			}
		}
	}
	if o := b.unit[i]; o != 0 && o != self {
		return true, true
	}
	return true, false
}

func (b *Battle) blockedFor(cl UnitClass, c Cell, self EntityID) bool {
	ok, occ := b.cellState(cl, c, self)
	return !ok || occ
}

// stepCost is the cost of entering c: 1, or ceil(100/(100-slow)) on a live slowing building for INF (spec 5.1).
func (b *Battle) stepCost(u *entity, c Cell) int {
	if u.class() == ClassINF {
		if id := b.bld[b.idx(c)]; id != 0 {
			e := &b.ents[id-1]
			if e.alive && e.bd.SlowPct > 0 {
				d := 100 - e.bd.SlowPct
				return (100 + d - 1) / d
			}
		}
	}
	return 1
}

// canStep checks one 8-neighbour step from -> to for unit u (spec 2.3): cell passable and free, diagonal
// "staircase" squeeze forbidden for non-air units.
func (b *Battle) canStep(u *entity, from, to Cell) RejectCode {
	cl := u.class()
	ok, occ := b.cellState(cl, to, u.id)
	if !ok {
		return RejectBadPath
	}
	if occ {
		return RejectCellOccupied
	}
	if cl != ClassAIR && from.X != to.X && from.Y != to.Y {
		if b.blockedFor(cl, Cell{to.X, from.Y}, u.id) && b.blockedFor(cl, Cell{from.X, to.Y}, u.id) {
			return RejectBadPath
		}
	}
	return 0
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// Cheb is the Chebyshev distance (spec 2.2).
func Cheb(a, b Cell) int {
	return max(abs(int(a.X)-int(b.X)), abs(int(a.Y)-int(b.Y)))
}

// footDist is the Chebyshev distance between the footprints of two entities (cell-to-building: min over the
// building's cells), spec 2.2.
func footDist(a, b *entity) int {
	gap := func(a0, aw, b0, bw int) int {
		return max(0, max(b0-(a0+aw-1), a0-(b0+bw-1)))
	}
	return max(gap(int(a.pos.X), a.w, int(b.pos.X), b.w), gap(int(a.pos.Y), a.h, int(b.pos.Y), b.h))
}
