package sim

import "testing"

// Unit tests of the defence phase (SIM-3, spec 7). Numbers come from the test fixture, not from balance.

func endT(t *testing.T, b *Battle, turn int) []Event {
	t.Helper()
	evs, err := b.Apply(end(turn))
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func kinds(evs []Event) []EventKind {
	out := make([]EventKind, len(evs))
	for i, e := range evs {
		out[i] = e.Kind
	}
	return out
}

// TestDefenceF01Table pins the spec 12 table: the nest hits the brawler every turn from turn 1 (80, 60, 40).
func TestDefenceF01Table(t *testing.T) {
	var sc scenario
	for _, s := range scenarios() {
		if s.name == "G06" {
			sc = s
		}
	}
	b, err := NewBattle(sc.snap, sc.seed)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]int{1: 80, 2: 60, 3: 40}
	for _, c := range sc.cmds {
		if c.Turn > 3 {
			break
		}
		b.Apply(c)
		if c.Op == OpEndTurn {
			if hpOf(b, 4) != want[c.Turn] {
				t.Fatalf("after turn %d the brawler has %d hp, want %d", c.Turn, hpOf(b, 4), want[c.Turn])
			}
		}
	}
	if hpOf(b, 1) != 50 {
		t.Fatalf("nest hp %d, want 50", hpOf(b, 1))
	}
}

func TestDefenceRangeAndClasses(t *testing.T) {
	b := fight(t, []PlacedBuilding{pb("d_mg_nest", 12, 10)}, nil, "u_rifleman", "u_rifleman", "v_tank", "u_drone")
	// ids: nest 1; riflemen 2, 3; tank 4; drone 5
	tele(b, 2, 5, 10) // dist 7: in range
	tele(b, 3, 4, 10) // dist 8: out
	tele(b, 4, 11, 12)
	evs := endT(t, b, 1)
	if hpOf(b, 2) != 30 || hpOf(b, 3) != 50 || hpOf(b, 4) != 1100 || hpOf(b, 5) != 70 {
		t.Fatalf("hp %d %d %d %d", hpOf(b, 2), hpOf(b, 3), hpOf(b, 4), hpOf(b, 5))
	}
	if len(evs) != 2 || evs[1].Kind != EventAttacked || evs[1].Actor != 1 || evs[1].Target != 2 {
		t.Fatalf("events %+v", evs)
	}
	// the drone is a class the nest shoots: nearer than the rifleman
	tele(b, 5, 8, 10) // dist 4
	endT(t, b, 2)
	if hpOf(b, 5) != 50 || hpOf(b, 2) != 30 {
		t.Fatalf("drone %d rifleman %d", hpOf(b, 5), hpOf(b, 2))
	}
}

func TestDefenceMinRangeAndSplash(t *testing.T) {
	b := fight(t, []PlacedBuilding{pb("d_mortar", 6, 20)}, func(s *Snapshot) {
		tweakBuilding(s, "d_mortar", func(d *BuildingDef) { d.SplashCells = 1 })
	}, "u_rifleman", "u_rifleman", "u_rifleman")
	// ids: mortar 1; riflemen 2, 3, 4
	tele(b, 2, 4, 20) // dist 2 < min_range 3
	tele(b, 3, 3, 22) // dist 3
	tele(b, 4, 3, 23) // dist 3, next to 3
	endT(t, b, 1)
	if hpOf(b, 2) != 50 || hpOf(b, 3) != 28 || hpOf(b, 4) != 37 { // 22, and 22 x (100-40)/100 = 13 by the splash
		t.Fatalf("hp %d %d %d", hpOf(b, 2), hpOf(b, 3), hpOf(b, 4))
	}
}

