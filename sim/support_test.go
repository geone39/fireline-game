package sim

import "testing"

// Unit tests of the support layer (SIM-5, spec 5.3, 9): abilities, spells, zones, the hero. Numbers come from the
// test fixture (not balance); the nest (d_mg_nest) shoots INF/AIR for 20 at range 7.

func cast(turn int, id string, x, y int) Command {
	return Command{Turn: turn, Op: OpCast, ID: id, To: cell(x, y)}
}

func abil(turn int, u EntityID, kind string, target EntityID) Command {
	return Command{Turn: turn, Op: OpAbility, Unit: u, ID: kind, Target: target}
}

func allSpells() []ArmySlot {
	var out []ArmySlot
	for _, sp := range fixtureRules().Spells {
		out = append(out, ArmySlot{sp.ID, 1, 3})
	}
	return out
}

// support starts a battle with every army unit and every spell in stock; units go to the rim in the given order
// (the hero too, when named).
func support(t *testing.T, blds []PlacedBuilding, hero string, units ...string) *Battle {
	t.Helper()
	var ds []du
	for i, u := range units {
		ds = append(ds, du{u, i % 2, 4 + i/2})
	}
	return started(t, func(s *Snapshot) {
		s.Buildings = blds
		allArmy(s)
		s.Hero = hero
		s.Spells = allSpells()
	}, ds...)
}

func TestAbilityHeal(t *testing.T) {
	b := support(t, nil, "", "u_rifleman", "u_medic", "v_engineer", "v_tank") // 1 rifleman, 2 medic, 3 engineer, 4 tank
	wantReject(t, b, abil(1, 2, AbilityHeal, 1), RejectInvalidTarget)         // full hit points
	b.ent(1).hp = 20
	b.ent(4).hp = 500
	wantReject(t, b, abil(1, 99, AbilityHeal, 1), RejectUnknownEntity)
	wantReject(t, b, abil(1, 2, AbilityHeal, 99), RejectUnknownEntity)
	wantReject(t, b, abil(1, 2, AbilityHeal, 2), RejectInvalidTarget) // not oneself
	wantReject(t, b, abil(1, 2, AbilityHeal, 4), RejectInvalidTarget) // the medic heals infantry only
	wantReject(t, b, abil(1, 3, AbilityHeal, 1), RejectInvalidTarget) // the engineer heals vehicles only
	wantReject(t, b, abil(1, 1, AbilityHeal, 2), RejectInvalidTarget) // a rifleman cannot heal
	wantReject(t, b, abil(1, 2, "nope", 1), RejectInvalidTarget)      // unknown kind
	tele(b, 1, 20, 20)
	wantReject(t, b, abil(1, 2, AbilityHeal, 1), RejectOutOfRange)
	tele(b, 1, 0, 4)
	evs, err := b.Apply(abil(1, 2, AbilityHeal, 1))
	if err != nil || len(b.rejected) != 9 || b.ent(1).hp != 35 || b.ent(2).apLeft != 0 {
		t.Fatalf("heal: %v hp %d rej %v", err, b.ent(1).hp, b.rejected)
	}
	if len(evs) != 1 || evs[0].Kind != EventHealed || evs[0].Value != 15 {
		t.Fatalf("events %+v", evs)
	}
	wantReject(t, b, abil(1, 2, AbilityHeal, 1), RejectNoAP)
	applyOK(t, b, abil(1, 3, AbilityHeal, 4)) // +40 on the tank
	endT(t, b, 1)
	applyOK(t, b, abil(2, 2, AbilityHeal, 1))
	if b.ent(1).hp != 50 || b.ent(4).hp != 540 {
		t.Fatalf("hp %d %d: heal must stop at max_hp", b.ent(1).hp, b.ent(4).hp)
	}
}

