package sim

import (
	"reflect"
	"testing"
)

// Unit tests of Attack, damage, splash, traps, ending and result (SIM-2). Entity ids follow the canonical
// order: buildings by (Y, X, ID), then the deployed units in Deploy order.

func atk(turn int, u, t EntityID) Command {
	return Command{Turn: turn, Op: OpAttack, Unit: u, Target: t}
}

func allArmy(s *Snapshot) {
	for _, u := range s.Rules.Units {
		if !u.Hero {
			s.Army = append(s.Army, ArmySlot{u.ID, 1, 9})
		}
	}
	s.Hero, s.HeroLevel = "h_commander", 1
}

// tele moves a deployed unit by hand: isolates a rule from the walking needed to reach it.
func tele(b *Battle, id EntityID, x, y int) {
	e := b.ent(id)
	b.lift(e)
	b.place(e, cell(x, y))
}

func tweakBuilding(s *Snapshot, id string, f func(*BuildingDef)) {
	for i := range s.Rules.Buildings {
		if s.Rules.Buildings[i].ID == id {
			f(&s.Rules.Buildings[i])
			return
		}
	}
	panic("no building " + id)
}

// fight starts a battle with every unit type available, the given buildings and the units deployed on the rim;
// the test then teleports them where it needs.
func fight(t *testing.T, blds []PlacedBuilding, mut func(*Snapshot), units ...string) *Battle {
	t.Helper()
	var ds []du
	for i, u := range units {
		ds = append(ds, du{u, i % 2, 4 + i/2})
	}
	return started(t, func(s *Snapshot) {
		s.Buildings = blds
		allArmy(s)
		if mut != nil {
			mut(s)
		}
	}, ds...)
}

func TestAttackRejects(t *testing.T) {
	blds := []PlacedBuilding{pb("d_wall", 5, 5), pb("d_mg_nest", 12, 10), pb("x_statue", 9, 12), pb("d_mine", 9, 14),
		{ID: "d_mine", Level: 1, Pos: cell(9, 16), Revealed: true}}
	// ids: wall 1, nest 2, statue 3, hidden mine 4, revealed mine 5; units: rifleman 6, sniper 7, medic 8, brawler 9
	b := fight(t, blds, nil, "u_rifleman", "u_sniper", "u_medic", "u_brawler")
	wantReject(t, b, atk(1, 99, 2), RejectUnknownEntity)
	wantReject(t, b, atk(1, 6, 99), RejectUnknownEntity)
	wantReject(t, b, atk(1, 2, 6), RejectNotOwner)      // a building is not ours
	wantReject(t, b, atk(1, 8, 2), RejectInvalidTarget) // the medic cannot attack
	wantReject(t, b, atk(1, 6, 7), RejectInvalidTarget) // own unit
	wantReject(t, b, atk(1, 6, 3), RejectInvalidTarget) // decor
	wantReject(t, b, atk(1, 6, 4), RejectInvalidTarget) // hidden mine
	wantReject(t, b, atk(1, 6, 2), RejectOutOfRange)    // far away
	tele(b, 7, 5, 6)                                    // sniper next to the wall: min_range 2
	wantReject(t, b, atk(1, 7, 1), RejectBelowMinRange)
	tele(b, 7, 5, 8) // dist 3 to the wall (5,5)
	applyOK(t, b, atk(1, 7, 1))
	if hpOf(b, 1) != 900-28 {
		t.Fatalf("wall hp %d", hpOf(b, 1))
	}
	wantReject(t, b, atk(1, 7, 1), RejectNoAP) // sniper ap 1
	// the revealed mine can be shot and is simply removed
	tele(b, 6, 9, 17)
	applyOK(t, b, atk(1, 6, 5))
	if b.ent(5).alive || b.ent(6).hp != b.ent(6).maxHP {
		t.Fatal("a shot revealed mine must vanish without a blast")
	}
	wantReject(t, b, atk(1, 6, 5), RejectInvalidTarget) // already destroyed
	if d, tot := b.destruction(); d != 0 || tot != 1 {  // only the nest counts
		t.Fatalf("destruction %d/%d", d, tot)
	}
}

func hpOf(b *Battle, id EntityID) int { return b.ent(id).hp }