func TestDefenceTeslaChain(t *testing.T) {
	b := fight(t, []PlacedBuilding{pb("d_tesla", 10, 10)}, nil, "u_rifleman", "u_rifleman", "v_tank", "u_brawler", "u_rifleman", "u_drone", "u_rifleman")
	// ids: tesla 1; riflemen 2, 3; tank 4; brawler 5; rifleman 6; drone 7; rifleman 8
	for id, c := range map[EntityID]Cell{2: cell(4, 10), 3: cell(4, 12), 4: cell(3, 14), 5: cell(5, 15), 6: cell(3, 16), 7: cell(4, 11), 8: cell(9, 20)} {
		tele(b, id, int(c.X), int(c.Y))
	}
	endT(t, b, 1)
	// chain: 2 (nearest, lowest id of the 6-cell tie) -> 3 -> tank -> brawler; 6 is the fifth, 7 is air, 8 is 8 cells off
	got := []int{hpOf(b, 2), hpOf(b, 3), hpOf(b, 4), hpOf(b, 5), hpOf(b, 6), hpOf(b, 7), hpOf(b, 8)}
	want := []int{38, 38, 1088, 88, 50, 70, 50}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("hp %v, want %v", got, want)
		}
	}
	// a jump longer than chain_cells ends the arc
	tele(b, 3, 4, 14) // 4 from unit 2: out of the 3-cell jump
	tele(b, 4, 4, 18)
	tele(b, 5, 9, 20)
	tele(b, 6, 9, 25)
	endT(t, b, 2)
	if hpOf(b, 2) != 26 || hpOf(b, 3) != 38 || hpOf(b, 4) != 1088 {
		t.Fatalf("second arc: %d %d %d", hpOf(b, 2), hpOf(b, 3), hpOf(b, 4))
	}
}

func TestDefenceWireDot(t *testing.T) {
	b := fight(t, []PlacedBuilding{pb("d_wire", 6, 6), pb("d_wire", 6, 8), pb("d_wire", 6, 10)}, nil, "u_rifleman", "v_tank", "u_drone")
	// ids: wires 1, 2, 3; rifleman 4; tank 5; drone 6
	tele(b, 4, 6, 6)
	tele(b, 5, 6, 8)
	tele(b, 6, 6, 10)
	evs := endT(t, b, 1)
	if hpOf(b, 4) != 46 || hpOf(b, 5) != 1100 || hpOf(b, 6) != 70 {
		t.Fatalf("hp %d %d %d", hpOf(b, 4), hpOf(b, 5), hpOf(b, 6))
	}
	if len(evs) != 3 || evs[1].Kind != EventEffectTick || evs[1].Actor != 1 || evs[1].Value != 4 || evs[2].Kind != EventAttacked {
		t.Fatalf("events %+v", evs)
	}
	b.hurt(0, b.ent(1), 90) // the wire is shot away: no more damage
	endT(t, b, 2)
	if hpOf(b, 4) != 46 {
		t.Fatalf("a destroyed wire still hurts: %d", hpOf(b, 4))
	}
}

func TestDefenceWireKillsAndEnds(t *testing.T) {
	b := fight(t, []PlacedBuilding{pb("d_wire", 6, 6)}, nil, "u_rifleman")
	tele(b, 2, 6, 6)
	b.ent(2).hp = 3
	evs := endT(t, b, 1)
	if b.ph != phaseOver || b.endReason != EndNoUnits || evs[len(evs)-1].Kind != EventBattleEnded {
		t.Fatalf("phase %d reason %d events %+v", b.ph, b.endReason, evs)
	}
}

// TestDefenceOrder: bunker release, wire, buildings, garrison - in this order, within each step by EntityID.
func TestDefenceOrder(t *testing.T) {
	b := fight(t, []PlacedBuilding{pb("d_wire", 9, 6), pb("d_mg_nest", 12, 6), pb("d_bunker", 8, 8)}, func(s *Snapshot) {
		s.Garrison = []GarrisonUnit{{"u_scout", 1}}
	}, "u_rifleman")
	// ids: wire 1, nest 2, bunker 3; scout 4; rifleman 5
	tele(b, 5, 9, 6)
	evs := endT(t, b, 1)
	type ea struct {
		k EventKind
		a EntityID
	}
	var got []ea
	for _, e := range evs {
		got = append(got, ea{e.Kind, e.Actor})
	}
	want := []ea{{EventTurnEnded, 0}, {EventReleased, 4}, {EventEffectTick, 1}, {EventAttacked, 1}, {EventAttacked, 2}, {EventAttacked, 4}, {EventAttacked, 4}}
	if len(got) != len(want) {
		t.Fatalf("events %+v", evs)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events %+v, want %+v", got, want)
		}
	}
	if hpOf(b, 5) != 50-4-20-6-6 {
		t.Fatalf("rifleman hp %d", hpOf(b, 5))
	}
}