func TestAbilityJam(t *testing.T) {
	// nest 1 at (12,10), HQ 2 at (30,30); saboteur 3, rifleman 4
	b := support(t, []PlacedBuilding{pb("d_mg_nest", 12, 10), pb("b_hq", 30, 30)}, "", "u_saboteur", "u_rifleman")
	tele(b, 3, 9, 10)
	tele(b, 4, 6, 10)
	wantReject(t, b, abil(1, 3, AbilityJam, 1), RejectOutOfRange) // dist 3: the saboteur has to stand right next to it
	wantReject(t, b, abil(1, 3, AbilityJam, 2), RejectInvalidTarget)
	wantReject(t, b, abil(1, 3, AbilityJam, 4), RejectInvalidTarget) // our own unit
	tele(b, 3, 11, 10)
	applyOK(t, b, abil(1, 3, AbilityJam, 1))
	if b.ent(1).disabledUntil != 2 { // turn 1 + jam_turns 2 - 1
		t.Fatalf("disabledUntil %d", b.ent(1).disabledUntil)
	}
	endT(t, b, 1)
	endT(t, b, 2)
	if b.ent(3).hp != b.ent(3).maxHP || b.ent(4).hp != b.ent(4).maxHP {
		t.Fatal("a jammed nest must not fire on turns 1 and 2")
	}
	endT(t, b, 3)
	if b.ent(3).hp == b.ent(3).maxHP {
		t.Fatal("the nest must fire again on turn 3")
	}
}

func TestAbilityUnload(t *testing.T) {
	// two passengers aboard an APC (carries 2 in the fixture); a wall box leaves two free cells
	b := newB(t, func(s *Snapshot) {
		s.Buildings = []PlacedBuilding{pb("d_wall", 9, 9), pb("d_wall", 10, 9), pb("d_wall", 11, 9), pb("d_wall", 9, 10), pb("d_wall", 9, 11), pb("d_wall", 11, 10), pb("d_wall", 11, 11)}
		s.Army = []ArmySlot{{"v_apc", 1, 1}, {"u_rifleman", 1, 2}, {"u_brawler", 1, 1}}
	})
	// walls 1..7, apc 8, riflemen 9, 10, brawler 11
	applyOK(t, b, dep("v_apc", 1, 20))
	applyOK(t, b, dep("u_rifleman", 1, 21))
	applyOK(t, b, dep("u_rifleman", 1, 22))
	applyOK(t, b, ld(9, 8))
	applyOK(t, b, ld(10, 8))
	applyOK(t, b, Command{Op: OpStart})
	wantReject(t, b, abil(1, 9, AbilityUnload, 0), RejectInvalidTarget)  // a rifleman has no cargo
	wantReject(t, b, abil(1, 8, AbilityUnload, 11), RejectInvalidTarget) // no such passenger
	wantReject(t, b, abil(1, 8, AbilityUnload, 8), RejectInvalidTarget)  // not aboard
	tele(b, 8, 10, 10)                                                   // the APC into the middle of the wall box: (10,10) is free, the box is open at (10,11)
	// free cells around (10,10): (10,11) only ((9..11, 9) walls, (9,10),(11,10) walls, (9,11),(11,11) walls)
	applyOK(t, b, abil(1, 8, AbilityUnload, 0))
	if b.ent(9).aboard != 0 || !b.ent(9).onField || b.ent(9).pos != cell(10, 11) || b.ent(9).apLeft != 0 {
		t.Fatalf("first passenger: %+v", b.ent(9))
	}
	if b.ent(10).aboard != 8 || len(b.ent(8).cargo) != 1 || b.ent(8).apLeft != 0 {
		t.Fatal("the second passenger must stay aboard, the carrier spent its AP")
	}
	endT(t, b, 1)
	wantReject(t, b, abil(2, 8, AbilityUnload, 0), RejectCellOccupied) // no free cell: nothing changes, no AP spent
	tele(b, 9, 20, 20)
	applyOK(t, b, abil(2, 8, AbilityUnload, 10))
	if b.ent(10).pos != cell(10, 11) || len(b.ent(8).cargo) != 0 {
		t.Fatalf("second passenger %+v", b.ent(10))
	}
	wantReject(t, b, abil(2, 8, AbilityUnload, 0), RejectNoAP)
}

