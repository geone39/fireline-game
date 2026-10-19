package sim

import (
	"reflect"
	"testing"
)

// Property tests: random snapshots + random command logs. Rerunning gives an identical Result/StateHash,
// shuffling Snapshot.Buildings/Garrison changes nothing (spec 11.8), and the world invariants hold after
// every command.

type gen struct{ r *RNG }

func (g gen) n(k int) int       { return g.r.Intn(k) }
func (g gen) chance(p int) bool { return g.n(100) < p }

func (g gen) rimCell() Cell {
	if g.chance(50) {
		return cell([]int{0, 1, 42, 43}[g.n(4)], g.n(44))
	}
	return cell(g.n(44), []int{0, 1, 42, 43}[g.n(4)])
}

func (g gen) cell() Cell {
	if g.chance(70) {
		return g.rimCell()
	}
	return cell(g.n(44), g.n(44))
}

var propBuildings = []string{"d_wall", "d_wall", "d_wall", "d_wire", "d_mine", "d_mine", "d_spring_trap", "d_fence", "d_tank_trap", "d_mg_nest", "d_mortar", "b_hq", "b_vault", "x_statue", "d_bunker"}
var propUnits = []string{"u_rifleman", "u_brawler", "u_sniper", "u_medic", "u_para", "v_apc", "v_tank", "u_drone", "s_gunboat", "s_lander", "v_turret", "u_breacher", "u_grenadier", "u_flamer", "u_heavy", "v_buggy"}

func (g gen) snapshot() Snapshot {
	s := Snapshot{ContentVersion: "prop", BaseID: "p", Map: []string{"plain", "coastal"}[g.n(2)], Rules: fixtureRules()}
	for tries := 0; tries < 60; tries++ {
		id := propBuildings[g.n(len(propBuildings))]
		pbld := PlacedBuilding{ID: id, Level: 1 + g.n(3), Pos: g.buildPos(), Revealed: g.chance(30)}
		cand := s
		cand.Buildings = append(append([]PlacedBuilding(nil), s.Buildings...), pbld)
		if _, err := NewBattle(cand, 1); err == nil { // legal placement (inside the zone, no overlap, dry)
			s = cand
		}
	}
	for _, u := range propUnits {
		s.Army = append(s.Army, ArmySlot{ID: u, Level: 1 + g.n(2), Count: 1 + g.n(6)})
	}
	if g.chance(50) {
		s.Hero, s.HeroLevel = "h_commander", 1+g.n(3)
	}
	for k := g.n(4); k > 0; k-- {
		s.Garrison = append(s.Garrison, GarrisonUnit{"u_guard", 1 + g.n(2)})
	}
	return s
}

// buildPos favours the strip next to the landing rim so that the units meet traps and walls quickly.
func (g gen) buildPos() Cell {
	if g.chance(60) {
		if g.chance(50) {
			return cell(2+g.n(8), 2+g.n(38))
		}
		return cell(2+g.n(38), 2+g.n(8))
	}
	return cell(2+g.n(38), 2+g.n(38))
}

func (g gen) shuffled(s Snapshot) Snapshot {
	s.Buildings = append([]PlacedBuilding(nil), s.Buildings...)
	for i := len(s.Buildings) - 1; i > 0; i-- {
		j := g.n(i + 1)
		s.Buildings[i], s.Buildings[j] = s.Buildings[j], s.Buildings[i]
	}
	return s
}

