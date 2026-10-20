package sim

import "testing"

// The benchmark of the DoD. The old wording ("5000 ticks/s") belonged to the real-time tick model that ADR-003
// replaced; the unit is now a TURN of the battle: one EndTurn = the player's phase closes and the whole defence
// phase runs. The load is the worst case of spec 11.8 (150 buildings, 120 garrison units, 120 attackers, all of
// them alive for the first turns). benchMinTurnsPerSec is a tripwire for regressions (about a quarter of what a
// 2021 desktop does), not a target: the numbers measured are in sim/README.md.
//
// ASSUMPTION: docs/design/battle-sim.md 11.8 wants "30 turns <= battle.bench_ms" but the key is not in content;
// 500 turns/s means at most 2 ms per worst-case turn, i.e. a 30-turn battle in 60 ms (OQ-SIM12).

const benchMinTurnsPerSec = 500

var raceEnabled bool // set by race_test.go: the race detector slows everything down 5-10x

// midBase is a smaller raid: 50 buildings, 30 garrison units, 30 attackers. Slower per turn than the full base
// because the attackers survive longer, so all ten defence phases of the benchmark run with live targets.
func midBase(s *Snapshot) {
	s.Buildings = append(append([]PlacedBuilding(nil), s.Buildings[:49]...), s.Buildings[len(s.Buildings)-1])
	s.Garrison = s.Garrison[:30]
	s.Army = []ArmySlot{{"u_heavy", 1, 30}}
}

// benchFullBaseTurns plays up to ten defence phases of the full base per iteration and reports turns/s.
func benchFullBaseTurns(bn *testing.B) { benchTurns(bn, nil) }

func benchTurns(bn *testing.B, mut func(*Snapshot)) {
	turns := 0
	for i := 0; i < bn.N; i++ {
		bn.StopTimer()
		b := fullBaseWith(bn, mut)
		bn.StartTimer()
		for turn := 1; turn <= 10 && b.ph == phasePlayer; turn++ {
			b.Apply(end(turn))
			turns++
		}
	}
	bn.ReportMetric(float64(turns)/bn.Elapsed().Seconds(), "turns/s")
}

// BenchmarkFullBaseTurnsPerSecond: go test -run xxx -bench TurnsPerSecond
func BenchmarkFullBaseTurnsPerSecond(bn *testing.B) { benchFullBaseTurns(bn) }

// BenchmarkMidBaseTurnsPerSecond: the smaller raid.
func BenchmarkMidBaseTurnsPerSecond(bn *testing.B) { benchTurns(bn, midBase) }

func TestBenchmarkThreshold(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("timing test: not in -short / -race")
	}
	best := 0.0
	for try := 0; try < 3 && best < benchMinTurnsPerSec; try++ { // a noisy CI machine gets three chances
		if v := testing.Benchmark(benchFullBaseTurns).Extra["turns/s"]; v > best {
			best = v
		}
	}
	t.Logf("full base: %.0f turns/s (threshold %d)", best, benchMinTurnsPerSec)
	if best < benchMinTurnsPerSec {
		t.Fatalf("%.0f turns/s is below the threshold %d", best, benchMinTurnsPerSec)
	}
}