func TestLanderLandsOnDryCellsOnly(t *testing.T) {
	b := newB(t, func(s *Snapshot) {
		s.Map, s.Buildings = "coastal", nil
		s.Army = []ArmySlot{{"s_lander", 1, 1}, {"u_rifleman", 1, 2}}
	})
	applyOK(t, b, dep("s_lander", 42, 30)) // 1
	applyOK(t, b, dep("u_rifleman", 1, 30))
	applyOK(t, b, dep("u_rifleman", 1, 31))
	applyOK(t, b, ld(2, 1))
	applyOK(t, b, ld(3, 1))
	applyOK(t, b, Command{Op: OpStart})
	wantReject(t, b, abil(1, 1, AbilityUnload, 0), RejectCellOccupied) // open water all round
	applyOK(t, b, mv(1, 1, 41, 30, 40, 30, 39, 30))
	endT(t, b, 1)
	applyOK(t, b, mv(2, 1, 38, 30, 37, 30, 36, 30))
	endT(t, b, 2)
	applyOK(t, b, abil(3, 1, AbilityUnload, 0)) // (36,30) has land at x=35: (35,29), (35,30), (35,31)
	if b.ent(2).pos != cell(35, 29) || b.ent(3).pos != cell(35, 30) {
		t.Fatalf("passengers at %v %v", b.ent(2).pos, b.ent(3).pos)
	}
}

func TestCastRejectsAndSpellsPerTurn(t *testing.T) {
	b := support(t, []PlacedBuilding{pb("b_hq", 30, 30)}, "", "u_rifleman")
	wantReject(t, b, cast(1, "c_nope", 5, 5), RejectNoStock)
	wantReject(t, b, cast(1, "c_airstrike", -1, 5), RejectOutOfZone)
	wantReject(t, b, cast(1, "c_airstrike", 44, 5), RejectOutOfZone)
	applyOK(t, b, cast(1, "c_smoke", 5, 5))
	applyOK(t, b, cast(1, "c_smoke", 5, 5))
	wantReject(t, b, cast(1, "c_smoke", 5, 5), RejectNoAP) // spells_per_turn 2
	endT(t, b, 1)
	applyOK(t, b, cast(2, "c_smoke", 5, 5))
	endT(t, b, 2)
	wantReject(t, b, cast(3, "c_smoke", 5, 5), RejectNoStock) // 3 in stock
	r := b.Result()
	if len(r.SpellsUsed) != 1 || r.SpellsUsed[0] != (SlotCount{"c_smoke", 1, 3}) {
		t.Fatalf("spells used %+v", r.SpellsUsed)
	}
	// a spell has no AP cost
	if b.ent(1).apLeft != b.ent(1).ap {
		t.Fatal("Cast must not take AP")
	}
}

func TestAirstrikeSplitsOverTurnsAndSparesOwn(t *testing.T) {
	// nest 1 (60 hp) at (12,10); HQ 2 at (30,30); tank 3 (the nest cannot shoot it), rifleman 4 stays away
	b := support(t, []PlacedBuilding{pb("d_mg_nest", 12, 10), pb("b_hq", 30, 30)}, "", "v_tank", "u_rifleman")
	tele(b, 3, 12, 13)
	tele(b, 4, 30, 5)
	applyOK(t, b, cast(1, "c_airstrike", 12, 10)) // 62 over 4 turns: 17, 15, 15, 15
	if hpOf(b, 1) != 60 {
		t.Fatal("the first step belongs to the defence phase, not to the cast")
	}
	want := []int{60 - 17, 60 - 32, 60 - 47}
	for i, w := range want {
		endT(t, b, i+1)
		if hpOf(b, 1) != w {
			t.Fatalf("turn %d: nest hp %d, want %d", i+1, hpOf(b, 1), w)
		}
	}
	if hpOf(b, 3) != b.ent(3).maxHP {
		t.Fatal("an airstrike must not hurt own units")
	}
	endT(t, b, 4) // the fourth step finishes it
	if b.ent(1).alive || len(b.zones) != 0 {
		t.Fatalf("nest alive %v, zones %v", b.ent(1).alive, b.zones)
	}
	if d, _ := b.destruction(); d != 1 {
		t.Fatal("the nest must count as destroyed")
	}
}

