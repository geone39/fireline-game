package sim

import "fmt"

// SIM-8 golden scenarios G33..G35: the damage of a defence building grows with its level (contract 0.2.6).
// Rules are the test fixture (not balance): d_mg_nest dmg 20, growth 1170 permille per level.
func scenariosSim8() []scenario {
	var all []scenario
	for i, lvl := range []int{1, 5, 10} {
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{{ID: "d_mg_nest", Level: lvl, Pos: cell(10, 10)}, pb("b_hq", 20, 30)}
		tweakBuilding(&s, "d_mg_nest", func(d *BuildingDef) { d.DmgGrowthPermille = 1170 })
		s.Army = []ArmySlot{{"u_brawler", 1, 1}}
		// ids: nest 1, hq 2; brawler 3
		all = append(all, scenario{name: fmt.Sprintf("G%d", 33+i),
			about: fmt.Sprintf("nest level %d, dmg_growth 1170: the brawler walks into range and is shot for 20 x M[%d] per turn (20 / 37 / 82 at levels 1 / 5 / 10); at level 10 the brawler dies on turn 2 and the final Retreat is refused (battle_over)", lvl, lvl),
			snap:  s, seed: uint64(33 + i), wantRejects: -1, cmds: []Command{
				dep("u_brawler", 1, 10), {Op: OpStart},
				mv(1, 3, 2, 10, 3, 10, 4, 10), end(1),
				end(2),
				{Turn: 3, Op: OpRetreat},
			}})
	}
	return all
}
