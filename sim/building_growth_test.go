package sim

import "testing"

// SIM-8: damage growth of defence buildings by level (spec 3.2, contract 0.2.6).

func TestBuildingDmgGrowth(t *testing.T) {
	// M[n] = floor(M[n-1]*1170/1000): 1000, 1170, 1368, 1600, 1872 (level 5), ..., level 10 = 4102 (integer chain)
	for _, c := range []struct{ g, lvl, want int }{
		{0, 1, 20}, {0, 10, 20}, // zero = no growth
		{1170, 1, 20}, {1170, 2, 23}, {1170, 5, 37}, {1170, 10, 82},
	} {
		e := &entity{level: c.lvl, bd: &BuildingDef{Dmg: 20, DmgGrowthPermille: c.g}}
		if got := e.buildingDmg(); got != c.want {
			t.Errorf("g=%d level %d: dmg %d, want %d", c.g, c.lvl, got, c.want)
		}
	}
}

func TestNestShootsByLevel(t *testing.T) {
	for _, c := range []struct{ lvl, g, want int }{{1, 1170, 20}, {5, 1170, 37}, {10, 1170, 82}, {10, 0, 20}} {
		b := started(t, func(s *Snapshot) {
			s.Buildings = []PlacedBuilding{{ID: "d_mg_nest", Level: c.lvl, Pos: cell(12, 10)}}
			tweakBuilding(s, "d_mg_nest", func(d *BuildingDef) { d.DmgGrowthPermille = c.g; d.HP = 1000 })
			s.Rules.Units[unitIdx(s, "u_brawler")].HP = 1000
		}, du{"u_brawler", 1, 10}) // ids: nest 1, brawler 2
		tele(b, 2, 8, 10)
		before := hpOf(b, 2)
		applyOK(t, b, end(1))
		if got := before - hpOf(b, 2); got != c.want {
			t.Errorf("level %d g=%d: hit for %d, want %d", c.lvl, c.g, got, c.want)
		}
	}
}

func TestNegativeBuildingGrowthRejected(t *testing.T) {
	s := f01Snapshot()
	tweakBuilding(&s, "d_mg_nest", func(d *BuildingDef) { d.DmgGrowthPermille = -1 })
	if _, err := NewBattle(s, 1); err == nil {
		t.Fatal("negative growth accepted")
	}
}

func unitIdx(s *Snapshot, id string) int {
	for i := range s.Rules.Units {
		if s.Rules.Units[i].ID == id {
			return i
		}
	}
	panic("no unit " + id)
}