func TestAttackAPTwoAllowsMoveAndAttack(t *testing.T) {
	b := fight(t, []PlacedBuilding{pb("d_wall", 5, 5)}, nil, "u_rifleman", "u_brawler") // wall 1; rifleman 2, brawler 3
	tele(b, 2, 4, 8)
	tele(b, 3, 4, 7)
	applyOK(t, b, mv(1, 2, 5, 8)) // ap 2: move ...
	applyOK(t, b, atk(1, 2, 1))   // ... then shoot (dist 3 <= 4)
	applyOK(t, b, mv(1, 3, 5, 6)) // ap 1: the move uses it
	wantReject(t, b, atk(1, 3, 1), RejectNoAP)
	applyOK(t, b, end(1))
	applyOK(t, b, atk(2, 3, 1)) // new turn, AP back; brawler melee dist 1
	if want := 900 - 10 - 30; hpOf(b, 1) != want {
		t.Fatalf("wall hp %d want %d", hpOf(b, 1), want)
	}
}

func TestSplashFalloffAndSides(t *testing.T) {
	var blds []PlacedBuilding
	for x := 10; x <= 14; x++ {
		blds = append(blds, pb("d_wall", x, 10))
	}
	// walls 1..5 at x=10..14; grenadier 6, rifleman 7 (ours, inside the blast)
	b := fight(t, blds, nil, "u_grenadier", "u_rifleman")
	tele(b, 6, 6, 10)
	tele(b, 7, 12, 11)
	applyOK(t, b, atk(1, 6, 3))
	want := []int{900 - 12, 900 - 16, 900 - 20, 900 - 16, 900 - 12} // d=2: 20*(200-80)/200, d=1: 20*(200-40)/200
	for i, w := range want {
		if hpOf(b, EntityID(i+1)) != w {
			t.Fatalf("wall %d hp %d want %d", i+1, hpOf(b, EntityID(i+1)), w)
		}
	}
	if hpOf(b, 7) != b.ent(7).maxHP {
		t.Fatal("splash hit our own unit")
	}
}

func TestVsMult(t *testing.T) {
	blds := []PlacedBuilding{pb("d_wire", 10, 10), pb("d_fence", 10, 12), pb("d_mg_nest", 14, 10)}
	b := fight(t, blds, nil, "u_flamer") // wire 1, nest 2, fence 3, flamer 4
	tele(b, 4, 9, 11)
	applyOK(t, b, atk(1, 4, 1))
	if hpOf(b, 1) != 90-45 { // 30 x 1500 / 1000
		t.Fatalf("wire hp %d", hpOf(b, 1))
	}
	applyOK(t, b, end(1))
	applyOK(t, b, atk(2, 4, 3))
	if hpOf(b, 3) != 300-15 { // 30 x 500 / 1000
		t.Fatalf("fence hp %d", hpOf(b, 3))
	}
}

func TestBreacher(t *testing.T) {
	blds := []PlacedBuilding{pb("d_wall", 10, 10), pb("d_fence", 10, 11), pb("d_wall", 10, 12), pb("d_mg_nest", 14, 10)}
	// wall 1 (10,10), nest 2 (14,10), fence 3 (10,11), wall 4 (10,12); breacher 5, rifleman 6
	b := fight(t, blds, nil, "u_breacher", "u_rifleman")
	tele(b, 5, 9, 11)
	tele(b, 6, 8, 11)
	applyOK(t, b, atk(1, 5, 3))
	if b.ent(3).alive || hpOf(b, 1) != 900-384 || hpOf(b, 4) != 900-384 { // 16 x 40 = 640; at d=1: 640 x 60 / 100
		t.Fatalf("fence alive=%v wall hp %d %d", b.ent(3).alive, hpOf(b, 1), hpOf(b, 4))
	}
	if b.ent(5).alive {
		t.Fatal("the breacher must be spent")
	}
	if got := b.Result().UnitsLost; !reflect.DeepEqual(got, []SlotCount{{"u_breacher", 1, 1}}) {
		t.Fatalf("units lost %+v", got)
	}
	applyOK(t, b, mv(1, 6, 9, 11, 10, 11)) // the fence cell is open at once
	// against a non-wall the burst is not multiplied
	b2 := fight(t, blds, nil, "u_breacher")
	tele(b2, 5, 13, 11)
	applyOK(t, b2, atk(1, 5, 2))
	if hpOf(b2, 2) != 60-16 {
		t.Fatalf("nest hp %d", hpOf(b2, 2))
	}
}