// next builds a random command against the live battle (so a good share of them is legal).
func (g gen) next(b *Battle) Command {
	turn := b.turn
	if g.chance(5) {
		turn += g.n(4) - 1 // sometimes a bad / implicit turn
	}
	nEnt := len(b.ents)
	anyID := func() EntityID { return EntityID(g.n(nEnt + 3)) }
	ownUnit := func() EntityID {
		for tries := 0; tries < 8; tries++ {
			if e := b.ent(EntityID(1 + g.n(nEnt))); e != nil && e.owner == ownerAttacker {
				return e.id
			}
		}
		return anyID()
	}
	if b.ph == phaseDeploy {
		switch k := g.n(100); {
		case k < 70 || (k < 90 && len(b.ents) == nBuild(b)):
			slots := append([]string{b.snap.Hero}, propUnits...)
			id := slots[g.n(len(slots))]
			return Command{Turn: 0, Op: OpDeploy, ID: id, Level: 1 + g.n(3), To: g.cell()}
		case k < 78:
			return Command{Turn: 0, Op: OpLoad, Unit: ownUnit(), Target: ownUnit()}
		case k < 97:
			return Command{Turn: 0, Op: OpStart}
		case k < 98:
			return Command{Turn: 0, Op: OpRetreat}
		default:
			return Command{Turn: turn, Op: Op(g.n(9)), Unit: anyID(), To: g.cell()}
		}
	}
	if g.chance(45) { // a purposeful player: shoot what is in range, else walk towards the nearest target
		if e := b.ent(ownUnit()); e != nil && e.ud != nil && e.alive && e.onField && e.apLeft > 0 {
			if ts := b.targetsInRange(e); len(ts) > 0 && e.ud.canAttack() {
				return Command{Turn: turn, Op: OpAttack, Unit: e.id, Target: ts[g.n(len(ts))]}
			}
			if p, _, ok := b.approach(e); ok && len(p) > 0 && e.ud.MoveCells > 0 {
				return Command{Turn: turn, Op: OpMove, Unit: e.id, Path: p}
			}
		}
	}
	switch k := g.n(100); {
	case k < 30: // Attack: mostly at something in range, sometimes at anything
		u := ownUnit()
		c := Command{Turn: turn, Op: OpAttack, Unit: u, Target: anyID()}
		if e := b.ent(u); e != nil && e.ud != nil && g.chance(70) {
			if ts := b.targetsInRange(e); len(ts) > 0 {
				c.Target = ts[g.n(len(ts))]
			}
		}
		return c
	case k < 78:
		u := ownUnit()
		c := Command{Turn: turn, Op: OpMove, Unit: u}
		e := b.ent(u)
		if e == nil || e.ud == nil {
			return c
		}
		cur := e.pos
		aim := e.pos // walk towards a random defender entity part of the time
		if g.chance(50) {
			if t := b.ent(EntityID(1 + g.n(max(1, nBuild(b))))); t != nil {
				aim = t.pos
			}
		}
		sign := func(v int) int {
			if v < 0 {
				return -1
			}
			if v > 0 {
				return 1
			}
			return 0
		}
		for steps := 1 + g.n(max(1, e.ud.MoveCells)+g.n(2)); steps > 0; steps-- {
			var to Cell
			if g.chance(95) { // a legal-looking neighbour
				d := dirs[g.n(8)]
				if aim != e.pos && g.chance(80) {
					d = [2]int{sign(int(aim.X) - int(cur.X)), sign(int(aim.Y) - int(cur.Y))}
				}
				to = cell(int(cur.X)+d[0], int(cur.Y)+d[1])
				if b.inMap(to) && b.canStep(e, cur, to) != 0 {
					continue
				}
			} else {
				to = g.cell()
			}
			c.Path = append(c.Path, to)
			cur = to
		}
		return c
	case k < 96:
		return Command{Turn: turn, Op: OpEndTurn}
	case k < 97:
		return Command{Turn: turn, Op: OpRetreat}
	default: // out-of-phase deploy-type commands
		return Command{Turn: turn, Op: []Op{OpDeploy, OpLoad, OpStart}[g.n(3)], Unit: anyID(), ID: "u_rifleman", Level: 1, To: g.cell()}
	}
}

// nBuild is the number of defender entities (buildings + garrison), i.e. the first attacker id - 1.
func nBuild(b *Battle) int {
	n := 0
	for i := range b.ents {
		if b.ents[i].owner == ownerDefender {
			n++
		}
	}
	return n
}