// TestOverheat: 3 turns of fire, 2 dark, again; a turn without a target restarts the count; a jammed trap neither
// fires nor cools.
func TestOverheat(t *testing.T) {
	b := fight(t, []PlacedBuilding{pb("d_flame_trap", 4, 10)}, nil, "u_heavy")
	tele(b, 2, 1, 10)
	var hits []int
	prev := hpOf(b, 2)
	for turn := 1; turn <= 10; turn++ {
		endT(t, b, turn)
		hits = append(hits, prev-hpOf(b, 2))
		prev = hpOf(b, 2)
	}
	want := []int{15, 15, 15, 0, 0, 15, 15, 15, 0, 0}
	for i := range want {
		if hits[i] != want[i] {
			t.Fatalf("damage per turn %v, want %v", hits, want)
		}
	}

	b = fight(t, []PlacedBuilding{pb("d_flame_trap", 4, 10)}, nil, "u_heavy")
	tele(b, 2, 1, 10)
	endT(t, b, 1)
	endT(t, b, 2)
	if b.ent(1).heat != 2 {
		t.Fatalf("heat %d", b.ent(1).heat)
	}
	tele(b, 2, 1, 20) // out of reach: the count restarts
	endT(t, b, 3)
	if b.ent(1).heat != 0 || b.ent(1).cd != 0 {
		t.Fatalf("heat %d cd %d", b.ent(1).heat, b.ent(1).cd)
	}
	tele(b, 2, 1, 10)
	endT(t, b, 4)
	endT(t, b, 5)
	if b.ent(1).cd != 0 {
		t.Fatalf("overheated after only 2 turns: cd %d", b.ent(1).cd)
	}
	b.ent(1).disabledUntil = 6 // jammed: no fire, no cooling
	before := hpOf(b, 2)
	endT(t, b, 6)
	if hpOf(b, 2) != before || b.ent(1).heat != 2 {
		t.Fatalf("a jammed trap acted: hp %d heat %d", hpOf(b, 2), b.ent(1).heat)
	}
}

func bunkerBattle(t *testing.T, mut func(*Snapshot), units ...string) *Battle {
	return fight(t, []PlacedBuilding{pb("d_bunker", 20, 20)}, func(s *Snapshot) {
		s.Garrison = []GarrisonUnit{{"u_scout", 1}}
		if mut != nil {
			mut(s)
		}
	}, units...)
}

// firstUnits returns the first garrison unit and the first attacker unit (ids follow the buildings).
func firstUnits(b *Battle) (*entity, *entity) {
	var g, a *entity
	for i := range b.ents {
		e := &b.ents[i]
		if e.ud != nil && g == nil && e.owner == ownerDefender {
			g = e
		}
		if e.ud != nil && a == nil && e.owner == ownerAttacker {
			a = e
		}
	}
	return g, a
}

func TestBunkerTriggerByProximity(t *testing.T) {
	b := bunkerBattle(t, nil, "u_rifleman")
	// ids: bunker 1; scout 2; rifleman 3. The default reach is the scout's range (3)
	tele(b, 3, 16, 21) // 4 cells from the footprint
	endT(t, b, 1)
	if b.ent(2).onField || b.ent(2).inside != 1 {
		t.Fatal("released too early")
	}
	tele(b, 3, 17, 21) // 3 cells
	evs := endT(t, b, 2)
	if !b.ent(2).onField || b.ent(2).inside != 0 || b.ent(2).pos != cell(19, 19) {
		t.Fatalf("scout %+v", b.ent(2))
	}
	if evs[1].Kind != EventReleased || evs[1].Actor != 2 || evs[1].Target != 1 || evs[1].Cell != cell(19, 19) {
		t.Fatalf("events %+v", evs)
	}
	if hpOf(b, 3) != 38 { // it acts in the same phase: 2 AP, 2 shots of 6
		t.Fatalf("rifleman hp %d", hpOf(b, 3))
	}

	b = bunkerBattle(t, func(s *Snapshot) { tweakBuilding(s, "d_bunker", func(d *BuildingDef) { d.TriggerCells = 5 }) }, "u_rifleman")
	tele(b, 3, 15, 21) // 5 cells: inside the explicit trigger_cells
	endT(t, b, 1)
	if !b.ent(2).onField {
		t.Fatal("trigger_cells ignored")
	}
}

func TestBunkerTriggerByDamageEvenWhenDestroyed(t *testing.T) {
	b := bunkerBattle(t, nil, "u_rifleman")
	tele(b, 3, 2, 2)
	b.hurt(3, b.ent(1), b.ent(1).hp) // destroyed by the player
	endT(t, b, 1)
	if !b.ent(2).onField {
		t.Fatal("the garrison of a destroyed bunker stays inside")
	}
}

