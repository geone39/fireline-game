package sim

// SIM-5 golden scenarios G21..G29: support abilities, carriers, ships, spells, zone effects, heroes, paratroopers.
// Rules are the test fixture (not balance). Entity ids follow spec 2.4: buildings by (Y, X, ID), then garrison
// units, then the attackers in Deploy order (units born in battle - decoys - take the next free id).

func scenariosSim5() []scenario {
	var all []scenario
	a := atk
	hq := func(s *Snapshot, x, y int) { s.Buildings = append(s.Buildings, pb("b_hq", x, y)) }

	// G21: heal (medic on infantry, engineer on vehicles), jam (saboteur), rejected abilities.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_mg_nest", 6, 10), pb("d_mine", 4, 16)}
		hq(&s, 30, 30)
		tweakBuilding(&s, "d_mg_nest", func(d *BuildingDef) { d.Dmg = 8 })
		s.Army = []ArmySlot{{"u_rifleman", 1, 1}, {"u_medic", 1, 1}, {"v_engineer", 1, 1}, {"v_tank", 1, 1}, {"u_saboteur", 1, 1}}
		// ids: nest 1, mine 2, hq 3; rifleman 4, medic 5, engineer 6, tank 7, saboteur 8
		all = append(all, scenario{name: "G21", about: "abilities: the nest (8 dmg) hurts the saboteur, the medic heals it (capped at max hp), the tank steps on a mine and the engineer heals it (the mine also splashed the engineer, who cannot heal itself), the saboteur jams the nest for 2 turns and it stays silent; rejected: heal on the wrong class / self / full hp / out of range, jam out of range and on a non-gun building, no_ap",
			snap: s, seed: 21, wantRejects: 9, cmds: []Command{
				dep("u_rifleman", 1, 10), dep("u_medic", 1, 12), dep("v_engineer", 1, 14), dep("v_tank", 1, 16), dep("u_saboteur", 1, 8), {Op: OpStart},
				abil(1, 5, AbilityHeal, 4), // 1 reject: the rifleman is at full hp
				mv(1, 8, 2, 9, 3, 9, 4, 9), mv(1, 5, 2, 11, 3, 11), mv(1, 6, 2, 15, 3, 15), mv(1, 7, 2, 16, 3, 16),
				end(1),                     // nest: saboteur 40 -> 32
				mv(2, 7, 4, 16),            // the mine: tank -18, splash on the engineer (3,15): 10
				abil(2, 6, AbilityHeal, 4), // 2 reject: the engineer heals vehicles only
				abil(2, 6, AbilityHeal, 7), // +18 (capped at max hp)
				abil(2, 5, AbilityHeal, 5), // 3 reject: not oneself
				abil(2, 5, AbilityHeal, 8), // +8: saboteur 32 -> 40
				abil(2, 8, AbilityJam, 1),  // 4 reject: dist 2, the saboteur has to be right next to the nest
				mv(2, 8, 5, 9),
				abil(2, 6, AbilityHeal, 7), // 5 reject: no_ap (the engineer has healed already)
				end(2),
				abil(3, 8, AbilityJam, 3),  // 6 reject: the HQ has no gun
				abil(3, 8, AbilityJam, 1),  // jammed until turn 4
				abil(3, 8, AbilityJam, 1),  // 7 reject: no_ap
				abil(3, 4, AbilityHeal, 5), // 8 reject: a rifleman cannot heal
				mv(3, 4, 2, 10, 3, 10, 4, 10), a(3, 4, 1),
				end(3), // the jammed nest is silent
				a(4, 4, 1), a(4, 4, 1),
				abil(4, 5, AbilityHeal, 4), // 9 reject: the rifleman is whole (out_of_range is covered in unit tests)
				end(4),                     // still jammed (turns 3 and 4)
				end(5),                     // the nest fires again
				{Turn: 6, Op: OpRetreat},
			}})
	}

	// G22: carriers - unloading one passenger, then the rest; a fragile carrier dies and its cargo is thrown out.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_tesla", 14, 20)}
		hq(&s, 30, 30)
		s.Army = []ArmySlot{{"v_apc", 1, 1}, {"v_buggy", 1, 1}, {"u_rifleman", 1, 2}, {"u_brawler", 1, 2}}
		// ids: tesla 1, hq 2; apc 3, buggy 4, riflemen 5, 6, brawlers 7, 8
		all = append(all, scenario{name: "G22", about: "carriers: the APC unloads one passenger (Target) and, next turn, the rest in (Y,X) order; unloaded units have no AP; the fragile buggy is shot to pieces by the tesla chain and throws its cargo out with its remaining hp",
			snap: s, seed: 22, wantRejects: 3, cmds: []Command{
				dep("v_apc", 1, 20), dep("v_buggy", 1, 24), dep("u_rifleman", 1, 21), dep("u_rifleman", 1, 22), dep("u_brawler", 1, 25), dep("u_brawler", 1, 26),
				ld(5, 3), ld(6, 3), ld(7, 4), ld(8, 4), {Op: OpStart},
				abil(1, 5, AbilityUnload, 0), // 1 reject: a rifleman carries nothing
				mv(1, 3, 2, 20, 3, 20, 4, 20), mv(1, 4, 2, 24, 3, 24, 4, 24), end(1),
				abil(2, 3, AbilityUnload, 5),          // rifleman 5 -> (3,19)
				abil(2, 3, AbilityUnload, 0),          // 2 reject: no_ap
				mv(2, 4, 5, 24, 6, 24, 7, 24), end(2), // buggy in tesla range: -12, chain
				abil(3, 3, AbilityUnload, 6), // rifleman 6 -> (4,19)
				abil(3, 3, AbilityUnload, 0), // 3 reject: no_ap
				end(3),                       // second hit: the buggy dies, brawlers 7, 8 are put around (7,24)
				mv(4, 7, 7, 22, 8, 22, 9, 22, 10, 22),
				{Turn: 4, Op: OpRetreat},
			}})
	}

	// G23: ships - water only, coastal battery shoots ships and ships shoot the shore; the lander lands what fits.
	{
		s := f01Snapshot()
		s.Map = "coastal"
		s.Buildings = []PlacedBuilding{pb("d_coastal", 32, 30)}
		hq(&s, 20, 30)
		tweakBuilding(&s, "d_coastal", func(d *BuildingDef) { d.HP = 100 })
		s.Army = []ArmySlot{{"s_gunboat", 1, 1}, {"s_lander", 1, 1}, {"u_rifleman", 1, 2}}
		// ids: hq 1, coastal 2; gunboat 3, lander 4, riflemen 5, 6
		all = append(all, scenario{name: "G23", about: "ships: water-only deploy and move, the coastal battery (vs SEA) shoots the gunboat, the gunboat shells the shore from the water, the lander carries two riflemen and lands the one that fits on the single dry cell",
			snap: s, seed: 23, wantRejects: 4, cmds: []Command{
				dep("s_gunboat", 1, 35),   // 1 reject: ships only on water
				dep("u_rifleman", 43, 40), // 2 reject: infantry only on land
				dep("s_gunboat", 42, 30), dep("s_lander", 42, 34), dep("u_rifleman", 1, 30), dep("u_rifleman", 1, 31),
				ld(5, 4), ld(6, 4), {Op: OpStart},
				mv(1, 3, 41, 30, 40, 30, 39, 30, 38, 30), // 3 reject: path_cost
				mv(1, 3, 41, 30, 40, 30, 39, 30), mv(1, 4, 41, 34, 40, 34, 39, 34), end(1),
				a(2, 3, 2), mv(2, 4, 38, 34, 37, 34, 36, 34), end(2),
				a(3, 3, 2), abil(3, 4, AbilityUnload, 0), end(3), // one rifleman at (35,33)
				a(4, 3, 2), mv(4, 4, 35, 33), // 4 reject: the lander cannot go ashore
				mv(4, 5, 35, 32), end(4), // the landed rifleman walks on
				a(5, 3, 2),       // the battery is destroyed
				mv(5, 4, 35, 34), // along the water, the second rifleman still aboard
				{Turn: 5, Op: OpRetreat},
			}})
	}

	// G24: spells: airstrike split over turns (weakened by a jammer), EMP, breach, medkit, limits.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_mg_nest", 12, 10), pb("d_jammer", 16, 10), pb("d_wall", 10, 14), pb("d_wall", 11, 14), pb("d_wall", 12, 14), pb("d_wire", 11, 15), pb("d_mortar", 20, 20)}
		hq(&s, 30, 30)
		s.Army = []ArmySlot{{"u_rifleman", 1, 2}, {"v_tank", 1, 1}}
		s.Spells = []ArmySlot{{"c_airstrike", 1, 2}, {"c_emp", 1, 1}, {"c_medkit", 1, 1}, {"c_breach", 1, 1}}
		// ids by (Y,X): nest 1 (12,10), jammer 2 (16,10), walls 3, 4, 5 (y 14), wire 6 (11,15), mortar 7, hq 8; rifleman 9, 10, tank 11
		all = append(all, scenario{name: "G24", about: "spells: an airstrike under a 50% jammer (62 -> 31 over 4 turns: 10, 7, 7, 7) hurts the nest only inside its radius; EMP weakened to 1 turn; the third spell of a turn is refused (spells_per_turn 2); breach removes walls and leaves wire; medkit heals a wounded rifleman over 3 turns; no_stock after the stack is spent; the tank (not in the nest's vs) stays whole",
			snap: s, seed: 24, wantRejects: 4, cmds: []Command{
				dep("u_rifleman", 1, 10), dep("u_rifleman", 1, 12), dep("v_tank", 1, 14), {Op: OpStart},
				cast(1, "c_airstrike", 12, 10), cast(1, "c_emp", 12, 10),
				cast(1, "c_medkit", 1, 11), // 1 reject: spells_per_turn
				mv(1, 9, 2, 10, 3, 10, 4, 10), mv(1, 10, 2, 12, 3, 12, 4, 12), mv(1, 11, 2, 14, 3, 14),
				end(1), // airstrike step 10; the nest was jammed for turn 1 only
				cast(2, "c_breach", 11, 14),
				cast(2, "c_breach", 11, 14),        // 2 reject: no_stock
				mv(2, 11, 3, 15), mv(2, 10, 5, 12), // the rifleman steps into the nest range (7)
				cast(2, "c_nope", 5, 5), // 3 reject: unknown spell
				end(2),                  // step 7; nest shoots again: wounds a rifleman
				cast(3, "c_medkit", 4, 11), cast(3, "c_airstrike", 12, 10),
				cast(3, "c_airstrike", 12, 10), // 4 reject: no_stock (both airstrikes are spent; the second one stacks with the first)
				end(3),
				end(4),
				end(5),
				{Turn: 6, Op: OpRetreat},
			}})
	}

	// G25: smoke, adrenaline, commander aura.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_mg_nest", 12, 10), pb("d_wall", 14, 14)}
		hq(&s, 30, 30)
		s.Hero, s.HeroLevel = "h_marshal", 1
		s.Army = []ArmySlot{{"u_rifleman", 1, 2}}
		s.Spells = []ArmySlot{{"c_smoke", 1, 1}, {"c_adrenaline", 1, 1}}
		// ids: nest 1, wall 2, hq 3; hero 4, riflemen 5, 6
		all = append(all, scenario{name: "G25", about: "h_marshal aura (+40% move: 3 -> 4) lets a rifleman walk 4 cells; smoke hides everything inside beyond 1 cell for 2 turns (a rifleman that walks out of it is shot); adrenaline (+50% move, +25% dmg) adds to the aura (5 cells, dmg x1.45 in aura) and expires after 2 turns",
			snap: s, seed: 25, wantRejects: 1, cmds: []Command{
				{Op: OpDeploy, ID: "h_marshal", Level: 1, To: cell(1, 10)}, dep("u_rifleman", 1, 11), dep("u_rifleman", 1, 12), {Op: OpStart},
				mv(1, 4, 2, 10, 3, 10),
				mv(1, 5, 2, 11, 3, 11, 4, 11, 5, 11),        // aura: 4 cells
				mv(1, 6, 2, 12, 3, 12, 4, 12, 5, 12, 6, 12), // 5 reject: 5 cells, only 4 in the aura
				cast(1, "c_smoke", 4, 11),
				end(1), // nest: nobody (smoke)
				cast(2, "c_adrenaline", 4, 11),
				mv(2, 5, 6, 11, 7, 11, 8, 11, 9, 11, 10, 11), // aura 400 + adrenaline 500 -> 3 + floor(2.7) = 5 cells
				mv(2, 6, 2, 12, 3, 12, 4, 12, 5, 12, 6, 12),
				a(2, 5, 1), // adrenaline damage
				end(2),     // the smoke is still on, but the rifleman at (10,11) stands outside it
				a(3, 5, 1), a(3, 5, 1),
				end(3), // the smoke is gone, adrenaline too
				a(4, 5, 1),
				{Turn: 4, Op: OpRetreat},
			}})
	}

	// G26: hero zone damage with a cooldown, level-2 damage growth.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_mg_nest", 6, 10)}
		hq(&s, 30, 30)
		tweakBuilding(&s, "d_mg_nest", func(d *BuildingDef) { d.HP = 200 })
		s.Hero, s.HeroLevel = "h_cover", 2
		// ids: nest 1, hq 2; hero 3
		all = append(all, scenario{name: "G26", about: "a_cover_fire (h_cover L2): airstrike around the hero (61 over 2 turns: 31 + 30 on the nest), cooldown 3 turns (cooldown reject on turns 4, 5; ready on 6), the hero's own damage grows 40 -> 60 on level 2",
			snap: s, seed: 26, wantRejects: 4, cmds: []Command{
				{Op: OpDeploy, ID: "h_cover", Level: 2, To: cell(1, 10)}, {Op: OpStart},
				mv(1, 3, 2, 10, 3, 10), end(1),
				mv(2, 3, 4, 10), end(2),
				abil(3, 3, AbilityHero, 0), // zone (4,10) r2: the nest footprint x 6..7 is at distance 2
				abil(3, 3, AbilityHero, 0), // 1 reject: no_ap
				end(3),
				abil(4, 3, AbilityHero, 0), // 2 reject: cooldown
				end(4),
				abil(5, 3, AbilityHero, 0), // 3 reject: cooldown
				a(5, 3, 1),                 // 60 on level 2
				end(5),
				abil(6, 3, AbilityHero, 0),
				abil(6, 3, AbilityHeal, 0), // 4 reject: no such ability
				{Turn: 6, Op: OpRetreat},
			}})
	}

	// G27: hero decoy: a unit the defence shoots at; it is not army (no_units ignores it, no loss).
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_mg_nest", 12, 10)}
		hq(&s, 30, 30)
		s.Hero, s.HeroLevel = "h_tech", 1
		s.Army = []ArmySlot{{"u_rifleman", 1, 1}}
		// ids: nest 1, hq 2; hero 3, rifleman 4; the decoys become 5 and 6
		all = append(all, scenario{name: "G27", about: "a_decoy_turret (h_tech): decoy within 3 cells (out_of_range, cell_occupied rejected), the nest shoots the decoy (the only target in range), the decoy cannot act (no_ap), it expires after 2 turns, no loss is recorded, the cooldown (2 turns) lets the hero make a second one on turn 4",
			snap: s, seed: 27, wantRejects: 3, cmds: []Command{
				{Op: OpDeploy, ID: "h_tech", Level: 1, To: cell(1, 10)}, dep("u_rifleman", 1, 30), {Op: OpStart},
				{Turn: 1, Op: OpAbility, Unit: 3, ID: AbilityHero, To: cell(9, 10)}, // 1 reject: out of range
				{Turn: 1, Op: OpAbility, Unit: 3, ID: AbilityHero, To: cell(1, 10)}, // 2 reject: the hero stands there
				mv(1, 3, 2, 10, 3, 10), end(1),
				{Turn: 2, Op: OpAbility, Unit: 3, ID: AbilityHero, To: cell(6, 10)},
				mv(2, 5, 7, 10), // 3 reject: the decoy never acts (no AP)
				end(2),
				end(3), // the decoy lives through turn 3: spawned on 2, lifetime 2
				{Turn: 4, Op: OpAbility, Unit: 3, ID: AbilityHero, To: cell(4, 10)},
				end(4),
				{Turn: 5, Op: OpRetreat},
			}})
	}

	// G28: hero vanish: hidden from the defence until the first shot, which is x4 against the HQ.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_mg_nest", 6, 12), pb("b_hq", 9, 6)}
		s.Hero, s.HeroLevel = "h_ghost", 1
		s.Army = nil
		// ids: HQ (9,6) 1, nest (6,12) 2; hero 3
		all = append(all, scenario{name: "G28", about: "a_vanish (h_ghost): hidden for 2 turns (the nest in range has no target), the first shot out of hiding is x4 against the HQ (30 -> 120) and ends the hiding, then the nest shoots the hero; the next shot is plain 30; the cooldown (4 turns) refuses a new vanish",
			snap: s, seed: 28, wantRejects: 3, cmds: []Command{
				{Op: OpDeploy, ID: "h_ghost", Level: 1, To: cell(1, 10)}, {Op: OpStart},
				abil(1, 3, AbilityHero, 0),
				end(1), // nobody to shoot at: the hero is hidden
				a(2, 3, 1),
				a(2, 3, 1), // 1 reject: no_ap
				end(2),     // the nest sees the hero now
				a(3, 3, 1),
				abil(3, 3, AbilityHero, 0), // 2 reject: no_ap
				end(3),
				abil(4, 3, AbilityHero, 0), // 3 reject: cooldown
				{Turn: 4, Op: OpRetreat},
			}})
	}

	// G29: paratroopers: a group of 4 within the edge band, the group fills rings around To skipping blocked cells.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_wall", 2, 11), pb("d_wall", 3, 11), pb("d_mg_nest", 8, 12)}
		hq(&s, 30, 30)
		tweakBuilding(&s, "d_mg_nest", func(d *BuildingDef) { d.Dmg = 5 })
		s.Army = []ArmySlot{{"u_para", 1, 8}}
		// ids: wall (2,11) 1, wall (3,11) 2, nest (8,12) 3, hq 4; paratroopers 5..8, then 9..12
		all = append(all, scenario{name: "G29", about: "u_paratrooper: one Deploy places 4 units, To within 3 cells of the edge (inside the build zone allowed), the other three on the nearest free cells by rings and (Y,X) order, walls skipped; far cell, occupied cell, no stock rejected; the corner group is cut by the map edge",
			snap: s, seed: 29, wantRejects: 4, cmds: []Command{
				dep("u_para", 10, 10), // 1 reject: out of the edge band
				dep("u_para", 2, 11),  // 2 reject: a wall stands there
				dep("u_para", 2, 12),  // 2,12 and the nearest free cells by rings: (1,11), (1,12), (3,12) (the walls are skipped)
				dep("u_para", 2, 12),  // 3 reject: occupied
				dep("u_para", 0, 0),   // corner group
				dep("u_para", 0, 20),  // 4 reject: no stock (8 = 2 groups)
				{Op: OpStart},
				mv(1, 5, 2, 13), mv(1, 6, 0, 11), mv(1, 7, 1, 13), mv(1, 8, 4, 12),
				end(1),
				a(2, 8, 3),
				{Turn: 2, Op: OpRetreat},
			}})
	}
	return all
}