func checkInvariants(t *testing.T, b *Battle) {
	t.Helper()
	for i := range b.ents {
		e := &b.ents[i]
		if (e.hp == 0) != !e.alive || (!e.alive && ((e.ud != nil && e.onField) || e.aboard != 0)) {
			t.Fatalf("entity %d: hp %d alive %v onField %v aboard %d", e.id, e.hp, e.alive, e.onField, e.aboard)
		}
	}
	seenU := make(map[EntityID]bool) // test code: lookups only
	for i := range b.ents {
		e := &b.ents[i]
		if e.apLeft > e.ap || e.hp > e.maxHP || e.hp < 0 {
			t.Fatalf("entity %d: ap %d/%d hp %d/%d", e.id, e.apLeft, e.ap, e.hp, e.maxHP)
		}
		if e.ud != nil && e.onField {
			if got := b.layer(e.class())[b.idx(e.pos)]; got != e.id {
				t.Fatalf("entity %d at %v not in its layer (%d)", e.id, e.pos, got)
			}
			ok, occ := b.cellState(e.class(), e.pos, e.id)
			if !ok || occ {
				t.Fatalf("entity %d stands on an illegal cell %v (ok=%v occ=%v)", e.id, e.pos, ok, occ)
			}
			if seenU[e.id] {
				t.Fatal("dup")
			}
			seenU[e.id] = true
		}
		if e.aboard != 0 {
			if e.onField || b.ent(e.aboard) == nil {
				t.Fatalf("passenger %d inconsistent", e.id)
			}
			found := false
			for _, p := range b.ent(e.aboard).cargo {
				found = found || p == e.id
			}
			if !found {
				t.Fatalf("passenger %d missing from carrier cargo", e.id)
			}
		}
		if e.ud != nil && len(e.cargo) > e.ud.Carries {
			t.Fatalf("carrier %d over capacity", e.id)
		}
	}
	for _, grid := range [][]EntityID{b.unit, b.air} {
		for i, id := range grid {
			if id == 0 {
				continue
			}
			e := b.ent(id)
			if e == nil || !e.onField || b.idx(e.pos) != i {
				t.Fatalf("grid cell %d points at a stale entity %d", i, id)
			}
		}
	}
}

func replay(t *testing.T, snap Snapshot, cmds []Command, seed uint64) Result {
	t.Helper()
	r, err := Run(snap, cmds, seed)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestPropertyRunTwiceSameResult1000(t *testing.T) {
	accepted, rejected := 0, 0
	codes := map[RejectCode]int{}
	evCount := map[EventKind]int{}
	ends := map[EndReason]int{}
	for seed := uint64(1); seed <= 1000; seed++ {
		g := gen{NewRNG(seed * 7919)}
		snap := g.snapshot()
		b, err := NewBattle(snap, seed)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		var cmds []Command
		for i := 0; i < 300 && b.ph != phaseOver; i++ {
			c := g.next(b)
			evs, err := b.Apply(c)
			if err != nil {
				t.Fatalf("seed %d: %v", seed, err)
			}
			for _, e := range evs {
				evCount[e.Kind]++
			}
			cmds = append(cmds, c)
			checkInvariants(t, b)
		}
		accepted += len(cmds) - len(b.rejected)
		rejected += len(b.rejected)
		for _, r := range b.rejected {
			codes[r.Code]++
		}

		ends[b.endReason]++
		r1 := replay(t, snap, cmds, seed)
		r2 := replay(t, snap, cmds, seed)
		if !reflect.DeepEqual(r1, r2) || r1.StateHash != r2.StateHash {
			t.Fatalf("seed %d: two runs differ:\n%+v\n%+v", seed, r1, r2)
		}
		// incremental form == Run
		b2, _ := NewBattle(snap, seed)
		for _, c := range cmds {
			b2.Apply(c)
		}
		if b2.ph != phaseOver {
			b2.finish(EndLogEnd)
		}
		if r3 := b2.Result(); !reflect.DeepEqual(r1, r3) {
			t.Fatalf("seed %d: Run != Apply loop:\n%+v\n%+v", seed, r1, r3)
		}
		// a snapshot with shuffled building order plays identically (spec 11.8)
		if r4 := replay(t, g.shuffled(snap), cmds, seed); !reflect.DeepEqual(r1, r4) {
			t.Fatalf("seed %d: Snapshot.Buildings order leaks into the result:\n%+v\n%+v", seed, r1, r4)
		}
		// a different seed must not change SIM-1 rules (no randomness yet) except through the hashed RNG state
		if r5 := replay(t, snap, cmds, seed+1); r5.StateHash == r1.StateHash {
			t.Fatalf("seed %d: the seed is not part of StateHash", seed)
		}
	}
	if accepted < 10000 || rejected < 10000 || len(codes) < 12 {
		t.Fatalf("generator is too one-sided: accepted %d rejected %d codes %v", accepted, rejected, codes)
	}
	if evCount[EventAttacked] < 1000 || evCount[EventDestroyed] < 100 || evCount[EventKilled] < 30 || evCount[EventTrapTriggered] < 30 {
		t.Fatalf("the generator does not reach combat: %v", evCount)
	}
	t.Logf("accepted %d, rejected %d, %d distinct codes, events %v, endings %v", accepted, rejected, len(codes), evCount, ends)
}