func TestBunkerJammedNoPlaceAndTrapsIgnored(t *testing.T) {
	b := bunkerBattle(t, nil, "u_rifleman")
	tele(b, 3, 17, 21)
	b.ent(1).disabledUntil = 1
	endT(t, b, 1)
	if b.ent(2).onField {
		t.Fatal("a jammed bunker released its garrison")
	}
	b.ent(1).disabledUntil = 0
	// ring full of walls: no room, the unit waits; one wall gone: it comes out next turn
	b = bunkerBattle(t, func(s *Snapshot) {
		for y := 19; y <= 23; y++ {
			for x := 19; x <= 23; x++ {
				if x == 19 || x == 23 || y == 19 || y == 23 {
					s.Buildings = append(s.Buildings, pb("d_wall", x, y))
				}
			}
		}
	}, "u_rifleman")
	sc, rf := firstUnits(b)
	tele(b, rf.id, 17, 21)
	endT(t, b, 1)
	if sc.onField {
		t.Fatal("released into a full ring")
	}
	b.hurt(0, b.ent(b.bld[b.idx(cell(19, 21))]), 900)
	endT(t, b, 2)
	if !sc.onField || sc.pos != cell(19, 21) {
		t.Fatalf("scout %+v", sc)
	}

	// hidden traps do not fire on defenders: the scout is released onto a mine and walks off it
	b = bunkerBattle(t, func(s *Snapshot) { s.Buildings = append(s.Buildings, pb("d_mine", 19, 19)) }, "u_brawler")
	tele(b, 4, 19, 17)
	endT(t, b, 1)
	if !b.ent(1).alive || b.ent(1).hp != b.ent(1).maxHP {
		t.Fatal("the mine hurt its own side")
	}
}

func TestGarrisonWalksAndShoots(t *testing.T) {
	b := bunkerBattle(t, nil, "u_brawler")
	// ids: bunker 1; scout 2; brawler 3
	b.hurt(3, b.ent(1), 1) // opens the bunker
	tele(b, 3, 20, 30)
	endT(t, b, 1)
	s := b.ent(2)
	if s.pos == cell(19, 19) && !s.onField {
		t.Fatal("not released")
	}
	// one walk of move_cells (3) towards the target, then no target in reach: the second AP is not a second walk
	if Cheb(cell(19, 19), s.pos) != 3 || Cheb(s.pos, cell(20, 30)) != Cheb(cell(19, 19), cell(20, 30))-3 {
		t.Fatalf("scout at %v", s.pos)
	}

	// target 6 cells away: walks to the best standing cell (17,22) and shoots with the second AP
	b = bunkerBattle(t, nil, "u_brawler")
	b.hurt(3, b.ent(1), 1)
	tele(b, 3, 20, 25)
	endT(t, b, 1)
	if b.ent(2).pos != cell(17, 22) || hpOf(b, 3) != 94 {
		t.Fatalf("scout at %v, brawler %d", b.ent(2).pos, hpOf(b, 3))
	}
}

func TestGarrisonBoxedInAndSupportStay(t *testing.T) {
	// the only free ring cell (19,19) is a dead end: the unit comes out but cannot go anywhere
	boxed := func(s *Snapshot) {
		for y := 19; y <= 23; y++ {
			for x := 19; x <= 23; x++ {
				if (x == 19 || x == 23 || y == 19 || y == 23) && (x != 19 || y != 19) {
					s.Buildings = append(s.Buildings, pb("d_wall", x, y))
				}
			}
		}
		s.Buildings = append(s.Buildings, pb("d_wall", 18, 18), pb("d_wall", 19, 18), pb("d_wall", 20, 18), pb("d_wall", 18, 19), pb("d_wall", 18, 20))
	}
	b := bunkerBattle(t, boxed, "u_brawler")
	sc, br := firstUnits(b)
	b.hurt(br.id, b.ent(b.bld[b.idx(cell(20, 20))]), 1)
	tele(b, br.id, 30, 30)
	endT(t, b, 1)
	if !sc.onField || sc.pos != cell(19, 19) {
		t.Fatalf("scout %v onField %v", sc.pos, sc.onField)
	}
	endT(t, b, 2)
	if sc.pos != cell(19, 19) {
		t.Fatalf("a boxed-in scout moved to %v", sc.pos)
	}

	// support units (no attack) are released but do nothing
	b = bunkerBattle(t, func(s *Snapshot) {
		s.Garrison = []GarrisonUnit{{"u_medic", 1}}
		tweakBuilding(s, "d_bunker", func(d *BuildingDef) { d.TriggerCells = 3 }) // a medic has no range to take it from
	}, "u_rifleman")
	tele(b, 3, 17, 21)
	endT(t, b, 1)
	if !b.ent(2).onField || b.ent(2).pos != cell(19, 19) || hpOf(b, 3) != 50 {
		t.Fatalf("medic %+v rifleman %d", b.ent(2).pos, hpOf(b, 3))
	}
}