func TestAirstrikeSkipsDecorAndHiddenTraps(t *testing.T) {
	b := support(t, []PlacedBuilding{pb("x_statue", 12, 10), pb("d_mine", 12, 13), pb("d_wall", 14, 10), pb("b_hq", 30, 30)}, "", "u_rifleman")
	applyOK(t, b, cast(1, "c_airstrike", 12, 11))
	endT(t, b, 1)
	// ids by (Y, X): statue 1, wall 2, mine 3, HQ 4
	if hpOf(b, 1) != 1 || !b.ent(3).alive || hpOf(b, 2) != 900-17 {
		t.Fatalf("statue %d mine alive %v wall %d", hpOf(b, 1), b.ent(3).alive, hpOf(b, 2))
	}
}

func TestJammerWeakensAirstrikeAndEMP(t *testing.T) {
	// jammer 1 at (14,10) (aura 5, 50%); nest 2 at (12,12); HQ 3
	blds := []PlacedBuilding{pb("d_jammer", 14, 10), pb("d_mg_nest", 12, 12), pb("b_hq", 30, 30)}
	b := support(t, blds, "", "u_rifleman")
	applyOK(t, b, cast(1, "c_airstrike", 12, 12)) // 62 -> 31 over 4: 10, 7, 7, 7; the jammer (r 2 from the centre) is hit too
	endT(t, b, 1)
	if hpOf(b, 2) != 60-10 || hpOf(b, 1) != 300-10 {
		t.Fatalf("nest %d jammer %d", hpOf(b, 2), hpOf(b, 1))
	}
	// out of the aura (cheb to the jammer footprint 7 > 5) the spell is whole
	b = support(t, blds, "", "u_rifleman")
	applyOK(t, b, cast(1, "c_airstrike", 12, 17)) // jammer footprint y 10..11: gap 6
	endT(t, b, 1)
	if hpOf(b, 1) != 300 {
		t.Fatalf("jammer hit by an airstrike 6 cells away: %d", hpOf(b, 1))
	}
	// a disabled generator does not weaken; EMP duration 2 -> 1 under a live one
	b = support(t, blds, "", "u_rifleman")
	b.ent(1).disabledUntil = 9
	applyOK(t, b, cast(1, "c_airstrike", 12, 12))
	endT(t, b, 1)
	if hpOf(b, 2) != 60-17 {
		t.Fatalf("nest %d: a disabled jammer must not weaken the spell", hpOf(b, 2))
	}
	b = support(t, blds, "", "u_rifleman")
	applyOK(t, b, cast(1, "c_emp", 12, 12)) // weakened 2 -> 1 turn
	if b.ent(2).disabledUntil != 1 || b.ent(1).disabledUntil != 1 {
		t.Fatalf("EMP: nest until %d jammer until %d", b.ent(2).disabledUntil, b.ent(1).disabledUntil)
	}
}

func TestEMPDisablesDefencesForItsDuration(t *testing.T) {
	b := support(t, []PlacedBuilding{pb("d_mg_nest", 12, 10), pb("b_hq", 30, 30), pb("d_wall", 12, 12)}, "", "u_rifleman")
	tele(b, 4, 8, 10)                       // ids: nest 1, wall 2, HQ 3, rifleman 4
	applyOK(t, b, cast(1, "c_emp", 12, 10)) // duration 2: turns 1 and 2
	if b.ent(1).disabledUntil != 2 || b.ent(2).disabledUntil != 0 {
		t.Fatalf("EMP: nest %d wall %d (only defence buildings go dark)", b.ent(1).disabledUntil, b.ent(2).disabledUntil)
	}
	endT(t, b, 1)
	endT(t, b, 2)
	if hpOf(b, 4) != b.ent(4).maxHP {
		t.Fatal("the nest must be silent for 2 turns")
	}
	endT(t, b, 3)
	if hpOf(b, 4) == b.ent(4).maxHP {
		t.Fatal("the nest must be back on turn 3")
	}
}