func TestMineInterruptsAndSplashes(t *testing.T) {
	// mine at (3,10): burst 18, splash 1, falloff 40 -> 18 in the centre, 10 at distance 1
	b := fight(t, []PlacedBuilding{pb("d_mine", 3, 10)}, nil, "u_rifleman", "u_brawler", "v_tank", "u_drone", "h_commander")
	// mine 1; rifleman 2, brawler 3, tank 4, drone 5, hero 6
	tele(b, 2, 1, 10)
	tele(b, 3, 3, 11)
	tele(b, 4, 4, 9)
	tele(b, 5, 3, 9)
	tele(b, 6, 2, 11)
	evs, _ := b.Apply(mv(1, 2, 2, 10, 3, 10, 4, 10)) // the third cell is never reached
	if b.ent(2).pos != cell(3, 10) || b.ent(2).apLeft != 1 {
		t.Fatalf("rifleman at %v ap %d: the walk must stop on the mine and cost 1 AP", b.ent(2).pos, b.ent(2).apLeft)
	}
	if b.ent(1).alive {
		t.Fatal("mine not consumed")
	}
	for id, want := range map[EntityID]int{2: 50 - 18, 3: 100 - 10, 4: 1100 - 10, 5: 70, 6: 1600 - 10} {
		if hpOf(b, id) != want {
			t.Fatalf("entity %d hp %d want %d", id, hpOf(b, id), want)
		}
	}
	if b.Result().HeroDamageTaken != 10 {
		t.Fatalf("hero damage %d", b.Result().HeroDamageTaken)
	}
	kinds := map[EventKind]int{}
	for _, e := range evs {
		kinds[e.Kind]++
	}
	if kinds[EventTrapTriggered] != 1 || kinds[EventAttacked] != 4 || kinds[EventMoved] != 1 {
		t.Fatalf("events %+v", evs)
	}
	applyOK(t, b, mv(1, 3, 3, 12)) // the brawler moves on
}

func TestMineKillsAndEndsBattle(t *testing.T) {
	b := fight(t, []PlacedBuilding{pb("d_mine", 3, 10), pb("b_hq", 30, 30)}, func(s *Snapshot) {
		tweakBuilding(s, "d_mine", func(d *BuildingDef) { d.Burst = 60 })
	}, "u_rifleman")
	tele(b, 3, 2, 10) // mine 1, hq 2, rifleman 3
	applyOK(t, b, mv(1, 3, 3, 10))
	r := b.Result()
	if r.EndReason != EndNoUnits || r.Stars != 0 || len(r.UnitsLost) != 1 || r.TurnsUsed != 1 {
		t.Fatalf("%+v", r)
	}
	wantReject(t, b, end(1), RejectBattleOver)
}

func TestSpringTrap(t *testing.T) {
	// trap at (3,10): throws INF with max_hp <= 60, 3 cells, opposite to the last step, over walls
	blds := []PlacedBuilding{pb("d_spring_trap", 4, 10), pb("d_wall", 2, 10)}
	// sorted by (Y, X): wall (2,10) = 1, trap (4,10) = 2; medic 3, heavy 4, rifleman 5, brawler 6
	b := fight(t, blds, nil, "u_medic", "u_heavy", "u_rifleman", "u_brawler")
	tele(b, 3, 3, 10)
	applyOK(t, b, mv(1, 3, 4, 10)) // step east onto the trap -> thrown west over the wall to (1,10)
	if b.ent(3).pos != cell(1, 10) || b.ent(2).alive {
		t.Fatalf("medic at %v trap alive %v", b.ent(3).pos, b.ent(2).alive)
	}
	// a heavy unit (max_hp 520 > 60) does not fire it
	b = fight(t, blds, nil, "u_medic", "u_heavy", "u_rifleman", "u_brawler")
	tele(b, 4, 3, 10)
	applyOK(t, b, mv(1, 4, 4, 10))
	if !b.ent(2).alive || b.ent(4).pos != cell(4, 10) {
		t.Fatal("a heavy unit must stand on an idle trap")
	}
	// landing cell taken: the farthest free cell before it
	b = fight(t, blds, nil, "u_medic", "u_rifleman")
	tele(b, 3, 3, 10)
	tele(b, 4, 1, 10)
	applyOK(t, b, mv(1, 3, 4, 10))
	if b.ent(3).pos != cell(3, 10) {
		t.Fatalf("medic at %v", b.ent(3).pos)
	}
}

