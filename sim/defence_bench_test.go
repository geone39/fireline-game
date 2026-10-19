package sim

import "testing"

// fullBase is the HQ10-sized load of spec 11.8: 150 buildings (10 bunkers holding 120 garrison units, defences,
// walls, wire, mines), packed row by row into the build zone, and 120 heavy attackers on the rim.
func fullBase(tb testing.TB) *Battle {
	tb.Helper()
	s := f01Snapshot()
	s.Buildings = nil
	tweakBuilding(&s, "d_bunker", func(d *BuildingDef) { d.Garrison = 12 })
	plan := []string{"d_bunker", "d_mg_nest", "d_mortar", "d_flame_trap", "d_tesla", "d_wall", "d_wall", "d_wire", "d_mine", "d_mg_nest"}
	size := map[string]int{"d_bunker": 3, "d_mortar": 3, "d_mg_nest": 2, "d_flame_trap": 2, "d_tesla": 2}
	occ := make([]bool, 44*44)
	free := func(x, y, n int) bool {
		if x+n > 42 || y+n > 42 {
			return false
		}
		for j := 0; j < n; j++ {
			for i := 0; i < n; i++ {
				if occ[(y+j)*44+x+i] {
					return false
				}
			}
		}
		return true
	}
	n := 0
	for k := 0; n < 149; k++ {
		id := plan[k%len(plan)]
		sz := max(1, size[id])
		for y := 2; y < 42 && n < 149; y++ {
			placed := false
			for x := 2; x < 42; x++ {
				if free(x, y, sz) {
					for j := 0; j < sz; j++ {
						for i := 0; i < sz; i++ {
							occ[(y+j)*44+x+i] = true
						}
					}
					s.Buildings = append(s.Buildings, pb(id, x, y))
					n++
					placed = true
					break
				}
			}
			if placed {
				break
			}
		}
	}
	s.Buildings = append(s.Buildings, pb("b_hq", 38, 38))
	for i := 0; i < 120; i++ {
		s.Garrison = append(s.Garrison, GarrisonUnit{[]string{"u_guard", "u_scout"}[i%2], 1})
	}
	s.Army = []ArmySlot{{"u_heavy", 1, 120}}
	b, err := NewBattle(s, 1)
	if err != nil {
		tb.Fatal(err)
	}
	for i := 0; i < 120; i++ {
		c := cell(i%2, 2+i/2) // left rim, 80 cells, then the top rim
		if i >= 80 {
			c = cell(2+(i-80)%40, (i-80)/40)
		}
		if _, err := b.Apply(dep("u_heavy", int(c.X), int(c.Y))); err != nil || len(b.rejected) > 0 {
			tb.Fatalf("deploy %d at %v: %v %v", i, c, err, b.rejected)
		}
	}
	b.Apply(Command{Op: OpStart})
	return b
}

func TestFullBaseDefencePhase(t *testing.T) {
	b := fullBase(t)
	if len(b.ents) != 150+120+120 {
		t.Fatalf("%d entities", len(b.ents))
	}
	for turn := 1; turn <= 5 && b.ph == phasePlayer; turn++ {
		endT(t, b, turn)
		checkInvariants(t, b)
	}
	released := 0
	for i := range b.ents {
		if e := &b.ents[i]; e.owner == ownerDefender && e.ud != nil && e.onField {
			released++
		}
	}
	if released == 0 {
		t.Fatal("no garrison came out of the bunkers")
	}
}

// BenchmarkDefencePhaseFullBase measures the first defence phase of a full base: every bunker opens, the
// garrison walks (Dijkstra per unit), 150 buildings pick targets.
func BenchmarkDefencePhaseFullBase(bn *testing.B) {
	for i := 0; i < bn.N; i++ {
		bn.StopTimer()
		b := fullBase(bn)
		bn.StartTimer()
		if _, err := b.Apply(end(1)); err != nil {
			bn.Fatal(err)
		}
	}
}

// BenchmarkDefenceTenTurnsFullBase: ten consecutive phases (the attackers idle and are shot to pieces).
func BenchmarkDefenceTenTurnsFullBase(bn *testing.B) {
	for i := 0; i < bn.N; i++ {
		bn.StopTimer()
		b := fullBase(bn)
		bn.StartTimer()
		for turn := 1; turn <= 10 && b.ph == phasePlayer; turn++ {
			b.Apply(end(turn))
		}
	}
}