func TestBreachDestroysWallsInRadius(t *testing.T) {
	// walls 1..3 on (10..12, 10); fence at (10,12); tank trap (14,10); wire (10,11); HQ
	b := support(t, []PlacedBuilding{pb("d_wall", 10, 10), pb("d_wall", 11, 10), pb("d_wall", 12, 10), pb("d_wire", 10, 11), pb("d_fence", 10, 12),
		pb("d_tank_trap", 13, 10), pb("d_wall", 14, 10), pb("b_hq", 30, 30)}, "", "u_rifleman")
	// ids by (Y,X): walls 1,2,3 (y 10), trap 4 (13,10), wall 5 (14,10), wire 6 (10,11), fence 7 (10,12), hq 8
	applyOK(t, b, cast(1, "c_breach", 10, 10)) // radius 2: x 8..12, y 8..12
	for id, alive := range map[EntityID]bool{1: false, 2: false, 3: false, 4: true, 5: true, 6: true, 7: false, 8: true} {
		if b.ent(id).alive != alive {
			t.Fatalf("entity %d alive=%v", id, b.ent(id).alive)
		}
	}
	if d, tot := b.destruction(); d != 0 || tot != 1 {
		t.Fatalf("walls must not count: %d/%d", d, tot)
	}
	tele(b, 9, 11, 9)
	applyOK(t, b, mv(1, 9, 11, 10)) // the breached cell is walkable at once
}

func TestSmokeHidesFromDefenceBeyondOneCell(t *testing.T) {
	b := support(t, []PlacedBuilding{pb("d_mg_nest", 12, 10), pb("b_hq", 30, 30)}, "", "u_rifleman") // nest 1, hq 2, rifleman 3
	tele(b, 3, 8, 10)                                                                                // dist 4
	applyOK(t, b, cast(1, "c_smoke", 8, 10))                                                         // radius 3, 2 turns
	endT(t, b, 1)
	endT(t, b, 2)
	if hpOf(b, 3) != 50 {
		t.Fatal("a smoked unit must not be shot")
	}
	endT(t, b, 3) // the screen is gone
	if hpOf(b, 3) != 30 {
		t.Fatalf("hp %d", hpOf(b, 3))
	}
	// adjacent to the shooter the smoke does not help
	b = support(t, []PlacedBuilding{pb("d_mg_nest", 12, 10), pb("b_hq", 30, 30)}, "", "u_rifleman")
	tele(b, 3, 11, 10)
	applyOK(t, b, cast(1, "c_smoke", 11, 10))
	endT(t, b, 1)
	if hpOf(b, 3) != 30 {
		t.Fatalf("hp %d: an adjacent unit is still a target", hpOf(b, 3))
	}
}