func TestCarrierDeathUnloads(t *testing.T) {
	b := newB(t, func(s *Snapshot) {
		s.Buildings = []PlacedBuilding{pb("d_mine", 5, 10)}
		allArmy(s)
	})
	applyOK(t, b, dep("v_buggy", 1, 10)) // mine 1, buggy 2, rifleman 3, brawler 4
	applyOK(t, b, dep("u_rifleman", 1, 11))
	applyOK(t, b, dep("u_brawler", 1, 12))
	applyOK(t, b, ld(3, 2))
	applyOK(t, b, ld(4, 2))
	applyOK(t, b, Command{Op: OpStart})
	tele(b, 2, 4, 10)
	applyOK(t, b, mv(1, 2, 5, 10)) // the buggy (hp 15) dies on the mine (18)
	if b.ent(2).alive {
		t.Fatal("buggy survived")
	}
	// ring (4..6, 9..11) scanned by (Y, X): (4,9) and (5,9)
	if !b.ent(3).onField || b.ent(3).pos != cell(4, 9) || b.ent(4).pos != cell(5, 9) || b.ent(3).apLeft != 0 || b.ent(3).aboard != 0 {
		t.Fatalf("passengers at %v / %v ap %d", b.ent(3).pos, b.ent(4).pos, b.ent(3).apLeft)
	}
	if got := b.Result().UnitsLost; !reflect.DeepEqual(got, []SlotCount{{"v_buggy", 1, 1}}) {
		t.Fatalf("lost %+v", got)
	}
	checkInvariants(t, b)
}

func TestCarrierDeathWithoutRoomKillsPassengers(t *testing.T) {
	b := newB(t, func(s *Snapshot) {
		s.Buildings = nil
		allArmy(s)
	})
	applyOK(t, b, dep("v_buggy", 1, 10))
	applyOK(t, b, dep("u_rifleman", 1, 11))
	applyOK(t, b, dep("u_rifleman", 1, 12))
	applyOK(t, b, ld(2, 1))
	applyOK(t, b, ld(3, 1))
	for i := 0; i < 8; i++ { // eight blockers around (10,10)
		applyOK(t, b, dep("u_brawler", 1, 20+i))
	}
	applyOK(t, b, Command{Op: OpStart})
	tele(b, 1, 10, 10)
	ring := []Cell{{9, 9}, {10, 9}, {11, 9}, {9, 10}, {11, 10}, {9, 11}, {10, 11}, {11, 11}}
	for i, c := range ring {
		tele(b, EntityID(4+i), int(c.X), int(c.Y))
	}
	b.kill(0, b.ent(1))
	if !b.ent(2).alive || b.ent(2).pos != cell(10, 10) || b.ent(3).alive {
		t.Fatalf("first passenger %v alive %v, second alive %v", b.ent(2).pos, b.ent(2).alive, b.ent(3).alive)
	}
	checkInvariants(t, b)
}

func baseOf(hq, vault, nest bool) []PlacedBuilding {
	var l []PlacedBuilding
	if hq {
		l = append(l, pb("b_hq", 20, 10))
	}
	if vault {
		l = append(l, pb("b_vault", 20, 20))
	}
	if nest {
		l = append(l, pb("d_mg_nest", 12, 10))
	}
	return l
}

func TestFullDestructionEndsBattle(t *testing.T) {
	b := fight(t, baseOf(false, false, true), nil, "v_tank") // nest 1, tank 2 (dmg 55 vs hp 60)
	tele(b, 2, 8, 10)
	applyOK(t, b, atk(1, 2, 1))
	applyOK(t, b, end(1))
	applyOK(t, b, atk(2, 2, 1))
	r := b.Result()
	if r.EndReason != EndFullDestruction || r.Destruction != 100 || r.Stars != 2 || r.StarFlags != StarHalf|StarFull || r.HQDestroyed || r.TurnsUsed != 2 {
		t.Fatalf("%+v", r)
	}
	wantReject(t, b, atk(2, 2, 1), RejectBattleOver)
}