func TestDefenceEndsAndTurnLimit(t *testing.T) {
	b := fight(t, []PlacedBuilding{pb("d_mg_nest", 12, 10), pb("b_hq", 30, 30)}, func(s *Snapshot) {
		tweakBuilding(s, "d_mg_nest", func(d *BuildingDef) { d.Dmg = 100 })
	}, "u_rifleman", "u_rifleman")
	tele(b, 3, 8, 10)
	tele(b, 4, 8, 11)
	evs := endT(t, b, 1)
	// the nest kills one rifleman and the battle goes on with the other
	if b.ph != phasePlayer || b.turn != 2 || kinds(evs)[len(evs)-1] == EventBattleEnded {
		t.Fatalf("phase %d turn %d events %v", b.ph, b.turn, kinds(evs))
	}
	evs = endT(t, b, 2)
	last := evs[len(evs)-1]
	if b.ph != phaseOver || b.endReason != EndNoUnits || last.Kind != EventBattleEnded || last.Value != int(EndNoUnits) {
		t.Fatalf("reason %d events %v", b.endReason, kinds(evs))
	}
	if r := b.Result(); r.TurnsUsed != 2 || r.EndReason != EndNoUnits || len(r.UnitsLost) != 1 || r.UnitsLost[0].Count != 2 {
		t.Fatalf("result %+v", r)
	}

	// max_turns: the defence of the last turn still acts, then the fuse ends the battle
	b = fight(t, []PlacedBuilding{pb("d_mg_nest", 12, 10), pb("b_hq", 30, 30)}, func(s *Snapshot) { s.Rules.MaxTurns = 1 }, "u_rifleman")
	tele(b, 3, 8, 10)
	evs = endT(t, b, 1)
	if b.endReason != EndTurnLimit || hpOf(b, 3) != 30 || evs[len(evs)-1].Value != int(EndTurnLimit) {
		t.Fatalf("reason %d hp %d", b.endReason, hpOf(b, 2))
	}
	// ... and when the last unit dies in that very phase the cause is no_units
	b = fight(t, []PlacedBuilding{pb("d_mg_nest", 12, 10), pb("b_hq", 30, 30)}, func(s *Snapshot) { s.Rules.MaxTurns = 1 }, "u_rifleman")
	tele(b, 3, 8, 10)
	b.ent(3).hp = 20
	endT(t, b, 1)
	if b.endReason != EndNoUnits {
		t.Fatalf("reason %d", b.endReason)
	}
}

func TestPickTargetMatchesSortedList(t *testing.T) {
	for _, name := range []string{"G13", "G14", "G15", "G18", "G19"} {
		var sc scenario
		for _, s := range scenarios() {
			if s.name == name {
				sc = s
			}
		}
		b, err := NewBattle(sc.snap, sc.seed)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range sc.cmds {
			b.Apply(c)
			for i := range b.ents {
				s := &b.ents[i]
				list := b.targetsInRange(s)
				got := b.pickTarget(s)
				if (got == nil) != (len(list) == 0) || (got != nil && got.id != list[0]) {
					t.Fatalf("%s: entity %d picks %v, list %v", name, s.id, got, list)
				}
			}
		}
	}
}

func TestDefenceRulesValidated(t *testing.T) {
	for _, f := range []func(*BuildingDef){
		func(d *BuildingDef) { d.OverheatFireTurns = 2 },
		func(d *BuildingDef) { d.Chain = 3 },
		func(d *BuildingDef) { d.DotDmg = -1 },
		func(d *BuildingDef) { d.TriggerCells = -1 },
	} {
		s := f01Snapshot()
		tweakBuilding(&s, "d_mg_nest", f)
		if _, err := NewBattle(s, 1); err == nil {
			t.Fatal("bad defence numbers accepted")
		}
	}
}
