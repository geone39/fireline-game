package sim

// SIM-2 golden scenarios G06..G12: damage, destruction, traps, ending, stars, loot basis. Rules are the test
// fixture (not balance). Entity ids follow spec 2.4: buildings by (Y, X, ID), then the units in Deploy order.

func scenariosSim2() []scenario {
	var all []scenario
	a := atk

	// G06: spec 12 F01 as written.
	all = append(all, scenario{name: "G06", about: "F01 (spec 12): the nest is destroyed on turn 4 -> Destruction 50, 1 star (half), EndReason retreat, TurnsUsed 4",
		snap: f01Snapshot(), seed: 6, cmds: []Command{
			dep("u_rifleman", 1, 10), dep("u_brawler", 1, 12), {Op: OpStart},
			mv(1, 3, 2, 10, 3, 10, 4, 10), mv(1, 4, 2, 12, 3, 12, 4, 12, 5, 12), end(1),
			mv(2, 3, 5, 10, 6, 10, 7, 10), mv(2, 4, 6, 12, 7, 12, 8, 12, 9, 12), end(2),
			mv(3, 3, 8, 10), a(3, 3, 1), mv(3, 4, 10, 12, 11, 12), end(3),
			a(4, 4, 1), a(4, 3, 1), a(4, 3, 1), {Turn: 4, Op: OpRetreat},
		}})

	// G07: full destruction: HQ first (star), then the nest; the next command is battle_over.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("b_hq", 8, 8), pb("d_mg_nest", 8, 13)}
		tweakBuilding(&s, "b_hq", func(d *BuildingDef) { d.HP = 100; d.HPGrowthPermille = 0 })
		s.Army = []ArmySlot{{"v_tank", 1, 2}}
		// ids: hq (8,8) = 1, nest (8,13) = 2; tanks 3, 4
		all = append(all, scenario{name: "G07", about: "full destruction: both tanks kill the HQ (star) on turn 2 and the nest on turn 3 -> 3 stars, Destruction 100, EndReason full_destruction; the next command is battle_over",
			snap: s, seed: 7, wantRejects: 1, cmds: []Command{
				dep("v_tank", 1, 10), dep("v_tank", 1, 11), {Op: OpStart},
				mv(1, 3, 2, 10, 3, 10), mv(1, 4, 2, 11, 3, 11), end(1),
				a(2, 3, 1), a(2, 4, 1), end(2),
				a(3, 3, 2), a(3, 4, 2),
				a(3, 4, 2),
			}})
	}

	// G08: two mines wipe out the army: no_units.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_mine", 3, 10), pb("d_mine", 3, 14), pb("d_mg_nest", 20, 20)}
		tweakBuilding(&s, "d_mine", func(d *BuildingDef) { d.Burst = 100 })
		s.Army = []ArmySlot{{"u_rifleman", 1, 1}, {"u_brawler", 1, 1}}
		// ids: mine (3,10) = 1, mine (3,14) = 2, nest = 3; rifleman 4, brawler 5
		all = append(all, scenario{name: "G08", about: "no_units: both units step on 100-damage hidden mines, each walk stops on the mine, EndReason no_units, 2 units lost, TurnsUsed 1",
			snap: s, seed: 8, cmds: []Command{
				dep("u_rifleman", 1, 10), dep("u_brawler", 1, 14), {Op: OpStart},
				mv(1, 4, 2, 10, 3, 10, 4, 10), mv(1, 5, 2, 14, 3, 14, 4, 14, 5, 14),
			}})
	}

	// G09: Attack rules.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_wall", 5, 5), {ID: "d_mine", Level: 1, Pos: cell(7, 6), Revealed: true}, pb("d_mine", 9, 6),
			pb("x_statue", 9, 12), pb("d_mg_nest", 12, 10)}
		s.Army = []ArmySlot{{"u_sniper", 1, 1}, {"u_rifleman", 1, 1}, {"u_medic", 1, 1}}
		// ids: wall 1, revealed mine 2, hidden mine 3, nest 4, statue 5; sniper 6, rifleman 7, medic 8
		all = append(all, scenario{name: "G09", about: "Attack: no_ap, out_of_range, below_min_range, invalid_target (own, decor, hidden mine, unarmed, destroyed), unknown_entity, not_owner; ap 2 = move + shot; a revealed mine is shot without a blast",
			snap: s, seed: 9, wantRejects: 12, cmds: []Command{
				dep("u_sniper", 1, 5), dep("u_rifleman", 1, 6), dep("u_medic", 1, 12), {Op: OpStart},
				a(1, 6, 1),                 // ok: wall 900 -> 872
				a(1, 6, 1),                 // no_ap
				a(1, 7, 4),                 // out_of_range (nest, 11 cells)
				a(1, 8, 4),                 // invalid_target (the medic has no attack)
				a(1, 7, 6),                 // invalid_target (own unit)
				a(1, 7, 5),                 // invalid_target (decor)
				a(1, 7, 3),                 // invalid_target (hidden mine)
				a(1, 99, 1),                // unknown_entity
				a(1, 7, 99),                // unknown_entity
				a(1, 4, 1),                 // not_owner (a building)
				mv(1, 7, 2, 6, 3, 6, 4, 6), // ap 2: move ...
				a(1, 7, 2),                 // ... then shoot the revealed mine (3 cells): destroyed, no blast
				a(1, 7, 1),                 // no_ap (both used)
				end(1),
				a(2, 6, 2), // invalid_target (destroyed)
				mv(2, 6, 2, 5, 3, 5), end(2),
				mv(3, 6, 4, 5), end(3), // next to the wall
				a(4, 6, 1), // below_min_range
				{Turn: 4, Op: OpRetreat},
			}})
	}

	// G10: splash falloff and vs_mult.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_wall", 10, 10), pb("d_wall", 11, 10), pb("d_wall", 12, 10), pb("d_wall", 13, 10), pb("d_wall", 14, 10),
			pb("d_wire", 10, 16), pb("d_fence", 10, 18), pb("d_mg_nest", 30, 30)}
		s.Army = []ArmySlot{{"u_grenadier", 1, 1}, {"u_flamer", 1, 1}}
		// ids: walls 1..5, wire 6, fence 7, nest 8; grenadier 9, flamer 10
		all = append(all, scenario{name: "G10", about: "splash radius 2 with 40% falloff on a wall line (20/16/12), flamer vs_mult: wire x1.5, walls x0.5; uncounted targets leave Destruction at 0",
			snap: s, seed: 10, cmds: []Command{
				dep("u_grenadier", 1, 10), dep("u_flamer", 1, 16), {Op: OpStart},
				mv(1, 9, 2, 10, 3, 10), mv(1, 10, 2, 16, 3, 16), end(1),
				mv(2, 9, 4, 10, 5, 10), mv(2, 10, 4, 16, 5, 16), end(2),
				mv(3, 9, 6, 10), mv(3, 10, 6, 16, 7, 16), end(3),
				a(4, 9, 3), mv(4, 10, 8, 16), end(4),
				a(5, 9, 3), a(5, 10, 6), end(5),
				a(6, 9, 3), a(6, 10, 6), end(6),
				a(7, 10, 7), {Turn: 7, Op: OpRetreat},
			}})
	}

	// G11: breacher.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_wall", 10, 10), pb("d_fence", 10, 11), pb("d_wall", 10, 12), pb("d_mg_nest", 14, 10)}
		s.Army = []ArmySlot{{"u_breacher", 1, 1}, {"u_rifleman", 1, 1}}
		// ids: wall 1, nest 2, fence 3, wall 4; breacher 5, rifleman 6
		all = append(all, scenario{name: "G11", about: "u_breacher: burst 16 x wall_mult 40 destroys the fence, the walls next to it take 60%, the breacher is lost; the rifleman walks through the open cell and shoots the nest",
			snap: s, seed: 11, wantRejects: 1, cmds: []Command{
				dep("u_breacher", 1, 11), dep("u_rifleman", 1, 12), {Op: OpStart},
				mv(1, 5, 2, 11, 3, 11, 4, 11), mv(1, 6, 2, 12, 3, 12, 4, 12), end(1),
				mv(2, 5, 5, 11, 6, 11, 7, 11), mv(2, 6, 5, 12, 6, 12, 7, 12), end(2),
				mv(3, 5, 8, 11, 9, 11), mv(3, 6, 8, 12), end(3),
				mv(4, 6, 9, 11, 10, 11, 11, 11), // cell_occupied: the breacher stands at (9,11); the fence behind it still stands
				a(4, 5, 3), end(4),              // fence gone, walls 900 -> 516, breacher destroyed
				mv(5, 6, 9, 11, 10, 11, 11, 11), a(5, 6, 2), end(5),
				a(6, 6, 2), a(6, 6, 2), {Turn: 6, Op: OpRetreat},
			}})
	}

	// G12: traps, a carrier and a storage.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_mine", 3, 10), pb("d_wall", 2, 15), pb("d_spring_trap", 4, 14), pb("d_spring_trap", 4, 17),
			pb("d_mine", 5, 20), pb("b_vault", 8, 30), pb("b_hq", 30, 30)}
		s.Army = []ArmySlot{{"v_tank", 1, 2}, {"u_brawler", 1, 1}, {"u_rifleman", 1, 3}, {"u_drone", 1, 1},
			{"u_medic", 1, 1}, {"u_heavy", 1, 1}, {"v_buggy", 1, 1}}
		// ids: mine 1, wall 2, spring 3, spring 4, mine 5, vault 6, hq 7; tank 8, brawler 9, rifleman 10, drone 11,
		// medic 12, heavy 13, buggy 14, riflemen 15 16, tank 17
		all = append(all, scenario{name: "G12", about: "traps: mine blast 18/10 with falloff (air spared), spring trap throws the medic over the wall and not the heavy, the fragile APC dies on a mine and unloads its riflemen, the vault takes 110 permille",
			snap: s, seed: 12, cmds: []Command{
				dep("v_tank", 1, 9), dep("u_brawler", 1, 11), dep("u_rifleman", 1, 10), dep("u_drone", 0, 10),
				dep("u_medic", 1, 16), dep("u_heavy", 1, 14), dep("v_buggy", 1, 20), dep("u_rifleman", 1, 21), dep("u_rifleman", 1, 22), dep("v_tank", 1, 30),
				ld(15, 14), ld(16, 14), {Op: OpStart},
				mv(1, 8, 2, 9), mv(1, 9, 2, 11), mv(1, 11, 1, 10, 2, 10),
				mv(1, 10, 2, 10, 3, 10), // mine: the walk stops, blast on the rifleman, the tank and the brawler
				mv(1, 12, 2, 16, 3, 16), mv(1, 13, 2, 14), mv(1, 14, 2, 20, 3, 20, 4, 20), mv(1, 17, 2, 30, 3, 30), end(1),
				mv(2, 12, 4, 17), // spring trap: thrown back over the wall at (2,15)
				mv(2, 13, 3, 14),
				mv(2, 14, 5, 20), // the APC (15 hp) dies on the mine, the passengers are set down
				a(2, 17, 6), end(2),
				mv(3, 13, 4, 14), // the heavy stands on an idle trap
				{Turn: 3, Op: OpRetreat},
			}})
	}
	return all
}