func TestStarsHQHalfFullAndLoot(t *testing.T) {
	mut := func(s *Snapshot) {
		tweakBuilding(s, "b_hq", func(d *BuildingDef) { d.HP = 80; d.HPGrowthPermille = 0 })
	}
	// sorted: nest (12,10)=1, hq (20,10)=2, vault (20,20)=3; tanks 4,5,6
	b := fight(t, baseOf(true, true, true), mut, "v_tank", "v_tank", "v_tank")
	tele(b, 4, 16, 10)
	tele(b, 5, 24, 12)
	tele(b, 6, 24, 20)
	applyOK(t, b, atk(1, 4, 2))
	applyOK(t, b, atk(1, 5, 2)) // 110 >= 80: hq destroyed
	applyOK(t, b, atk(1, 6, 3)) // vault 55 of 500
	r := b.Result()
	// one of three counted buildings: HQ star only, 33%
	if r.Stars != 1 || r.StarFlags != StarHQ || !r.HQDestroyed || r.Destruction != 33 || r.EndReason != EndNone {
		t.Fatalf("%+v", r)
	}
	if want := []StorageHit{{Entity: 3, ContentID: "b_vault", Level: 1, DamagePermille: 110}}; !reflect.DeepEqual(r.LootBasis.Storages, want) ||
		r.LootBasis.PercentDestroyed != 33 || r.LootBasis.Stars != 1 {
		t.Fatalf("loot %+v", r.LootBasis)
	}
	applyOK(t, b, end(1))
	tele(b, 4, 16, 10)
	tele(b, 5, 16, 11)
	applyOK(t, b, atk(2, 4, 1))
	applyOK(t, b, atk(2, 5, 1)) // nest down too: 2 of 3 -> half
	r = b.Result()
	if r.Stars != 2 || r.StarFlags != StarHQ|StarHalf || r.Destruction != 66 {
		t.Fatalf("%+v", r)
	}
	for turn := 2; b.ph != phaseOver; turn++ { // finish the vault: all three flags
		applyOK(t, b, atk(turn, 6, 3))
		if b.ph != phaseOver {
			applyOK(t, b, end(turn))
		}
	}
	r = b.Result()
	if r.Stars != 3 || r.StarFlags != StarHQ|StarHalf|StarFull || r.Destruction != 100 || r.EndReason != EndFullDestruction {
		t.Fatalf("%+v", r)
	}
	if r.LootBasis.Storages[0].DamagePermille != 1000 {
		t.Fatalf("a destroyed storage reports 1000, got %+v", r.LootBasis)
	}
}

func TestTurnLimitAndRetreat(t *testing.T) {
	b := fight(t, baseOf(true, false, false), func(s *Snapshot) { s.Rules.MaxTurns = 3 }, "u_rifleman")
	applyOK(t, b, end(1))
	applyOK(t, b, end(2))
	if b.ph == phaseOver {
		t.Fatal("ended early")
	}
	applyOK(t, b, end(3))
	if r := b.Result(); r.EndReason != EndTurnLimit || r.TurnsUsed != 3 {
		t.Fatalf("%+v", r)
	}
	b = fight(t, baseOf(true, false, false), nil, "u_rifleman")
	applyOK(t, b, Command{Turn: 1, Op: OpRetreat})
	if r := b.Result(); r.EndReason != EndRetreat || r.TurnsUsed != 1 || r.Stars != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestHeroLossDoesNotShrinkArmy(t *testing.T) {
	b := fight(t, []PlacedBuilding{pb("d_mine", 3, 10), pb("b_hq", 30, 30)}, func(s *Snapshot) {
		tweakBuilding(s, "d_mine", func(d *BuildingDef) { d.Burst = 5000 })
	}, "h_commander", "u_rifleman") // hero 2, rifleman 3
	tele(b, 3, 2, 10)              // mine 1, hq 2, hero 3, rifleman 4
	applyOK(t, b, mv(1, 3, 3, 10)) // the hero dies on the mine; the rifleman stays far away
	r := b.Result()
	if r.HeroSurvived || len(r.UnitsLost) != 0 || r.HeroDamageTaken != 1600 || r.EndReason != EndNone {
		t.Fatalf("%+v", r)
	}
}

// TestGoldenG12Positions pins the human-checkable facts of the trap golden replay, so a wrong golden cannot be
// regenerated unnoticed.
func TestGoldenG12Positions(t *testing.T) {
	var sc scenario
	for _, s := range scenariosSim2() {
		if s.name == "G12" {
			sc = s
		}
	}
	b, err := NewBattle(sc.snap, sc.seed)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range sc.cmds {
		if _, err := b.Apply(c); err != nil {
			t.Fatal(err)
		}
	}
	at := func(id EntityID) Cell { return b.ent(id).pos }
	hp := func(id EntityID) int { return b.ent(id).hp }
	if at(12) != cell(1, 14) || at(13) != cell(4, 14) {
		t.Fatalf("medic %v heavy %v", at(12), at(13))
	}
	if at(15) != cell(4, 19) || at(16) != cell(5, 19) || b.ent(14).alive {
		t.Fatalf("passengers %v %v, apc alive %v", at(15), at(16), b.ent(14).alive)
	}
	if hp(10) != 32 || hp(8) != 1090 || hp(9) != 90 || hp(11) != 70 {
		t.Fatalf("hp rifleman %d tank %d brawler %d drone %d", hp(10), hp(8), hp(9), hp(11))
	}
	for id, spent := range map[EntityID]bool{1: true, 3: false, 4: true, 5: true} {
		if b.ent(id).alive == spent {
			t.Fatalf("trap %d: alive=%v", id, b.ent(id).alive)
		}
	}
}
