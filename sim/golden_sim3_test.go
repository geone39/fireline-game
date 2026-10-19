package sim

// SIM-3 golden scenarios G13..G20: the defence phase (spec 7). Rules are the test fixture (not balance).
// Entity ids follow spec 2.4: buildings by (Y, X, ID), then garrison units, then the attackers in Deploy order.

func scenariosSim3() []scenario {
	var all []scenario
	a := atk
	hq := func(s *Snapshot, x, y int) { s.Buildings = append(s.Buildings, pb("b_hq", x, y)) }

	// G13: the nest chooses by (dist, hp, EntityID) and only shoots INF/AIR.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_mg_nest", 10, 10)}
		hq(&s, 20, 30)
		s.Army = []ArmySlot{{"u_rifleman", 1, 2}, {"u_brawler", 1, 1}, {"v_tank", 1, 1}, {"u_drone", 1, 1}}
		// ids: nest 1, hq 2; rifleman 3, rifleman 4, brawler 5, tank 6, drone 7
		all = append(all, scenario{name: "G13", about: "nest 20 dmg range 7 vs INF/AIR: nearest first, then lowest hp, then lowest id; the tank in range is never shot",
			snap: s, seed: 13, cmds: []Command{
				dep("u_rifleman", 1, 10), dep("u_rifleman", 1, 12), dep("u_brawler", 1, 8), dep("v_tank", 1, 14), dep("u_drone", 0, 10), {Op: OpStart},
				mv(1, 3, 2, 10, 3, 10, 4, 10), mv(1, 4, 2, 12, 3, 12, 4, 12), mv(1, 6, 2, 14, 3, 14), end(1), // 6 = 6 -> id 3 (50 -> 30)
				mv(2, 5, 2, 8, 3, 8, 4, 8, 5, 8), mv(2, 7, 1, 10, 2, 10, 3, 10, 4, 10, 5, 10), end(2), // 5 = 5: drone (70) before brawler (100)
				mv(3, 3, 5, 10, 6, 10, 7, 10), end(3), // id 3 at dist 3 (30 -> 10)
				mv(4, 4, 5, 12, 6, 12, 7, 12), end(4), // 3 = 3, hp 10 < 50: id 3 dies
				{Turn: 5, Op: OpRetreat},
			}})
	}

	// G14: the mortar cannot hit what is closer than min_range; its blast hurts the neighbours.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_mortar", 6, 20)}
		hq(&s, 20, 30)
		tweakBuilding(&s, "d_mortar", func(d *BuildingDef) { d.SplashCells = 1 })
		s.Army = []ArmySlot{{"u_rifleman", 1, 3}}
		// ids: mortar 1, hq 2; riflemen 3, 4, 5
		all = append(all, scenario{name: "G14", about: "mortar range 10, min_range 3, splash 1 (40% falloff): a unit inside min_range is safe, the blast hurts the adjacent unit, and when the first one retreats inside min_range the target switches and the blast finishes it off",
			snap: s, seed: 14, cmds: []Command{
				dep("u_rifleman", 1, 20), dep("u_rifleman", 1, 22), dep("u_rifleman", 1, 23), {Op: OpStart},
				mv(1, 3, 2, 20, 3, 20, 4, 20), mv(1, 4, 2, 22, 3, 22), mv(1, 5, 2, 23, 3, 23), end(1), // 4 (d3) takes 22, 5 (d3) 13
				end(2),                  // id 4: 28 -> 6; id 5: 37 -> 24
				mv(3, 4, 4, 22), end(3), // id 4 steps inside min_range: id 5 (d3) is the target, the blast (13 > 6 hp left) finishes id 4
				{Turn: 4, Op: OpRetreat},
			}})
	}

	// G15: tesla chain: up to 4 targets, each the nearest to the previous one, INF/VEH only.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_tesla", 10, 10)}
		hq(&s, 20, 30)
		s.Army = []ArmySlot{{"u_rifleman", 1, 3}, {"v_tank", 1, 1}, {"u_brawler", 1, 1}, {"u_drone", 1, 1}}
		// ids: tesla 1, hq 2; rifleman 3 (4,10), rifleman 4 (4,12), tank 5 (3,14), brawler 6 (5,15), rifleman 7 (3,16), drone 8
		all = append(all, scenario{name: "G15", about: "d_tesla chain 4, jump 3: rifleman 3 -> rifleman 4 -> tank -> brawler, full damage each; the fifth unit and the drone (not in vs) are spared",
			snap: s, seed: 15, cmds: []Command{
				dep("u_rifleman", 1, 10), dep("u_rifleman", 1, 12), dep("v_tank", 1, 14), dep("u_brawler", 1, 15), dep("u_rifleman", 1, 16), dep("u_drone", 0, 11), {Op: OpStart},
				mv(1, 3, 2, 10, 3, 10, 4, 10), mv(1, 4, 2, 12, 3, 12, 4, 12), mv(1, 5, 2, 14, 3, 14), mv(1, 6, 2, 15, 3, 15, 4, 15, 5, 15),
				mv(1, 7, 2, 16, 3, 16), mv(1, 8, 1, 11, 2, 11, 3, 11, 4, 11), end(1),
				end(2),
				{Turn: 3, Op: OpRetreat},
			}})
	}

	// G16: wire damage over time hurts INF standing on it, not vehicles or air; a destroyed wire stops.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_wire", 3, 20), pb("d_wire", 3, 24), pb("d_wire", 3, 26)}
		hq(&s, 30, 30)
		s.Army = []ArmySlot{{"u_rifleman", 1, 2}, {"u_drone", 1, 1}, {"v_tank", 1, 2}}
		// ids: wire 1 (3,20), 2 (3,24), 3 (3,26), hq 4; riflemen 5, 6; drone 7; tanks 8, 9
		all = append(all, scenario{name: "G16", about: "d_wire dot 4: a rifleman on a wire loses 4 per defence phase until a tank shoots the wire away (2 shots), the other keeps losing; a tank and a drone on wires take nothing",
			snap: s, seed: 16, cmds: []Command{
				dep("u_rifleman", 1, 20), dep("u_rifleman", 1, 24), dep("u_drone", 0, 24), dep("v_tank", 1, 22), dep("v_tank", 1, 26), {Op: OpStart},
				mv(1, 5, 2, 20, 3, 20), mv(1, 6, 2, 24, 3, 24), mv(1, 7, 1, 24, 2, 24, 3, 24), a(1, 8, 1), mv(1, 9, 2, 26, 3, 26), end(1),
				a(2, 8, 1), end(2),
				end(3),
				{Turn: 4, Op: OpRetreat},
			}})
	}

	// G17: flame trap overheats after 3 turns of fire and is dark for 2.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_flame_trap", 4, 10)}
		hq(&s, 30, 30)
		s.Army = []ArmySlot{{"u_heavy", 1, 1}}
		// ids: trap 1, hq 2; heavy 3 (1,10) at dist 3
		cmds := []Command{dep("u_heavy", 1, 10), {Op: OpStart}}
		for t := 1; t <= 9; t++ {
			cmds = append(cmds, end(t))
		}
		cmds = append(cmds, Command{Turn: 10, Op: OpRetreat})
		all = append(all, scenario{name: "G17", about: "d_flame_trap range 3, overheat after 3 turns of fire, dark for 2: shots on turns 1-3, 6-8 (6 x 15 = 90 damage on the heavy), silent on 4, 5 and 9",
			snap: s, seed: 17, cmds: cmds})
	}

	// G18: two bunkers: one opens when an attacker comes within reach, the other when it is hurt.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_bunker", 14, 20), pb("d_bunker", 14, 30)}
		hq(&s, 30, 36)
		s.Garrison = []GarrisonUnit{{"u_scout", 1}, {"u_scout", 1}, {"u_scout", 1}, {"u_scout", 1}}
		s.Army = []ArmySlot{{"u_rifleman", 1, 1}, {"u_sniper", 1, 1}}
		// ids: bunker A 1, bunker B 2, hq 3; scouts 4, 5 (A), 6, 7 (B); rifleman 8, sniper 9
		all = append(all, scenario{name: "G18", about: "bunker A releases its two scouts when the rifleman comes within 3 cells (the garrison range), bunker B when the sniper hurts it; the released scouts act in the same phase",
			snap: s, seed: 18, cmds: []Command{
				dep("u_rifleman", 1, 21), dep("u_sniper", 1, 30), {Op: OpStart},
				mv(1, 8, 2, 21, 3, 21, 4, 21), mv(1, 9, 2, 30, 3, 30), end(1),
				mv(2, 8, 5, 21, 6, 21, 7, 21), mv(2, 9, 4, 30, 5, 30), end(2),
				mv(3, 8, 8, 21, 9, 21, 10, 21), a(3, 9, 2), end(3), // bunker B hurt -> opens
				mv(4, 8, 11, 21), end(4), // within 3 of bunker A -> opens
				end(5),
				{Turn: 6, Op: OpRetreat},
			}})
	}

	// G19: a released scout walks round a wall to reach a target (Dijkstra, tie-breaks) and shoots with its 2nd AP.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_bunker", 12, 20)}
		for y := 14; y <= 26; y++ {
			s.Buildings = append(s.Buildings, pb("d_wall", 8, y))
		}
		hq(&s, 30, 30)
		s.Garrison = []GarrisonUnit{{"u_scout", 1}}
		s.Army = []ArmySlot{{"v_tank", 1, 1}, {"u_brawler", 1, 1}}
		// ids by (Y,X): walls (8,14)..(8,19) = 1..6, wall (8,20) = 7, bunker (12,20) = 8, walls (8,21)..(8,26) = 9..14, hq 15; scout 16; tank 17; brawler 18
		cmds := []Command{
			dep("v_tank", 1, 22), dep("u_brawler", 1, 20), {Op: OpStart},
			mv(1, 17, 2, 22, 3, 22), end(1), // 9 cells from the bunker: nothing happens
			mv(2, 17, 4, 22, 5, 22), end(2), // 7 cells, still outside the 3-cell trigger
			a(3, 17, 8), end(3), // the tank (range 7) hurts the bunker: the scout is out and goes for the nearest target
		}
		for t := 4; t <= 9; t++ {
			cmds = append(cmds, end(t))
		}
		cmds = append(cmds, Command{Turn: 10, Op: OpRetreat})
		all = append(all, scenario{name: "G19", about: "garrison AI: the scout released at (11,19) walks 3 cells per turn along the cheapest approach to the tank (wall column with gaps at y=13 and y=27), shoots when in range",
			snap: s, seed: 19, cmds: cmds})
	}

	// G20: the defence phase ends the battle: no attacker units left; later actors do not act.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_mg_nest", 10, 10), pb("d_mg_nest", 10, 14), pb("b_hq", 30, 30)}
		tweakBuilding(&s, "d_mg_nest", func(d *BuildingDef) { d.Dmg = 100 })
		s.Army = []ArmySlot{{"u_rifleman", 1, 1}}
		// ids: nest 1, nest 2, hq 3; rifleman 4
		all = append(all, scenario{name: "G20", about: "the first nest kills the only rifleman in the defence phase of turn 1: EndReason no_units, TurnsUsed 1, the next command is battle_over",
			snap: s, seed: 20, wantRejects: 1, cmds: []Command{
				dep("u_rifleman", 1, 11), {Op: OpStart},
				mv(1, 4, 2, 11, 3, 11, 4, 11), end(1),
				mv(2, 4, 5, 11),
			}})
	}
	return all
}
