package sim

import (
	"fmt"
	"testing"
)

// SIM-9: spell level growth (contract 0.2.7). Fixture rules, not balance.

func growSpells(s *Snapshot, g int) {
	for i := range s.Rules.Spells {
		s.Rules.Spells[i].LevelGrowthPermille = g
	}
}

func TestSpellLevelGrowthScalesAirstrikeAndMedkit(t *testing.T) {
	// airstrike 62, medkit 40; growth 1200: M = 1000, 1200, 1440
	cases := []struct{ level, strike, heal int }{{1, 62, 40}, {2, 74, 48}, {3, 89, 57}}
	for _, c := range cases {
		b := started(t, func(s *Snapshot) {
			s.Buildings = []PlacedBuilding{pb("d_mg_nest", 12, 10), pb("b_hq", 30, 30)}
			growSpells(s, 1200)
			s.Spells = []ArmySlot{{"c_airstrike", c.level, 1}, {"c_medkit", c.level, 1}}
		}, du{"u_rifleman", 0, 4})
		applyOK(t, b, cast(1, "c_airstrike", 12, 10))
		applyOK(t, b, cast(1, "c_medkit", 12, 10))
		got := map[zoneKind]int{}
		for _, z := range b.zones {
			got[z.kind] = z.amount
		}
		if got[zoneAirstrike] != c.strike || got[zoneMedkit] != c.heal {
			t.Fatalf("level %d: airstrike %d medkit %d, want %d %d", c.level, got[zoneAirstrike], got[zoneMedkit], c.strike, c.heal)
		}
	}
}

func TestSpellLevelZeroGrowthChangesNothing(t *testing.T) {
	for _, level := range []int{1, 2, 9} {
		b := started(t, func(s *Snapshot) {
			s.Buildings = []PlacedBuilding{pb("d_mg_nest", 12, 10), pb("b_hq", 30, 30)}
			s.Spells = []ArmySlot{{"c_airstrike", level, 1}}
		}, du{"u_rifleman", 0, 4})
		applyOK(t, b, cast(1, "c_airstrike", 12, 10))
		if a := b.zones[0].amount; a != 62 {
			t.Fatalf("level %d without growth: amount %d, want 62", level, a)
		}
	}
}

func TestSpellLevelGrowthKeepsRadiusAndDuration(t *testing.T) {
	b := started(t, func(s *Snapshot) {
		s.Buildings = []PlacedBuilding{pb("d_mg_nest", 12, 10), pb("b_hq", 30, 30)}
		growSpells(s, 1500)
		s.Spells = []ArmySlot{{"c_airstrike", 3, 1}}
	}, du{"u_rifleman", 0, 4})
	applyOK(t, b, cast(1, "c_airstrike", 12, 10))
	z := b.zones[0]
	if z.r != 3 || z.last-z.first+1 != 4 {
		t.Fatalf("radius %d, turns %d: must not grow", z.r, z.last-z.first+1)
	}
}

func TestSpellLevelGrowthNegativeRejected(t *testing.T) {
	r := fixtureRules()
	r.Spells[0].LevelGrowthPermille = -1
	if err := r.validate(); err == nil {
		t.Fatal("negative growth must be refused")
	}
}

// scenariosSim9 are G36..G38: an airstrike of level 1 / 2 / 3, growth 1200 permille (62 / 74 / 89).
func scenariosSim9() []scenario {
	var all []scenario
	for i, lvl := range []int{1, 2, 3} {
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_mg_nest", 12, 10), pb("b_hq", 30, 30)}
		growSpells(&s, 1200)
		s.Army = []ArmySlot{{"v_tank", 1, 1}}
		s.Spells = []ArmySlot{{"c_airstrike", lvl, 1}}
		// ids: nest 1, hq 2; tank 3 (out of the nest's reach)
		all = append(all, scenario{name: fmt.Sprintf("G%d", 36+i),
			about: fmt.Sprintf("airstrike level %d, growth 1200: dmg_total 62 x M[%d] = 62 / 74 / 89 over 4 turns on the 60 hp nest (levels 1 / 2 / 3: nest falls on turn 4 / 4 / 3); radius and duration do not grow", lvl, lvl),
			snap:  s, seed: uint64(36 + i), wantRejects: 0, cmds: []Command{
				dep("v_tank", 1, 14), {Op: OpStart},
				cast(1, "c_airstrike", 12, 10), end(1), end(2), end(3), end(4),
				{Turn: 5, Op: OpRetreat},
			}})
	}
	return all
}