func TestAdrenalineMembersMoveAndDamage(t *testing.T) {
	// wall 1 at (10,10), HQ 2; riflemen 3 (in the zone) and 4 (outside)
	b := support(t, []PlacedBuilding{pb("d_wall", 10, 10), pb("b_hq", 30, 30)}, "", "u_rifleman", "u_rifleman")
	tele(b, 3, 8, 12)
	tele(b, 4, 20, 20)
	applyOK(t, b, cast(1, "c_adrenaline", 8, 12)) // r 3, +50% move (3 -> 4), +25% dmg, 2 turns
	applyOK(t, b, mv(1, 3, 8, 11, 8, 10, 8, 9, 8, 8))
	wantReject(t, b, mv(1, 4, 20, 19, 20, 18, 20, 17, 20, 16), RejectPathCost) // not a member: 3 cells only
	tele(b, 3, 8, 10)
	applyOK(t, b, atk(1, 3, 1)) // 10 x 1250 / 1000 = 12
	if hpOf(b, 1) != 900-12 {
		t.Fatalf("wall hp %d", hpOf(b, 1))
	}
	tele(b, 4, 8, 12) // walks into the zone after the cast: no bonus (the set is fixed at the cast)
	applyOK(t, b, atk(1, 4, 1))
	if hpOf(b, 1) != 900-12-10 {
		t.Fatalf("wall hp %d", hpOf(b, 1))
	}
	endT(t, b, 1)
	applyOK(t, b, atk(2, 3, 1)) // still active on turn 2
	if hpOf(b, 1) != 900-12-10-12 {
		t.Fatalf("wall hp %d", hpOf(b, 1))
	}
	endT(t, b, 2)
	applyOK(t, b, atk(3, 3, 1)) // expired
	if hpOf(b, 1) != 900-12-10-12-10 || len(b.zones) != 0 {
		t.Fatalf("wall hp %d zones %d", hpOf(b, 1), len(b.zones))
	}
}

func TestCommanderAura(t *testing.T) {
	// wall 1; hero 3 = h_marshal (aura 3 cells: +20% dmg, +40% move); rifleman 4
	b := support(t, []PlacedBuilding{pb("d_wall", 10, 10), pb("b_hq", 30, 30)}, "h_marshal", "h_marshal", "u_rifleman")
	// ids: wall 1, hq 2, hero 3, rifleman 4
	tele(b, 3, 6, 10)
	tele(b, 4, 8, 10) // 2 cells from the hero
	if got := b.moveLimit(b.ent(4)); got != 4 {
		t.Fatalf("move limit %d, want 3+1", got)
	}
	applyOK(t, b, atk(1, 4, 1)) // 10 x 1200 / 1000 = 12
	if hpOf(b, 1) != 900-12 {
		t.Fatalf("wall hp %d", hpOf(b, 1))
	}
	tele(b, 4, 12, 12) // 6 cells away: outside the aura
	if got := b.moveLimit(b.ent(4)); got != 3 {
		t.Fatalf("move limit %d", got)
	}
	if got := b.moveLimit(b.ent(3)); got != 2 {
		t.Fatalf("the hero gets no aura: %d", got)
	}
	// aura + adrenaline add up: 400 + 500 = 900 permille -> 3 + floor(3 x 900 / 1000) = 5; dmg 1 + 0.2 + 0.25
	tele(b, 4, 8, 10)
	applyOK(t, b, cast(1, "c_adrenaline", 8, 10))
	if got := b.moveLimit(b.ent(4)); got != 5 {
		t.Fatalf("move limit %d", got)
	}
	endT(t, b, 1)
	hp := hpOf(b, 1)
	applyOK(t, b, atk(2, 4, 1)) // 10 x 1450 / 1000 = 14
	if hp-hpOf(b, 1) != 14 {
		t.Fatalf("damage %d", hp-hpOf(b, 1))
	}
	b.ent(3).hp, b.ent(3).alive = 0, false // a dead commander gives nothing
	if got := b.moveLimit(b.ent(4)); got != 3+max(1, 3*500/1000) {
		t.Fatalf("move limit %d", got)
	}
}

