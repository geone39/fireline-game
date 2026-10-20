package sim

import (
	"os"
	"strconv"
	"testing"
	"time"
)

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

// The tripwire is robust to a noisy shared runner: one discarded warm-up run, then the BEST of benchRuns
// measurements (noise only ever slows a run down, so the maximum is the least biased estimate). The threshold
// comes from SIM_BENCH_MIN_TURNS_PER_S; without it it is benchMinTurnsPerSec locally and benchMinTurnsPerSecCI
// when the CI variable is set (GitHub Actions sets CI=true), which is logged. A 4x slowdown from the ~2000
// turns/s the engine does fails the test in both modes.
const (
	benchMinTurnsPerSecCI = 350 // ASSUMPTION: CI runners are 2-3x slower and noisier than a desktop; 1/6 of the norm
	benchRuns             = 5
	benchTargetTurnsPerS  = 2000 // owner's goal (2026-10-19), only logged
	benchEnvMin           = "SIM_BENCH_MIN_TURNS_PER_S"
)

// benchThreshold returns the minimum turns/s and where it came from.
func benchThreshold(t *testing.T) (float64, string) {
	if s := os.Getenv(benchEnvMin); s != "" {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil || v <= 0 {
			t.Fatalf("%s=%q: want a positive number", benchEnvMin, s)
		}
		return v, benchEnvMin
	}
	if os.Getenv("CI") != "" {
		return benchMinTurnsPerSecCI, "CI default"
	}
	return benchMinTurnsPerSec, "local default"
}

// bestTurnsPerSec runs one warm-up and then runs measurements of mk, returns the best turns/s.
func bestTurnsPerSec(runs int, mk func(*testing.B)) float64 {
	testing.Benchmark(mk) // warm-up: page cache, CPU frequency, GC pacing; discarded
	best := 0.0
	for i := 0; i < runs; i++ {
		if v := testing.Benchmark(mk).Extra["turns/s"]; v > best {
			best = v
		}
	}
	return best
}

func TestBenchmarkThreshold(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("timing test: not in -short / -race")
	}
	min, src := benchThreshold(t)
	best := bestTurnsPerSec(benchRuns, benchFullBaseTurns)
	t.Logf("full base: best of %d runs = %.0f turns/s; threshold %.0f (%s); target ~%d", benchRuns, best, min, src, benchTargetTurnsPerS)
	if best < min {
		t.Fatalf("%.0f turns/s is below the threshold %.0f (%s)", best, min, src)
	}
}

// TestBenchmarkThresholdCatchesRegression: an artificial 3 ms busy-wait per turn (a regression of 6x or more)
// must be far below the lowest threshold we ever use, otherwise the tripwire is blind.
func TestBenchmarkThresholdCatchesRegression(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("timing test: not in -short / -race")
	}
	slow := func(bn *testing.B) {
		turns := 0
		for i := 0; i < bn.N; i++ {
			bn.StopTimer()
			b := fullBaseWith(bn, nil)
			bn.StartTimer()
			for turn := 1; turn <= 10 && b.ph == phasePlayer; turn++ {
				b.Apply(end(turn))
				turns++
				for start := time.Now(); time.Since(start) < 3*time.Millisecond; {
				}
			}
		}
		bn.ReportMetric(float64(turns)/bn.Elapsed().Seconds(), "turns/s")
	}
	got := bestTurnsPerSec(2, slow)
	t.Logf("slowed full base: %.0f turns/s (must stay below %d)", got, benchMinTurnsPerSecCI)
	if got >= benchMinTurnsPerSecCI {
		t.Fatalf("slowed engine still does %.0f turns/s: the threshold cannot catch a regression", got)
	}
}