func TestHeroCoverFireCooldownAndLevelDamage(t *testing.T) {
	// nest 1 (12,10), HQ 2; hero 3 (h_cover L1) next to the nest
	b := support(t, []PlacedBuilding{pb("d_mg_nest", 12, 10), pb("b_hq", 30, 30)}, "h_cover", "h_cover")
	tele(b, 3, 14, 12)
	wantReject(t, b, abil(1, 3, AbilityHeal, 0), RejectInvalidTarget)
	applyOK(t, b, abil(1, 3, AbilityHero, 0)) // 61 over 2 turns: 31, 30 around (14,12), r 2: the nest footprint is within 2
	wantReject(t, b, abil(1, 3, AbilityHero, 0), RejectNoAP)
	if b.ent(3).cd != 3 {
		t.Fatalf("cooldown %d", b.ent(3).cd)
	}
	endT(t, b, 1)
	if hpOf(b, 1) != 60-31 {
		t.Fatalf("nest hp %d", hpOf(b, 1))
	}
	wantReject(t, b, abil(2, 3, AbilityHero, 0), RejectCooldown) // cd 2
	endT(t, b, 2)                                                // second step: 30 -> 60-31-30 < 0
	if b.ent(1).alive {
		t.Fatal("the nest must be gone after both steps")
	}
	wantReject(t, b, abil(3, 3, AbilityHero, 0), RejectCooldown) // cd 1
	endT(t, b, 3)
	applyOK(t, b, abil(4, 3, AbilityHero, 0)) // cd 0 on turn 4
	// level growth of the hero damage: 40 x 1500 / 1000 on level 2
	b = newB(t, func(s *Snapshot) {
		s.Buildings = []PlacedBuilding{pb("d_wall", 5, 5), pb("b_hq", 30, 30)}
		s.Hero, s.HeroLevel = "h_cover", 2
	})
	applyOK(t, b, Command{Op: OpDeploy, ID: "h_cover", Level: 2, To: cell(1, 5)}) // wall 1, hq 2, hero 3
	applyOK(t, b, Command{Op: OpStart})
	tele(b, 3, 5, 7)
	applyOK(t, b, atk(1, 3, 1))
	if hpOf(b, 1) != 900-60 {
		t.Fatalf("level 2 hero damage: wall hp %d", hpOf(b, 1))
	}
}

func TestHeroDecoyIsATargetNotAnArmyUnit(t *testing.T) {
	// nest 1, hq 2, hero 3 (h_tech), rifleman 4 stays far away
	b := support(t, []PlacedBuilding{pb("d_mg_nest", 12, 10), pb("b_hq", 30, 30)}, "h_tech", "h_tech", "u_rifleman")
	tele(b, 3, 4, 10)
	tele(b, 4, 4, 30)
	wantReject(t, b, Command{Turn: 1, Op: OpAbility, Unit: 3, ID: AbilityHero, To: cell(9, 10)}, RejectOutOfRange) // radius 3
	wantReject(t, b, Command{Turn: 1, Op: OpAbility, Unit: 3, ID: AbilityHero, To: cell(4, 10)}, RejectCellOccupied)
	wantReject(t, b, Command{Turn: 1, Op: OpAbility, Unit: 3, ID: AbilityHero, To: cell(-1, 10)}, RejectOutOfRange)
	evs, err := b.Apply(Command{Turn: 1, Op: OpAbility, Unit: 3, ID: AbilityHero, To: cell(7, 10)})
	if err != nil || len(b.rejected) != 3 || b.ent(5) == nil || b.ent(5).kind != KindDecoy || b.ent(5).pos != cell(7, 10) {
		t.Fatalf("decoy: %v %v", err, b.rejected)
	}
	if evs[0].Kind != EventSpawned || evs[0].Target != 5 {
		t.Fatalf("events %+v", evs)
	}
	wantReject(t, b, Command{Turn: 1, Op: OpMove, Unit: 5, Path: path(8, 10)}, RejectNoAP) // the decoy never acts
	endT(t, b, 1)
	if hpOf(b, 5) != 60 { // dist 5 to the nest; the hero stands at dist 8: the decoy is the only target
		t.Fatalf("decoy hp %d", hpOf(b, 5))
	}
	endT(t, b, 2) // lifetime 2 turns: gone at the end of turn 2
	if b.ent(5).alive {
		t.Fatal("the decoy must expire")
	}
	r := b.Result()
	if len(r.UnitsLost) != 0 {
		t.Fatalf("a decoy is not a loss: %+v", r.UnitsLost)
	}
	// it keeps no battle alive: with the real units dead the battle is over
	b = support(t, []PlacedBuilding{pb("d_mg_nest", 12, 10), pb("b_hq", 30, 30)}, "h_tech", "h_tech")
	tele(b, 3, 4, 10)
	applyOK(t, b, Command{Turn: 1, Op: OpAbility, Unit: 3, ID: AbilityHero, To: cell(7, 10)})
	b.hurt(0, b.ent(3), 9999)
	if r := b.checkEnd(); r != EndNoUnits {
		t.Fatalf("end %v: a decoy must not count as an army unit", r)
	}
}

func TestHeroVanishHidesAndMultipliesTheFirstShot(t *testing.T) {
	// nest 1 (12,10), HQ 2 (4x4 at (20,10)), hero 3 (h_ghost), rifleman 4 (bait)
	b := support(t, []PlacedBuilding{pb("d_mg_nest", 12, 10), pb("b_hq", 20, 10)}, "h_ghost", "h_ghost", "u_rifleman")
	tele(b, 3, 8, 10) // nest dist 4
	tele(b, 4, 4, 30)
	applyOK(t, b, abil(1, 3, AbilityHero, 0)) // hidden on turns 1 and 2
	endT(t, b, 1)
	if hpOf(b, 3) != 300 {
		t.Fatal("a hidden hero is not a target")
	}
	tele(b, 3, 13, 10) // HQ footprint x 20..23: dist 7 (range 12)
	hq := hpOf(b, 2)
	applyOK(t, b, atk(2, 3, 2)) // 30 x 4 against the HQ, and the vanish ends
	if hq-hpOf(b, 2) != 120 || b.ent(3).hidden(2) {
		t.Fatalf("hq lost %d, hidden %v", hq-hpOf(b, 2), b.ent(3).hidden(2))
	}
	endT(t, b, 2)
	if hpOf(b, 3) == 300 {
		t.Fatal("the nest must see the hero once it has fired")
	}
	// a shot at a target outside the bonus group is not multiplied, and it ends the hiding as well
	b = support(t, []PlacedBuilding{pb("d_wall", 8, 10), pb("b_hq", 30, 30)}, "h_ghost", "h_ghost")
	tele(b, 3, 4, 10)
	applyOK(t, b, abil(1, 3, AbilityHero, 0))
	endT(t, b, 1)
	applyOK(t, b, atk(2, 3, 1))
	if hpOf(b, 1) != 900-30 || b.ent(3).hidden(2) {
		t.Fatalf("wall hp %d", hpOf(b, 1))
	}
}

func TestConcealedUnitsAreNotWalkedTo(t *testing.T) {
	// a released guard does not chase a unit it may not target (spec 7.4): smoke, or a hidden hero
	mk := func() *Battle {
		b := newB(t, func(s *Snapshot) {
			s.Buildings = []PlacedBuilding{pb("d_bunker", 20, 20), pb("b_hq", 30, 30)}
			s.Garrison = []GarrisonUnit{{"u_guard", 1}}
			s.Army = []ArmySlot{{"u_rifleman", 1, 1}}
			s.Spells = allSpells()
		})
		applyOK(t, b, dep("u_rifleman", 1, 20)) // bunker 1, hq 2, guard 3, rifleman 4
		applyOK(t, b, Command{Op: OpStart})
		tele(b, 4, 12, 22)
		b.ent(1).damaged = true // opens the bunker in the first defence phase
		return b
	}
	b := mk()
	endT(t, b, 1)
	if !b.ent(3).onField || b.ent(3).pos == (Cell{}) {
		t.Fatal("the guard must be out")
	}
	free := b.ent(3).pos
	endT(t, b, 2) // not smoked: the guard walks towards the rifleman
	if b.ent(3).pos == free {
		t.Fatal("without smoke the guard approaches")
	}
	b = mk()
	applyOK(t, b, cast(1, "c_smoke", 12, 22)) // 2 turns
	endT(t, b, 1)
	free = b.ent(3).pos
	endT(t, b, 2)
	if b.ent(3).pos != free || hpOf(b, 4) != 50 {
		t.Fatalf("the guard walked to %v although the target is smoked (rifleman hp %d)", b.ent(3).pos, hpOf(b, 4))
	}
}
